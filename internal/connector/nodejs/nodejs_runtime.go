package nodejs

import (
	"errors"
	"fmt"
	"strings"
)

type InputType uint8

const (
	InputTypeAuto InputType = iota
	InputTypeModule
	InputTypeCommonJS
)

const runtimeModuleImports = `import { createReadStream, createWriteStream } from 'fs';
import { createInterface } from 'readline/promises';`

const runtimeCommonJSImports = `"use strict";
const { createReadStream, createWriteStream } = require('fs');
const { createInterface } = require('readline/promises');

const __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};`

const runtimeResourceTemplate = `%s;

globalThis.TerraformError = class TerraformError extends Error {
  constructor(summary, detail) {
    super(summary + ': ' + detail);
    this.summary = summary;
    this.detail = detail;
  }
};

const ipcInput = createReadStream(null, { fd: 3, encoding: 'utf-8', autoClose: true });
const ipcOutput = createWriteStream(null, { fd: 4, encoding: 'utf-8',  autoClose: true });

function writeOutput(value) {
  ipcOutput.write(JSON.stringify(value) + '\n');
}

async function handleInput(id, action, payload, provider) {
  console.debug('[%%s] Processing IPC:', id, { action, payload });
  try {
    let response;

    switch (action) {
      case 'create':
        provider.validateInput(payload);
        response = await provider.create(payload);
        break;
      case 'read':
        provider.validateInput(payload.input);
        response = await provider.read(payload.input, payload.state);
        if (response.input) {
          provider.validateInput(response.input);
        }
        break;
      case 'update':
        provider.validateInput(payload.newInput);
        response = await provider.update(payload.oldInput, payload.newInput, payload.state);
        break;
      case 'delete':
        provider.validateInput(payload.input);
        response = await provider.delete(payload.input, payload.state);
        break;
      case 'import':
        response = await provider.import(payload.id);
        provider.validateInput(response.input);
        break;
      default:
        throw new TerraformError('Invalid method', 'Method \'' + request.method + '\' is not supported');
    }

    console.debug('[%%s] IPC response:', id, { response });
    writeOutput({
      id,
      success: true,
      result: response,
    });
  } catch (error) {
    if (error instanceof TerraformError) {
      console.error('[%%s] %%s: %%s', id, error.summary, error.detail);
      writeOutput({
        id,
        success: false,
        result: {
          summary: error.summary,
          detail: error.detail,
        },
      });
    } else {
      console.error('[%%s] Unexpected error:', id, error);
      writeOutput({
        id,
        success: false,
        result: {
          summary: 'Unexpected error',
          detail: error instanceof Error ? error.message : String(error),
        },
      });
    }
  }
}

async function runtimeMain() {
  let provider;
  
  try {
    ({ default: provider } = %s);
  } catch (error) {
    if (error instanceof TerraformError) {
      writeOutput({
        success: false,
        result: {
          summary: error.summary,
          detail: error.detail,
        },
      });
    } else {
      console.error('Unexpected error:', error);
      writeOutput({
        success: false,
        result: {
          summary: 'Unexpected error',
          detail: error instanceof Error ? error.message : String(error),
        },
      });
    }
  }

  writeOutput({
    success: true,
    result: {
      name: provider.name,
      requiresReplace: provider.requiresReplace ?? [],
    },
  });

  for await (const inputJson of createInterface(ipcInput)) {
    const input = JSON.parse(inputJson);
    void handleInput(input.id, input.action, input.input, provider);
  }
}

runtimeMain()
  .catch(reason => console.error(reason));`

const runtimeDataSourceTemplate = `%s

globalThis.TerraformError = class TerraformError extends Error {
  constructor(summary, detail) {
    super(summary + ': ' + detail);
    this.summary = summary;
    this.detail = detail;
  }
};

const ipcInput = createReadStream(null, { fd: 3, encoding: 'utf-8', autoClose: true });
const ipcOutput = createWriteStream(null, { fd: 4, encoding: 'utf-8',  autoClose: true });

function writeOutput(value) {
  ipcOutput.write(JSON.stringify(value) + '\n');
}

async function handleInput(id, action, payload, provider) {
  try {
  	console.debug('[%%s] Read with input:', id, payload);
    const state = await provider.read(payload);
    console.debug('[%%s] Read result:', id, state);

    writeOutput({
      id,
      success: true,
      result: state,
    });
  } catch (error) {
    if (error instanceof TerraformError) {
      console.error('[%%s] %%s: %%s', id, error.summary, error.detail);
      writeOutput({
        id,
        success: false,
        result: {
          summary: error.summary,
          detail: error.detail,
        },
      });
    } else {
      console.error('[%%s] Unexpected error:', id, error);
      writeOutput({
        id,
        success: false,
        result: {
          summary: 'Unexpected error',
          detail: error instanceof Error ? error.message : String(error),
        },
      });
    }
  }
}

async function runtimeMain() {
  let provider;
  
  try {
    ({ default: provider } = %s);
  } catch (error) {
    if (error instanceof TerraformError) {
      writeOutput({
        success: false,
        result: {
          summary: error.summary,
          detail: error.detail,
        },
      });
    } else {
      writeOutput({
        success: false,
        result: {
          summary: 'Unexpected error',
          detail: error instanceof Error ? error.message : String(error),
        },
      });
    }
  }

  writeOutput({
    success: true,
    result: {
      name: provider.name,
    },
  });

  for await (const inputJson of createInterface(ipcInput)) {
    const input = JSON.parse(inputJson);
    void handleInput(input.id, input.action, input.input, provider);
  }
}

runtimeMain()
  .catch(reason => console.error(reason));`

func isScriptModule(path string, inputTypeSelection InputType) (bool, error) {
	var isModule bool

	switch inputTypeSelection {
	case InputTypeAuto:
		if strings.HasSuffix(path, ".cjs") {
			isModule = false
		} else if strings.HasSuffix(path, ".mjs") {
			isModule = true
		} else {
			return false, errors.New("node.js script must either have .cjs or .mjs file extension or the input type must be specified")
		}
	case InputTypeModule:
		isModule = true
	case InputTypeCommonJS:
		isModule = false
	}
	return isModule, nil
}

func buildResourceRuntime(path string, inputTypeSelection InputType) (string, string, error) {
	isModule, err := isScriptModule(path, inputTypeSelection)
	if err != nil {
		return "", "", err
	}

	var code string
	var inputType string

	if isModule {
		code = fmt.Sprintf(runtimeResourceTemplate, runtimeModuleImports, "await import('./"+path+"')")
		inputType = "module"
	} else {
		code = fmt.Sprintf(runtimeResourceTemplate, runtimeCommonJSImports, "__importDefault(require('./"+path+"'))")
		inputType = "commonjs"
	}

	return code, inputType, nil
}

func buildDataSourceRuntime(path string, inputTypeSelection InputType) (string, string, error) {
	isModule, err := isScriptModule(path, inputTypeSelection)
	if err != nil {
		return "", "", err
	}

	var code string
	var inputType string
	if isModule {
		code = fmt.Sprintf(runtimeDataSourceTemplate, runtimeModuleImports, "await import('./"+path+"')")
		inputType = "module"
	} else {
		code = fmt.Sprintf(runtimeDataSourceTemplate, runtimeCommonJSImports, "__importDefault(require('./"+path+"'))")
		inputType = "commonjs"
	}

	return code, inputType, nil
}
