package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/daemon"
	"github.com/shoe-point-0/agent-box/internal/daemonclient"
	"github.com/shoe-point-0/agent-box/internal/doctor"
	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/version"
)

const (
	name           = "abox"
	manifestSuffix = ".abox/box.yaml"
	startupBudget  = 5 * time.Second
	startupPoll    = 25 * time.Millisecond
	maxManifest    = 1 << 20
)

type daemonAPI interface {
	Handshake(context.Context) (*contract.Failure, error)
	List(context.Context) (contract.ListOutput, *contract.Failure, error)
	Plan(context.Context, daemon.PlanRequest) (contract.PlanOutput, *contract.Failure, error)
	Up(context.Context, daemon.MutationRequest, string, func(daemonclient.Event) error) (*contract.Failure, error)
	Remove(context.Context, daemon.MutationRequest, string, func(daemonclient.Event) error) (*contract.Failure, error)
	Close()
}

type dependencies struct {
	getwd       func() (string, error)
	userPaths   func() (daemon.UserPaths, error)
	newClient   func(string) (daemonAPI, error)
	startDaemon func(daemon.UserPaths) (<-chan error, error)
	doctor      func(context.Context, doctor.Request, string) (doctor.Report, error)
	random      io.Reader
}

type upOptions struct {
	taskKey      string
	manifestPath string
	json         bool
	help         bool
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return runWith(ctx, args, stdout, stderr, dependencies{
		getwd: os.Getwd, userPaths: daemon.DefaultUserPaths,
		newClient:   func(socket string) (daemonAPI, error) { return daemonclient.New(socket) },
		startDaemon: startDaemon,
		doctor: func(ctx context.Context, request doctor.Request, cwd string) (doctor.Report, error) {
			observer, err := doctor.NewLocalObserver(cwd)
			if err != nil {
				return doctor.Report{}, err
			}
			return doctor.Inspect(ctx, request, observer)
		},
		random: rand.Reader,
	})
}

