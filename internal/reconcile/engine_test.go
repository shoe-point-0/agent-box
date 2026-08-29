package reconcile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/journal"
	"github.com/shoe-point-0/agent-box/internal/state"
)

func TestInvalidGraphsFailBeforeObservationOrApply(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		operations []Operation
	}{
		{
			name: "missing dependency",
			operations: []Operation{&fakeOperation{
				key: "runtime", dependencies: []string{"worktree"},
			}},
		},
		{
			name: "cycle",
			operations: []Operation{
				&fakeOperation{key: "a", dependencies: []string{"b"}},
				&fakeOperation{key: "b", dependencies: []string{"a"}},
			},
		},
		{
			name:       "duplicate key",
			operations: []Operation{&fakeOperation{key: "a"}, &fakeOperation{key: "a"}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := &memoryRecorder{}
			engine := newTestEngine(t, 2, recorder)
			if err := engine.Execute(t.Context(), test.operations); !errors.Is(err, ErrInvalidGraph) {
				t.Fatalf("Execute() error = %v, want ErrInvalidGraph", err)
			}
			for _, operation := range test.operations {
				fake, ok := operation.(*fakeOperation)
				if ok && (fake.observeCount.Load() != 0 || fake.applyCount.Load() != 0) {
					t.Fatalf("invalid graph invoked operation %q", fake.key)
				}
			}
			if got := recorder.snapshot(); len(got) != 0 {
				t.Fatalf("invalid graph persisted transitions: %#v", got)
			}
		})
	}
}

func TestUnknownPartialExecutionIsReobservedAfterRestart(t *testing.T) {
	t.Parallel()
	var exists atomic.Bool
	operation := &fakeOperation{
		key: "runtime-host",
		observe: func(context.Context) (Observation, error) {
			return Observation{Ready: exists.Load()}, nil
		},
		apply: func(context.Context, Observation) (Result, error) {
			exists.Store(true)
			return Result{Created: true}, nil
		},
	}

	// The prior process durably recorded intent and then died after the provider
	// created the resource but before it could append an after-event.
	priorRecorder := &memoryRecorder{}
	if err := priorRecorder.Record(t.Context(), Transition{Kind: TransitionApplyStarted, Key: operation.key}); err != nil {
		t.Fatal(err)
	}
	exists.Store(true)

	second := newTestEngine(t, 1, &memoryRecorder{})
	if err := second.Execute(t.Context(), []Operation{operation}); err != nil {
		t.Fatal(err)
	}
	if operation.observeCount.Load() != 1 || operation.applyCount.Load() != 0 {
		t.Fatalf("restart observe/apply counts = %d/%d, want 1/0", operation.observeCount.Load(), operation.applyCount.Load())
	}
}

func TestRecorderFailureStopsBeforeMutation(t *testing.T) {
	t.Parallel()
	operation := &fakeOperation{key: "resource", apply: createdResult}
	engine := newTestEngine(t, 1, &memoryRecorder{failKind: TransitionApplyStarted})
	if err := engine.Execute(t.Context(), []Operation{operation}); err == nil {
		t.Fatal("Execute() succeeded despite failure to persist apply start")
	}
	if operation.applyCount.Load() != 0 {
		t.Fatalf("apply count = %d, want 0", operation.applyCount.Load())
	}
}

func TestCompensationTouchesOnlyResourcesCreatedByCurrentAttempt(t *testing.T) {
	t.Parallel()
	var compensated []string
	var mutex sync.Mutex
	compensate := func(key string) func(context.Context, Result) error {
		return func(_ context.Context, _ Result) error {
			mutex.Lock()
			defer mutex.Unlock()
			compensated = append(compensated, key)
			return nil
		}
	}

	operations := []Operation{
		&fakeOperation{key: "existing", observe: readyObservation, compensate: compensate("existing")},
		&fakeOperation{key: "created", apply: createdResult, compensate: compensate("created")},
		&fakeOperation{key: "failed", apply: func(context.Context, Observation) (Result, error) {
			return Result{}, errors.New("injected apply failure")
		}, compensate: compensate("failed")},
	}
	engine := newTestEngine(t, 3, &memoryRecorder{})
	if err := engine.Execute(t.Context(), operations); err == nil {
		t.Fatal("Execute() succeeded despite injected failure")
	}
	if !slices.Equal(compensated, []string{"created"}) {
		t.Fatalf("compensated resources = %v, want [created]", compensated)
	}
}

func TestFailedApplyCanCompensateItsCreatedPartialResult(t *testing.T) {
	t.Parallel()
	var compensated atomic.Int32
	operation := &fakeOperation{
		key: "partial",
		apply: func(context.Context, Observation) (Result, error) {
			return Result{Created: true, Value: "owned-handle"}, errors.New("failure after create")
		},
		compensate: func(_ context.Context, result Result) error {
			if result.Value != "owned-handle" {
				return fmt.Errorf("result value = %v", result.Value)
			}
			compensated.Add(1)
			return nil
		},
	}
	engine := newTestEngine(t, 1, &memoryRecorder{})
	if err := engine.Execute(t.Context(), []Operation{operation}); err == nil {
		t.Fatal("Execute() succeeded despite partial failure")
	}
	if compensated.Load() != 1 {
		t.Fatalf("compensation count = %d, want 1", compensated.Load())
	}
}

