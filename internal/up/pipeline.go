package up

import (
	"context"
	"errors"
	"fmt"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/journal"
)

// BuildReadyConfig binds the runtime-specific filesystem adapters only after
// Compose has produced the immutable RuntimeSpec they must execute against.
type BuildReadyConfig func(context.Context, PreparedAttempt, FoundationResult, ComposeResult) (ReadyConfig, error)

// PipelineConfig binds the existing concrete reconcilers into one complete
// preparation-to-READY execution path.
type PipelineConfig struct {
	Preparation  PreparationConfig
	Foundation   FoundationConfig
	Compose      ComposeConfig
	ComposeApply ComposeApplyConfig
	BuildReady   BuildReadyConfig
}

type prepareStage func(context.Context, journal.Operation) (PreparedAttempt, error)
type foundationStage func(context.Context, PreparedAttempt) (FoundationResult, error)
type composeInputsStage func(context.Context, PreparedAttempt, FoundationResult) (compose.RuntimeSpec, error)
type composeApplyStage func(context.Context, PreparedAttempt, compose.RuntimeSpec) (ComposeResult, error)
type readyStage func(context.Context, PreparedAttempt, FoundationResult, ComposeResult) error

// Pipeline is one daemon-owned, dependency-ordered up attempt. Every stage
// consumes fresh evidence from the preceding stage; no stage is skipped on a
// warm run.
type Pipeline struct {
	prepare       prepareStage
	foundation    foundationStage
	composeInputs composeInputsStage
	composeApply  composeApplyStage
	ready         readyStage
}

// NewPipeline wires the production stage implementations without executing or
// observing external state. Runtime-specific readiness construction remains
// delayed until the freshly compiled and applied Compose result exists.
func NewPipeline(config PipelineConfig) (*Pipeline, error) {
	if config.BuildReady == nil {
		return nil, errors.New("configure Box up pipeline: READY config builder is required")
	}
	return &Pipeline{
		prepare: func(ctx context.Context, operation journal.Operation) (PreparedAttempt, error) {
			return PrepareAttempt(ctx, config.Preparation, operation)
		},
		foundation: func(ctx context.Context, attempt PreparedAttempt) (FoundationResult, error) {
			return ReconcileFoundation(ctx, config.Foundation, attempt)
		},
		composeInputs: func(ctx context.Context, attempt PreparedAttempt, foundation FoundationResult) (compose.RuntimeSpec, error) {
			return ReconcileComposeInputs(ctx, config.Compose, attempt, foundation)
		},
		composeApply: func(ctx context.Context, attempt PreparedAttempt, spec compose.RuntimeSpec) (ComposeResult, error) {
			return ReconcileCompose(ctx, config.ComposeApply, attempt, spec)
		},
		ready: func(ctx context.Context, attempt PreparedAttempt, foundation FoundationResult, result ComposeResult) error {
			ready, err := config.BuildReady(ctx, attempt, foundation, result)
			if err != nil {
				return fmt.Errorf("reconcile Box pipeline: build READY config: %w", err)
			}
			return ReconcileReady(ctx, ready, attempt, foundation, result)
		},
	}, nil
}

// Execute rehydrates desired state under the coordinator's fenced lease and
// runs every concrete boundary through the atomic READY publication.
func (p *Pipeline) Execute(ctx context.Context, operation journal.Operation) error {
	if p == nil || p.prepare == nil || p.foundation == nil || p.composeInputs == nil || p.composeApply == nil || p.ready == nil {
		return errors.New("reconcile Box pipeline: stages are incomplete")
	}
	attempt, err := p.prepare(ctx, operation)
	if err != nil {
		return fmt.Errorf("reconcile Box pipeline: prepare attempt: %w", err)
	}
	foundation, err := p.foundation(ctx, attempt)
	if err != nil {
		return fmt.Errorf("reconcile Box pipeline: foundation: %w", err)
	}
	spec, err := p.composeInputs(ctx, attempt, foundation)
	if err != nil {
		return fmt.Errorf("reconcile Box pipeline: Compose inputs: %w", err)
	}
	result, err := p.composeApply(ctx, attempt, spec)
	if err != nil {
		return fmt.Errorf("reconcile Box pipeline: Compose runtime: %w", err)
	}
	if err := p.ready(ctx, attempt, foundation, result); err != nil {
		return fmt.Errorf("reconcile Box pipeline: READY: %w", err)
	}
	return nil
}

// AssemblePipelineCoordinator places the complete pipeline behind the same
// daemon lease lifetime used by OperationRunner.
func AssemblePipelineCoordinator(config Config, pipeline *Pipeline) (*Coordinator, error) {
	if pipeline == nil {
		return nil, errors.New("assemble Box up pipeline: pipeline is required")
	}
	config.Execute = func(ctx context.Context) error {
		return pipeline.Execute(ctx, config.Operation)
	}
	return NewCoordinator(config)
}
