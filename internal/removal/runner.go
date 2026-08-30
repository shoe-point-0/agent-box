package removal

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

// OperationInput is the command boundary for one digest-bound removal attempt.
// An empty or stale preview digest can inventory losses but cannot acquire a
// lease or mutate a resource.
type OperationInput struct {
	PreviewDigest             string `json:"previewDigest,omitempty"`
	Yes                       bool   `json:"yes,omitempty"`
	DiscardWorktreeChanges    bool   `json:"discardWorktreeChanges,omitempty"`
	DiscardUnpublishedCommits bool   `json:"discardUnpublishedCommits,omitempty"`
	DeleteVolumes             bool   `json:"deleteVolumes,omitempty"`
}

// AssembleOperation reconstructs one operation-owned coordinator. Assembly must
// not observe or mutate external resources; Run invokes Preview explicitly.
type AssembleOperation func(context.Context, journal.Operation) (*Coordinator, error)

// OperationEmit persists one already-redacted event in the owning operation.
type OperationEmit func(string, any) error

// OperationRunner bridges the daemon operation contract to refusal-first
// preview and execution without coupling this package to HTTP transport.
type OperationRunner struct {
	Assemble AssembleOperation
}

// Run always emits the exact preview before considering authorization. It
// returns a stable failure rather than provider diagnostics, which may contain
// workload data or secrets.
func (r OperationRunner) Run(ctx context.Context, operation journal.Operation, raw json.RawMessage, emit OperationEmit) *contract.Failure {
	if r.Assemble == nil || emit == nil || operation.Kind != "remove" || operation.State != journal.StateRunning {
		return operationFailure(contract.CodeInternalInvariant, "removal operation is not configured")
	}
	input, err := decodeOperationInput(raw)
	if err != nil {
		return operationFailure(contract.CodeCLIUsage, "removal authorization is invalid")
	}
	coordinator, err := r.Assemble(ctx, operation)
	if err != nil {
		return classifyOperationError(err)
	}
	if coordinator == nil {
		return operationFailure(contract.CodeInternalInvariant, "removal coordinator is unavailable")
	}

	preview, err := coordinator.Preview(ctx, operation.BoxID)
	if err != nil {
		return classifyOperationError(err)
	}
	if err := emit("removal.preview", preview); err != nil {
		return operationFailure(contract.CodeInternalInvariant, "cannot persist removal preview")
	}

	result, err := coordinator.Execute(ctx, preview, Authorization(input))
	if err != nil {
		return classifyOperationError(err)
	}
	if err := emit("removal.completed", struct {
		Completed []string `json:"completed"`
		Remaining []string `json:"remaining"`
	}{Completed: nonNil(result.Completed), Remaining: nonNil(result.Remaining)}); err != nil {
		return operationFailure(contract.CodeInternalInvariant, "cannot persist removal completion")
	}
	return nil
}

func decodeOperationInput(raw json.RawMessage) (OperationInput, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var input OperationInput
	if err := decoder.Decode(&input); err != nil {
		return OperationInput{}, fmt.Errorf("decode removal authorization: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return OperationInput{}, errors.New("decode removal authorization: trailing content")
	}
	return input, nil
}

func classifyOperationError(err error) *contract.Failure {
	if errors.Is(err, ErrSafetyRefusal) || errors.Is(err, ErrInventoryDrift) {
		return operationFailure(contract.CodeCleanupRefused, "removal requires a fresh preview and explicit authorization")
	}
	if errors.Is(err, ErrRemovalSnapshotAbsent) {
		return operationFailure(contract.CodeOwnershipConflict, "removal ownership authority is unavailable")
	}
	var coded interface{ ErrorCode() contract.ErrorCode }
	if errors.As(err, &coded) && coded.ErrorCode().Valid() {
		return operationFailure(coded.ErrorCode(), "removal boundary failed")
	}
	return operationFailure(contract.CodeInfrastructureApplyFailed, "removal did not converge")
}

func operationFailure(code contract.ErrorCode, message string) *contract.Failure {
	return &contract.Failure{
		SchemaVersion: contract.ErrorSchemaVersion,
		Code:          code,
		Message:       message,
		Retryable:     false,
	}
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