func runWith(ctx context.Context, args []string, stdout, stderr io.Writer, deps dependencies) int {
	if len(args) == 0 || len(args) == 1 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h") {
		return writeHelp(stdout)
	}
	if len(args) == 1 && (args[0] == "--version" || args[0] == "version") {
		if _, err := fmt.Fprintln(stdout, version.Current().String(name)); err != nil {
			return contract.ExitInternal
		}
		return contract.ExitOK
	}
	if args[0] == "init" {
		options, parseFailure := parseInit(args[1:])
		if parseFailure != nil {
			return reportFailure(stdout, stderr, false, parseFailure)
		}
		if options.help {
			return writeInitHelp(stdout)
		}
		cwd, err := deps.getwd()
		if err != nil {
			return reportFailure(stdout, stderr, false, failure(contract.CodePrerequisiteFailed, "cannot resolve current directory"))
		}
		manifestPath, initFailure := initializeWorkspace(cwd, options.force)
		if initFailure != nil {
			return reportFailure(stdout, stderr, false, initFailure)
		}
		if _, err := fmt.Fprintf(stdout, "created %s\n", manifestPath); err != nil {
			return contract.ExitInternal
		}
		return contract.ExitOK
	}
	if args[0] == "list" {
		options, parseFailure := parseList(args[1:])
		if parseFailure != nil {
			return reportFailure(stdout, stderr, options.json, parseFailure)
		}
		if options.help {
			return writeListHelp(stdout)
		}
		paths, err := deps.userPaths()
		if err != nil {
			return reportFailure(stdout, stderr, options.json, failure(contract.CodePrerequisiteFailed, "cannot resolve user-scoped daemon paths"))
		}
		client, connectFailure := connect(ctx, paths, deps)
		if connectFailure != nil {
			return reportFailure(stdout, stderr, options.json, connectFailure)
		}
		defer client.Close()
		output, listFailure, transportErr := client.List(ctx)
		if transportErr != nil {
			return reportFailure(stdout, stderr, options.json, classifyTransport(transportErr))
		}
		if listFailure != nil {
			return reportFailure(stdout, stderr, options.json, listFailure)
		}
		if options.json {
			if err := json.NewEncoder(stdout).Encode(output); err != nil {
				return contract.ExitInternal
			}
		} else if err := renderList(stdout, output); err != nil {
			return contract.ExitInternal
		}
		return contract.ExitOK
	}
	if args[0] == "doctor" {
		options, parseFailure := parseDoctor(args[1:])
		if parseFailure != nil {
			return reportFailure(stdout, stderr, options.json, parseFailure)
		}
		if options.help {
			return writeDoctorHelp(stdout)
		}
		cwd, err := deps.getwd()
		if err != nil {
			return reportFailure(stdout, stderr, options.json, failure(contract.CodePrerequisiteFailed, "cannot resolve current directory"))
		}
		cwd, err = filepath.Abs(cwd)
		if err != nil || deps.doctor == nil {
			return reportFailure(stdout, stderr, options.json, failure(contract.CodeInternalInvariant, "cannot configure read-only doctor"))
		}
		report, err := deps.doctor(ctx, doctor.Request{Mode: options.mode, TaskKey: options.taskKey}, filepath.Clean(cwd))
		if err != nil {
			if ctx.Err() != nil {
				return reportFailure(stdout, stderr, options.json, failure(contract.CodePrerequisiteFailed, "doctor observation was canceled"))
			}
			return reportFailure(stdout, stderr, options.json, failure(contract.CodeInternalInvariant, "doctor observation failed"))
		}
		if options.json {
			if err := json.NewEncoder(stdout).Encode(report); err != nil {
				return contract.ExitInternal
			}
		} else if err := renderDoctor(stdout, report); err != nil {
			return contract.ExitInternal
		}
		if report.Failed() {
			return contract.ExitPrerequisite
		}
		return contract.ExitOK
	}
	if args[0] == "plan" {
		options, parseFailure := parsePlan(args[1:])
		if parseFailure != nil {
			return reportFailure(stdout, stderr, options.json, parseFailure)
		}
		if options.help {
			return writePlanHelp(stdout)
		}
		cwd, err := deps.getwd()
		if err != nil {
			return reportFailure(stdout, stderr, options.json, failure(contract.CodePrerequisiteFailed, "cannot resolve current directory"))
		}
		manifestPath, _, manifestFailure := resolveManifest(cwd, options.manifestPath)
		if manifestFailure != nil {
			return reportFailure(stdout, stderr, options.json, manifestFailure)
		}
		workspaceID, err := identity.Workspace(manifestPath)
		if err != nil {
			return reportFailure(stdout, stderr, options.json, failure(contract.CodeManifestInvalid, "manifest path is not canonical"))
		}
		boxID, err := identity.Box(workspaceID, options.taskKey)
		if err != nil {
			return reportFailure(stdout, stderr, options.json, failure(contract.CodeCLIUsage, "task key is invalid"))
		}
		paths, err := deps.userPaths()
		if err != nil {
			return reportFailure(stdout, stderr, options.json, failure(contract.CodePrerequisiteFailed, "cannot resolve user-scoped daemon paths"))
		}
		client, connectFailure := connect(ctx, paths, deps)
		if connectFailure != nil {
			return reportFailure(stdout, stderr, options.json, connectFailure)
		}
		defer client.Close()
		output, planFailure, transportErr := client.Plan(ctx, daemon.PlanRequest{
			WorkspaceID: workspaceID.String(), BoxID: boxID.String(), TaskKey: options.taskKey, ManifestPath: manifestPath,
		})
		if transportErr != nil {
			return reportFailure(stdout, stderr, options.json, classifyTransport(transportErr))
		}
		if planFailure != nil {
			return reportFailure(stdout, stderr, options.json, planFailure)
		}
		if options.json {
			if err := json.NewEncoder(stdout).Encode(output); err != nil {
				return contract.ExitInternal
			}
		} else if err := renderPlan(stdout, output); err != nil {
			return contract.ExitInternal
		}
		return contract.ExitOK
	}
	if args[0] == "rm" {
		return runRemove(ctx, args[1:], stdout, stderr, deps)
	}
	if args[0] != "up" {
		return reportFailure(stdout, stderr, false, failure(contract.CodeCLIUsage, "unknown command"))
	}
	options, parseFailure := parseUp(args[1:])
	if parseFailure != nil {
		return reportFailure(stdout, stderr, options.json, parseFailure)
	}
	if options.help {
		return writeUpHelp(stdout)
	}

	cwd, err := deps.getwd()
	if err != nil {
		return reportFailure(stdout, stderr, options.json, failure(contract.CodePrerequisiteFailed, "cannot resolve current directory"))
	}
	manifestPath, manifestDigest, manifestFailure := resolveManifest(cwd, options.manifestPath)
	if manifestFailure != nil {
		return reportFailure(stdout, stderr, options.json, manifestFailure)
	}
	workspaceID, err := identity.Workspace(manifestPath)
	if err != nil {
		return reportFailure(stdout, stderr, options.json, failure(contract.CodeManifestInvalid, "manifest path is not canonical"))
	}
	boxID, err := identity.Box(workspaceID, options.taskKey)
	if err != nil {
		return reportFailure(stdout, stderr, options.json, failure(contract.CodeCLIUsage, "task key is invalid"))
	}
	key, err := newIdempotencyKey(deps.random)
	if err != nil {
		return reportFailure(stdout, stderr, options.json, failure(contract.CodeInternalInvariant, "cannot create request identity"))
	}
	paths, err := deps.userPaths()
	if err != nil {
		return reportFailure(stdout, stderr, options.json, failure(contract.CodePrerequisiteFailed, "cannot resolve user-scoped daemon paths"))
	}
	client, connectFailure := connect(ctx, paths, deps)
	if connectFailure != nil {
		return reportFailure(stdout, stderr, options.json, connectFailure)
	}
	defer client.Close()

	encoder := json.NewEncoder(stdout)
	consume := func(event daemonclient.Event) error {
		if options.json {
			return encoder.Encode(contract.Event{
				SchemaVersion: contract.EventSchemaVersion, OperationID: event.OperationID,
				Sequence: event.Sequence, Kind: event.Kind, Details: event.Details, OccurredAt: event.OccurredAt,
			})
		}
		if event.Kind != "operation.finished" {
			_, err := fmt.Fprintf(stderr, "abox: %s\n", event.Kind)
			return err
		}
		return nil
	}
	operationFailure, transportErr := client.Up(ctx, daemon.MutationRequest{
		WorkspaceID: workspaceID.String(), BoxID: boxID.String(), TaskKey: options.taskKey,
		ManifestPath: manifestPath, ManifestDigest: manifestDigest, Input: json.RawMessage(`{}`),
	}, key, consume)
	if transportErr != nil {
		return reportFailure(stdout, stderr, options.json, classifyTransport(transportErr))
	}
	if operationFailure != nil {
		return reportFailure(stdout, stderr, options.json, operationFailure)
	}
	if !options.json {
		if _, err := fmt.Fprintln(stderr, "abox: operation complete"); err != nil {
			return contract.ExitInternal
		}
	}
	return contract.ExitOK
}

