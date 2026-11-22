package removal_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/journal"
	"github.com/shoe-point-0/agent-box/internal/removal"
	"github.com/shoe-point-0/agent-box/internal/state"
)

func TestJournalLifecycleRenewsFencedLeaseAndPersistsTransitions(t *testing.T) {
	operationJournal, operation := beginRemovalOperation(t)
	owner := journal.Owner{InstanceID: "daemon-removal", ProcessStartID: "process-removal", BootID: "boot-removal"}
	const ttl = 300 * time.Millisecond
	guard, err := removal.NewJournalLeaseGuard(removal.JournalLeaseGuardConfig{
		Journal: operationJournal, Operation: operation, Owner: owner, TTL: ttl,
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder, err := removal.NewEventRecorder(operation.ID, func(ctx context.Context, operationID, kind string, details json.RawMessage, occurredAt time.Time) error {
		_, appendErr := operationJournal.AppendEvent(ctx, operationID, kind, details, occurredAt)
		return appendErr
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	log := []string{}
	step := newStep("compose/project", removal.StageCompose, present("owned-project"), &log)
	contender := journal.Owner{InstanceID: "daemon-contender", ProcessStartID: "process-contender", BootID: "boot-removal"}
	step.removeFn = func() error {
		time.Sleep(700 * time.Millisecond)
		_, acquireErr := operationJournal.AcquireLease(t.Context(), operation.BoxID, operation.ID, contender, time.Now().UTC(), ttl)
		if !errors.Is(acquireErr, journal.ErrLeaseHeld) {
			return errors.New("renewed removal lease was not held against a contender")
		}
		return nil
	}
	coordinator, err := removal.New(removal.Config{
		Steps: []removal.Step{step}, Lease: guard, Quiescer: fakeQuiescer{log: &log}, Recorder: recorder,
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := coordinator.Preview(t.Context(), operation.BoxID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.Execute(t.Context(), preview, authorizeAll(preview))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(result.Completed, []string{"compose/project"}) || len(result.Remaining) != 0 {
		t.Fatalf("result = %#v", result)
	}

	events, err := operationJournal.Events(t.Context(), operation.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	kinds := make([]string, len(events))
	for index, event := range events {
		kinds[index] = event.Kind
		var details struct {
			Resource string `json:"resource"`
		}
		if err := json.Unmarshal(event.Details, &details); err != nil || details.Resource != "compose/project" {
			t.Fatalf("event %d details = %s, error = %v", index, event.Details, err)
		}
	}
	if !slices.Equal(kinds, []string{"removal.remove-started", "removal.remove-succeeded"}) {
		t.Fatalf("event kinds = %v", kinds)
	}

	successor, err := operationJournal.AcquireLease(t.Context(), operation.BoxID, operation.ID, contender, time.Now().UTC(), ttl)
	if err != nil {
		t.Fatalf("acquire released lease: %v", err)
	}
	if successor.Fence != 2 {
		t.Fatalf("successor fence = %d, want 2", successor.Fence)
	}
	if err := operationJournal.ReleaseLease(t.Context(), successor, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
}

func TestJournalLifecycleRejectsMismatchedBoxBeforeLeaseWrite(t *testing.T) {
	operationJournal, operation := beginRemovalOperation(t)
	guard, err := removal.NewJournalLeaseGuard(removal.JournalLeaseGuardConfig{
		Journal: operationJournal, Operation: operation,
		Owner: journal.Owner{InstanceID: "daemon-removal", ProcessStartID: "process-removal", BootID: "boot-removal"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := guard.Acquire(t.Context(), "other-box"); err == nil {
		t.Fatal("Acquire accepted a Box outside the owning operation")
	}
}

func TestLeaseRenewalFailureStopsBeforeNextDestructiveStep(t *testing.T) {
	leaseStore := &failingRenewalJournal{}
	guard, err := removal.NewJournalLeaseGuard(removal.JournalLeaseGuardConfig{
		Journal: leaseStore,
		Operation: journal.Operation{
			ID: "operation-removal", BoxID: "box-removal", State: journal.StateRunning,
		},
		Owner: journal.Owner{InstanceID: "daemon-removal", ProcessStartID: "process-removal", BootID: "boot-removal"},
		TTL:   60 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	log := []string{}
	first := newStep("compose/project", removal.StageCompose, present("owned-project"), &log)
	first.removeFn = func() error {
		time.Sleep(80 * time.Millisecond)
		return nil
	}
	second := newStep("runtime-host/box-removal", removal.StageRuntimeHost, present("owned-host"), &log)
	coordinator, err := removal.New(removal.Config{
		Steps: []removal.Step{first, second}, Lease: guard,
		Quiescer: fakeQuiescer{log: &log}, Recorder: fakeRecorder{log: &log},
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := coordinator.Preview(t.Context(), "box-removal")
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.Execute(t.Context(), preview, authorizeAll(preview))
	if !errors.Is(err, journal.ErrLeaseLost) {
		t.Fatalf("Execute error = %v, want lease loss", err)
	}
	if !slices.Equal(result.Completed, []string{"compose/project"}) ||
		!slices.Equal(result.Remaining, []string{"runtime-host/box-removal"}) {
		t.Fatalf("result = %#v", result)
	}
	if first.removes != 1 || second.removes != 0 {
		t.Fatalf("remove calls = %d/%d, want 1/0", first.removes, second.removes)
	}
}

type failingRenewalJournal struct{}

func (*failingRenewalJournal) AcquireLease(_ context.Context, boxID, operationID string, owner journal.Owner, now time.Time, ttl time.Duration) (journal.Lease, error) {
	return journal.Lease{
		BoxID: boxID, OperationID: operationID, Owner: owner, Fence: 1,
		AcquiredAt: now, RenewedAt: now, ExpiresAt: now.Add(ttl),
	}, nil
}

func (*failingRenewalJournal) RenewLease(context.Context, journal.Lease, time.Time, time.Duration) (journal.Lease, error) {
	return journal.Lease{}, journal.ErrLeaseLost
}

func (*failingRenewalJournal) ReleaseLease(context.Context, journal.Lease, time.Time) error {
	return nil
}

func beginRemovalOperation(t *testing.T) (*journal.Journal, journal.Operation) {
	t.Helper()
	store, err := state.Open(t.Context(), filepath.Join(t.TempDir(), "state", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	operationJournal, err := journal.New(store)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(t.TempDir(), "workspace", ".abox", "box.yaml")
	workspaceID, err := identity.Workspace(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	boxID, err := identity.Box(workspaceID, "TASK-REMOVE")
	if err != nil {
		t.Fatal(err)
	}
	operation, created, err := operationJournal.Begin(t.Context(), journal.BeginRequest{
		WorkspaceID: workspaceID, BoxID: boxID, TaskKey: "TASK-REMOVE",
		ManifestPath: manifestPath, ManifestDigest: "sha256:manifest",
		OperationID: "operation-removal", IdempotencyKey: "request-removal", Kind: "rm",
		RequestDigest: "sha256:request-removal", StartedAt: time.Now().UTC(),
	})
	if err != nil || !created {
		t.Fatalf("begin removal operation: created=%t error=%v", created, err)
	}
	return operationJournal, operation
}

func TestLeaseReleaseDoesNotReportItsOwnRenewalCancellation(t *testing.T) {
	for _, test := range []struct {
		name    string
		failure error
	}{
		{name: "intentional release"},
		{name: "concurrent lease loss", failure: journal.ErrLeaseLost},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &blockedRenewalJournal{started: make(chan struct{}), failure: test.failure}
			guard, err := removal.NewJournalLeaseGuard(removal.JournalLeaseGuardConfig{
				Journal:   store,
				Operation: journal.Operation{ID: "operation-release", BoxID: "box-release", State: journal.StateRunning},
				Owner:     journal.Owner{InstanceID: "daemon", ProcessStartID: "process", BootID: "boot"},
				TTL:       3 * time.Millisecond,
			})
			if err != nil {
				t.Fatal(err)
			}
			lease, err := guard.Acquire(t.Context(), "box-release")
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-store.started:
			case <-time.After(time.Second):
				t.Fatal("renewal did not start")
			}
			err = guard.Release(t.Context(), lease)
			if !errors.Is(err, test.failure) {
				t.Fatalf("Release() = %v, want %v", err, test.failure)
			}
		})
	}
}

type blockedRenewalJournal struct {
	failingRenewalJournal
	started chan struct{}
	failure error
}

func (store *blockedRenewalJournal) RenewLease(ctx context.Context, _ journal.Lease, _ time.Time, _ time.Duration) (journal.Lease, error) {
	close(store.started)
	<-ctx.Done()
	if store.failure != nil {
		return journal.Lease{}, store.failure
	}
	return journal.Lease{}, ctx.Err()
}
