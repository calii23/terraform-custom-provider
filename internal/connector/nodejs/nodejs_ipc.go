package nodejs

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"github.com/calii23/terraform-provider-custom/internal/connector"
	"github.com/google/uuid"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

type ipcRequest struct {
	ID     string                 `json:"id"`
	Action string                 `json:"action"`
	Input  map[string]interface{} `json:"input"`
}

type ipcResponse struct {
	ID      string                 `json:"id"`
	Success bool                   `json:"success"`
	Result  map[string]interface{} `json:"result"`
}

type nodejsIpc struct {
	connectorName    string
	scanner          *bufio.Scanner
	output           io.WriteCloser
	process          *exec.Cmd
	processOutput    bytes.Buffer
	writeMutex       sync.Mutex
	responseChannels map[string]chan ipcResponse
	errorChanel      chan error
	operationTimeout time.Duration
}

func (n *nodejsIpc) listen() {
	for n.scanner.Scan() {
		responseJson := n.scanner.Bytes()
		var response ipcResponse
		err := json.Unmarshal(responseJson, &response)
		if err != nil {
			n.errorChanel <- errors.New("Failed to unmarshal response: " + err.Error())
			continue
		}

		responseChannel, exists := n.responseChannels[response.ID]
		if exists {
			responseChannel <- response
		} else {
			n.errorChanel <- errors.New("No response channel found for ID: " + response.ID)
		}
	}
}

func (n *nodejsIpc) requestAction(action string, input map[string]interface{}) (*map[string]interface{}, *connector.ConnectorError) {
	if err := n.checkAlive(); err != nil {
		return nil, err
	}

	idValue, err := uuid.NewRandom()
	if err != nil {
		return nil, makeNodejsError(n.connectorName, action, "Failed to generate unique ID", err.Error(), n.processOutput)
	}
	id := idValue.String()

	n.writeMutex.Lock()
	err = json.NewEncoder(n.output).Encode(ipcRequest{
		ID:     id,
		Action: action,
		Input:  input,
	})
	n.writeMutex.Unlock()

	if err != nil {
		return nil, makeNodejsError(n.connectorName, action, "Failed to write request to Node.js process", err.Error(), n.processOutput)
	}

	responseChannel := make(chan ipcResponse)
	n.responseChannels[id] = responseChannel
	defer delete(n.responseChannels, id)

	select {
	case response := <-responseChannel:
		result := response.Result
		if response.Success {
			return &result, nil
		} else {
			return nil, extractErrorFromResponse(result, n.connectorName, action)
		}
	case err := <-n.errorChanel:
		return nil, makeNodejsError(n.connectorName, action, "Failed to read response", err.Error(), n.processOutput)
	case <-time.After(n.operationTimeout):
		return nil, makeNodejsError(n.connectorName, action, "Timeout waiting for response", "No response received", n.processOutput)
	}
}

func (n *nodejsIpc) checkAlive() *connector.ConnectorError {
	if n.process.Process.Signal(syscall.Signal(0)) != nil {
		return &connector.ConnectorError{
			Summary: n.connectorName + ": Process is not alive",
			Detail:  "Process has exited unexpectedly and cannot process requests",
		}
	}

	return nil
}

func extractErrorFromResponse(response map[string]interface{}, connectorName string, action string) *connector.ConnectorError {
	summary, ok := response["summary"].(string)
	if !ok {
		summary = "Unknown error"
	}

	detail, ok := response["detail"].(string)
	if !ok {
		detail = "No detail provided"
	}

	summary = connectorName + " during " + action + ": " + summary

	return &connector.ConnectorError{Summary: summary, Detail: detail}
}
