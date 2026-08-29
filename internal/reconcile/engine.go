// Package reconcile validates and executes idempotent operation DAGs.
package reconcile

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
)

const maxOperationKeyBytes = 512

var ErrInvalidGraph = errors.New("invalid operation graph")

// Observation is read-only external state captured immediately before a
// mutation decision. Value belongs to the operation and is never journaled by
// the engine.
type Observation struct {
	Ready bool
	Value any
}

// Result describes an apply attempt. Created must be true only when this
// execution created a resource that did not exist in Observation.
type Result struct {
	Created bool
	Value   any
}

// Operation is one stable, independently observable reconciliation step.
type Operation interface {
	Key() string
	Dependencies() []string
	Observe(context.Context) (Observation, error)
	Apply(context.Context, Observation) (Result, error)
	Compensate(context.Context, Result) error
}

// Transition brackets an external side effect in the durable operation event
// stream. Result values are deliberately excluded because they may contain
// provider data or secrets.
type Transition struct {
	Kind    string
	Key     string
	Created bool
}

const (
	TransitionApplyStarted          = "apply-started"
	TransitionApplySucceeded        = "apply-succeeded"
	TransitionApplyFailed           = "apply-failed"
	TransitionCompensationStarted   = "compensation-started"
	TransitionCompensationSucceeded = "compensation-succeeded"
	TransitionCompensationFailed    = "compensation-failed"
)

// Recorder durably persists operation transitions. A started transition must
// be committed before its corresponding external side effect begins.
type Recorder interface {
	Record(context.Context, Transition) error
}

// Config defines explicit execution and cleanup bounds.
type Config struct {
	MaxParallel         int
	CompensationTimeout time.Duration
	AfterEventTimeout   time.Duration
	Recorder            Recorder
}

// Engine executes one already-owned Box mutation attempt. The caller must hold
// the Box's fenced daemon lease for the entire call.
type Engine struct {
	maxParallel         int
	compensationTimeout time.Duration
	afterEventTimeout   time.Duration
	recorder            Recorder
	executionMutex      sync.Mutex
}

// New constructs an Engine with no implicit concurrency or cleanup defaults.
func New(config Config) (*Engine, error) {
	if config.MaxParallel < 1 {
		return nil, errors.New("create reconcile engine: max parallel must be positive")
	}
	if config.CompensationTimeout <= 0 {
		return nil, errors.New("create reconcile engine: compensation timeout must be positive")
	}
	if config.AfterEventTimeout <= 0 {
		return nil, errors.New("create reconcile engine: after-event timeout must be positive")
	}
	if config.Recorder == nil {
		return nil, errors.New("create reconcile engine: recorder is nil")
	}
	return &Engine{
		maxParallel:         config.MaxParallel,
		compensationTimeout: config.CompensationTimeout,
		afterEventTimeout:   config.AfterEventTimeout,
		recorder:            config.Recorder,
	}, nil
}

type graph struct {
	operations map[string]Operation
	indegree   map[string]int
	dependents map[string][]string
}

type applied struct {
	key       string
	operation Operation
	result    Result
}

type stepResult struct {
	key     string
	applied *applied
	err     error
}

// Execute validates the complete graph before observation, then runs stable
// topological batches. Every invocation re-observes external reality; journal
// history is audit evidence, not a substitute for observation after a crash.
func (e *Engine) Execute(ctx context.Context, operations []Operation) error {
	e.executionMutex.Lock()
	defer e.executionMutex.Unlock()

	g, err := validate(operations)
	if err != nil {
		return err
	}

	ready := initialReady(g.indegree)
	created := make([]applied, 0, len(operations))
	completed := 0
	for len(ready) > 0 {
		limit := min(e.maxParallel, len(ready))
		batch := slices.Clone(ready[:limit])
		ready = ready[limit:]

		results, batchCreated := e.runBatch(ctx, g.operations, batch)
		created = append(created, batchCreated...)
		var executionErr error
		for _, result := range results {
			if result.err != nil {
				if executionErr == nil {
					executionErr = fmt.Errorf("operation %q: %w", result.key, result.err)
				}
				continue
			}
			completed++
			for _, dependent := range g.dependents[result.key] {
				g.indegree[dependent]--
				if g.indegree[dependent] == 0 {
					ready = append(ready, dependent)
				}
			}
		}
		if executionErr != nil {
			return errors.Join(executionErr, e.compensate(ctx, created))
		}
		slices.Sort(ready)
	}
	if completed != len(operations) {
		return fmt.Errorf("%w: cycle detected", ErrInvalidGraph)
	}
	return nil
}

func (e *Engine) runBatch(ctx context.Context, operations map[string]Operation, keys []string) ([]stepResult, []applied) {
	completed := make(chan stepResult, len(keys))
	for _, key := range keys {
		go func() {
			completed <- e.runStep(ctx, key, operations[key])
		}()
	}
	results := make([]stepResult, 0, len(keys))
	created := make([]applied, 0, len(keys))
	for range keys {
		result := <-completed
		results = append(results, result)
		if result.applied != nil {
			created = append(created, *result.applied)
		}
	}
	// Graph advancement and primary error selection remain deterministic while
	// created preserves completion order for reverse-order compensation.
	slices.SortFunc(results, func(left, right stepResult) int {
		return strings.Compare(left.key, right.key)
	})
	return results, created
}

