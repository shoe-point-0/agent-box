package compose

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/lima"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/plan"
	"github.com/shoe-point-0/agent-box/internal/process"
)

type RuntimeErrorKind string

const (
	RuntimeInvalid       RuntimeErrorKind = "invalid-specification"
	RuntimePolicyDrift   RuntimeErrorKind = "policy-drift"
	RuntimeMalformed     RuntimeErrorKind = "malformed-output"
	RuntimeCommandFailed RuntimeErrorKind = "command-failed"
	RuntimeCanceled      RuntimeErrorKind = "canceled"
	RuntimePostcondition RuntimeErrorKind = "postcondition-failed"
	RuntimeNotOwned      RuntimeErrorKind = "not-owned"
)

// RuntimeError classifies Compose runtime failures without embedding guest
// output, which can contain workload data or secrets.
type RuntimeError struct {
	Kind      RuntimeErrorKind
	Operation string
	Err       error
}

func (e *RuntimeError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("compose %s: %s: %v", e.Operation, e.Kind, e.Err)
	}
	return fmt.Sprintf("compose %s: %s", e.Operation, e.Kind)
}

func (e *RuntimeError) Unwrap() error { return e.Err }

func (e *RuntimeError) ErrorCode() contract.ErrorCode {
	switch e.Kind {
	case RuntimeInvalid:
		return contract.CodeInternalInvariant
	case RuntimePolicyDrift, RuntimeNotOwned:
		return contract.CodeIdentityConflict
	case RuntimeMalformed:
		return contract.CodeAPIIncompatible
	case RuntimePostcondition:
		return contract.CodeInfrastructureApplyFailed
	default:
		return contract.CodeInfrastructureApplyFailed
	}
}

// RuntimeInvoker is the narrow guest-control contract implemented by
// lima.Adapter. The process result remains available on every exit path.
type RuntimeInvoker interface {
	GuestCommandInvoker
	Invoke(context.Context, lima.Spec, string, []string, io.Writer, io.Writer) (process.Result, error)
}

// RuntimeSpec binds a compiler result to one immutable runtime host and Plan.
// Runtime commands never infer project identity from a directory or environment.
type RuntimeSpec struct {
	RuntimeHost  lima.Spec
	Plan         contract.Plan
	Generation   int64
	DockerPath   string
	Workdir      string
	Compiled     Result
	PathEvidence ownership.PathSetRecord
}

// RecordRuntimePaths captures the complete Machine path set consumed by a
// Compose apply: declared binds, repository inputs, and generated files.
func RecordRuntimePaths(compiled contract.Plan, generated Result) (ownership.PathSetRecord, error) {
	return ownership.RecordPathSet(runtimePaths(compiled, generated))
}

type PortBinding struct {
	ContainerPort string
	HostIP        string
	HostPort      string
}

type ContainerObservation struct {
	ID       string
	Name     string
	Service  string
	ImageID  string
	Image    string
	State    string
	Health   string
	Labels   map[string]string
	Ports    []PortBinding
	Networks []string
	Mounts   []MountObservation
}

type MountObservation struct {
	Type        string
	Source      string
	Destination string
	ReadWrite   bool
}

type NetworkObservation struct {
	ID         string
	Name       string
	Labels     map[string]string
	Containers []string
}

type VolumeObservation struct {
	ID         string
	Name       string
	Driver     string
	Mountpoint string
	Labels     map[string]string
}

type ImageObservation struct {
	ID             string
	RepositoryTags []string
	Labels         map[string]string
	ReferencedBy   []string
}

type RuntimeObservation struct {
	ProjectName string
	Containers  []ContainerObservation
	Networks    []NetworkObservation
	Volumes     []VolumeObservation
	Images      []ImageObservation
}

// RuntimeRecords is the exact persisted identity required before removal.
type RuntimeRecords struct {
	Resources []ownership.ComposeResourceRecord
	Volumes   []ownership.VolumeRecord
}

