package up

import (
	"context"
	"errors"
	"fmt"
)

// BuildLimaConfig derives runtime-host intent only after generated state and Git
// have converged. This ordering lets the builder record fresh path evidence for
// every mount before Lima can consume it.
type BuildLimaConfig func(context.Context, PreparedAttempt, GeneratedPlanResult, GitResult) (LimaConfig, error)

// FoundationConfig binds the first concrete, dependency-ordered portion of the
// up graph. Each reconciler owns durable partial authority, so a failed later
// stage is resumed rather than compensated by deleting earlier resources.
type FoundationConfig struct {
	Generated GeneratedPlanConfig
	Git       GitConfig
	BuildLima BuildLimaConfig
}

// FoundationResult carries freshly observed authority into the filesystem,
// Compose, context, and READY stages that follow this graph prefix.
type FoundationResult struct {
	Generated GeneratedPlanResult
	Git       GitResult
	Lima      LimaResult
}

// ReconcileFoundation executes generated Plan -> Git -> Lima in dependency
// order. Every stage re-observes its boundary through its concrete reconciler;
// retained proof, not successful return history, controls crash recovery.
func ReconcileFoundation(ctx context.Context, config FoundationConfig, attempt PreparedAttempt) (FoundationResult, error) {
	if config.BuildLima == nil {
		return FoundationResult{}, errors.New("reconcile Box foundation: Lima config builder is required")
	}
	generated, err := ReconcileGeneratedPlan(ctx, config.Generated, attempt)
	if err != nil {
		return FoundationResult{}, fmt.Errorf("reconcile Box foundation: generated Plan: %w", err)
	}
	git, err := ReconcileGit(ctx, config.Git, attempt)
	if err != nil {
		return FoundationResult{}, fmt.Errorf("reconcile Box foundation: Git: %w", err)
	}
	limaConfig, err := config.BuildLima(ctx, attempt, generated, git)
	if err != nil {
		return FoundationResult{}, fmt.Errorf("reconcile Box foundation: build Lima config: %w", err)
	}
	runtimeHost, err := ReconcileLima(ctx, limaConfig, attempt, generated)
	if err != nil {
		return FoundationResult{}, fmt.Errorf("reconcile Box foundation: Lima: %w", err)
	}
	return FoundationResult{Generated: generated, Git: git, Lima: runtimeHost}, nil
}