func parseUp(args []string) (upOptions, *contract.Failure) {
	var options upOptions
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "--json":
			if options.json {
				return options, failure(contract.CodeCLIUsage, "--json may be specified only once")
			}
			options.json = true
		case argument == "--help" || argument == "-h":
			options.help = true
		case argument == "--manifest":
			if options.manifestPath != "" || index+1 == len(args) {
				return options, failure(contract.CodeCLIUsage, "--manifest requires one path")
			}
			index++
			options.manifestPath = args[index]
		case len(argument) > len("--manifest=") && argument[:len("--manifest=")] == "--manifest=":
			if options.manifestPath != "" {
				return options, failure(contract.CodeCLIUsage, "--manifest may be specified only once")
			}
			options.manifestPath = argument[len("--manifest="):]
		case len(argument) > 0 && argument[0] == '-':
			return options, failure(contract.CodeCLIUsage, "unknown up option")
		case options.taskKey == "":
			options.taskKey = argument
		default:
			return options, failure(contract.CodeCLIUsage, "up accepts exactly one task key")
		}
	}
	if options.help {
		if options.taskKey != "" || options.manifestPath != "" || options.json {
			return options, failure(contract.CodeCLIUsage, "--help cannot be combined with up arguments")
		}
		return options, nil
	}
	if err := identity.ValidateTaskKey(options.taskKey); err != nil {
		return options, failure(contract.CodeCLIUsage, "up requires one valid task key")
	}
	return options, nil
}

