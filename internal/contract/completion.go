package contract

import "errors"

const (
	OperationFinished  = "operation.finished"
	OperationSucceeded = "succeeded"
	OperationFailed    = "failed"
)

// OperationResult is the persisted and streamed payload of operation.finished.
type OperationResult struct {
	APIVersion string    `json:"apiVersion"`
	State      string    `json:"state"`
	ErrorCode  ErrorCode `json:"errorCode,omitempty"`
}

// NewOperationResult converts a runner outcome to its redacted terminal result.
func NewOperationResult(failure *Failure) OperationResult {
	if failure == nil {
		return OperationResult{APIVersion: DaemonAPIVersion, State: OperationSucceeded}
	}
	code := failure.Code
	if !code.Valid() {
		code = CodeInternalInvariant
	}
	return OperationResult{APIVersion: DaemonAPIVersion, State: OperationFailed, ErrorCode: code}
}

// Validate rejects malformed results from storage or the wire without normalizing them.
func (r OperationResult) Validate() error {
	if r.APIVersion != DaemonAPIVersion {
		return errors.New("invalid terminal event")
	}
	switch r.State {
	case OperationSucceeded:
		if r.ErrorCode != "" {
			return errors.New("successful terminal event contains an error")
		}
	case OperationFailed:
		if !r.ErrorCode.Valid() {
			return errors.New("terminal event contains an unknown error")
		}
	default:
		return errors.New("terminal event has invalid state")
	}
	return nil
}
