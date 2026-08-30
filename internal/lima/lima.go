// Package lima provides the typed, version-pinned limactl boundary for one
// Agent Box runtime host.
package lima

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/process"
	"github.com/shoe-point-0/agent-box/internal/toolchain"
)

type ErrorKind string

const (
	InvalidSpecification ErrorKind = "invalid-specification"
	UnsupportedVersion   ErrorKind = "unsupported-version"
	MalformedOutput      ErrorKind = "malformed-output"
	NotFound             ErrorKind = "not-found"
	PartialCreation      ErrorKind = "partial-creation"
	ConfigMismatch       ErrorKind = "config-mismatch"
	Canceled             ErrorKind = "canceled"
	CommandFailed        ErrorKind = "command-failed"
	InvalidState         ErrorKind = "invalid-state"
	PostconditionFailed  ErrorKind = "postcondition-failed"
)

// Error classifies adapter failures without exposing raw limactl output.
type Error struct {
	Kind      ErrorKind
	Operation string
	Resource  string
	Observed  State
	Err       error
}

func (e *Error) Error() string {
	subject := e.Operation
	if e.Resource != "" {
		subject += " " + e.Resource
	}
	if e.Err != nil {
		return fmt.Sprintf("lima %s: %s: %v", subject, e.Kind, e.Err)
	}
	return fmt.Sprintf("lima %s: %s", subject, e.Kind)
}

func (e *Error) Unwrap() error { return e.Err }

func (e *Error) ErrorCode() contract.ErrorCode {
	switch e.Kind {
	case InvalidSpecification:
		return contract.CodeInternalInvariant
	case UnsupportedVersion, MalformedOutput:
		return contract.CodeAPIIncompatible
	case NotFound:
		return contract.CodeNotFound
	case ConfigMismatch:
		return contract.CodeIdentityConflict
	case InvalidState:
		return contract.CodeOperationConflict
	default:
		return contract.CodeInfrastructureApplyFailed
	}
}

type State string

const (
	Absent  State = "absent"
	Stopped State = "stopped"
	Running State = "running"
	Partial State = "partial"
)

// Observation is independently decoded from limactl's JSON Lines inventory.
type Observation struct {
	Name         string
	State        State
	LimaStatus   string
	Metadata     map[string]string
	LimaVersion  string
	config       *limaConfig
	inspectError []string
}

func (o Observation) ownership() ownership.LimaInstanceObservation {
	return ownership.LimaInstanceObservation{Name: o.Name, Metadata: o.Metadata}
}

// Adapter owns all limactl invocations and their compatibility checks.
type Adapter struct {
	lima   toolchain.Artifact
	runner toolchain.Runner
	dir    string
	env    map[string]string
}

func NewAdapter(lima toolchain.Artifact, runner toolchain.Runner, dir string, env map[string]string) (*Adapter, error) {
	identity := lima.Identity()
	if identity.Component != toolchain.Lima || identity.Name != "" || identity.Version != SupportedVersion {
		return nil, &Error{Kind: InvalidSpecification, Operation: "configure", Err: errors.New("verified Lima 2.0.0 artifact is required")}
	}
	if runner == nil {
		return nil, &Error{Kind: InvalidSpecification, Operation: "configure", Err: errors.New("runner is nil")}
	}
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return nil, &Error{Kind: InvalidSpecification, Operation: "configure", Err: errors.New("working directory must be clean and absolute")}
	}
	return &Adapter{lima: lima, runner: runner, dir: dir, env: cloneMap(env)}, nil
}

// Inspect checks executable compatibility and then observes the named instance.
func (a *Adapter) Inspect(ctx context.Context, spec Spec) (Observation, error) {
	if _, err := compileConfig(spec); err != nil {
		return Observation{}, err
	}
	if err := a.checkVersion(ctx); err != nil {
		return Observation{}, err
	}
	return a.inspect(ctx, spec.RuntimeHost.InstanceName)
}

