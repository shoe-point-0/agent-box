package filesystem

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/lima"
)

const (
	containerMountInfoReader = "/bin/cat"
	containerTestPath        = "/usr/bin/test"
	canaryPrefix             = ".abox-mount-canary-"
)

// RuntimeHost is the live guest-control boundary implemented by lima.Adapter.
// InspectMounts must inspect current Lima state rather than return generated
// configuration.
type RuntimeHost interface {
	InspectMounts(context.Context, lima.Spec) ([]lima.MountObservation, error)
	Capture(context.Context, lima.Spec, string, []string) (lima.Capture, error)
}

// ContainerRuntime is the fresh Docker observation boundary implemented by
// compose.Runtime.
type ContainerRuntime interface {
	Observe(context.Context, compose.RuntimeSpec) (compose.RuntimeObservation, error)
}

// LiveSpec contains the immutable runtime inputs needed to collect all four
// filesystem views. CatPath and TestPath are pinned absolute guest utility
// paths. CanaryRoot is a Machine-owned directory outside every declared mount.
type LiveSpec struct {
	Runtime    compose.RuntimeSpec
	CatPath    string
	TestPath   string
	CanaryRoot string
}

// LiveVerifier collects independent live observations and immediately applies
// the exact mount-set contract. It retains the partial observation on failure
// for in-process decisions, while errors expose no command output or paths.
type LiveVerifier struct {
	host    RuntimeHost
	runtime ContainerRuntime
}

func NewLiveVerifier(host RuntimeHost, runtime ContainerRuntime) (*LiveVerifier, error) {
	if host == nil || runtime == nil {
		return nil, failure(InvalidObservation, "collector", "runtime boundary is nil")
	}
	return &LiveVerifier{host: host, runtime: runtime}, nil
}

func (v *LiveVerifier) Verify(ctx context.Context, spec LiveSpec) (observed Observation, result error) {
	if !validLiveSpec(spec) {
		return observed, failure(InvalidObservation, "collector", "live specification is invalid")
	}
	canary, err := createCanary(spec.CanaryRoot)
	if err != nil {
		return observed, failure(InvalidObservation, "canary-create", "undeclared mount canary cannot be created safely")
	}
	defer func() {
		if err := canary.remove(); err != nil {
			result = failure(ProbeFailed, "canary-cleanup", "undeclared mount canary cleanup failed")
		}
	}()

	mounts, err := v.host.InspectMounts(ctx, spec.Runtime.RuntimeHost)
	if err != nil {
		return observed, collectionFailure(ctx, "lima-config", err)
	}
	observed.Lima = append([]lima.MountObservation(nil), mounts...)

	docker, err := v.runtime.Observe(ctx, spec.Runtime)
	if err != nil {
		return observed, collectionFailure(ctx, "docker-inspect", err)
	}
	observed.Docker = docker

	guest, err := v.invoke(ctx, spec, "guest-mountinfo", []string{spec.CatPath, "--", "/proc/self/mountinfo"})
	if err != nil {
		return observed, err
	}
	observed.GuestMountInfo = guest

	containers := append([]compose.ContainerObservation(nil), docker.Containers...)
	slices.SortFunc(containers, func(left, right compose.ContainerObservation) int {
		return strings.Compare(left.ID, right.ID)
	})
	observed.ContainerMountInfo = make(map[string][]byte, len(containers))
	for _, container := range containers {
		if !safeContainerID(container.ID) {
			return observed, failure(InvalidObservation, "docker-inspect", "container identity is invalid")
		}
		if _, duplicate := observed.ContainerMountInfo[container.ID]; duplicate {
			return observed, failure(InvalidObservation, "docker-inspect", "container set contains a duplicate ID")
		}
		observed.ContainerMountInfo[container.ID] = nil
	}
	for _, container := range containers {
		data, err := v.invoke(ctx, spec, "container-mountinfo", []string{
			spec.Runtime.DockerPath, "exec", "--", container.ID, containerMountInfoReader, "/proc/self/mountinfo",
		})
		if err != nil {
			return observed, err
		}
		observed.ContainerMountInfo[container.ID] = data
	}

	if err := Verify(spec.Runtime.Plan, observed); err != nil {
		return observed, err
	}
	if err := canary.validate(); err != nil {
		return observed, failure(ProbeFailed, "canary-machine", "undeclared mount canary identity changed")
	}
	if err := v.requireCanaryAbsent(ctx, spec, canary.path, containers); err != nil {
		return observed, err
	}
	if err := canary.validate(); err != nil {
		return observed, failure(ProbeFailed, "canary-machine", "undeclared mount canary identity changed")
	}
	return observed, nil
}