// RecordsFromObservation snapshots the IDs and labels needed for a later,
// independently re-observed ownership proof.
func RecordsFromObservation(observation RuntimeObservation) RuntimeRecords {
	records := RuntimeRecords{}
	for _, container := range observation.Containers {
		records.Resources = append(records.Resources, ownership.ComposeResourceRecord{
			Kind: "container", ID: container.ID, Labels: ownershipRecordLabels(container.Labels),
		})
	}
	for _, network := range observation.Networks {
		records.Resources = append(records.Resources, ownership.ComposeResourceRecord{
			Kind: "network", ID: network.ID, Labels: ownershipRecordLabels(network.Labels),
		})
	}
	for _, volume := range observation.Volumes {
		records.Volumes = append(records.Volumes, ownership.VolumeRecord{ID: volume.ID, Labels: ownershipRecordLabels(volume.Labels)})
	}
	slices.SortFunc(records.Resources, func(left, right ownership.ComposeResourceRecord) int {
		if compared := strings.Compare(left.Kind, right.Kind); compared != 0 {
			return compared
		}
		return strings.Compare(left.ID, right.ID)
	})
	slices.SortFunc(records.Volumes, func(left, right ownership.VolumeRecord) int {
		return strings.Compare(left.ID, right.ID)
	})
	return records
}

func ownershipRecordLabels(labels map[string]string) map[string]string {
	recorded := make(map[string]string, 6)
	for _, key := range []string{
		"com.docker.compose.project", "dev.abox.managed", "dev.abox.workspace",
		ownership.ComposeBoxIDFact, "dev.abox.plan", "dev.abox.generation",
	} {
		if value := labels[key]; value != "" {
			recorded[key] = value
		}
	}
	return recorded
}

type Runtime struct {
	invoker RuntimeInvoker
}

func NewRuntime(invoker RuntimeInvoker) (*Runtime, error) {
	if invoker == nil {
		return nil, runtimeError(RuntimeInvalid, "configure", errors.New("invoker is nil"))
	}
	return &Runtime{invoker: invoker}, nil
}

// Observe reads the complete project-owned runtime set and every image
// referenced by its containers. It mutates no Docker resource.
func (r *Runtime) Observe(ctx context.Context, spec RuntimeSpec) (RuntimeObservation, error) {
	if err := r.preflight(ctx, spec); err != nil {
		return RuntimeObservation{}, err
	}
	return r.observe(ctx, spec)
}

// Pull fetches only image-backed services; build-backed services are handled by
// Build. Compose owns the service classification from the normalized model.
func (r *Runtime) Pull(ctx context.Context, spec RuntimeSpec) (process.Result, error) {
	if err := r.preflight(ctx, spec); err != nil {
		return process.Result{}, err
	}
	return r.runCompose(ctx, spec, "pull", nil, nil, "pull", "--ignore-buildable")
}

func (r *Runtime) Build(ctx context.Context, spec RuntimeSpec) (process.Result, error) {
	if err := r.preflight(ctx, spec); err != nil {
		return process.Result{}, err
	}
	return r.runCompose(ctx, spec, "build", nil, nil, "build")
}

// Apply is idempotent for an unchanged compiled model and always re-observes
// after Compose returns, including when the command failed.
func (r *Runtime) Apply(ctx context.Context, spec RuntimeSpec) (RuntimeObservation, error) {
	if err := r.preflight(ctx, spec); err != nil {
		return RuntimeObservation{}, err
	}
	_, runErr := r.runCompose(ctx, spec, "apply", nil, nil, "up", "--detach", "--wait", "--remove-orphans")
	after, observeErr := r.observe(ctx, spec)
	if runErr != nil || observeErr != nil {
		return after, errors.Join(runErr, observeErr)
	}
	expected, err := servicesFromModel(spec.Compiled.ResolvedModel)
	if err != nil {
		return after, runtimeError(RuntimeInvalid, "apply", err)
	}
	observed := make(map[string]struct{}, len(after.Containers))
	for _, container := range after.Containers {
		observed[container.Service] = struct{}{}
	}
	for _, service := range expected {
		if _, ok := observed[service]; !ok {
			return after, runtimeError(RuntimePostcondition, "apply", fmt.Errorf("service %q has no observed container", service))
		}
	}
	return after, nil
}

func (r *Runtime) Stop(ctx context.Context, spec RuntimeSpec) (RuntimeObservation, error) {
	if err := r.preflight(ctx, spec); err != nil {
		return RuntimeObservation{}, err
	}
	_, runErr := r.runCompose(ctx, spec, "stop", nil, nil, "stop")
	after, observeErr := r.observe(ctx, spec)
	if runErr != nil || observeErr != nil {
		return after, errors.Join(runErr, observeErr)
	}
	for _, container := range after.Containers {
		if container.State == "running" || container.State == "restarting" || container.State == "paused" {
			return after, runtimeError(RuntimePostcondition, "stop", errors.New("a project container remains active"))
		}
	}
	return after, nil
}

