// Package guesttoolchain verifies and provisions the pinned container toolchain
// already present in an Agent Box runtime host.
package guesttoolchain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/lima"
	"github.com/shoe-point-0/agent-box/internal/process"
	"github.com/shoe-point-0/agent-box/internal/toolchain"
)

type Layer string

const (
	ArtifactLayer Layer = "artifact"
	VersionLayer  Layer = "version"
	ServiceLayer  Layer = "service"
	RuntimeLayer  Layer = "runtime"
	CanceledLayer Layer = "canceled"
)

// Error identifies the guest-toolchain layer that failed without including
// command output, which may contain unrelated guest data.
type Error struct {
	Layer     Layer
	Operation string
	Artifact  string
	Err       error
}

func (e *Error) Error() string {
	subject := e.Operation
	if e.Artifact != "" {
		subject += " " + e.Artifact
	}
	if e.Err != nil {
		return fmt.Sprintf("guest toolchain %s: %s: %v", subject, e.Layer, e.Err)
	}
	return fmt.Sprintf("guest toolchain %s: %s", subject, e.Layer)
}

func (e *Error) Unwrap() error { return e.Err }

func (e *Error) ErrorCode() contract.ErrorCode {
	if e.Layer == ArtifactLayer || e.Layer == VersionLayer {
		return contract.CodePrerequisiteFailed
	}
	return contract.CodeInfrastructureApplyFailed
}

// Invoker is the narrow guest-control contract implemented by lima.Adapter.
type Invoker interface {
	Invoke(context.Context, lima.Spec, string, []string, io.Writer, io.Writer) (process.Result, error)
}

// Spec binds verified Machine-side identities to their preinstalled guest paths.
// Provision never downloads, installs, or discovers an implicit default.
type Spec struct {
	RuntimeHost   lima.Spec
	Toolchain     toolchain.Toolchain
	DockerPath    string
	ComposePath   string
	UtilityPaths  map[string]string
	Workdir       string
	DockerSocket  string
	DockerService string
}

// Observation is the verified post-provision runtime state.
type Observation struct {
	DockerVersion  string
	ComposeVersion string
	CgroupVersion  string
	StorageDriver  string
	DockerRootDir  string
	NetworkDrivers []string
	SocketMode     string
}

type Provisioner struct {
	invoker Invoker
}

func New(invoker Invoker) (*Provisioner, error) {
	if invoker == nil {
		return nil, &Error{Layer: ArtifactLayer, Operation: "configure", Err: errors.New("invoker is nil")}
	}
	return &Provisioner{invoker: invoker}, nil
}

// Provision verifies every immutable input before starting Docker, then
// re-observes all runtime prerequisites. Repeating it performs the same
// deterministic sequence and relies on systemctl enable --now idempotence.
func (p *Provisioner) Provision(ctx context.Context, spec Spec) (Observation, error) {
	artifacts, err := compile(spec)
	if err != nil {
		return Observation{}, err
	}
	if err := verifyMachineArtifacts(ctx, artifacts); err != nil {
		return Observation{}, err
	}
	for _, artifact := range artifacts {
		if err := p.verifyGuestDigest(ctx, spec, artifact); err != nil {
			return Observation{}, err
		}
	}
	if err := p.verifyVersions(ctx, spec); err != nil {
		return Observation{}, err
	}
	if _, err := p.invoke(ctx, spec, ServiceLayer, "start", "docker", spec.UtilityPaths["systemctl"], "enable", "--now", spec.DockerService); err != nil {
		return Observation{}, err
	}
	return p.observeRuntime(ctx, spec)
}

type artifactBinding struct {
	name     string
	artifact toolchain.Artifact
	path     string
}

