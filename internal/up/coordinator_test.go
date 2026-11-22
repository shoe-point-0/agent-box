package up

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/journal"
	"github.com/shoe-point-0/agent-box/internal/reconcile"
	"github.com/shoe-point-0/agent-box/internal/state"
)

func TestCoordinatorColdThenWarmReobservesWithoutDuplicateApply(t *testing.T) {
	operationJournal, closeStore := testJournal(t)
	defer closeStore()
	owner := journal.Owner{InstanceID: "daemon-fixture", ProcessStartID: "process-fixture", BootID: "boot-fixture"}
	stages := newStageFixture()
	manifest := filepath.Join(t.TempDir(), ".abox", "box.yaml")

	for attempt := 1; attempt <= 2; attempt++ {
		operation := beginUp(t, operationJournal, manifest, attempt)
		recorder := &memoryRecorder{}
		engine, err := reconcile.New(reconcile.Config{
			MaxParallel: 1, CompensationTimeout: time.Second, AfterEventTimeout: time.Second, Recorder: recorder,
		})
		if err != nil {
			t.Fatal(err)
		}
		coordinator, err := NewCoordinator(Config{
			Journal: operationJournal, Operation: operation, Owner: owner,
			Execute:  func(ctx context.Context) error { return engine.Execute(ctx, stages.operations()) },
			LeaseTTL: time.Second, RenewInterval: 100 * time.Millisecond, CleanupTimeout: time.Second,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := coordinator.Reconcile(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := operationJournal.Complete(t.Context(), operation.ID, contract.NewOperationResult(nil), time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		if got := recorder.transitionCount(reconcile.TransitionApplyStarted); got != map[int]int{1: 9, 2: 0}[attempt] {
			t.Fatalf("attempt %d apply transitions = %d", attempt, got)
		}
	}

	for _, stage := range stages.ordered {
		if stage.observes != 2 {
			t.Errorf("stage %s observations = %d, want 2", stage.key, stage.observes)
		}
		if stage.applies != 1 {
			t.Errorf("stage %s applies = %d, want 1", stage.key, stage.applies)
		}
	}
}

func TestCoordinatorRenewsLeaseDuringLongExecution(t *testing.T) {
	leaseJournal := &countingLeaseJournal{}
	operation := journal.Operation{ID: "operation-long", BoxID: "box-long", Kind: "up"}
	coordinator, err := NewCoordinator(Config{
		Journal: leaseJournal, Operation: operation,
		Owner: journal.Owner{InstanceID: "daemon", ProcessStartID: "process", BootID: "boot"},
		Execute: func(ctx context.Context) error {
			select {
			case <-time.After(90 * time.Millisecond):
				return nil
			case <-ctx.Done():
				return context.Cause(ctx)
			}
		},
		LeaseTTL: 60 * time.Millisecond, RenewInterval: 20 * time.Millisecond, CleanupTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := coordinator.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if leaseJournal.renewals.Load() < 3 {
		t.Fatalf("lease renewals = %d, want at least 3", leaseJournal.renewals.Load())
	}
	if leaseJournal.releases.Load() != 1 {
		t.Fatalf("lease releases = %d, want 1", leaseJournal.releases.Load())
	}
}

func TestCoordinatorLeaseLossCancelsExecutionWithoutUnprovenRelease(t *testing.T) {
	leaseJournal := &countingLeaseJournal{renewErr: journal.ErrLeaseLost}
	canceled := make(chan struct{})
	coordinator, err := NewCoordinator(Config{
		Journal: leaseJournal, Operation: journal.Operation{ID: "operation-lost", BoxID: "box-lost", Kind: "up"},
		Owner: journal.Owner{InstanceID: "daemon", ProcessStartID: "process", BootID: "boot"},
		Execute: func(ctx context.Context) error {
			<-ctx.Done()
			close(canceled)
			return context.Cause(ctx)
		},
		LeaseTTL: 100 * time.Millisecond, RenewInterval: 10 * time.Millisecond, CleanupTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = coordinator.Reconcile(t.Context())
	if !errors.Is(err, journal.ErrLeaseLost) {
		t.Fatalf("reconcile error = %v, want lease lost", err)
	}
	select {
	case <-canceled:
	default:
		t.Fatal("executor was not canceled after lease loss")
	}
	if leaseJournal.releases.Load() != 0 {
		t.Fatalf("lease releases = %d, want 0", leaseJournal.releases.Load())
	}
}

type stageFixture struct {
	ordered []*stageOperation
}

func newStageFixture() *stageFixture {
	keys := []string{"validate", "git", "plan", "runtime-host", "mounts", "compose", "readiness", "context", "ready"}
	fixture := &stageFixture{ordered: make([]*stageOperation, len(keys))}
	for index, key := range keys {
		operation := &stageOperation{key: key}
		if index > 0 {
			operation.dependencies = []string{keys[index-1]}
		}
		fixture.ordered[index] = operation
	}
	return fixture
}

func (f *stageFixture) operations() []reconcile.Operation {
	operations := make([]reconcile.Operation, len(f.ordered))
	for index, operation := range f.ordered {
		operations[index] = operation
	}
	return operations
}

type stageOperation struct {
	key          string
	dependencies []string
	ready        bool
	observes     int
	applies      int
}

func (o *stageOperation) Key() string            { return o.key }
func (o *stageOperation) Dependencies() []string { return append([]string(nil), o.dependencies...) }
func (o *stageOperation) Observe(context.Context) (reconcile.Observation, error) {
	o.observes++
	return reconcile.Observation{Ready: o.ready}, nil
}
func (o *stageOperation) Apply(context.Context, reconcile.Observation) (reconcile.Result, error) {
	o.applies++
	o.ready = true
	return reconcile.Result{Created: true}, nil
}
func (o *stageOperation) Compensate(context.Context, reconcile.Result) error {
	o.ready = false
	return nil
}

type memoryRecorder struct {
	mu          sync.Mutex
	transitions []reconcile.Transition
}

func (r *memoryRecorder) Record(_ context.Context, transition reconcile.Transition) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.transitions = append(r.transitions, transition)
	return nil
}

func (r *memoryRecorder) transitionCount(kind string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, transition := range r.transitions {
		if transition.Kind == kind {
			count++
		}
	}
	return count
}

type countingLeaseJournal struct {
	renewals atomic.Int64
	releases atomic.Int64
	renewErr error
}

func (j *countingLeaseJournal) AcquireLease(_ context.Context, boxID, operationID string, owner journal.Owner, now time.Time, ttl time.Duration) (journal.Lease, error) {
	return journal.Lease{BoxID: boxID, OperationID: operationID, Owner: owner, Fence: 1, AcquiredAt: now, RenewedAt: now, ExpiresAt: now.Add(ttl)}, nil
}

func (j *countingLeaseJournal) RenewLease(_ context.Context, lease journal.Lease, now time.Time, ttl time.Duration) (journal.Lease, error) {
	j.renewals.Add(1)
	if j.renewErr != nil {
		return journal.Lease{}, j.renewErr
	}
	lease.RenewedAt, lease.ExpiresAt = now, now.Add(ttl)
	return lease, nil
}

func (j *countingLeaseJournal) ReleaseLease(_ context.Context, _ journal.Lease, _ time.Time) error {
	j.releases.Add(1)
	return nil
}

func testJournal(t *testing.T) (*journal.Journal, func()) {
	t.Helper()
	store, err := state.Open(t.Context(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	operationJournal, err := journal.New(store)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	return operationJournal, func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	}
}

func beginUp(t *testing.T, operationJournal *journal.Journal, manifest string, attempt int) journal.Operation {
	t.Helper()
	workspaceID, err := identity.Workspace(manifest)
	if err != nil {
		t.Fatal(err)
	}
	boxID, err := identity.Box(workspaceID, "TASK-1")
	if err != nil {
		t.Fatal(err)
	}
	operation, created, err := operationJournal.Begin(t.Context(), journal.BeginRequest{
		WorkspaceID: workspaceID, BoxID: boxID, TaskKey: "TASK-1", ManifestPath: manifest,
		ManifestDigest: "sha256:manifest", OperationID: fmt.Sprintf("operation-%d", attempt),
		IdempotencyKey: fmt.Sprintf("key-%d", attempt), Kind: "up", RequestDigest: fmt.Sprintf("sha256:request-%d", attempt),
		StartedAt: time.Now().UTC(),
	})
	if err != nil || !created {
		t.Fatalf("begin attempt %d: created=%t error=%v", attempt, created, err)
	}
	return operation
}

func TestOperationRunnerRejectsUnknownInputAndClassifiesLeaseConflict(t *testing.T) {
	operation := journal.Operation{ID: "operation", BoxID: "box", Kind: "up"}
	runner := OperationRunner{Assemble: func(context.Context, journal.Operation) (*Coordinator, error) {
		return nil, journal.ErrLeaseHeld
	}}
	emit := func(string, any) error { return nil }
	if failure := runner.Run(t.Context(), operation, json.RawMessage(`{"unknown":true}`), emit); failure == nil || failure.Code == "" {
		t.Fatalf("unknown input failure = %#v", failure)
	}
	failure := runner.Run(t.Context(), operation, json.RawMessage(`{}`), emit)
	if failure == nil || failure.Code != "ABOX_OPERATION_CONFLICT" || !failure.Retryable {
		t.Fatalf("lease conflict failure = %#v", failure)
	}
}