// Create creates an absent instance, adopts a matching existing instance, and
// always re-observes after an attempted side effect.
func (a *Adapter) Create(ctx context.Context, spec Spec, configPath string) (Observation, error) {
	if _, err := compileConfig(spec); err != nil {
		return Observation{}, err
	}
	if !filepath.IsAbs(configPath) || filepath.Clean(configPath) != configPath {
		return Observation{}, &Error{Kind: InvalidSpecification, Operation: "create", Resource: spec.RuntimeHost.InstanceName, Err: errors.New("config path must be clean and absolute")}
	}
	if err := verifyConfigFile(configPath, spec); err != nil {
		return Observation{}, err
	}
	if err := a.checkVersion(ctx); err != nil {
		return Observation{}, err
	}
	before, err := a.inspect(ctx, spec.RuntimeHost.InstanceName)
	if err != nil {
		return Observation{}, err
	}
	if before.State != Absent {
		return validateObservation("create", spec, before)
	}
	_, runErr := a.run(ctx, "create", "--name="+spec.RuntimeHost.InstanceName, "--tty=false", configPath)
	return a.afterMutation(ctx, "create", spec, runErr, false)
}

// Start starts a matching stopped instance and is a no-op when it is running.
func (a *Adapter) Start(ctx context.Context, spec Spec) (Observation, error) {
	if err := a.checkSpecAndVersion(ctx, spec); err != nil {
		return Observation{}, err
	}
	before, err := a.inspect(ctx, spec.RuntimeHost.InstanceName)
	if err != nil {
		return Observation{}, err
	}
	if _, err := validateObservation("start", spec, before); err != nil {
		return before, err
	}
	if before.State == Running {
		return before, nil
	}
	_, runErr := a.run(ctx, "start", "--tty=false", spec.RuntimeHost.InstanceName)
	return a.afterMutation(ctx, "start", spec, runErr, false)
}

// Stop stops a matching running instance and is a no-op when it is stopped.
func (a *Adapter) Stop(ctx context.Context, spec Spec) (Observation, error) {
	if err := a.checkSpecAndVersion(ctx, spec); err != nil {
		return Observation{}, err
	}
	before, err := a.inspect(ctx, spec.RuntimeHost.InstanceName)
	if err != nil {
		return Observation{}, err
	}
	if _, err := validateObservation("stop", spec, before); err != nil {
		return before, err
	}
	if before.State == Stopped {
		return before, nil
	}
	_, runErr := a.run(ctx, "stop", "--tty=false", spec.RuntimeHost.InstanceName)
	return a.afterMutation(ctx, "stop", spec, runErr, false)
}

// Delete removes only the exact recorded instance after live metadata and the
// immutable config have independently matched. Absence is idempotent success.
func (a *Adapter) Delete(ctx context.Context, spec Spec, recorded ownership.LimaInstanceRecord) (Observation, error) {
	if err := a.checkSpecAndVersion(ctx, spec); err != nil {
		return Observation{}, err
	}
	before, err := a.inspect(ctx, spec.RuntimeHost.InstanceName)
	if err != nil {
		return Observation{}, err
	}
	if before.State == Absent {
		return before, nil
	}
	if _, err := validateObservation("delete", spec, before); err != nil {
		return before, err
	}
	expectedRecord := recordedIdentity(spec)
	if recorded.Name != expectedRecord.Name || !equalFacts(recorded.Metadata, expectedRecord.Metadata) {
		return before, &ownership.ProofError{Resource: "lima instance", Fact: "recorded identity differs from immutable Plan"}
	}
	if _, err := ownership.ProveLimaInstance(recorded, before.ownership()); err != nil {
		return before, err
	}
	_, runErr := a.run(ctx, "delete", "--tty=false", spec.RuntimeHost.InstanceName)
	return a.afterMutation(ctx, "delete", spec, runErr, true)
}

