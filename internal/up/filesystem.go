package up

import (
	"context"
	"errors"
	"fmt"

	"github.com/shoe-point-0/agent-box/internal/filesystem"
)

// FilesystemReadinessGate joins fresh runtime-host, Docker, container-kernel,
// and workspace-service observations immediately before READY publication.
type FilesystemReadinessGate interface {
	VerifyAndPublish(context.Context, filesystem.LiveSpec, filesystem.ReadyPublisher) error
}

var _ FilesystemReadinessGate = (*filesystem.ReadinessGate)(nil)

// FilesystemConfig supplies the pinned guest utilities and Machine-owned canary
// root used by the live filesystem boundary.
type FilesystemConfig struct {
	Gate       FilesystemReadinessGate
	CatPath    string
	TestPath   string
	CanaryRoot string
}

// FilesystemResult carries the fresh mount and Docker observations through the
// readiness callback. Compose.Observation is replaced with the observation
// collected by the readiness gate rather than the earlier post-apply view.
type FilesystemResult struct {
	Compose     ComposeResult
	Observation filesystem.Observation
}

// FilesystemPublisher performs the remaining context and atomic READY work.
// It runs only inside the readiness gate after all live checks and final path
// identity proofs have passed.
type FilesystemPublisher func(context.Context, FilesystemResult) error

// ReconcileFilesystem consumes retained Compose identities, re-observes every
// filesystem layer, proves the fresh Docker resource set still matches those
// identities, and invokes publish while the readiness gate remains active.
func ReconcileFilesystem(ctx context.Context, config FilesystemConfig, attempt PreparedAttempt, composeResult ComposeResult, publish FilesystemPublisher) error {
	if config.Gate == nil || publish == nil {
		return errors.New("reconcile filesystem: readiness gate and publisher are required")
	}
	if err := validateComposeAttempt(attempt, composeResult.Spec); err != nil {
		return fmt.Errorf("reconcile filesystem: Compose result: %w", err)
	}
	if err := verifyExactRuntimeRecords(composeResult.Records, composeResult.Observation); err != nil {
		return fmt.Errorf("reconcile filesystem: prove Compose result: %w", err)
	}

	spec := filesystem.LiveSpec{
		Runtime:    composeResult.Spec,
		CatPath:    config.CatPath,
		TestPath:   config.TestPath,
		CanaryRoot: config.CanaryRoot,
	}
	return config.Gate.VerifyAndPublish(ctx, spec, func(ctx context.Context, observed filesystem.Observation) error {
		if err := verifyExactRuntimeRecords(composeResult.Records, observed.Docker); err != nil {
			return fmt.Errorf("reconcile filesystem: prove fresh runtime: %w", err)
		}
		freshCompose := composeResult
		freshCompose.Records = cloneRuntimeRecords(composeResult.Records)
		freshCompose.Observation = observed.Docker
		return publish(ctx, FilesystemResult{Compose: freshCompose, Observation: observed})
	})
}
