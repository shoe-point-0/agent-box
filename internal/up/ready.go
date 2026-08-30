package up

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/contextdoc"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/lima"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/state"
)

// ReadyStore owns the single durable transaction that publishes a generation
// and retires its matching partial reconciliation authority.
type ReadyStore interface {
	PublishReady(context.Context, state.ReadyGeneration) error
}

var _ ReadyStore = (*state.Store)(nil)

// WriteContext materializes one validated context document atomically.
type WriteContext func(string, contextdoc.Input) (contract.Context, error)

// ReadyConfig binds the filesystem readiness gate to context materialization
// and the final durable READY transaction.
type ReadyConfig struct {
	Filesystem      FilesystemConfig
	Store           ReadyStore
	Commands        []contract.CommandContext
	SensitiveValues []string
	WriteContext    WriteContext
	Now             func() time.Time
}

// ReconcileReady runs the context write and READY transaction only from inside
// the filesystem gate's final, freshly revalidated publication callback.
func ReconcileReady(ctx context.Context, config ReadyConfig, attempt PreparedAttempt, foundation FoundationResult, composeResult ComposeResult) error {
	if config.Store == nil {
		return errors.New("reconcile READY: state store is required")
	}
	writeContext := config.WriteContext
	if writeContext == nil {
		writeContext = contextdoc.Write
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}

	return ReconcileFilesystem(ctx, config.Filesystem, attempt, composeResult, func(ctx context.Context, filesystemResult FilesystemResult) error {
		observedAt := now().UTC()
		input, repositories, err := readyContextInput(attempt, foundation, filesystemResult, config.Commands, config.SensitiveValues)
		if err != nil {
			return err
		}
		contextDigest, err := materializeContext(attempt.ContextPath, input, writeContext)
		if err != nil {
			return fmt.Errorf("reconcile READY: materialize context: %w", err)
		}
		runtimeDigest, err := runtimeConfigDigest(attempt, foundation)
		if err != nil {
			return err
		}

		publication := state.ReadyGeneration{
			Plan: attempt.Plan, Generation: attempt.Generation,
			PlanPath: attempt.PlanPath, ContextPath: attempt.ContextPath,
			ContextDigest: contextDigest, RuntimeConfigDigest: runtimeDigest,
			Repositories: repositories,
			RemovalSnapshot: state.RemovalSnapshot{
				BoxID: attempt.Plan.BoxID, Generation: attempt.Generation, PlanDigest: attempt.Plan.PlanDigest,
				BoxRoot:             foundation.Generated.BoxRoot,
				Repositories:        []state.RepositoryRemovalRecord{{Facts: attempt.Repository, Store: foundation.Git.Store}},
				LimaArtifact:        foundation.Lima.Artifact,
				LimaSpec:            foundation.Lima.Spec,
				Lima:                foundation.Lima.Instance,
				ComposeSpec:         filesystemResult.Compose.Spec,
				Compose:             filesystemResult.Compose.Records,
				LimaPathEvidence:    foundation.Lima.Spec.PathEvidence,
				ComposePathEvidence: filesystemResult.Compose.Spec.PathEvidence,
			},
			ObservedAt: observedAt, PublishedAt: now().UTC(),
		}
		if err := config.Store.PublishReady(ctx, publication); err != nil {
			return fmt.Errorf("reconcile READY: publish generation: %w", err)
		}
		return nil
	})
}

func materializeContext(path string, input contextdoc.Input, write WriteContext) (contract.Digest, error) {
	_, expected, err := contextdoc.Generate(input)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 {
			return "", errors.New("retained context has unsafe type or mode")
		}
		current, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read retained context: %w", err)
		}
		if !bytes.Equal(current, expected) {
			return "", errors.New("retained context differs from fresh READY evidence")
		}
		return contentDigest(current), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect retained context: %w", err)
	}
	if _, err := write(path, input); err != nil {
		return "", err
	}
	digest, err := digestGeneratedFile(path, 0o600)
	if err != nil {
		return "", err
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(actual, expected) {
		return "", errors.New("written context differs from fresh READY evidence")
	}
	return digest, nil
}