func compile(spec Spec) ([]artifactBinding, error) {
	if !cleanAbsolute(spec.Workdir) || !cleanAbsolute(spec.DockerPath) || !cleanAbsolute(spec.ComposePath) ||
		!cleanAbsolute(spec.DockerSocket) || spec.DockerService != "docker.service" {
		return nil, invalid("paths must be clean and absolute and service must be docker.service")
	}
	dockerIdentity := spec.Toolchain.Docker.Identity()
	composeIdentity := spec.Toolchain.Compose.Identity()
	if dockerIdentity.Component != toolchain.Docker || dockerIdentity.Name != "" || composeIdentity.Component != toolchain.Compose || composeIdentity.Name != "" {
		return nil, invalid("verified Docker and Compose artifacts are required")
	}
	for _, required := range []string{"sha256sum", "stat", "systemctl"} {
		if _, ok := spec.Toolchain.Utilities[required]; !ok {
			return nil, invalid("required utility artifact is missing: " + required)
		}
	}
	if len(spec.Toolchain.Utilities) != len(spec.UtilityPaths) {
		return nil, invalid("utility artifact and guest path sets differ")
	}
	bindings := []artifactBinding{
		{name: "docker", artifact: spec.Toolchain.Docker, path: spec.DockerPath},
		{name: "compose", artifact: spec.Toolchain.Compose, path: spec.ComposePath},
	}
	names := make([]string, 0, len(spec.Toolchain.Utilities))
	for name := range spec.Toolchain.Utilities {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		artifact := spec.Toolchain.Utilities[name]
		path, ok := spec.UtilityPaths[name]
		identity := artifact.Identity()
		if !ok || !cleanAbsolute(path) || identity.Component != toolchain.Utility || identity.Name != name {
			return nil, invalid("utility identity or guest path is invalid: " + name)
		}
		bindings = append(bindings, artifactBinding{name: "utility/" + name, artifact: artifact, path: path})
	}
	return bindings, nil
}

func verifyMachineArtifacts(ctx context.Context, artifacts []artifactBinding) error {
	for _, artifact := range artifacts {
		file, err := artifact.artifact.Open(ctx)
		if err != nil {
			return classify(ctx, ArtifactLayer, "verify-machine", artifact.name, err)
		}
		if err := file.Close(); err != nil {
			return &Error{Layer: ArtifactLayer, Operation: "verify-machine", Artifact: artifact.name, Err: err}
		}
	}
	return nil
}

func (p *Provisioner) verifyGuestDigest(ctx context.Context, spec Spec, artifact artifactBinding) error {
	result, err := p.invoke(ctx, spec, ArtifactLayer, "verify-guest", artifact.name, spec.UtilityPaths["sha256sum"], "--", artifact.path)
	if err != nil {
		return err
	}
	want := strings.TrimPrefix(string(artifact.artifact.Identity().Digest), "sha256:")
	if string(result.Stdout) != want+"  "+artifact.path+"\n" {
		return &Error{Layer: ArtifactLayer, Operation: "verify-guest", Artifact: artifact.name, Err: errors.New("content does not match expected digest")}
	}
	return nil
}

func (p *Provisioner) verifyVersions(ctx context.Context, spec Spec) error {
	docker, err := p.invoke(ctx, spec, VersionLayer, "version", "docker", spec.DockerPath, "--version")
	if err != nil {
		return err
	}
	prefix := "Docker version " + spec.Toolchain.Docker.Identity().Version + ","
	if !strings.HasPrefix(strings.TrimSpace(string(docker.Stdout)), prefix) {
		return &Error{Layer: VersionLayer, Operation: "version", Artifact: "docker", Err: errors.New("docker did not report the pinned version")}
	}
	compose, err := p.invoke(ctx, spec, VersionLayer, "version", "compose", spec.ComposePath, "version", "--short")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(compose.Stdout)) != spec.Toolchain.Compose.Identity().Version {
		return &Error{Layer: VersionLayer, Operation: "version", Artifact: "compose", Err: errors.New("compose did not report the pinned version")}
	}
	return nil
}

type dockerInfo struct {
	ServerVersion   string   `json:"ServerVersion"`
	Driver          string   `json:"Driver"`
	CgroupVersion   string   `json:"CgroupVersion"`
	DockerRootDir   string   `json:"DockerRootDir"`
	SecurityOptions []string `json:"SecurityOptions"`
	Plugins         struct {
		Network []string `json:"Network"`
	} `json:"Plugins"`
}