func resolveManifest(cwd, explicit string) (string, string, *contract.Failure) {
	candidate := explicit
	if candidate == "" {
		current, err := filepath.Abs(cwd)
		if err != nil {
			return "", "", failure(contract.CodeManifestInvalid, "cannot resolve manifest search path")
		}
		for {
			path := filepath.Join(current, filepath.FromSlash(manifestSuffix))
			if _, err := os.Lstat(path); err == nil {
				candidate = path
				break
			} else if !errors.Is(err, os.ErrNotExist) {
				return "", "", failure(contract.CodeManifestInvalid, "cannot inspect manifest path")
			}
			parent := filepath.Dir(current)
			if parent == current {
				return "", "", failure(contract.CodeManifestInvalid, "no .abox/box.yaml found in this directory or its parents")
			}
			current = parent
		}
	} else if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(cwd, candidate)
	}
	canonical, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", "", failure(contract.CodeManifestInvalid, "manifest does not resolve to a readable file")
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return "", "", failure(contract.CodeManifestInvalid, "cannot canonicalize manifest path")
	}
	canonical = filepath.Clean(canonical)
	file, err := os.Open(canonical)
	if err != nil {
		return "", "", failure(contract.CodeManifestInvalid, "manifest is not readable")
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return "", "", failure(contract.CodeManifestInvalid, "manifest is not a regular file")
	}
	hash := sha256.New()
	count, err := io.Copy(hash, io.LimitReader(file, maxManifest+1))
	closeErr := file.Close()
	if err != nil {
		return "", "", failure(contract.CodeManifestInvalid, "manifest cannot be read")
	}
	if closeErr != nil {
		return "", "", failure(contract.CodeManifestInvalid, "manifest cannot be closed after reading")
	}
	if count > maxManifest {
		return "", "", failure(contract.CodeManifestInvalid, "manifest exceeds the input limit")
	}
	return canonical, "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func connect(ctx context.Context, paths daemon.UserPaths, deps dependencies) (daemonAPI, *contract.Failure) {
	client, err := deps.newClient(paths.Socket)
	if err != nil {
		return nil, failure(contract.CodeInternalInvariant, "cannot construct daemon client")
	}
	handshakeFailure, handshakeErr := client.Handshake(ctx)
	if handshakeErr == nil {
		if handshakeFailure != nil {
			client.Close()
			return nil, handshakeFailure
		}
		return client, nil
	}
	if !errors.Is(handshakeErr, daemonclient.ErrUnavailable) {
		client.Close()
		return nil, classifyTransport(handshakeErr)
	}
	launchDone, launchErr := deps.startDaemon(paths)
	deadline := time.NewTimer(startupBudget)
	defer deadline.Stop()
	ticker := time.NewTicker(startupPoll)
	defer ticker.Stop()
	var launchGrace *time.Timer
	var launchGraceDone <-chan time.Time
	markLaunchFailed := func() {
		if launchGrace == nil {
			// A concurrent CLI may have launched the winning daemon. Give that
			// endpoint one bounded chance before reporting this launch failure.
			launchGrace = time.NewTimer(time.Second)
			launchGraceDone = launchGrace.C
		}
	}
	defer func() {
		if launchGrace != nil {
			launchGrace.Stop()
		}
	}()
	if launchErr != nil {
		markLaunchFailed()
	}
	for {
		select {
		case <-ctx.Done():
			client.Close()
			return nil, failure(contract.CodeDaemonUnavailable, "daemon startup was canceled")
		case <-deadline.C:
			client.Close()
			return nil, failure(contract.CodeDaemonUnavailable, "daemon did not become ready")
		case <-launchGraceDone:
			client.Close()
			return nil, failure(contract.CodeDaemonUnavailable, "daemon could not be launched; recovery may be required")
		case <-launchDone:
			launchDone = nil
			markLaunchFailed()
		case <-ticker.C:
			handshakeFailure, handshakeErr = client.Handshake(ctx)
			if handshakeErr == nil {
				if handshakeFailure != nil {
					client.Close()
					return nil, handshakeFailure
				}
				return client, nil
			}
			if !errors.Is(handshakeErr, daemonclient.ErrUnavailable) {
				client.Close()
				return nil, classifyTransport(handshakeErr)
			}
		}
	}
}