// Remove requires the complete recorded live set to pass fresh ID-and-label
// ownership proofs before Compose is allowed to remove containers, networks, or
// volumes. Images are observed dependencies and are intentionally retained.
func (r *Runtime) Remove(ctx context.Context, spec RuntimeSpec, recorded RuntimeRecords) (RuntimeObservation, error) {
	if err := r.preflight(ctx, spec); err != nil {
		return RuntimeObservation{}, err
	}
	before, err := r.observe(ctx, spec)
	if err != nil {
		return before, err
	}
	if err := VerifyRemovalRecords(recorded, before); err != nil {
		return before, runtimeError(RuntimeNotOwned, "remove", err)
	}
	_, runErr := r.runCompose(ctx, spec, "remove", nil, nil, "down", "--volumes", "--remove-orphans")
	after, observeErr := r.observe(ctx, spec)
	if runErr != nil || observeErr != nil {
		return after, errors.Join(runErr, observeErr)
	}
	if len(after.Containers) != 0 || len(after.Networks) != 0 || len(after.Volumes) != 0 {
		return after, runtimeError(RuntimePostcondition, "remove", errors.New("project resources remain"))
	}
	return after, nil
}

// Exec runs an argv command without a TTY and preserves the exact process
// result and exit error returned by the guest boundary.
func (r *Runtime) Exec(ctx context.Context, spec RuntimeSpec, service string, argv []string, stdout, stderr io.Writer) (process.Result, error) {
	if err := r.preflight(ctx, spec); err != nil {
		return process.Result{}, err
	}
	if err := validateServiceAndArgv(spec, service, argv); err != nil {
		return process.Result{}, runtimeError(RuntimeInvalid, "exec", err)
	}
	arguments := []string{"exec", "--no-tty", service}
	arguments = append(arguments, argv...)
	return r.runCompose(ctx, spec, "exec", stdout, stderr, arguments...)
}

// Logs streams through the bounded process boundary. Follow runs until Compose
// exits or ctx is canceled; Tail must be non-negative.
func (r *Runtime) Logs(ctx context.Context, spec RuntimeSpec, service string, follow bool, tail int, stdout, stderr io.Writer) (process.Result, error) {
	if err := r.preflight(ctx, spec); err != nil {
		return process.Result{}, err
	}
	if tail < 0 {
		return process.Result{}, runtimeError(RuntimeInvalid, "logs", errors.New("tail must be non-negative"))
	}
	if service != "" {
		if err := validateServiceAndArgv(spec, service, []string{"logs"}); err != nil {
			return process.Result{}, runtimeError(RuntimeInvalid, "logs", err)
		}
	}
	arguments := []string{"logs", "--no-color", "--tail", strconv.Itoa(tail)}
	if follow {
		arguments = append(arguments, "--follow")
	}
	if service != "" {
		arguments = append(arguments, service)
	}
	return r.runCompose(ctx, spec, "logs", stdout, stderr, arguments...)
}

func (r *Runtime) preflight(ctx context.Context, spec RuntimeSpec) error {
	if err := validateRuntimeSpec(spec); err != nil {
		return runtimeError(RuntimeInvalid, "preflight", err)
	}
	result, err := r.invoke(ctx, spec, "preflight", r.composeArgv(spec, "config", "--format", "json"), nil, nil)
	if err != nil {
		return err
	}
	if result.StdoutTruncated || result.StderrTruncated || len(bytes.TrimSpace(result.Stderr)) != 0 {
		return runtimeError(RuntimeMalformed, "preflight", errors.New("normalization output was truncated or included stderr"))
	}
	services, err := serviceNames(result.Stdout)
	if err != nil {
		return runtimeError(RuntimeMalformed, "preflight", err)
	}
	if err := validatePolicy(result.Stdout, spec.Plan, true); err != nil {
		return runtimeError(RuntimePolicyDrift, "preflight", err)
	}
	canonical, err := validateAndCanonicalize(result.Stdout, spec.Plan, spec.Generation, services, nil)
	if err != nil {
		return runtimeError(RuntimePolicyDrift, "preflight", err)
	}
	digest := sha256.Sum256(canonical)
	actualDigest := contract.Digest("sha256:" + hex.EncodeToString(digest[:]))
	if !bytes.Equal(canonical, spec.Compiled.ResolvedModel) || actualDigest != spec.Compiled.ResolvedModelDigest {
		return runtimeError(RuntimePolicyDrift, "preflight", errors.New("live normalized model differs from retained compiler evidence"))
	}
	return nil
}