func (e *Engine) runStep(ctx context.Context, key string, operation Operation) stepResult {
	before, err := operation.Observe(ctx)
	if err != nil {
		return stepResult{key: key, err: fmt.Errorf("observe: %w", err)}
	}
	if before.Ready {
		return stepResult{key: key}
	}
	if err := e.recorder.Record(ctx, Transition{Kind: TransitionApplyStarted, Key: key}); err != nil {
		return stepResult{key: key, err: fmt.Errorf("persist apply start: %w", err)}
	}

	result, applyErr := operation.Apply(ctx, before)
	transition := Transition{Kind: TransitionApplySucceeded, Key: key, Created: result.Created}
	if applyErr != nil {
		transition.Kind = TransitionApplyFailed
	}
	recordErr := e.recordAfterSideEffect(ctx, transition)

	step := stepResult{key: key}
	if result.Created {
		step.applied = &applied{key: key, operation: operation, result: result}
	}
	if applyErr != nil || recordErr != nil {
		step.err = errors.Join(applyErr, wrapIf(recordErr, "persist apply result"))
	}
	return step
}

func (e *Engine) compensate(ctx context.Context, created []applied) error {
	if len(created) == 0 {
		return nil
	}
	cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), e.compensationTimeout)
	defer cancel()

	var compensationErr error
	for index := len(created) - 1; index >= 0; index-- {
		item := created[index]
		key := item.key
		if err := e.recorder.Record(cleanupContext, Transition{
			Kind: TransitionCompensationStarted, Key: key, Created: true,
		}); err != nil {
			compensationErr = errors.Join(compensationErr, fmt.Errorf("operation %q persist compensation start: %w", key, err))
			continue
		}
		err := item.operation.Compensate(cleanupContext, item.result)
		transition := Transition{Kind: TransitionCompensationSucceeded, Key: key, Created: true}
		if err != nil {
			transition.Kind = TransitionCompensationFailed
		}
		recordErr := e.recordAfterSideEffect(cleanupContext, transition)
		if err != nil || recordErr != nil {
			compensationErr = errors.Join(compensationErr,
				fmt.Errorf("operation %q compensate: %w", key, errors.Join(err, wrapIf(recordErr, "persist compensation result"))))
		}
	}
	if compensationErr != nil {
		return fmt.Errorf("compensation: %w", compensationErr)
	}
	return nil
}

// recordAfterSideEffect gets a fresh bounded context because the side effect
// may have completed while consuming or canceling the request context. Losing
// the matching after-event would unnecessarily force unknown-partial recovery.
func (e *Engine) recordAfterSideEffect(parent context.Context, transition Transition) error {
	recordContext, cancel := context.WithTimeout(context.WithoutCancel(parent), e.afterEventTimeout)
	defer cancel()
	return e.recorder.Record(recordContext, transition)
}

func validate(operations []Operation) (graph, error) {
	g := graph{
		operations: make(map[string]Operation, len(operations)),
		indegree:   make(map[string]int, len(operations)),
		dependents: make(map[string][]string, len(operations)),
	}
	for index, operation := range operations {
		if operation == nil {
			return graph{}, fmt.Errorf("%w: operation %d is nil", ErrInvalidGraph, index)
		}
		key := operation.Key()
		if strings.TrimSpace(key) == "" || key != strings.TrimSpace(key) ||
			len(key) > maxOperationKeyBytes || strings.IndexByte(key, 0) >= 0 {
			return graph{}, fmt.Errorf("%w: operation %d has an invalid key", ErrInvalidGraph, index)
		}
		if _, exists := g.operations[key]; exists {
			return graph{}, fmt.Errorf("%w: duplicate operation key %q", ErrInvalidGraph, key)
		}
		g.operations[key] = operation
		g.indegree[key] = 0
	}
	for key, operation := range g.operations {
		dependencies := operation.Dependencies()
		seen := make(map[string]struct{}, len(dependencies))
		for _, dependency := range dependencies {
			if _, exists := g.operations[dependency]; !exists {
				return graph{}, fmt.Errorf("%w: operation %q depends on missing operation %q", ErrInvalidGraph, key, dependency)
			}
			if _, duplicate := seen[dependency]; duplicate {
				return graph{}, fmt.Errorf("%w: operation %q repeats dependency %q", ErrInvalidGraph, key, dependency)
			}
			seen[dependency] = struct{}{}
			g.indegree[key]++
			g.dependents[dependency] = append(g.dependents[dependency], key)
		}
	}
	for key := range g.dependents {
		slices.Sort(g.dependents[key])
	}
	if hasCycle(g.indegree, g.dependents) {
		return graph{}, fmt.Errorf("%w: cycle detected", ErrInvalidGraph)
	}
	return g, nil
}

func hasCycle(indegree map[string]int, dependents map[string][]string) bool {
	remaining := make(map[string]int, len(indegree))
	for key, count := range indegree {
		remaining[key] = count
	}
	ready := initialReady(remaining)
	visited := 0
	for len(ready) > 0 {
		key := ready[0]
		ready = ready[1:]
		visited++
		for _, dependent := range dependents[key] {
			remaining[dependent]--
			if remaining[dependent] == 0 {
				ready = append(ready, dependent)
			}
		}
		slices.Sort(ready)
	}
	return visited != len(indegree)
}

func initialReady(indegree map[string]int) []string {
	ready := make([]string, 0, len(indegree))
	for key, count := range indegree {
		if count == 0 {
			ready = append(ready, key)
		}
	}
	slices.Sort(ready)
	return ready
}

func wrapIf(err error, message string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}
