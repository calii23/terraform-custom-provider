package connector

type ConnectorError struct {
	Summary string
	Detail  string
}

func MakeError(connectorName string, summary string, err error) *ConnectorError {
	return &ConnectorError{
		Summary: connectorName + ": " + summary,
		Detail:  err.Error(),
	}
}