func (p *Provisioner) observeRuntime(ctx context.Context, spec Spec) (Observation, error) {
	compose, err := p.invoke(ctx, spec, RuntimeLayer, "inspect", "compose", spec.DockerPath, "compose", "version", "--short")
	if err != nil {
		return Observation{}, err
	}
	if strings.TrimSpace(string(compose.Stdout)) != spec.Toolchain.Compose.Identity().Version {
		return Observation{}, &Error{Layer: RuntimeLayer, Operation: "inspect", Artifact: "compose", Err: errors.New("docker CLI did not load the pinned Compose plugin")}
	}
	result, err := p.invoke(ctx, spec, RuntimeLayer, "inspect", "docker", spec.DockerPath, "info", "--format", "{{json .}}")
	if err != nil {
		return Observation{}, err
	}
	var info dockerInfo
	decoder := json.NewDecoder(bytes.NewReader(result.Stdout))
	if err := decoder.Decode(&info); err != nil {
		return Observation{}, &Error{Layer: RuntimeLayer, Operation: "inspect", Artifact: "docker", Err: errors.New("malformed Docker info")}
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Observation{}, &Error{Layer: RuntimeLayer, Operation: "inspect", Artifact: "docker", Err: errors.New("docker info contains trailing data")}
	}
	if info.ServerVersion != spec.Toolchain.Docker.Identity().Version || info.CgroupVersion != "2" || info.Driver != "overlay2" || info.DockerRootDir == "" || !contains(info.Plugins.Network, "bridge") {
		return Observation{}, &Error{Layer: RuntimeLayer, Operation: "inspect", Artifact: "docker", Err: errors.New("version, cgroup v2, overlay2, root directory, or bridge network prerequisite failed")}
	}
	for _, option := range info.SecurityOptions {
		if strings.Contains(strings.ToLower(option), "rootless") {
			return Observation{}, &Error{Layer: RuntimeLayer, Operation: "inspect", Artifact: "docker", Err: errors.New("docker daemon is rootless")}
		}
	}
	stat, err := p.invoke(ctx, spec, RuntimeLayer, "inspect-socket", "docker", spec.UtilityPaths["stat"], "--format=%u:%g:%a:%F", "--", spec.DockerSocket)
	if err != nil {
		return Observation{}, err
	}
	mode, err := verifySocketStat(strings.TrimSpace(string(stat.Stdout)))
	if err != nil {
		return Observation{}, &Error{Layer: RuntimeLayer, Operation: "inspect-socket", Artifact: "docker", Err: err}
	}
	return Observation{
		DockerVersion: info.ServerVersion, ComposeVersion: spec.Toolchain.Compose.Identity().Version,
		CgroupVersion: info.CgroupVersion, StorageDriver: info.Driver, DockerRootDir: info.DockerRootDir,
		NetworkDrivers: append([]string(nil), info.Plugins.Network...), SocketMode: mode,
	}, nil
}

func (p *Provisioner) invoke(ctx context.Context, spec Spec, layer Layer, operation, artifact string, argv ...string) (process.Result, error) {
	result, err := p.invoker.Invoke(ctx, spec.RuntimeHost, spec.Workdir, argv, nil, nil)
	if err != nil {
		return result, classify(ctx, layer, operation, artifact, err)
	}
	if result.StdoutTruncated || result.StderrTruncated || len(bytes.TrimSpace(result.Stderr)) != 0 {
		return result, &Error{Layer: layer, Operation: operation, Artifact: artifact, Err: errors.New("command output was truncated or included stderr")}
	}
	return result, nil
}

func verifySocketStat(value string) (string, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 4 || parts[0] != "0" || parts[1] != "0" || parts[3] != "socket" {
		return "", errors.New("docker socket is not a root-owned socket")
	}
	mode := parts[2]
	if len(mode) < 3 || len(mode) > 4 {
		return "", errors.New("docker socket mode is malformed")
	}
	permission, err := strconv.ParseUint(mode[len(mode)-1:], 8, 3)
	if err != nil || permission&0o6 != 0 {
		return "", errors.New("docker socket permits world access")
	}
	return mode, nil
}

func classify(ctx context.Context, layer Layer, operation, artifact string, err error) error {
	var cancellation *process.CancellationError
	var limaErr *lima.Error
	if ctx.Err() != nil || errors.As(err, &cancellation) || (errors.As(err, &limaErr) && limaErr.Kind == lima.Canceled) {
		layer = CanceledLayer
	}
	return &Error{Layer: layer, Operation: operation, Artifact: artifact, Err: err}
}

func invalid(message string) error {
	return &Error{Layer: ArtifactLayer, Operation: "configure", Err: errors.New(message)}
}

func cleanAbsolute(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path && !strings.ContainsAny(path, "\x00\r\n")
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
