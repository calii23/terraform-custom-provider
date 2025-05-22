package nodejs

import (
	"bufio"
	"bytes"
	"errors"
	"github.com/calii23/terraform-provider-custom/internal/connector"
	"time"
)

func makeNodejsError(connectorName string, action string, summary string, err string, processOutput bytes.Buffer) *connector.ConnectorError {
	processOutputBytes := processOutput.Bytes()
	actionContext := ""
	if action != "" {
		actionContext = " during " + action
	}

	if len(processOutputBytes) > 0 {
		return &connector.ConnectorError{
			Summary: connectorName + actionContext + ": " + summary,
			Detail:  err + "\n\nProcess output:\n" + string(processOutputBytes),
		}
	}

	return &connector.ConnectorError{
		Summary: connectorName + actionContext + ": " + summary,
		Detail:  err,
	}
}

// scanWithTimeout performs a scanner.Scan() with a timeout.
func scanWithTimeout(scanner *bufio.Scanner, timeout time.Duration) (bool, error) {
	scanCh := make(chan bool)
	errCh := make(chan error)

	go func() {
		result := scanner.Scan()
		scanCh <- result
		if !result {
			if err := scanner.Err(); err != nil {
				errCh <- err
			} else {
				errCh <- nil
			}
		} else {
			errCh <- nil
		}
	}()

	select {
	case result := <-scanCh:
		return result, <-errCh
	case <-time.After(timeout):
		return false, errors.New("scanner.Scan() timed out after " + timeout.String())
	}
}