func readyContextInput(attempt PreparedAttempt, foundation FoundationResult, filesystemResult FilesystemResult, commands []contract.CommandContext, sensitiveValues []string) (contextdoc.Input, []state.ReadyRepository, error) {
	worktree := foundation.Git.Worktree
	if foundation.Generated.BoxRoot.BoxID != attempt.Plan.BoxID ||
		foundation.Generated.BoxRoot.Root.CanonicalPath != attempt.BoxRoot ||
		foundation.Git.Store.Repository != attempt.Repository.Name ||
		foundation.Git.Store.GitCommonDir != attempt.Repository.GitStorePath ||
		worktree.Repository != attempt.Repository.Name || worktree.CanonicalPath != attempt.Repository.WorktreePath ||
		worktree.CanonicalCommon != attempt.Repository.GitStorePath || worktree.Branch != attempt.Repository.Branch ||
		!worktree.Registered || !worktree.BranchPresent {
		return contextdoc.Input{}, nil, errors.New("reconcile READY: fresh foundation identity is incomplete")
	}

	services, err := readyServices(attempt.Plan.WorkspaceService.Name, filesystemResult.Observation.Docker.Containers)
	if err != nil {
		return contextdoc.Input{}, nil, err
	}
	input := contextdoc.Input{
		Plan: attempt.Plan, Generation: attempt.Generation, WorkspaceRoot: attempt.WorkspaceRoot,
		Observations: contextdoc.Observations{
			BoxID: attempt.Plan.BoxID, Generation: attempt.Generation, PlanDigest: attempt.Plan.PlanDigest, State: "READY",
			Repositories: []contract.ContextRepository{{
				Name: attempt.Repository.Name, Branch: worktree.Branch,
				BaseCommit: attempt.Repository.BaseCommit, Path: worktree.CanonicalPath,
			}},
			Services: services, Routes: []contract.RouteContext{}, Ports: []contract.PortContext{},
		},
		Commands: commands, SensitiveValues: sensitiveValues,
	}
	repositories := []state.ReadyRepository{{
		Facts: attempt.Repository, Store: foundation.Git.Store,
		CurrentCommit: worktree.Commit, Dirty: worktree.Dirty,
	}}
	return input, repositories, nil
}

func readyServices(workspaceService string, containers []compose.ContainerObservation) ([]contract.ServiceContext, error) {
	if workspaceService == "" || len(containers) == 0 {
		return nil, errors.New("reconcile READY: service observations are incomplete")
	}
	services := make([]contract.ServiceContext, 0, len(containers))
	seen := make(map[string]struct{}, len(containers))
	workspaceCount := 0
	for _, container := range containers {
		if container.Service == "" || container.State != "running" || (container.Health != "" && container.Health != "healthy") {
			return nil, errors.New("reconcile READY: a required service is not ready")
		}
		if _, duplicate := seen[container.Service]; duplicate {
			return nil, errors.New("reconcile READY: service observation is ambiguous")
		}
		seen[container.Service] = struct{}{}
		if container.Service == workspaceService {
			workspaceCount++
		}
		services = append(services, contract.ServiceContext{Name: container.Service, Ready: true})
	}
	if workspaceCount != 1 {
		return nil, errors.New("reconcile READY: workspace service observation is missing or ambiguous")
	}
	return services, nil
}

func runtimeConfigDigest(attempt PreparedAttempt, foundation FoundationResult) (contract.Digest, error) {
	if foundation.Lima.Spec.BoxID != attempt.Plan.BoxID || foundation.Lima.Spec.Generation != attempt.Generation ||
		foundation.Lima.Spec.PlanDigest != attempt.Plan.PlanDigest || foundation.Lima.Instance.Name != attempt.Plan.RuntimeHost.InstanceName {
		return "", errors.New("reconcile READY: fresh runtime-host identity is incomplete")
	}
	if err := ownership.VerifyPathSet([]string{attempt.LimaConfigPath}, foundation.Lima.ConfigEvidence); err != nil {
		return "", fmt.Errorf("reconcile READY: prove runtime config path: %w", err)
	}
	expected, err := lima.GenerateConfig(foundation.Lima.Spec)
	if err != nil {
		return "", fmt.Errorf("reconcile READY: generate runtime config: %w", err)
	}
	actual, err := os.ReadFile(attempt.LimaConfigPath)
	if err != nil {
		return "", fmt.Errorf("reconcile READY: read runtime config: %w", err)
	}
	if !bytes.Equal(actual, expected) {
		return "", errors.New("reconcile READY: runtime config content differs from fresh intent")
	}
	return contentDigest(actual), nil
}

func digestGeneratedFile(path string, mode os.FileMode) (contract.Digest, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != mode || filepath.Clean(path) != path {
		return "", errors.New("generated file has unsafe identity, type, or mode")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return contentDigest(data), nil
}

func contentDigest(data []byte) contract.Digest {
	digest := sha256.Sum256(data)
	return contract.Digest(fmt.Sprintf("sha256:%x", digest))
}