// Invoke executes an argv command through Lima's guest control channel.
func (a *Adapter) Invoke(ctx context.Context, spec Spec, workdir string, argv []string, stdout, stderr io.Writer) (process.Result, error) {
	if err := a.checkSpecAndVersion(ctx, spec); err != nil {
		return process.Result{}, err
	}
	if len(argv) == 0 || !filepath.IsAbs(workdir) || filepath.Clean(workdir) != workdir {
		return process.Result{}, &Error{Kind: InvalidSpecification, Operation: "invoke", Resource: spec.RuntimeHost.InstanceName, Err: errors.New("argv and a clean absolute guest workdir are required")}
	}
	before, err := a.inspect(ctx, spec.RuntimeHost.InstanceName)
	if err != nil {
		return process.Result{}, err
	}
	if _, err := validateObservation("invoke", spec, before); err != nil {
		return process.Result{}, err
	}
	if before.State != Running {
		return process.Result{}, &Error{Kind: InvalidState, Operation: "invoke", Resource: spec.RuntimeHost.InstanceName, Observed: before.State, Err: errors.New("instance is not running")}
	}
	args := []string{"shell", "--tty=false", "--workdir=" + workdir, spec.RuntimeHost.InstanceName, "--"}
	args = append(args, argv...)
	result, runErr := a.runWithWriters(ctx, stdout, stderr, args...)
	if runErr != nil {
		return result, classifyRunError("invoke", spec.RuntimeHost.InstanceName, before.State, ctx, runErr)
	}
	return result, nil
}

func (a *Adapter) checkSpecAndVersion(ctx context.Context, spec Spec) error {
	if _, err := compileConfig(spec); err != nil {
		return err
	}
	return a.checkVersion(ctx)
}

func (a *Adapter) checkVersion(ctx context.Context) error {
	result, err := a.run(ctx, "--version")
	if err != nil {
		return classifyRunError("version", "", Absent, ctx, err)
	}
	if result.StdoutTruncated || result.StderrTruncated {
		return &Error{Kind: MalformedOutput, Operation: "version", Err: errors.New("version output was truncated")}
	}
	const expected = "limactl version " + SupportedVersion
	if strings.TrimSpace(string(result.Stdout)) != expected || len(bytes.TrimSpace(result.Stderr)) != 0 {
		return &Error{Kind: UnsupportedVersion, Operation: "version", Err: errors.New("limactl did not report the pinned supported version")}
	}
	return nil
}

func (a *Adapter) inspect(ctx context.Context, name string) (Observation, error) {
	result, err := a.run(ctx, "list", "--tty=false", "--format=json", "--all-fields")
	if err != nil {
		return Observation{}, classifyRunError("inspect", name, Absent, ctx, err)
	}
	if result.StdoutTruncated || result.StderrTruncated || len(bytes.TrimSpace(result.Stderr)) != 0 {
		return Observation{}, &Error{Kind: MalformedOutput, Operation: "inspect", Resource: name, Err: errors.New("inventory output was truncated or included stderr")}
	}
	instances, err := decodeInventory(result.Stdout)
	if err != nil {
		return Observation{}, &Error{Kind: MalformedOutput, Operation: "inspect", Resource: name, Err: err}
	}
	for _, instance := range instances {
		if instance.Name == name {
			return instance, nil
		}
	}
	return Observation{Name: name, State: Absent}, nil
}

func (a *Adapter) afterMutation(ctx context.Context, operation string, spec Spec, runErr error, wantAbsent bool) (Observation, error) {
	after, inspectErr := a.inspect(ctx, spec.RuntimeHost.InstanceName)
	if inspectErr != nil {
		if runErr != nil {
			return after, classifyRunError(operation, spec.RuntimeHost.InstanceName, after.State, ctx, errors.Join(runErr, inspectErr))
		}
		return after, inspectErr
	}
	if runErr != nil {
		if after.State == Partial {
			return after, &Error{Kind: PartialCreation, Operation: operation, Resource: spec.RuntimeHost.InstanceName, Observed: after.State, Err: runErr}
		}
		return after, classifyRunError(operation, spec.RuntimeHost.InstanceName, after.State, ctx, runErr)
	}
	if wantAbsent {
		if after.State != Absent {
			return after, &Error{Kind: PostconditionFailed, Operation: operation, Resource: spec.RuntimeHost.InstanceName, Observed: after.State, Err: errors.New("instance remains after successful command")}
		}
		return after, nil
	}
	if after.State == Absent {
		return after, &Error{Kind: PostconditionFailed, Operation: operation, Resource: spec.RuntimeHost.InstanceName, Observed: after.State, Err: errors.New("instance is absent after successful command")}
	}
	validated, err := validateObservation(operation, spec, after)
	if err != nil {
		return after, err
	}
	want := Stopped
	if operation == "start" {
		want = Running
	}
	if validated.State != want && (operation != "create" || validated.State != Running) {
		return after, &Error{Kind: PostconditionFailed, Operation: operation, Resource: spec.RuntimeHost.InstanceName, Observed: after.State, Err: fmt.Errorf("wanted state %s", want)}
	}
	return after, nil
}

