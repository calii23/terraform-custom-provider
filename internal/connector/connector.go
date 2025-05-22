package connector

import (
	"context"
)

type ResourceConnector interface {
	Name() string

	RequiresReplacePaths() []string

	Create(ctx context.Context, input map[string]interface{}) (*map[string]interface{}, *ConnectorError)

	Read(ctx context.Context, input map[string]interface{}, currentState map[string]interface{}) (*map[string]interface{}, *map[string]interface{}, *ConnectorError)

	Update(ctx context.Context, oldInput map[string]interface{}, newInput map[string]interface{}, currentState map[string]interface{}) (*map[string]interface{}, *ConnectorError)

	Delete(ctx context.Context, input map[string]interface{}, currentState map[string]interface{}) *ConnectorError

	ImportState(ctx context.Context, id string) (*map[string]interface{}, *map[string]interface{}, *ConnectorError)
}

type DataSourceConnector interface {
	Name() string

	Read(ctx context.Context, input map[string]interface{}) (*map[string]interface{}, *ConnectorError)
}
