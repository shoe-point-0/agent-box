package up

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/journal"
)

// Emit persists an already-redacted command event before clients may see it.
type Emit func(kind string, details any) error

// Assemble constructs the attempt from current persisted and external state.
type Assemble func(context.Context, journal.Operation) (*Coordinator, error)

// OperationRunner adapts the up coordinator to daemon.Runner.
type OperationRunner struct {
	Assemble Assemble
}

// Run validates the opaque command input, assembles current dependencies, and
// returns only stable redacted failures to the daemon transport.
func (r OperationRunner) Run(ctx context.Context, operation journal.Operation, input json.RawMessage, emit Emit) *contract.Failure {
	if operation.Kind != "up" || r.Assemble == nil || emit == nil {
		return upFailure(contract.CodeInternalInvariant, false)
	}
	if err := decodeInput(input); err != nil {
		return upFailure(contract.CodeCLIUsage, false)
	}
	if err := emit("up.started", struct {
		BoxID string `json:"boxId"`
	}{BoxID: operation.BoxID}); err != nil {
		return upFailure(contract.CodeInternalInvariant, false)
	}
	coordinator, err := r.Assemble(ctx, operation)
	if err != nil || coordinator == nil {
		return classifyFailure(err)
	}
	if err := coordinator.Reconcile(ctx); err != nil {
		return classifyFailure(err)
	}
	if err := emit("up.ready", struct {
		BoxID string `json:"boxId"`
	}{BoxID: operation.BoxID}); err != nil {
		return upFailure(contract.CodeInternalInvariant, false)
	}
	return nil
}

func decodeInput(input json.RawMessage) error {
	if len(input) == 0 {
		input = json.RawMessage(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	var value struct{}
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("up input contains trailing data")
	}
	return nil
}

func classifyFailure(err error) *contract.Failure {
	code := contract.CodeInfrastructureApplyFailed
	retryable := false
	var coded interface{ ErrorCode() contract.ErrorCode }
	if errors.As(err, &coded) && coded.ErrorCode().Valid() {
		code = coded.ErrorCode()
	}
	if errors.Is(err, journal.ErrLeaseHeld) || errors.Is(err, journal.ErrLeaseLost) {
		code = contract.CodeOperationConflict
		retryable = true
	}
	if err == nil {
		code = contract.CodeInternalInvariant
	}
	return upFailure(code, retryable)
}

func upFailure(code contract.ErrorCode, retryable bool) *contract.Failure {
	if !code.Valid() {
		code = contract.CodeInternalInvariant
	}
	return &contract.Failure{
		SchemaVersion: contract.ErrorSchemaVersion,
		Code:          code,
		Message:       fmt.Sprintf("Box reconciliation failed (%s)", code),
		Retryable:     retryable,
	}
}
