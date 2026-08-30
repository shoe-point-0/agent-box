package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/shoe-point-0/agent-box/internal/boxlist"
	"github.com/shoe-point-0/agent-box/internal/boxplan"
	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/daemon"
	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/journal"
	"github.com/shoe-point-0/agent-box/internal/lima"
	"github.com/shoe-point-0/agent-box/internal/machinegit"
	"github.com/shoe-point-0/agent-box/internal/paths"
	plancompiler "github.com/shoe-point-0/agent-box/internal/plan"
	"github.com/shoe-point-0/agent-box/internal/process"
	"github.com/shoe-point-0/agent-box/internal/removal"
	"github.com/shoe-point-0/agent-box/internal/state"
	"github.com/shoe-point-0/agent-box/internal/toolchain"
	"github.com/shoe-point-0/agent-box/internal/version"
)

const name = "aboxd"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "--version" || args[0] == "version") {
		if _, err := fmt.Fprintln(stdout, version.Current().String(name)); err != nil {
			return 1
		}
		return 0
	}
	if len(args) > 0 && args[0] == "serve" {
		args = args[1:]
	}
	return serve(ctx, args, stderr)
}

func serve(ctx context.Context, args []string, stderr io.Writer) int {
	userPaths, err := daemon.DefaultUserPaths()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "aboxd: %v\n", err)
		return contract.ExitPrerequisite
	}
	runtimeDir, stateDatabase := userPaths.RuntimeDir, userPaths.StateDatabase
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&runtimeDir, "runtime-dir", runtimeDir, "secure daemon runtime directory")
	flags.StringVar(&stateDatabase, "state-db", stateDatabase, "Agent Box state database")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return contract.ExitUsage
	}

	store, err := state.Open(ctx, stateDatabase)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "aboxd: open state: %v\n", err)
		return contract.ExitInfrastructure
	}
	defer func() {
		if err := store.Close(); err != nil {
			_, _ = fmt.Fprintf(stderr, "aboxd: close state: %v\n", err)
		}
	}()
	operationJournal, err := journal.New(store)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "aboxd: create operation journal: %v\n", err)
		return contract.ExitInternal
	}
	boxInventory, err := boxlist.New(store)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "aboxd: create Box inventory: %v\n", err)
		return contract.ExitInternal
	}
	applicationRoot := filepath.Dir(stateDatabase)
	pathProvider, err := paths.New(applicationRoot, filepath.Join(applicationRoot, "cache"), filepath.Dir(userPaths.LogFile))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "aboxd: configure Box paths: %v\n", err)
		return contract.ExitInternal
	}
	gitClient := machinegit.Client{
		Home: applicationRoot, Path: os.Getenv("PATH"), ExcludesFile: pathProvider.GitExcludes(), Executor: process.Runner{},
	}
	if err := gitClient.Prepare(); err != nil {
		_, _ = fmt.Fprintf(stderr, "aboxd: configure Machine Git profile: %v\n", err)
		return contract.ExitInfrastructure
	}
	owner, err := journal.CurrentOwner()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "aboxd: create daemon owner: %v\n", err)
		return contract.ExitInfrastructure
	}
	writers := daemon.NewWriterGate()
	removeRunner := newProductionRemovalRunner(store, operationJournal, writers, pathProvider, gitClient, owner, applicationRoot)
	server, err := daemon.Start(daemon.Config{
		RuntimeDir: runtimeDir,
		Journal:    operationJournal,
		Writers:    writers,
		List:       boxInventory.List,
		Plan: func(ctx context.Context, request daemon.PlanRequest) (contract.PlanOutput, error) {
			gitPath, toolVersions, observeErr := observeToolVersions(ctx, applicationRoot)
			if observeErr != nil {
				return contract.PlanOutput{}, observeErr
			}
			requestGit := gitClient
			requestGit.GitPath = gitPath
			service := boxplan.Service{
				State: store, Paths: pathProvider, Runtime: plancompiler.RuntimeFacts{Driver: runtimeDriver()}, Tools: toolVersions,
				Git: requestGit,
			}
			return service.Compile(ctx, boxplan.Request{
				WorkspaceID: identity.WorkspaceID(request.WorkspaceID), BoxID: identity.BoxID(request.BoxID),
				TaskKey: request.TaskKey, ManifestPath: request.ManifestPath,
			})
		},
		Run: func(ctx context.Context, operation journal.Operation, input json.RawMessage, emit daemon.Emit) *contract.Failure {
			if operation.Kind == "remove" {
				return removeRunner.Run(ctx, operation, input, removal.OperationEmit(emit))
			}
			return &contract.Failure{Code: contract.CodeFeatureNotSupported}
		},
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "aboxd: start: %v\n", err)
		return contract.ExitInfrastructure
	}

	waitErr := server.Wait(ctx)
	shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		_, _ = fmt.Fprintf(stderr, "aboxd: shutdown: %v\n", err)
		return contract.ExitInfrastructure
	}
	if waitErr != nil && !errors.Is(waitErr, context.Canceled) {
		_, _ = fmt.Fprintf(stderr, "aboxd: serve: %v\n", waitErr)
		return contract.ExitInfrastructure
	}
	return contract.ExitOK
}

