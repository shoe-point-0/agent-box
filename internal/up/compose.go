package up

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/lima"
	"github.com/shoe-point-0/agent-box/internal/ownership"
)

// ComposeConfig binds deterministic Compose compilation to the running
// runtime host produced by the foundation prefix. Runtime apply is deliberately
// separate because its resource identities require durable partial authority.
type ComposeConfig struct {
	Runner      compose.Runner
	DockerPath  string
	Environment map[string]string
	Secrets     []compose.Secret
}

// ReconcileComposeInputs freshly normalizes repository-owned Compose intent,
// materializes the generated policy inputs, and records the exact Machine path
// set required by runtime apply and filesystem readiness. Exact warm output
// keeps its existing inode while every normalization boundary is rerun.
func ReconcileComposeInputs(ctx context.Context, config ComposeConfig, attempt PreparedAttempt, foundation FoundationResult) (compose.RuntimeSpec, error) {
	if err := validateComposeFoundation(config, attempt, foundation); err != nil {
		return compose.RuntimeSpec{}, err
	}
	compiled, err := compose.Compile(ctx, compose.Input{
		Plan: attempt.Plan, Directory: attempt.BoxRoot, Generation: attempt.Generation,
		ComposeCommand: []string{config.DockerPath, "compose"}, Environment: config.Environment,
		Secrets: config.Secrets, Runner: config.Runner,
	})
	if err != nil {
		return compose.RuntimeSpec{}, fmt.Errorf("reconcile Compose inputs: compile: %w", err)
	}
	spec := compose.RuntimeSpec{
		RuntimeHost: foundation.Lima.Spec, Plan: attempt.Plan, Generation: attempt.Generation,
		DockerPath: config.DockerPath, Workdir: attempt.BoxRoot, Compiled: compiled,
	}
	spec.PathEvidence, err = compose.RecordRuntimePaths(attempt.Plan, compiled)
	if err != nil {
		return compose.RuntimeSpec{}, fmt.Errorf("reconcile Compose inputs: record runtime paths: %w", err)
	}
	return spec, nil
}

func validateComposeFoundation(config ComposeConfig, attempt PreparedAttempt, foundation FoundationResult) error {
	if config.Runner == nil || !cleanAbsoluteGeneratedPath(config.DockerPath) ||
		attempt.Operation.ID == "" || attempt.Plan.BoxID != attempt.Operation.BoxID || attempt.Generation < 1 ||
		filepath.Dir(attempt.PlanPath) != attempt.BoxRoot {
		return errors.New("reconcile Compose inputs: configuration or prepared attempt is incomplete")
	}
	if foundation.Generated.BoxRoot.BoxID != attempt.Plan.BoxID ||
		foundation.Generated.BoxRoot.Root.CanonicalPath != attempt.BoxRoot {
		return errors.New("reconcile Compose inputs: generated-state authority differs from prepared attempt")
	}
	if _, err := ownership.ProvePath(filepath.Dir(attempt.BoxRoot), foundation.Generated.BoxRoot.Root); err != nil {
		return fmt.Errorf("reconcile Compose inputs: prove generated Box root: %w", err)
	}
	if foundation.Git.Store.GitCommonDir != attempt.Repository.GitStorePath ||
		foundation.Git.Worktree.CanonicalPath != attempt.Repository.WorktreePath ||
		foundation.Git.Worktree.CanonicalCommon != attempt.Repository.GitStorePath {
		return errors.New("reconcile Compose inputs: Git observation differs from prepared attempt")
	}
	storeIdentity, storeErr := ownership.InspectPath(attempt.Repository.GitStorePath)
	worktreeIdentity, worktreeErr := ownership.InspectPath(attempt.Repository.WorktreePath)
	if storeErr != nil || worktreeErr != nil || storeIdentity != foundation.Git.Store.Identity ||
		worktreeIdentity != foundation.Git.Worktree.WorktreeIdentity {
		return errors.New("reconcile Compose inputs: Git path identity changed after observation")
	}
	if !reflect.DeepEqual(foundation.Lima.Spec.RuntimeHost, attempt.Plan.RuntimeHost) ||
		!reflect.DeepEqual(foundation.Lima.Spec.Mounts, attempt.Plan.Mounts) ||
		foundation.Lima.Spec.BoxID != attempt.Plan.BoxID || foundation.Lima.Spec.PlanDigest != attempt.Plan.PlanDigest ||
		foundation.Lima.Spec.Generation != attempt.Generation || foundation.Lima.Observation.State != lima.Running ||
		foundation.Lima.Observation.Name != attempt.Plan.RuntimeHost.InstanceName || foundation.Lima.Instance.Name != attempt.Plan.RuntimeHost.InstanceName {
		return errors.New("reconcile Compose inputs: running runtime-host observation differs from prepared attempt")
	}
	runtimePaths := make([]string, 0, len(foundation.Lima.Spec.Mounts)+1)
	runtimePaths = append(runtimePaths, foundation.Lima.Spec.ImagePath)
	for _, mount := range foundation.Lima.Spec.Mounts {
		runtimePaths = append(runtimePaths, mount.Path)
	}
	if err := ownership.VerifyPathSet(runtimePaths, foundation.Lima.Spec.PathEvidence); err != nil {
		return errors.New("reconcile Compose inputs: runtime-host path identity changed after observation")
	}
	return nil
}
