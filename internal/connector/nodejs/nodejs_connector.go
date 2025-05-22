package nodejs

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/calii23/terraform-provider-custom/internal/connector"
	"io"
	"os"
	"os/exec"
	"path"
	"syscall"
	"time"
)

type initialResponse struct {
	name            string
	requiresReplace []string
}

func initializeNodejsConnector(
	executable string,
	nodejsArgs []string,
	connectorName string,
	script string,
	inputType string,
	cwd string,
	initializeTimeout time.Duration,
	operationTimeout time.Duration,
	logFile *string,
	environmentVariables map[string]string,
) (*initialResponse, *bytes.Buffer, *nodejsIpc, *connector.ConnectorError) {

	var args []string
	if nodejsArgs != nil {
		args = append(args, nodejsArgs...)
	}

	args = append(args, "--input-type", inputType, "-")

	// Create Node.js process using -e to accept script via stdin
	process := exec.Command(executable, args...)

	process.Dir = cwd

	env := os.Environ()

	for key, value := range environmentVariables {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	process.Env = env
	process.Stdin = bytes.NewBufferString(script)

	r3, w3, err := os.Pipe()
	if err != nil {
		return nil, nil, nil, connector.MakeError(connectorName, "Failed to create pipe for FD 3", err)
	}
	r4, w4, err := os.Pipe()
	if err != nil {
		return nil, nil, nil, connector.MakeError(connectorName, "Failed to create pipe for FD 4", err)
	}

	// Set FD 3 (Go to Node.js) and FD 4 (Node.js to Go)
	process.ExtraFiles = []*os.File{r3, w4}

	var processOutput bytes.Buffer

	if logFile != nil {
		logFileHandle, err := os.OpenFile(*logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			return nil, nil, nil, connector.MakeError(connectorName, "Failed to open log file", err)
		}

		process.Stdout = io.MultiWriter(logFileHandle, &processOutput)
		process.Stderr = io.MultiWriter(logFileHandle, &processOutput)
	} else {
		process.Stdout = &processOutput
		process.Stderr = &processOutput
	}

	err = process.Start()
	if err != nil {
		return nil, nil, nil, connector.MakeError(connectorName, "Failed to run Node.js runtime script", err)
	}

	// Close the write end of FD 3 and read end of FD 4 in the parent process
	err = r3.Close()
	if err != nil {
		return nil, nil, nil, connector.MakeError(connectorName, "Failed to close write end of FD 3", err)
	}

	err = w4.Close()
	if err != nil {
		return nil, nil, nil, connector.MakeError(connectorName, "Failed to close read end of FD 4", err)
	}

	scanner := bufio.NewScanner(r4)
	scanResult, err := scanWithTimeout(scanner, initializeTimeout)
	if !scanResult {
		time.Sleep(300 * time.Millisecond)
		if process.Process.Signal(syscall.Signal(0)) != nil {
			return nil, nil, nil, makeNodejsError(connectorName, "initialization", "Failed to start", "The runtime of the Node.js script could not be initialized", processOutput)
		}

		_ = process.Process.Kill()
		if err != nil {
			return nil, nil, nil, makeNodejsError(connectorName, "initialization", "Failed to read initial response", err.Error(), processOutput)
		}

		return nil, nil, nil, makeNodejsError(connectorName, "initialization", "Failed to read initial response", "No initial response received", processOutput)
	}

	var initialIpcResponse ipcResponse
	err = json.Unmarshal(scanner.Bytes(), &initialIpcResponse)
	if err != nil {
		_ = process.Process.Kill()
		return nil, nil, nil, makeNodejsError(connectorName, "initialization", "Failed to unmarshal initial response", err.Error(), processOutput)
	}

	var initialResponseResult initialResponse
	if initialIpcResponse.Success {
		name, ok := initialIpcResponse.Result["name"].(string)
		if !ok {
			_ = process.Process.Kill()
			return nil, nil, nil, makeNodejsError(connectorName, "initialization", "Failed to initialize", "The name is not a string", processOutput)
		}
		initialResponseResult.name = name

		var requiresReplace []string
		requiresReplaceJson, ok := initialIpcResponse.Result["requiresReplace"].([]interface{})
		if !ok {
			_ = process.Process.Kill()
			return nil, nil, nil, makeNodejsError(connectorName, "initialization", "Failed to initialize", "The `requiresReplace` field is not a string", processOutput)
		}

		for _, v := range requiresReplaceJson {
			element, ok := v.(string)
			if !ok {
				_ = process.Process.Kill()
				return nil, nil, nil, makeNodejsError(connectorName, "initialization", "Failed to initialize", "The `requiresReplace` has an element which is not a string", processOutput)
			}

			requiresReplace = append(requiresReplace, element)
		}

		initialResponseResult.requiresReplace = requiresReplace
	} else {
		return nil, nil, nil, extractErrorFromResponse(initialIpcResponse.Result, connectorName, "initialization")
	}

	ipc := nodejsIpc{
		connectorName:    connectorName,
		scanner:          scanner,
		process:          process,
		output:           w3,
		processOutput:    processOutput,
		operationTimeout: operationTimeout,
		responseChannels: make(map[string]chan ipcResponse),
		errorChanel:      make(chan error),
	}
	go ipc.listen()

	return &initialResponseResult, &processOutput, &ipc, nil
}

func NewNodejsResourceConnector(executable string, nodejsArgs []string, scriptPath string, initializeTimeout time.Duration, operationTimeout time.Duration, logFile *string, environmentVariables map[string]string, inputTypeSelection InputType) (connector.ResourceConnector, *connector.ConnectorError) {
	connectorName := "Node.js script `" + path.Base(scriptPath) + "`"
	script, inputType, err := buildResourceRuntime(path.Base(scriptPath), inputTypeSelection)
	if err != nil {
		return nil, connector.MakeError(connectorName, "Failed to build runtime script", err)
	}

	initialResponse, processOutput, ipc, connectorErr := initializeNodejsConnector(
		executable,
		nodejsArgs,
		connectorName,
		script,
		inputType,
		path.Dir(scriptPath),
		initializeTimeout,
		operationTimeout,
		logFile,
		environmentVariables)
	if connectorErr != nil {
		return nil, connectorErr
	}

	c := &nodejsResourceConnector{
		connectorName:   connectorName,
		name:            initialResponse.name,
		requiresReplace: initialResponse.requiresReplace,
		processOutput:   processOutput,
		ipc:             ipc,
	}

	return c, nil
}

func NewNodejsDataSourceConnector(executable string, nodejsArgs []string, scriptPath string, initializeTimeout time.Duration, operationTimeout time.Duration, logFile *string, environmentVariables map[string]string, inputTypeSelection InputType) (connector.DataSourceConnector, *connector.ConnectorError) {
	connectorName := "Node.js script `" + path.Base(scriptPath) + "`"
	script, inputType, err := buildDataSourceRuntime(path.Base(scriptPath), inputTypeSelection)
	if err != nil {
		return nil, connector.MakeError(connectorName, "Failed to build runtime script", err)
	}

	initialResponse, processOutput, ipc, connectorErr := initializeNodejsConnector(
		executable,
		nodejsArgs,
		connectorName,
		script,
		inputType,
		path.Dir(scriptPath),
		initializeTimeout,
		operationTimeout,
		logFile,
		environmentVariables)
	if connectorErr != nil {
		return nil, connectorErr
	}

	c := &nodejsDataSourceConnector{
		connectorName: connectorName,
		name:          initialResponse.name,
		processOutput: processOutput,
		ipc:           ipc,
	}

	return c, nil
}