func newProductionRemovalRunner(
	store *state.Store,
	operationJournal *journal.Journal,
	writers *daemon.WriterGate,
	pathProvider *paths.Provider,
	gitClient machinegit.Client,
	owner journal.Owner,
	applicationRoot string,
) removal.OperationRunner {
	return removal.OperationRunner{Assemble: func(ctx context.Context, operation journal.Operation) (*removal.Coordinator, error) {
		snapshot, found, err := store.CurrentRemovalSnapshot(ctx, operation.BoxID)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("assemble production Box removal: %w", removal.ErrRemovalSnapshotAbsent)
		}

		limaPath, err := exec.LookPath("limactl")
		if err != nil {
			return nil, fmt.Errorf("assemble production Box removal: locate Lima: %w", err)
		}
		limaPath, err = filepath.Abs(limaPath)
		if err != nil {
			return nil, fmt.Errorf("assemble production Box removal: canonicalize Lima path: %w", err)
		}
		provider, err := toolchain.NewPreinstalledProvider([]toolchain.Entry{{
			Component: toolchain.Lima,
			Candidate: toolchain.Candidate{Identity: snapshot.LimaArtifact, Path: limaPath},
		}})
		if err != nil {
			return nil, err
		}
		artifact, err := toolchain.VerifyArtifact(ctx, snapshot.LimaArtifact, provider)
		if err != nil {
			return nil, err
		}
		environment := map[string]string{
			"PATH": os.Getenv("PATH"), "HOME": applicationRoot, "LANG": "C", "LC_ALL": "C", "TZ": "UTC",
		}
		limaRuntime, err := lima.NewAdapter(artifact, process.Runner{}, applicationRoot, environment)
		if err != nil {
			return nil, err
		}
		composeRuntime, err := compose.NewRuntime(limaRuntime)
		if err != nil {
			return nil, err
		}
		gitPath, err := exec.LookPath("git")
		if err != nil {
			return nil, fmt.Errorf("assemble production Box removal: locate Git: %w", err)
		}
		gitClient.GitPath, err = filepath.Abs(gitPath)
		if err != nil {
			return nil, fmt.Errorf("assemble production Box removal: canonicalize Git path: %w", err)
		}
		return removal.AssembleCurrent(ctx, removal.AssemblyConfig{
			Snapshots: store, Journal: operationJournal, Operation: operation, Owner: owner,
			Quiescer: writers, Paths: pathProvider, MachineGit: gitClient,
			LimaArtifact: snapshot.LimaArtifact, Lima: limaRuntime, Compose: composeRuntime,
			AppendEvent: func(ctx context.Context, operationID, kind string, details json.RawMessage, occurredAt time.Time) error {
				_, err := operationJournal.AppendEvent(ctx, operationID, kind, details, occurredAt)
				return err
			},
		})
	}}
}

func runtimeDriver() string {
	if runtime.GOOS == "darwin" {
		return "lima-vz"
	}
	return "lima-qemu"
}

type toolObservationError struct{ message string }

func (e *toolObservationError) Error() string                 { return e.message }
func (e *toolObservationError) ErrorCode() contract.ErrorCode { return contract.CodePrerequisiteFailed }

func observeToolVersions(ctx context.Context, directory string) (string, contract.ToolVersions, error) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return "", contract.ToolVersions{}, &toolObservationError{message: "observe tool versions: git executable not found"}
	}
	gitPath, err = filepath.Abs(gitPath)
	if err != nil {
		return "", contract.ToolVersions{}, &toolObservationError{message: fmt.Sprintf("observe tool versions: canonicalize Git path: %v", err)}
	}
	type probe struct {
		name   string
		argv   []string
		prefix string
	}
	probes := []probe{
		{name: "Git", argv: []string{gitPath, "--version"}, prefix: "git version "},
		{name: "Lima", argv: []string{"limactl", "--version"}, prefix: "limactl version "},
		{name: "Docker", argv: []string{"docker", "--version"}, prefix: "Docker version "},
		{name: "Compose", argv: []string{"docker", "compose", "version", "--short"}},
	}
	versions := make([]string, 0, len(probes))
	for _, current := range probes {
		result, runErr := (process.Runner{}).Run(ctx, process.Command{
			Argv: current.argv, Dir: directory,
			Env: map[string]string{"PATH": os.Getenv("PATH"), "HOME": directory, "LANG": "C", "LC_ALL": "C", "TZ": "UTC"},
		})
		if runErr != nil || result.StdoutTruncated || result.StderrTruncated {
			return "", contract.ToolVersions{}, &toolObservationError{message: fmt.Sprintf("observe tool versions: %s unavailable", current.name)}
		}
		value := strings.TrimSpace(string(result.Stdout))
		value = strings.TrimPrefix(value, current.prefix)
		if current.name == "Docker" {
			value = strings.TrimSuffix(strings.Split(value, ",")[0], ",")
		}
		if value == "" || strings.ContainsAny(value, "\x00\r\n") {
			return "", contract.ToolVersions{}, &toolObservationError{message: fmt.Sprintf("observe tool versions: %s returned invalid output", current.name)}
		}
		versions = append(versions, value)
	}
	return gitPath, contract.ToolVersions{
		AgentBox: version.Current().Version, Git: versions[0], Lima: versions[1], Docker: versions[2], Compose: versions[3],
	}, nil
}
