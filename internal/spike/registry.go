package spike

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	ErrDuplicate = errors.New("spike already registered")
	ErrInvalid   = errors.New("invalid spike")
	ErrNotFound  = errors.New("spike not found")
)

// RunFunc executes one disposable substrate probe inside an owned environment.
type RunFunc func(ctx context.Context, env *Environment) error

// CleanupFunc reconciles scenario-specific resources before generic root cleanup.
type CleanupFunc func(ctx context.Context, workspace *Workspace, resources []Resource) error

type Scenario struct {
	Run     RunFunc
	Cleanup CleanupFunc
}

// Registry stores the substrate probes compiled into the disposable command.
type Registry struct {
	scenarios map[string]Scenario
}

func NewRegistry() *Registry {
	return &Registry{scenarios: make(map[string]Scenario)}
}

func (r *Registry) Register(name string, run RunFunc) error {
	return r.RegisterScenario(name, Scenario{Run: run})
}

func (r *Registry) RegisterScenario(name string, scenario Scenario) error {
	if r == nil {
		return fmt.Errorf("%w: nil registry", ErrInvalid)
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: empty name", ErrInvalid)
	}
	if scenario.Run == nil {
		return fmt.Errorf("%w: %s has nil runner", ErrInvalid, name)
	}
	if _, exists := r.scenarios[name]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicate, name)
	}

	r.scenarios[name] = scenario
	return nil
}

func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}

	names := make([]string, 0, len(r.scenarios))
	for name := range r.scenarios {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r *Registry) Run(ctx context.Context, name string, env *Environment) error {
	if r == nil {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}

	scenario, exists := r.scenarios[name]
	if !exists {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return scenario.Run(ctx, env)
}

func (r *Registry) Cleanup(ctx context.Context, name string, workspace *Workspace, resources []Resource) error {
	if r == nil {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	scenario, exists := r.scenarios[name]
	if !exists {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if scenario.Cleanup == nil {
		return nil
	}
	return scenario.Cleanup(ctx, workspace, resources)
}