func validateObservation(operation string, spec Spec, observation Observation) (Observation, error) {
	if observation.State == Absent {
		return observation, &Error{Kind: NotFound, Operation: operation, Resource: spec.RuntimeHost.InstanceName, Observed: Absent}
	}
	if observation.State == Partial || len(observation.inspectError) > 0 {
		return observation, &Error{Kind: PartialCreation, Operation: operation, Resource: spec.RuntimeHost.InstanceName, Observed: Partial, Err: errors.New("lima reports an incomplete or broken instance")}
	}
	if strings.TrimPrefix(observation.LimaVersion, "v") != SupportedVersion {
		return observation, &Error{Kind: UnsupportedVersion, Operation: operation, Resource: spec.RuntimeHost.InstanceName, Observed: observation.State, Err: errors.New("instance was created by an unsupported Lima version")}
	}
	if err := matchesConfig(spec, observation.config); err != nil {
		var adapterErr *Error
		if errors.As(err, &adapterErr) {
			adapterErr.Operation = operation
			adapterErr.Observed = observation.State
		}
		return observation, err
	}
	return observation, nil
}

func decodeInventory(data []byte) ([]Observation, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	instances := make([]Observation, 0)
	seen := make(map[string]struct{})
	for {
		var raw inventoryInstance
		err := decoder.Decode(&raw)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode JSON Lines inventory: %w", err)
		}
		if !validName(raw.Name) {
			return nil, errors.New("inventory contains an invalid instance name")
		}
		if _, exists := seen[raw.Name]; exists {
			return nil, errors.New("inventory contains duplicate instance names")
		}
		seen[raw.Name] = struct{}{}
		var state State
		switch raw.Status {
		case "Stopped":
			state = Stopped
		case "Running":
			state = Running
		case "", "Uninitialized", "Installing", "Broken":
			state = Partial
		default:
			return nil, fmt.Errorf("inventory contains unsupported status %q", raw.Status)
		}
		instances = append(instances, Observation{
			Name: raw.Name, State: state, LimaStatus: raw.Status, Metadata: ownershipMetadata(raw.Config),
			LimaVersion: raw.LimaVersion, config: raw.Config, inspectError: append([]string(nil), raw.Errors...),
		})
	}
	return instances, nil
}

type inventoryInstance struct {
	Name        string      `json:"name"`
	Status      string      `json:"status"`
	LimaVersion string      `json:"limaVersion"`
	Errors      []string    `json:"errors"`
	Config      *limaConfig `json:"config"`
}

func (a *Adapter) run(ctx context.Context, args ...string) (process.Result, error) {
	return a.runWithWriters(ctx, nil, nil, args...)
}

func (a *Adapter) runWithWriters(ctx context.Context, stdout, stderr io.Writer, args ...string) (process.Result, error) {
	return a.lima.Run(ctx, a.runner, toolchain.Invocation{
		Args: args, Dir: a.dir, Env: cloneMap(a.env), Stdout: stdout, Stderr: stderr,
	})
}

func classifyRunError(operation, resource string, observed State, ctx context.Context, err error) error {
	var cancellation *process.CancellationError
	if ctx.Err() != nil || errors.As(err, &cancellation) {
		return &Error{Kind: Canceled, Operation: operation, Resource: resource, Observed: observed, Err: err}
	}
	return &Error{Kind: CommandFailed, Operation: operation, Resource: resource, Observed: observed, Err: err}
}

func cloneMap(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func equalFacts(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range right {
		if left[key] != value {
			return false
		}
	}
	return true
}