func validLiveSpec(spec LiveSpec) bool {
	if !cleanAbsolute(spec.CatPath) || !cleanAbsolute(spec.TestPath) || !cleanAbsolute(spec.CanaryRoot) ||
		spec.Runtime.DockerPath == "" || spec.Runtime.Workdir == "" {
		return false
	}
	for _, mount := range spec.Runtime.Plan.Mounts {
		if contained(mount.Path, spec.CanaryRoot) {
			return false
		}
	}
	return true
}

func (v *LiveVerifier) requireCanaryAbsent(ctx context.Context, spec LiveSpec, path string, containers []compose.ContainerObservation) error {
	if err := v.invokeAbsent(ctx, spec, "guest-canary", []string{spec.TestPath, "!", "-e", path}); err != nil {
		return err
	}
	for _, container := range containers {
		if err := v.invokeAbsent(ctx, spec, "container-canary", []string{
			spec.Runtime.DockerPath, "exec", "--", container.ID, containerTestPath, "!", "-e", path,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (v *LiveVerifier) invokeAbsent(ctx context.Context, spec LiveSpec, layer string, argv []string) error {
	result, err := v.host.Capture(ctx, spec.Runtime.RuntimeHost, spec.Runtime.Workdir, argv)
	if err != nil {
		return canaryFailure(ctx, layer, err)
	}
	if result.StdoutTruncated || result.StderrTruncated || len(result.Stdout) != 0 || len(result.Stderr) != 0 {
		return failure(ProbeFailed, layer, "undeclared mount canary check returned output")
	}
	return nil
}

func (v *LiveVerifier) invoke(ctx context.Context, spec LiveSpec, layer string, argv []string) ([]byte, error) {
	result, err := v.host.Capture(ctx, spec.Runtime.RuntimeHost, spec.Runtime.Workdir, argv)
	if err != nil {
		return nil, collectionFailure(ctx, layer, err)
	}
	if result.StdoutTruncated || result.StderrTruncated || len(result.Stdout) == 0 ||
		len(result.Stdout) > maxMountInfoBytes || len(bytes.TrimSpace(result.Stderr)) != 0 {
		return nil, failure(MalformedMountInfo, layer, "mountinfo capture is empty, truncated, oversized, or included stderr")
	}
	return append([]byte(nil), result.Stdout...), nil
}

func collectionFailure(ctx context.Context, layer string, err error) error {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return failure(InvalidObservation, layer, "live collection was canceled")
	}
	return failure(InvalidObservation, layer, "live collection failed")
}

func canaryFailure(ctx context.Context, layer string, err error) error {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return failure(ProbeFailed, layer, "undeclared mount canary check was canceled")
	}
	return failure(ProbeFailed, layer, "undeclared mount canary is visible")
}

type mountCanary struct {
	path string
	info fs.FileInfo
}

func createCanary(root string) (mountCanary, error) {
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&fs.ModeSymlink != 0 {
		return mountCanary{}, errors.New("canary root is not a directory")
	}
	path, err := os.MkdirTemp(root, canaryPrefix)
	if err != nil {
		return mountCanary{}, err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 || filepath.Dir(path) != root {
		_ = os.Remove(path)
		return mountCanary{}, errors.New("canary path is invalid")
	}
	return mountCanary{path: path, info: info}, nil
}

func (c mountCanary) validate() error {
	info, err := os.Lstat(c.path)
	if err != nil || !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 || !os.SameFile(c.info, info) {
		return errors.New("canary identity changed")
	}
	return nil
}

func (c mountCanary) remove() error {
	if err := c.validate(); err != nil {
		return err
	}
	return os.Remove(c.path)
}

func safeContainerID(id string) bool {
	return id != "" && len(id) <= 255 && !strings.ContainsAny(id, "\x00\r\n")
}