func validateRuntimeSpec(spec RuntimeSpec) error {
	if _, err := plan.CanonicalJSON(spec.Plan); err != nil {
		return fmt.Errorf("invalid Plan: %w", err)
	}
	if spec.Generation < 1 || spec.Plan.Compose.ProjectName == "" || spec.Plan.Compose.ProjectName != spec.Plan.RuntimeHost.InstanceName {
		return errors.New("project identity or generation is invalid")
	}
	if !cleanRuntimePath(spec.DockerPath) || !cleanRuntimePath(spec.Workdir) ||
		!cleanRuntimePath(spec.Compiled.RootPath) || !cleanRuntimePath(spec.Compiled.OverridePath) ||
		!cleanRuntimePath(spec.Compiled.ResolvedModelPath) {
		return errors.New("runtime and generated paths must be clean and absolute")
	}
	if spec.Compiled.ProjectName != spec.Plan.Compose.ProjectName || len(spec.Compiled.ResolvedModel) == 0 || spec.Compiled.ResolvedModelDigest == "" {
		return errors.New("compiler evidence is incomplete or has inconsistent identity")
	}
	if err := ownership.VerifyPathSet(runtimePaths(spec.Plan, spec.Compiled), spec.PathEvidence); err != nil {
		return fmt.Errorf("runtime path evidence: %w", err)
	}
	if filepath.Dir(spec.Compiled.RootPath) != spec.Workdir || filepath.Dir(spec.Compiled.OverridePath) != spec.Workdir || filepath.Dir(spec.Compiled.ResolvedModelPath) != spec.Workdir {
		return errors.New("generated files are outside the runtime working directory")
	}
	if filepath.Base(spec.Compiled.RootPath) != RootFileName || filepath.Base(spec.Compiled.OverridePath) != OverrideFileName || filepath.Base(spec.Compiled.ResolvedModelPath) != ResolvedModelFileName {
		return errors.New("generated filenames do not match the compiler contract")
	}
	retained, err := os.ReadFile(spec.Compiled.ResolvedModelPath)
	if err != nil {
		return fmt.Errorf("read retained model: %w", err)
	}
	if !bytes.Equal(retained, append(append([]byte(nil), spec.Compiled.ResolvedModel...), '\n')) {
		return errors.New("retained model file differs from compiler evidence")
	}
	for _, path := range []string{spec.Compiled.RootPath, spec.Compiled.OverridePath} {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("generated Compose input is missing or not a regular file")
		}
	}
	return nil
}

func runtimePaths(compiled contract.Plan, generated Result) []string {
	paths := make([]string, 0, len(compiled.Mounts)+len(compiled.Compose.Includes)+3)
	for _, mount := range compiled.Mounts {
		paths = append(paths, mount.Path)
	}
	for _, include := range compiled.Compose.Includes {
		paths = append(paths, include.Path)
	}
	return append(paths, generated.RootPath, generated.OverridePath, generated.ResolvedModelPath)
}

func cleanRuntimePath(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path && !strings.ContainsAny(path, "\x00\r\n")
}

func (r *Runtime) composeArgv(spec RuntimeSpec, command ...string) []string {
	argv := []string{spec.DockerPath, "compose", "--project-name", spec.Plan.Compose.ProjectName, "--project-directory", spec.Workdir}
	for _, profile := range spec.Plan.Compose.Profiles {
		argv = append(argv, "--profile", profile)
	}
	argv = append(argv, "-f", spec.Compiled.RootPath, "-f", spec.Compiled.OverridePath)
	return append(argv, command...)
}

func (r *Runtime) runCompose(ctx context.Context, spec RuntimeSpec, operation string, stdout, stderr io.Writer, command ...string) (process.Result, error) {
	result, err := r.invoke(ctx, spec, operation, r.composeArgv(spec, command...), stdout, stderr)
	if err == nil && (result.StdoutTruncated || result.StderrTruncated) {
		err = runtimeError(RuntimeMalformed, operation, errors.New("command output exceeded the bounded capture limit"))
	}
	return result, err
}

