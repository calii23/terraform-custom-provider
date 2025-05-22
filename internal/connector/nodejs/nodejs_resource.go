package nodejs

import (
	"bytes"
	"context"
	"github.com/calii23/terraform-provider-custom/internal/connector"
)

var _ connector.ResourceConnector = (*nodejsResourceConnector)(nil)

type nodejsResourceConnector struct {
	connectorName   string
	requiresReplace []string
	name            string
	processOutput   *bytes.Buffer
	ipc             *nodejsIpc
}

func (n *nodejsResourceConnector) Name() string {
	return n.name
}

func (n *nodejsResourceConnector) RequiresReplacePaths() []string {
	return n.requiresReplace
}

func (n *nodejsResourceConnector) Create(_ context.Context, input map[string]interface{}) (*map[string]interface{}, *connector.ConnectorError) {
	return n.ipc.requestAction("create", input)
}

func (n *nodejsResourceConnector) Read(_ context.Context, input map[string]interface{}, currentState map[string]interface{}) (*map[string]interface{}, *map[string]interface{}, *connector.ConnectorError) {
	response, err := n.ipc.requestAction("read", map[string]interface{}{
		"input": input,
		"state": currentState,
	})
	if err != nil {
		return nil, nil, err
	}

	var updatedInput, updatedState *map[string]interface{}

	if (*response)["input"] == nil {
		updatedInput = nil
	} else {
		value, ok := (*response)["input"].(map[string]interface{})
		if !ok {
			return nil, nil, &connector.ConnectorError{
				Summary: n.connectorName + " during read: Invalid input type",
				Detail:  "Field `input` in response is expected to be an object",
			}
		}

		updatedInput = &value
	}

	if (*response)["state"] == nil {
		updatedState = nil
	} else {
		value, ok := (*response)["state"].(map[string]interface{})
		if !ok {
			return nil, nil, &connector.ConnectorError{
				Summary: n.connectorName + " during read: Invalid state type",
				Detail:  "Field `state` in response is expected to be an object",
			}
		}

		updatedState = &value
	}

	return updatedInput, updatedState, nil
}

func (n *nodejsResourceConnector) Update(_ context.Context, oldInput map[string]interface{}, newInput map[string]interface{}, currentState map[string]interface{}) (*map[string]interface{}, *connector.ConnectorError) {
	return n.ipc.requestAction("update", map[string]interface{}{
		"oldInput": oldInput,
		"newInput": newInput,
		"state":    currentState,
	})
}

func (n *nodejsResourceConnector) Delete(_ context.Context, input map[string]interface{}, currentState map[string]interface{}) *connector.ConnectorError {
	_, err := n.ipc.requestAction("delete", map[string]interface{}{
		"input": input,
		"state": currentState,
	})

	return err
}

func (n *nodejsResourceConnector) ImportState(_ context.Context, id string) (*map[string]interface{}, *map[string]interface{}, *connector.ConnectorError) {
	response, err := n.ipc.requestAction("import", map[string]interface{}{
		"id": id,
	})
	if err != nil {
		return nil, nil, err
	}

	input, ok := (*response)["input"].(map[string]interface{})
	if !ok {
		return nil, nil, &connector.ConnectorError{
			Summary: n.connectorName + " during import: Invalid input type",
			Detail:  "Field `input` in response is expected to be an object",
		}
	}

	state, ok := (*response)["state"].(map[string]interface{})
	if !ok {
		return nil, nil, &connector.ConnectorError{
			Summary: n.connectorName + " during import: Invalid state type",
			Detail:  "Field `state` in response is expected to be an object",
		}
	}

	return &input, &state, nil
}