func classifyTransport(err error) *contract.Failure {
	if errors.Is(err, daemonclient.ErrProtocol) {
		return failure(contract.CodeAPIIncompatible, "daemon returned an incompatible response")
	}
	return failure(contract.CodeDaemonUnavailable, "daemon transport is unavailable")
}

func newIdempotencyKey(reader io.Reader) (string, error) {
	value := make([]byte, 16)
	if _, err := io.ReadFull(reader, value); err != nil {
		return "", err
	}
	return "cli_" + hex.EncodeToString(value), nil
}

func failure(code contract.ErrorCode, message string) *contract.Failure {
	return &contract.Failure{SchemaVersion: contract.ErrorSchemaVersion, Code: code, Message: message, Retryable: false}
}

func reportFailure(stdout, stderr io.Writer, jsonMode bool, value *contract.Failure) int {
	if value == nil || !value.Code.Valid() {
		value = failure(contract.CodeInternalInvariant, "internal invariant failed")
	}
	value.SchemaVersion = contract.ErrorSchemaVersion
	var err error
	if jsonMode {
		err = json.NewEncoder(stdout).Encode(value)
	} else {
		_, err = fmt.Fprintf(stderr, "abox: %s: %s\n", value.Code, value.Message)
	}
	if err != nil {
		return contract.ExitInternal
	}
	return value.ExitCode()
}

func writeHelp(writer io.Writer) int {
	_, err := fmt.Fprint(writer, `Usage: abox <command>

Commands:
  init     create a strict starter manifest without provisioning
  doctor   inspect Machine or Box prerequisites without repair
  list     list recorded Boxes and observed drift
  plan     show proposed reconciliation operations without changing state
  up       submit a versioned up request to aboxd
  rm       preview and explicitly authorize refusal-first Box removal
  version  print build version
  help     show this help

Run "abox <command> --help" for command options.
`)
	if err != nil {
		return contract.ExitInternal
	}
	return contract.ExitOK
}

func writeUpHelp(writer io.Writer) int {
	_, err := fmt.Fprint(writer, `Usage: abox up [--json] [--manifest PATH] TASK

Options:
  --json           stream versioned NDJSON events on stdout
  --manifest PATH  use this manifest instead of discovering .abox/box.yaml
`)
	if err != nil {
		return contract.ExitInternal
	}
	return contract.ExitOK
}
