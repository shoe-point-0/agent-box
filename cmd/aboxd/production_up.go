package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/shoe-point-0/agent-box/internal/boxplan"
	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/filesystem"
	"github.com/shoe-point-0/agent-box/internal/gitstore"
	"github.com/shoe-point-0/agent-box/internal/guesttoolchain"
	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/installation"
	"github.com/shoe-point-0/agent-box/internal/journal"
	"github.com/shoe-point-0/agent-box/internal/lima"
	"github.com/shoe-point-0/agent-box/internal/machine"
	"github.com/shoe-point-0/agent-box/internal/machinegit"
	"github.com/shoe-point-0/agent-box/internal/paths"
	plancompiler "github.com/shoe-point-0/agent-box/internal/plan"
	"github.com/shoe-point-0/agent-box/internal/process"
	"github.com/shoe-point-0/agent-box/internal/state"
	"github.com/shoe-point-0/agent-box/internal/up"
	"github.com/shoe-point-0/agent-box/internal/version"
	"github.com/shoe-point-0/agent-box/internal/worktree"
)

func newProductionUpRunner(
	store *state.Store,
	operationJournal *journal.Journal,
	pathProvider *paths.Provider,
	gitClient machinegit.Client,
	owner journal.Owner,
	applicationRoot string,
	runtimeRoot string,
	installationPath string,
) up.OperationRunner {
	return up.OperationRunner{Assemble: func(ctx context.Context, operation journal.Operation) (*up.Coordinator, error) {
		installed, err := installation.Load(ctx, installationPath)
		if err != nil {
			return nil, err
		}
		if err := productionMachinePreflight(ctx, installed, applicationRoot, runtimeRoot); err != nil {
			return nil, err
		}
		requestGit, err := productionGitClient(gitClient)
		if err != nil {
			return nil, err
		}
		environment := machineEnvironment(applicationRoot)
		limaRuntime, err := lima.NewAdapter(installed.Toolchain.Lima, process.Runner{}, applicationRoot, environment)
		if err != nil {
			return nil, err
		}
		composeRuntime, err := compose.NewRuntime(limaRuntime)
		if err != nil {
			return nil, err
		}
		guestProvisioner, err := guesttoolchain.New(limaRuntime)
		if err != nil {
			return nil, err
		}
		composeBuilder, err := up.NewComposeConfigBuilder(limaRuntime, installed.DockerPath, guestEnvironment(), nil)
		if err != nil {
			return nil, err
		}
		boxID := identity.BoxID(operation.BoxID)
		workspaceRoot, err := pathProvider.WorkspaceRoot(boxID)
		if err != nil {
			return nil, err
		}
		pipeline, err := up.NewPipeline(up.PipelineConfig{
			Preparation: up.PreparationConfig{
				Intents: store, Snapshots: store, Paths: pathProvider,
				Compile: productionPlanCompiler(store, pathProvider, requestGit, installed),
			},
			Foundation: up.FoundationConfig{
				Generated: up.GeneratedPlanConfig{Authority: store},
				Git: up.GitConfig{
					Authority: store,
					Stores:    gitstore.Materializer{Git: requestGit},
					Worktrees: worktree.Manager{Git: requestGit, WorkspaceRoot: workspaceRoot},
				},
				BuildLima: productionLimaBuilder(store, limaRuntime, installed),
			},
			BuildGuest: func(_ context.Context, _ up.PreparedAttempt, foundation up.FoundationResult) (up.GuestConfig, error) {
				return up.GuestConfig{Provisioner: guestProvisioner, Spec: guesttoolchain.Spec{
					RuntimeHost: foundation.Lima.Spec, Toolchain: installed.Toolchain,
					DockerPath: installed.DockerPath, ComposePath: installed.ComposePath,
					UtilityPaths: cloneStrings(installed.UtilityPaths), Workdir: installed.Workdir,
					DockerSocket: installed.DockerSocket, DockerService: installed.DockerService,
				}}, nil
			},
			BuildCompose: composeBuilder,
			ComposeApply: up.ComposeApplyConfig{Authority: store, Runtime: composeRuntime},
			BuildReady: func(_ context.Context, _ up.PreparedAttempt, _ up.FoundationResult, _ up.GuestResult, result up.ComposeResult) (up.ReadyConfig, error) {
				workspace, err := filesystem.NewWorkspaceAdapter(limaRuntime, result.Spec)
				if err != nil {
					return up.ReadyConfig{}, err
				}
				gate, err := filesystem.NewReadinessGate(limaRuntime, composeRuntime, workspace)
				if err != nil {
					return up.ReadyConfig{}, err
				}
				return up.ReadyConfig{
					Store: store,
					Filesystem: up.FilesystemConfig{
						Gate: gate, CatPath: installed.UtilityPaths["cat"], TestPath: installed.UtilityPaths["test"], CanaryRoot: applicationRoot,
					},
				}, nil
			},
		})
		if err != nil {
			return nil, err
		}
		return up.AssemblePipelineCoordinator(up.Config{
			Journal: operationJournal, Operation: operation, Owner: owner,
		}, pipeline)
	}}
}

const (
	productionRouterAddress = "127.0.0.1:7777"
	minimumStateFreeBytes   = 1 << 30
	minimumRuntimeFreeBytes = 16 << 20
)

