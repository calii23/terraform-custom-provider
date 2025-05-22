package nodejs

import (
	"bytes"
	"context"
	"github.com/calii23/terraform-provider-custom/internal/connector"
)

var _ connector.DataSourceConnector = (*nodejsDataSourceConnector)(nil)

type nodejsDataSourceConnector struct {
	connectorName string
	name          string
	processOutput *bytes.Buffer
	ipc           *nodejsIpc
}

func (n *nodejsDataSourceConnector) Name() string {
	return n.name
}

func (n *nodejsDataSourceConnector) Read(_ context.Context, input map[string]interface{}) (*map[string]interface{}, *connector.ConnectorError) {
	return n.ipc.requestAction("read", input)
}