func (r *Runtime) invoke(ctx context.Context, spec RuntimeSpec, operation string, argv []string, stdout, stderr io.Writer) (process.Result, error) {
	var result process.Result
	var err error
	if len(argv) > 1 && argv[1] == "compose" {
		var inputs []lima.GuestInputFile
		inputs, err = guestComposeInputs(argv, spec.Workdir)
		if err == nil {
			result, err = r.invoker.InvokeCommand(ctx, spec.RuntimeHost, lima.GuestCommand{
				Argv: argv, Workdir: spec.Workdir, InputFiles: inputs, Stdout: stdout, Stderr: stderr,
				Environment: map[string]string{"PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "HOME": "/root", "LANG": "C", "LC_ALL": "C", "TZ": "UTC"},
			})
		}
	} else {
		result, err = r.invoker.Invoke(ctx, spec.RuntimeHost, spec.Workdir, argv, stdout, stderr)
	}
	if err == nil {
		return result, nil
	}
	kind := RuntimeCommandFailed
	var cancellation *process.CancellationError
	var limaErr *lima.Error
	if ctx.Err() != nil || errors.As(err, &cancellation) || (errors.As(err, &limaErr) && limaErr.Kind == lima.Canceled) {
		kind = RuntimeCanceled
	}
	return result, runtimeError(kind, operation, err)
}

func (r *Runtime) observe(ctx context.Context, spec RuntimeSpec) (RuntimeObservation, error) {
	observation := RuntimeObservation{ProjectName: spec.Plan.Compose.ProjectName}
	containerIDs, err := r.listIDs(ctx, spec, "container", "ps", "--all", "--quiet")
	if err != nil {
		return observation, err
	}
	if len(containerIDs) > 0 {
		data, err := r.inspect(ctx, spec, "container", containerIDs)
		if err != nil {
			return observation, err
		}
		observation.Containers, err = decodeContainers(data, expectedRuntimeLabels(spec), spec.Plan.Compose.ProjectName)
		if err != nil {
			return observation, runtimeError(RuntimeMalformed, "observe-containers", err)
		}
	}
	networkIDs, err := r.listIDs(ctx, spec, "network", "network", "ls", "--quiet")
	if err != nil {
		return observation, err
	}
	if len(networkIDs) > 0 {
		data, err := r.inspect(ctx, spec, "network", networkIDs)
		if err != nil {
			return observation, err
		}
		observation.Networks, err = decodeNetworks(data, expectedRuntimeLabels(spec), spec.Plan.Compose.ProjectName)
		if err != nil {
			return observation, runtimeError(RuntimeMalformed, "observe-networks", err)
		}
	}
	volumeIDs, err := r.listIDs(ctx, spec, "volume", "volume", "ls", "--quiet")
	if err != nil {
		return observation, err
	}
	if len(volumeIDs) > 0 {
		data, err := r.inspect(ctx, spec, "volume", volumeIDs)
		if err != nil {
			return observation, err
		}
		observation.Volumes, err = decodeVolumes(data, expectedRuntimeLabels(spec), spec.Plan.Compose.ProjectName)
		if err != nil {
			return observation, runtimeError(RuntimeMalformed, "observe-volumes", err)
		}
	}
	imageReferences := make(map[string][]string)
	for _, container := range observation.Containers {
		imageReferences[container.ImageID] = append(imageReferences[container.ImageID], container.ID)
	}
	if len(imageReferences) > 0 {
		imageIDs := make([]string, 0, len(imageReferences))
		for id := range imageReferences {
			imageIDs = append(imageIDs, id)
		}
		slices.Sort(imageIDs)
		data, err := r.inspect(ctx, spec, "image", imageIDs)
		if err != nil {
			return observation, err
		}
		observation.Images, err = decodeImages(data, imageReferences)
		if err != nil {
			return observation, runtimeError(RuntimeMalformed, "observe-images", err)
		}
	}
	return observation, nil
}

func (r *Runtime) listIDs(ctx context.Context, spec RuntimeSpec, kind string, command ...string) ([]string, error) {
	argv := append([]string{spec.DockerPath}, command...)
	argv = append(argv, "--filter", "label=com.docker.compose.project="+spec.Plan.Compose.ProjectName)
	result, err := r.invoke(ctx, spec, "observe-"+kind+"s", argv, nil, nil)
	if err != nil {
		return nil, err
	}
	if result.StdoutTruncated || result.StderrTruncated || len(bytes.TrimSpace(result.Stderr)) != 0 {
		return nil, runtimeError(RuntimeMalformed, "observe-"+kind+"s", errors.New("inventory output was truncated or included stderr"))
	}
	ids := strings.Fields(string(result.Stdout))
	originalCount := len(ids)
	for _, id := range ids {
		if !validRuntimeValue(id) {
			return nil, runtimeError(RuntimeMalformed, "observe-"+kind+"s", errors.New("inventory contains an invalid ID"))
		}
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if len(ids) != originalCount {
		return nil, runtimeError(RuntimeMalformed, "observe-"+kind+"s", errors.New("inventory contains duplicate IDs"))
	}
	return ids, nil
}

func (r *Runtime) inspect(ctx context.Context, spec RuntimeSpec, kind string, ids []string) ([]byte, error) {
	argv := []string{spec.DockerPath, kind, "inspect"}
	argv = append(argv, ids...)
	result, err := r.invoke(ctx, spec, "observe-"+kind+"s", argv, nil, nil)
	if err != nil {
		return nil, err
	}
	if result.StdoutTruncated || result.StderrTruncated || len(bytes.TrimSpace(result.Stderr)) != 0 {
		return nil, runtimeError(RuntimeMalformed, "observe-"+kind+"s", errors.New("inspect output was truncated or included stderr"))
	}
	return result.Stdout, nil
}

func expectedRuntimeLabels(spec RuntimeSpec) map[string]string {
	return ownershipLabels(spec.Plan, spec.Generation)
}

func validateObservedLabels(labels, expected map[string]string, project string) error {
	if labels["com.docker.compose.project"] != project {
		return errors.New("resource lacks the explicit Compose project label")
	}
	for key, value := range expected {
		if labels[key] != value {
			return errors.New("resource lacks an expected Agent Box ownership label")
		}
	}
	return nil
}

func decodeContainers(data []byte, expected map[string]string, project string) ([]ContainerObservation, error) {
	var raw []struct {
		ID     string `json:"Id"`
		Name   string `json:"Name"`
		Image  string `json:"Image"`
		Config struct {
			Image  string            `json:"Image"`
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
		State struct {
			Status string `json:"Status"`
			Health *struct {
				Status string `json:"Status"`
			} `json:"Health"`
		} `json:"State"`
		NetworkSettings struct {
			Ports map[string][]struct {
				HostIP   string `json:"HostIp"`
				HostPort string `json:"HostPort"`
			} `json:"Ports"`
			Networks map[string]json.RawMessage `json:"Networks"`
		} `json:"NetworkSettings"`
		Mounts []struct {
			Type        string `json:"Type"`
			Source      string `json:"Source"`
			Destination string `json:"Destination"`
			RW          bool   `json:"RW"`
		} `json:"Mounts"`
	}
	if err := decodeRuntimeJSON(data, &raw); err != nil {
		return nil, err
	}
	result := make([]ContainerObservation, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, item := range raw {
		if !validRuntimeValue(item.ID) || !validRuntimeValue(item.Image) || !validRuntimeValue(item.Config.Labels["com.docker.compose.service"]) {
			return nil, errors.New("container identity is incomplete")
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return nil, errors.New("container inspect contains a duplicate ID")
		}
		seen[item.ID] = struct{}{}
		if err := validateObservedLabels(item.Config.Labels, expected, project); err != nil {
			return nil, err
		}
		container := ContainerObservation{
			ID: item.ID, Name: strings.TrimPrefix(item.Name, "/"), Service: item.Config.Labels["com.docker.compose.service"],
			ImageID: item.Image, Image: item.Config.Image, State: item.State.Status, Labels: cloneMap(item.Config.Labels),
		}
		if item.State.Health != nil {
			container.Health = item.State.Health.Status
		}
		for port, bindings := range item.NetworkSettings.Ports {
			for _, binding := range bindings {
				container.Ports = append(container.Ports, PortBinding{ContainerPort: port, HostIP: binding.HostIP, HostPort: binding.HostPort})
			}
		}
		for name := range item.NetworkSettings.Networks {
			container.Networks = append(container.Networks, name)
		}
		for _, mount := range item.Mounts {
			container.Mounts = append(container.Mounts, MountObservation{
				Type: mount.Type, Source: mount.Source, Destination: mount.Destination, ReadWrite: mount.RW,
			})
		}
		slices.SortFunc(container.Ports, func(left, right PortBinding) int {
			return strings.Compare(left.ContainerPort+"\x00"+left.HostIP+"\x00"+left.HostPort, right.ContainerPort+"\x00"+right.HostIP+"\x00"+right.HostPort)
		})
		slices.Sort(container.Networks)
		slices.SortFunc(container.Mounts, func(left, right MountObservation) int {
			return strings.Compare(left.Destination+"\x00"+left.Source, right.Destination+"\x00"+right.Source)
		})
		result = append(result, container)
	}
	slices.SortFunc(result, func(left, right ContainerObservation) int { return strings.Compare(left.ID, right.ID) })
	return result, nil
}

func decodeNetworks(data []byte, expected map[string]string, project string) ([]NetworkObservation, error) {
	var raw []struct {
		ID         string                     `json:"Id"`
		Name       string                     `json:"Name"`
		Labels     map[string]string          `json:"Labels"`
		Containers map[string]json.RawMessage `json:"Containers"`
	}
	if err := decodeRuntimeJSON(data, &raw); err != nil {
		return nil, err
	}
	result := make([]NetworkObservation, 0, len(raw))
	for _, item := range raw {
		if !validRuntimeValue(item.ID) || !validRuntimeValue(item.Name) {
			return nil, errors.New("network identity is incomplete")
		}
		if err := validateObservedLabels(item.Labels, expected, project); err != nil {
			return nil, err
		}
		network := NetworkObservation{ID: item.ID, Name: item.Name, Labels: cloneMap(item.Labels)}
		for id := range item.Containers {
			network.Containers = append(network.Containers, id)
		}
		slices.Sort(network.Containers)
		result = append(result, network)
	}
	slices.SortFunc(result, func(left, right NetworkObservation) int { return strings.Compare(left.ID, right.ID) })
	return result, nil
}

func decodeVolumes(data []byte, expected map[string]string, project string) ([]VolumeObservation, error) {
	var raw []struct {
		Name       string            `json:"Name"`
		Driver     string            `json:"Driver"`
		Mountpoint string            `json:"Mountpoint"`
		Labels     map[string]string `json:"Labels"`
	}
	if err := decodeRuntimeJSON(data, &raw); err != nil {
		return nil, err
	}
	result := make([]VolumeObservation, 0, len(raw))
	for _, item := range raw {
		if !validRuntimeValue(item.Name) {
			return nil, errors.New("volume identity is incomplete")
		}
		if err := validateObservedLabels(item.Labels, expected, project); err != nil {
			return nil, err
		}
		result = append(result, VolumeObservation{
			ID: item.Name, Name: item.Name, Driver: item.Driver, Mountpoint: item.Mountpoint, Labels: cloneMap(item.Labels),
		})
	}
	slices.SortFunc(result, func(left, right VolumeObservation) int { return strings.Compare(left.ID, right.ID) })
	return result, nil
}

func decodeImages(data []byte, references map[string][]string) ([]ImageObservation, error) {
	var raw []struct {
		ID       string   `json:"Id"`
		RepoTags []string `json:"RepoTags"`
		Config   struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if err := decodeRuntimeJSON(data, &raw); err != nil {
		return nil, err
	}
	result := make([]ImageObservation, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, item := range raw {
		if !validRuntimeValue(item.ID) {
			return nil, errors.New("image identity is incomplete")
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return nil, errors.New("image inspect contains a duplicate ID")
		}
		seen[item.ID] = struct{}{}
		referencedBy, ok := references[item.ID]
		if !ok || len(referencedBy) == 0 {
			return nil, errors.New("image is not attributable to an observed container")
		}
		slices.Sort(item.RepoTags)
		slices.Sort(referencedBy)
		result = append(result, ImageObservation{
			ID: item.ID, RepositoryTags: append([]string(nil), item.RepoTags...), Labels: cloneMap(item.Config.Labels), ReferencedBy: append([]string(nil), referencedBy...),
		})
	}
	if len(seen) != len(references) {
		return nil, errors.New("one or more referenced images were not inspected")
	}
	slices.SortFunc(result, func(left, right ImageObservation) int { return strings.Compare(left.ID, right.ID) })
	return result, nil
}

func decodeRuntimeJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode Docker JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("docker JSON contains trailing data")
	}
	return nil
}

func servicesFromModel(model []byte) ([]string, error) {
	return serviceNames(model)
}

func validateServiceAndArgv(spec RuntimeSpec, service string, argv []string) error {
	if !validRuntimeValue(service) || len(argv) == 0 {
		return errors.New("service and argv are required")
	}
	for _, argument := range argv {
		if argument == "" || strings.IndexByte(argument, 0) >= 0 {
			return errors.New("argv contains an invalid argument")
		}
	}
	services, err := servicesFromModel(spec.Compiled.ResolvedModel)
	if err != nil {
		return err
	}
	if !slices.Contains(services, service) {
		return errors.New("service is absent from the retained model")
	}
	return nil
}

// VerifyRemovalRecords proves that every live Compose resource is the same
// persisted object Agent Box previously recorded. Recorded objects may already
// be absent so an interrupted removal can converge, but an unrecorded live ID
// or changed ownership label always refuses the next mutation.
func VerifyRemovalRecords(recorded RuntimeRecords, live RuntimeObservation) error {
	recordedResources := make(map[string]ownership.ComposeResourceRecord, len(recorded.Resources))
	for _, record := range recorded.Resources {
		key := record.Kind + "\x00" + record.ID
		if _, duplicate := recordedResources[key]; duplicate {
			return errors.New("recorded resource set contains a duplicate")
		}
		if _, err := ownership.ProveComposeResource(record, ownership.ComposeResourceObservation(record)); err != nil {
			return err
		}
		recordedResources[key] = record
	}

	liveResources := make(map[string]ownership.ComposeResourceObservation, len(live.Containers)+len(live.Networks))
	for _, container := range live.Containers {
		key := "container\x00" + container.ID
		if _, duplicate := liveResources[key]; duplicate {
			return errors.New("live resource set contains a duplicate")
		}
		liveResources[key] = ownership.ComposeResourceObservation{Kind: "container", ID: container.ID, Labels: container.Labels}
	}
	for _, network := range live.Networks {
		key := "network\x00" + network.ID
		if _, duplicate := liveResources[key]; duplicate {
			return errors.New("live resource set contains a duplicate")
		}
		liveResources[key] = ownership.ComposeResourceObservation{Kind: "network", ID: network.ID, Labels: network.Labels}
	}
	for key, observed := range liveResources {
		record, ok := recordedResources[key]
		if !ok {
			return errors.New("live resource is absent from recorded ownership set")
		}
		if _, err := ownership.ProveComposeResource(record, observed); err != nil {
			return err
		}
	}

	recordedVolumes := make(map[string]ownership.VolumeRecord, len(recorded.Volumes))
	for _, record := range recorded.Volumes {
		if _, duplicate := recordedVolumes[record.ID]; duplicate {
			return errors.New("recorded volume set contains a duplicate")
		}
		if _, err := ownership.ProveVolume(record, ownership.VolumeObservation(record)); err != nil {
			return err
		}
		recordedVolumes[record.ID] = record
	}
	liveVolumes := make(map[string]ownership.VolumeObservation, len(live.Volumes))
	for _, volume := range live.Volumes {
		if _, duplicate := liveVolumes[volume.ID]; duplicate {
			return errors.New("live volume set contains a duplicate")
		}
		liveVolumes[volume.ID] = ownership.VolumeObservation{ID: volume.ID, Labels: volume.Labels}
	}
	for id, observed := range liveVolumes {
		record, ok := recordedVolumes[id]
		if !ok {
			return errors.New("live volume is absent from recorded ownership set")
		}
		if _, err := ownership.ProveVolume(record, observed); err != nil {
			return err
		}
	}
	return nil
}

func validRuntimeValue(value string) bool {
	return value != "" && !strings.ContainsAny(value, "\x00\r\n")
}

func runtimeError(kind RuntimeErrorKind, operation string, err error) error {
	return &RuntimeError{Kind: kind, Operation: operation, Err: err}
}