func productionMachinePreflight(ctx context.Context, installed installation.Runtime, applicationRoot, runtimeRoot string) error {
	pathRequirements := []machine.PathRequirement{
		{Name: "state", Path: applicationRoot, MinFreeBytes: minimumStateFreeBytes},
		{Name: "runtime", Path: runtimeRoot, MinFreeBytes: minimumRuntimeFreeBytes},
	}
	observer, err := machine.NewLocalObserver(machine.LocalObserverConfig{
		Paths: pathRequirements, RouterAddress: productionRouterAddress,
	})
	if err != nil {
		return err
	}
	_, err = machine.Check(ctx, machine.Specification{
		Supported: []machine.Platform{{OS: "darwin", Architecture: "arm64", Accelerator: "vz"}},
		Paths:     pathRequirements, RouterAddress: productionRouterAddress,
		Toolchain: installed.Specification, Provider: installed.Provider,
	}, observer)
	return err
}

func productionLimaBuilder(store *state.Store, runtime *lima.Adapter, installed installation.Runtime) up.BuildLimaConfig {
	return func(_ context.Context, attempt up.PreparedAttempt, _ up.GeneratedPlanResult, _ up.GitResult) (up.LimaConfig, error) {
		spec := lima.Spec{
			RuntimeHost: attempt.Plan.RuntimeHost, Mounts: append([]contract.MountPlan(nil), attempt.Plan.Mounts...),
			BoxID: attempt.Plan.BoxID, PlanDigest: attempt.Plan.PlanDigest, Generation: attempt.Generation,
			GuestImage: installed.Toolchain.GuestImage.Identity(), ImagePath: installed.GuestImagePath,
		}
		evidence, err := lima.RecordRuntimePaths(spec)
		if err != nil {
			return up.LimaConfig{}, err
		}
		spec.PathEvidence = evidence
		return up.LimaConfig{Authority: store, Runtime: runtime, Artifact: installed.Toolchain.Lima.Identity(), Spec: spec}, nil
	}
}

func productionPlanCompiler(store *state.Store, pathProvider *paths.Provider, git machinegit.Client, installed installation.Runtime) up.CompilePlan {
	return func(ctx context.Context, request boxplan.Request) (contract.PlanOutput, error) {
		gitVersion, err := observeGitVersion(ctx, git, request.ManifestPath)
		if err != nil {
			return contract.PlanOutput{}, err
		}
		service := boxplan.Service{
			State: store, Paths: pathProvider, Runtime: plancompiler.RuntimeFacts{Driver: runtimeDriver()},
			Tools: contract.ToolVersions{
				AgentBox: version.Current().Version,
				Git:      gitVersion,
				Lima:     installed.Toolchain.Lima.Identity().Version,
				Docker:   installed.Toolchain.Docker.Identity().Version,
				Compose:  installed.Toolchain.Compose.Identity().Version,
			},
			Git: git,
		}
		return service.Compile(ctx, request)
	}
}

func productionPlanEndpoint(
	store *state.Store,
	pathProvider *paths.Provider,
	gitClient machinegit.Client,
	installationPath string,
) func(context.Context, boxplan.Request) (contract.PlanOutput, error) {
	return func(ctx context.Context, request boxplan.Request) (contract.PlanOutput, error) {
		installed, err := installation.Load(ctx, installationPath)
		if err != nil {
			return contract.PlanOutput{}, err
		}
		git, err := productionGitClient(gitClient)
		if err != nil {
			return contract.PlanOutput{}, err
		}
		return productionPlanCompiler(store, pathProvider, git, installed)(ctx, request)
	}
}

func productionGitClient(client machinegit.Client) (machinegit.Client, error) {
	path, err := exec.LookPath("git")
	if err != nil {
		return machinegit.Client{}, &toolObservationError{message: "observe tool versions: Git unavailable"}
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return machinegit.Client{}, &toolObservationError{message: "observe tool versions: canonicalize Git path"}
	}
	client.GitPath = path
	return client, nil
}

func observeGitVersion(ctx context.Context, git machinegit.Client, directory string) (string, error) {
	result, err := (process.Runner{}).Run(ctx, process.Command{
		Argv: []string{git.GitPath, "--version"}, Dir: filepath.Dir(directory), Env: machineEnvironment(filepath.Dir(directory)),
	})
	if err != nil || result.StdoutTruncated || result.StderrTruncated || len(result.Stderr) != 0 {
		return "", &toolObservationError{message: "observe tool versions: Git unavailable"}
	}
	value := strings.TrimSpace(string(result.Stdout))
	value = strings.TrimPrefix(value, "git version ")
	if value == "" || strings.ContainsAny(value, "\x00\r\n") {
		return "", &toolObservationError{message: "observe tool versions: Git returned invalid output"}
	}
	return value, nil
}

func machineEnvironment(home string) map[string]string {
	return map[string]string{"PATH": os.Getenv("PATH"), "HOME": home, "LANG": "C", "LC_ALL": "C", "TZ": "UTC"}
}

func guestEnvironment() map[string]string {
	return map[string]string{
		"PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME": "/root", "LANG": "C", "LC_ALL": "C", "TZ": "UTC",
	}
}

func cloneStrings(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