func TestParallelismIsBoundedAndDependenciesWait(t *testing.T) {
	t.Parallel()
	started := make(chan string, 4)
	release := make(chan struct{})
	makeBlocking := func(key string, dependencies ...string) Operation {
		return &fakeOperation{
			key: key, dependencies: dependencies,
			apply: func(ctx context.Context, _ Observation) (Result, error) {
				started <- key
				select {
				case <-release:
					return Result{}, nil
				case <-ctx.Done():
					return Result{}, ctx.Err()
				}
			},
		}
	}
	operations := []Operation{
		makeBlocking("a"), makeBlocking("b"), makeBlocking("c"),
		&fakeOperation{key: "dependent", dependencies: []string{"a", "b", "c"}},
	}
	engine := newTestEngine(t, 2, &memoryRecorder{})
	done := make(chan error, 1)
	go func() { done <- engine.Execute(t.Context(), operations) }()

	first := <-started
	second := <-started
	if first == second {
		t.Fatalf("duplicate starts %q", first)
	}
	select {
	case third := <-started:
		t.Fatalf("operation %q exceeded max parallel 2", third)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	dependent := operations[3].(*fakeOperation)
	if dependent.observeCount.Load() != 1 || dependent.applyCount.Load() != 1 {
		t.Fatalf("dependent observe/apply counts = %d/%d, want 1/1", dependent.observeCount.Load(), dependent.applyCount.Load())
	}
}

func TestConcurrentExecutionsAreSerialized(t *testing.T) {
	t.Parallel()
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	operation := &fakeOperation{key: "resource", apply: func(ctx context.Context, _ Observation) (Result, error) {
		started <- struct{}{}
		select {
		case <-release:
			return Result{}, nil
		case <-ctx.Done():
			return Result{}, ctx.Err()
		}
	}}
	engine := newTestEngine(t, 2, &memoryRecorder{})
	done := make(chan error, 2)
	go func() { done <- engine.Execute(t.Context(), []Operation{operation}) }()
	<-started
	go func() { done <- engine.Execute(t.Context(), []Operation{operation}) }()
	select {
	case <-started:
		t.Fatal("second mutation attempt started before the first completed")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}

func TestTransitionsBracketEveryExternalSideEffect(t *testing.T) {
	t.Parallel()
	recorder := &memoryRecorder{}
	operation := &fakeOperation{key: "resource"}
	operation.apply = func(context.Context, Observation) (Result, error) {
		if got := recorder.kinds(); !slices.Equal(got, []string{TransitionApplyStarted}) {
			return Result{}, fmt.Errorf("transitions before apply = %v", got)
		}
		return Result{Created: true}, nil
	}
	operation.compensate = func(context.Context, Result) error {
		want := []string{
			TransitionApplyStarted, TransitionApplySucceeded,
			TransitionApplyStarted, TransitionApplyFailed,
			TransitionCompensationStarted,
		}
		if got := recorder.kinds(); !slices.Equal(got, want) {
			return fmt.Errorf("transitions before compensation = %v", got)
		}
		return nil
	}
	failure := &fakeOperation{key: "failure", dependencies: []string{"resource"}, apply: func(context.Context, Observation) (Result, error) {
		return Result{}, errors.New("injected")
	}}
	engine := newTestEngine(t, 1, recorder)
	if err := engine.Execute(t.Context(), []Operation{operation, failure}); err == nil {
		t.Fatal("Execute() succeeded despite injected failure")
	}
	want := []string{
		TransitionApplyStarted, TransitionApplySucceeded,
		TransitionApplyStarted, TransitionApplyFailed,
		TransitionCompensationStarted, TransitionCompensationSucceeded,
	}
	if got := recorder.kinds(); !slices.Equal(got, want) {
		t.Fatalf("transition kinds = %v, want %v", got, want)
	}
}

func TestEventRecorderPersistsTransitionsInOperationJournal(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store, err := state.Open(t.Context(), filepath.Join(root, "state", "state.db"))
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
	workspaceID, err := identity.Workspace(filepath.Join(root, "workspace", ".abox", "box.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	boxID, err := identity.Box(workspaceID, "TASK-1")
	if err != nil {
		t.Fatal(err)
	}
	journalOperation, _, err := operationJournal.Begin(t.Context(), journal.BeginRequest{
		WorkspaceID: workspaceID, BoxID: boxID, TaskKey: "TASK-1",
		ManifestPath: filepath.Join(root, "workspace", ".abox", "box.yaml"), ManifestDigest: "sha256:manifest",
		OperationID: "operation-1", IdempotencyKey: "request-1", Kind: "up",
		RequestDigest: "sha256:request", StartedAt: time.Unix(1, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Unix(2, 0).UTC()
	recorder, err := NewEventRecorder(journalOperation.ID,
		func(ctx context.Context, operationID, kind string, details json.RawMessage, occurredAt time.Time) error {
			_, err := operationJournal.AppendEvent(ctx, operationID, kind, details, occurredAt)
			return err
		},
		func() time.Time { return clock },
	)
	if err != nil {
		t.Fatal(err)
	}
	engine := newTestEngine(t, 1, recorder)
	if err := engine.Execute(t.Context(), []Operation{&fakeOperation{key: "runtime", apply: createdResult}}); err != nil {
		t.Fatal(err)
	}
	events, err := operationJournal.Events(t.Context(), journalOperation.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Kind != "reconcile.apply-started" || events[1].Kind != "reconcile.apply-succeeded" {
		t.Fatalf("persisted events = %#v", events)
	}
	if !events[0].OccurredAt.Equal(clock) || string(events[1].Details) != `{"created":true,"operationKey":"runtime"}` {
		t.Fatalf("persisted event metadata = %#v", events)
	}
}

func TestCompensationUsesFreshBoundedContextAfterCancellation(t *testing.T) {
	t.Parallel()
	requestContext, cancel := context.WithCancel(t.Context())
	var checked atomic.Bool
	created := &fakeOperation{key: "created", apply: createdResult, compensate: func(ctx context.Context, _ Result) error {
		if ctx.Err() != nil {
			return fmt.Errorf("cleanup context already canceled: %w", ctx.Err())
		}
		if _, ok := ctx.Deadline(); !ok {
			return errors.New("cleanup context has no deadline")
		}
		checked.Store(true)
		return nil
	}}
	failure := &fakeOperation{key: "failure", dependencies: []string{"created"}, apply: func(context.Context, Observation) (Result, error) {
		cancel()
		return Result{}, context.Canceled
	}}
	engine := newTestEngine(t, 1, &memoryRecorder{})
	if err := engine.Execute(requestContext, []Operation{created, failure}); err == nil {
		t.Fatal("Execute() succeeded after cancellation")
	}
	if !checked.Load() {
		t.Fatal("bounded compensation did not run")
	}
}

type fakeOperation struct {
	key          string
	dependencies []string
	observe      func(context.Context) (Observation, error)
	apply        func(context.Context, Observation) (Result, error)
	compensate   func(context.Context, Result) error
	observeCount atomic.Int32
	applyCount   atomic.Int32
}

func (o *fakeOperation) Key() string { return o.key }

func (o *fakeOperation) Dependencies() []string { return slices.Clone(o.dependencies) }

func (o *fakeOperation) Observe(ctx context.Context) (Observation, error) {
	o.observeCount.Add(1)
	if o.observe != nil {
		return o.observe(ctx)
	}
	return Observation{}, nil
}

func (o *fakeOperation) Apply(ctx context.Context, before Observation) (Result, error) {
	o.applyCount.Add(1)
	if o.apply != nil {
		return o.apply(ctx, before)
	}
	return Result{}, nil
}

func (o *fakeOperation) Compensate(ctx context.Context, result Result) error {
	if o.compensate != nil {
		return o.compensate(ctx, result)
	}
	return nil
}

type memoryRecorder struct {
	mutex       sync.Mutex
	transitions []Transition
	failKind    string
}

func (r *memoryRecorder) Record(_ context.Context, transition Transition) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if transition.Kind == r.failKind {
		return errors.New("injected recorder failure")
	}
	r.transitions = append(r.transitions, transition)
	return nil
}

func (r *memoryRecorder) snapshot() []Transition {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return slices.Clone(r.transitions)
}

func (r *memoryRecorder) kinds() []string {
	transitions := r.snapshot()
	kinds := make([]string, len(transitions))
	for index, transition := range transitions {
		kinds[index] = transition.Kind
	}
	return kinds
}

func newTestEngine(t *testing.T, maxParallel int, recorder Recorder) *Engine {
	t.Helper()
	engine, err := New(Config{MaxParallel: maxParallel, CompensationTimeout: time.Second, Recorder: recorder})
	if err != nil {
		t.Fatal(err)
	}
	return engine
}

func readyObservation(context.Context) (Observation, error) {
	return Observation{Ready: true}, nil
}

func createdResult(context.Context, Observation) (Result, error) {
	return Result{Created: true}, nil
}

func TestNewRejectsUnsafeConfiguration(t *testing.T) {
	t.Parallel()
	tests := []Config{
		{MaxParallel: 0, CompensationTimeout: time.Second, Recorder: &memoryRecorder{}},
		{MaxParallel: 1, CompensationTimeout: 0, Recorder: &memoryRecorder{}},
		{MaxParallel: 1, CompensationTimeout: time.Second},
	}
	for _, config := range tests {
		if engine, err := New(config); err == nil || !reflect.ValueOf(engine).IsNil() {
			t.Fatalf("New(%+v) = (%v, %v), want error", config, engine, err)
		}
	}
}
