package removal_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/journal"
	"github.com/shoe-point-0/agent-box/internal/removal"
)

type emittedRemovalEvent struct {
	kind    string
	details any
}

func TestOperationRunnerEmitsExactPreviewAndRefusesBeforeLifecycle(t *testing.T) {
	t.Parallel()
	log := []string{}
	step := newStep("worktree/repo", removal.StageWorktree, removal.Fact{
		Present: true,
		Evidence: map[string]string{
			"branch": "abox/task/repo",
			"inode":  "device:1/inode:2",
		},
		Losses: []removal.Loss{{Kind: removal.LossWorktreeChange, Value: "/boxes/selected/workspace/repo/untracked.txt"}},
	}, &log)
	coordinator := newCoordinator(t, []*fakeStep{step}, &log, nil)
	events := []emittedRemovalEvent{}
	runner := removal.OperationRunner{Assemble: func(_ context.Context, _ journal.Operation) (*removal.Coordinator, error) {
		return coordinator, nil
	}}

	failure := runner.Run(t.Context(), removalOperation(), json.RawMessage(`{}`), func(kind string, details any) error {
		events = append(events, emittedRemovalEvent{kind: kind, details: details})
		return nil
	})
	if failure == nil || failure.Code != contract.CodeCleanupRefused || failure.ExitCode() != contract.ExitSafety {
		t.Fatalf("Run() failure = %#v, want cleanup refusal", failure)
	}
	if len(events) != 1 || events[0].kind != "removal.preview" {
		t.Fatalf("events = %#v, want one preview", events)
	}
	preview, ok := events[0].details.(removal.Preview)
	if !ok || preview.BoxID != "box-selected" || preview.Digest == "" || !slices.Equal(preview.Losses, step.fact.Losses) {
		t.Fatalf("preview event = %#v", events[0].details)
	}
	if !slices.Equal(log, []string{"worktree/repo.observe"}) || step.removes != 0 {
		t.Fatalf("refusal lifecycle/removes = %v/%d, want observation only", log, step.removes)
	}
}

func TestOperationRunnerExecutesOnlyDigestBoundExplicitAuthorization(t *testing.T) {
	t.Parallel()
	log := []string{}
	step := newStep("compose/selected", removal.StageCompose, removal.Fact{
		Present:  true,
		Evidence: evidence("owned-compose-id"),
		Losses:   []removal.Loss{{Kind: removal.LossVolume, Value: "selected-db"}},
	}, &log)
	coordinator := newCoordinator(t, []*fakeStep{step}, &log, nil)
	preview, err := coordinator.Preview(t.Context(), "box-selected")
	if err != nil {
		t.Fatal(err)
	}
	log = log[:0]
	input, err := json.Marshal(removal.OperationInput{
		PreviewDigest: preview.Digest,
		Yes:           true,
		DeleteVolumes: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	events := []emittedRemovalEvent{}
	runner := removal.OperationRunner{Assemble: func(_ context.Context, _ journal.Operation) (*removal.Coordinator, error) {
		return coordinator, nil
	}}

	failure := runner.Run(t.Context(), removalOperation(), input, func(kind string, details any) error {
		events = append(events, emittedRemovalEvent{kind: kind, details: details})
		return nil
	})
	if failure != nil {
		t.Fatalf("Run() failure = %#v", failure)
	}
	if got := []string{events[0].kind, events[1].kind}; !slices.Equal(got, []string{"removal.preview", "removal.completed"}) {
		t.Fatalf("event kinds = %v", got)
	}
	if step.removes != 1 {
		t.Fatalf("remove calls = %d, want 1", step.removes)
	}
	wantLifecycle := []string{
		"compose/selected.observe",
		"lease.acquire", "writers.quiesce", "compose/selected.observe",
		"compose/selected.observe", "event.remove-started.compose/selected", "compose/selected.remove", "event.remove-succeeded.compose/selected",
		"writers.resume", "lease.release",
	}
	if !slices.Equal(log, wantLifecycle) {
		t.Fatalf("lifecycle = %v, want %v", log, wantLifecycle)
	}
}

func TestOperationRunnerRejectsMalformedOrStaleAuthorizationBeforeMutation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input json.RawMessage
		code  contract.ErrorCode
	}{
		{name: "unknown field", input: json.RawMessage(`{"yes":true,"force":true}`), code: contract.CodeCLIUsage},
		{name: "trailing value", input: json.RawMessage(`{} {}`), code: contract.CodeCLIUsage},
		{name: "stale digest", input: json.RawMessage(`{"previewDigest":"sha256:stale","yes":true}`), code: contract.CodeCleanupRefused},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			log := []string{}
			step := newStep("runtime-host/selected", removal.StageRuntimeHost, removal.Fact{
				Present: true, Evidence: evidence("owned-runtime-id"), Losses: []removal.Loss{},
			}, &log)
			coordinator := newCoordinator(t, []*fakeStep{step}, &log, nil)
			assembled := 0
			runner := removal.OperationRunner{Assemble: func(_ context.Context, _ journal.Operation) (*removal.Coordinator, error) {
				assembled++
				return coordinator, nil
			}}
			failure := runner.Run(t.Context(), removalOperation(), test.input, func(string, any) error { return nil })
			if failure == nil || failure.Code != test.code || step.removes != 0 {
				t.Fatalf("Run() failure/removes = %#v/%d, want %s/0", failure, step.removes, test.code)
			}
			if test.code == contract.CodeCLIUsage && (assembled != 0 || len(log) != 0) {
				t.Fatalf("malformed input assembled/observed = %d/%v", assembled, log)
			}
			if test.code == contract.CodeCleanupRefused && !slices.Equal(log, []string{"runtime-host/selected.observe"}) {
				t.Fatalf("stale authorization lifecycle = %v, want preview only", log)
			}
		})
	}
}

func TestOperationRunnerFailsClosedWhenAuthorityOrPreviewEventIsUnavailable(t *testing.T) {
	t.Parallel()
	operation := removalOperation()
	runner := removal.OperationRunner{Assemble: func(_ context.Context, _ journal.Operation) (*removal.Coordinator, error) {
		return nil, removal.ErrRemovalSnapshotAbsent
	}}
	failure := runner.Run(t.Context(), operation, json.RawMessage(`{}`), func(string, any) error { return nil })
	if failure == nil || failure.Code != contract.CodeOwnershipConflict {
		t.Fatalf("missing authority failure = %#v", failure)
	}

	log := []string{}
	step := newStep("runtime-host/selected", removal.StageRuntimeHost, removal.Fact{
		Present: true, Evidence: evidence("runtime-id"), Losses: []removal.Loss{},
	}, &log)
	coordinator := newCoordinator(t, []*fakeStep{step}, &log, nil)
	runner.Assemble = func(_ context.Context, _ journal.Operation) (*removal.Coordinator, error) { return coordinator, nil }
	failure = runner.Run(t.Context(), operation, json.RawMessage(`{"yes":true}`), func(string, any) error {
		return errors.New("journal unavailable")
	})
	if failure == nil || failure.Code != contract.CodeInternalInvariant || step.removes != 0 ||
		!slices.Equal(log, []string{"runtime-host/selected.observe"}) {
		t.Fatalf("event failure/removes/lifecycle = %#v/%d/%v", failure, step.removes, log)
	}
}

func removalOperation() journal.Operation {
	return journal.Operation{ID: "operation-remove", BoxID: "box-selected", Kind: "remove", State: journal.StateRunning}
}
