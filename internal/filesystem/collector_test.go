package filesystem_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/filesystem"
	"github.com/shoe-point-0/agent-box/internal/lima"
)

func TestLiveVerifierCollectsFreshMountInfoByImmutableContainerID(t *testing.T) {
	plan, expected := exactFixture()
	expected.Docker.Containers = slices.Clone(expected.Docker.Containers)
	expected.Docker.Containers = append(expected.Docker.Containers, compose.ContainerObservation{
		ID: "container-0", Service: "workspace", Mounts: slices.Clone(expected.Docker.Containers[0].Mounts),
	})
	info := append([]byte(nil), expected.GuestMountInfo...)
	host := &collectorHost{
		mounts: expected.Lima,
		outputs: map[string]lima.Capture{
			"guest":       {Stdout: info},
			"container-0": {Stdout: info},
			"container-1": {Stdout: info},
		},
	}
	runtime := &collectorRuntime{observation: expected.Docker}
	verifier, err := filesystem.NewLiveVerifier(host, runtime)
	if err != nil {
		t.Fatal(err)
	}

	spec := liveSpec(t, plan)
	observed, err := verifier.Verify(t.Context(), spec)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if len(observed.ContainerMountInfo) != 2 || string(observed.GuestMountInfo) != string(info) {
		t.Fatalf("observation = %#v", observed)
	}
	wantCalls := [][]string{
		{"/usr/bin/cat", "--", "/proc/self/mountinfo"},
		{"/usr/bin/docker", "exec", "--", "container-0", "/bin/cat", "/proc/self/mountinfo"},
		{"/usr/bin/docker", "exec", "--", "container-1", "/bin/cat", "/proc/self/mountinfo"},
	}
	if len(host.calls) != 6 || !slices.EqualFunc(host.calls[:3], wantCalls, slices.Equal[[]string]) {
		t.Fatalf("mount collection calls = %#v, want %#v", host.calls, wantCalls)
	}
	canaryPath := host.calls[3][3]
	wantCanaryCalls := [][]string{
		{"/usr/bin/test", "!", "-e", canaryPath},
		{"/usr/bin/docker", "exec", "--", "container-0", "/usr/bin/test", "!", "-e", canaryPath},
		{"/usr/bin/docker", "exec", "--", "container-1", "/usr/bin/test", "!", "-e", canaryPath},
	}
	if !slices.EqualFunc(host.calls[3:], wantCanaryCalls, slices.Equal[[]string]) {
		t.Fatalf("canary calls = %#v, want %#v", host.calls[3:], wantCanaryCalls)
	}
	if _, err := os.Lstat(canaryPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("canary remains after verification: %v", err)
	}
	if host.inspectCalls != 1 || runtime.calls != 1 {
		t.Fatalf("inspect calls = %d runtime calls = %d, want 1 each", host.inspectCalls, runtime.calls)
	}
}

func TestLiveVerifierRefusesDuplicateContainerIdentityBeforeExec(t *testing.T) {
	plan, expected := exactFixture()
	expected.Docker.Containers = append(expected.Docker.Containers, expected.Docker.Containers[0])
	host := &collectorHost{
		mounts:  expected.Lima,
		outputs: map[string]lima.Capture{"guest": {Stdout: expected.GuestMountInfo}},
	}
	verifier, err := filesystem.NewLiveVerifier(host, &collectorRuntime{observation: expected.Docker})
	if err != nil {
		t.Fatal(err)
	}

	_, err = verifier.Verify(t.Context(), liveSpec(t, plan))
	assertError(t, err, filesystem.InvalidObservation, "docker-inspect")
	if len(host.calls) != 1 {
		t.Fatalf("calls = %#v, want only guest collection", host.calls)
	}
}

func TestLiveVerifierBoundsCollectionAndDoesNotLeakOutputOrErrors(t *testing.T) {
	plan, expected := exactFixture()
	secret := "secret-workload-path"
	tests := []struct {
		name  string
		host  *collectorHost
		run   *collectorRuntime
		kind  filesystem.ErrorKind
		layer string
	}{
		{
			name: "lima inspection",
			host: &collectorHost{inspectErr: errors.New(secret)},
			run:  &collectorRuntime{}, kind: filesystem.InvalidObservation, layer: "lima-config",
		},
		{
			name: "docker inspection",
			host: &collectorHost{mounts: expected.Lima},
			run:  &collectorRuntime{err: errors.New(secret)}, kind: filesystem.InvalidObservation, layer: "docker-inspect",
		},
		{
			name: "guest stderr",
			host: &collectorHost{mounts: expected.Lima, outputs: map[string]lima.Capture{
				"guest": {Stdout: expected.GuestMountInfo, Stderr: []byte(secret)},
			}},
			run: &collectorRuntime{observation: expected.Docker}, kind: filesystem.MalformedMountInfo, layer: "guest-mountinfo",
		},
		{
			name: "container truncation",
			host: &collectorHost{mounts: expected.Lima, outputs: map[string]lima.Capture{
				"guest":       {Stdout: expected.GuestMountInfo},
				"container-1": {Stdout: expected.GuestMountInfo, StdoutTruncated: true},
			}},
			run: &collectorRuntime{observation: expected.Docker}, kind: filesystem.MalformedMountInfo, layer: "container-mountinfo",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			verifier, err := filesystem.NewLiveVerifier(test.host, test.run)
			if err != nil {
				t.Fatal(err)
			}
			_, err = verifier.Verify(t.Context(), liveSpec(t, plan))
			assertError(t, err, test.kind, test.layer)
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("error leaked command data: %v", err)
			}
		})
	}
}

func TestLiveVerifierClassifiesCanceledCollectionWithoutInvokingLaterLayers(t *testing.T) {
	plan, _ := exactFixture()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	host := &collectorHost{inspectErr: context.Canceled}
	verifier, err := filesystem.NewLiveVerifier(host, &collectorRuntime{})
	if err != nil {
		t.Fatal(err)
	}

	_, err = verifier.Verify(ctx, liveSpec(t, plan))
	assertError(t, err, filesystem.InvalidObservation, "lima-config")
	if !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("error = %v, want stable cancellation reason", err)
	}
}

func TestLiveVerifierRejectsVisibleCanaryAndCleansMachineArtifact(t *testing.T) {
	plan, expected := exactFixture()
	secret := "secret-canary-runtime-error"
	tests := []struct {
		name         string
		key          string
		capture      lima.Capture
		err          error
		layer        string
		wantCanceled bool
	}{
		{name: "guest visibility", key: "guest", err: errors.New(secret), layer: "guest-canary"},
		{name: "container visibility", key: "container-1", err: errors.New(secret), layer: "container-canary"},
		{name: "container output", key: "container-1", capture: lima.Capture{Stdout: []byte(secret)}, layer: "container-canary"},
		{name: "guest cancellation", key: "guest", err: context.Canceled, layer: "guest-canary", wantCanceled: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			host := &collectorHost{
				mounts: expected.Lima,
				outputs: map[string]lima.Capture{
					"guest": {Stdout: expected.GuestMountInfo}, "container-1": {Stdout: expected.GuestMountInfo},
				},
				canaryOutputs: map[string]lima.Capture{test.key: test.capture},
				canaryErr:     map[string]error{test.key: test.err},
			}
			verifier, err := filesystem.NewLiveVerifier(host, &collectorRuntime{observation: expected.Docker})
			if err != nil {
				t.Fatal(err)
			}
			spec := liveSpec(t, plan)
			_, err = verifier.Verify(t.Context(), spec)
			assertError(t, err, filesystem.ProbeFailed, test.layer)
			if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), spec.CanaryRoot) {
				t.Fatalf("canary error leaked runtime data: %v", err)
			}
			if test.wantCanceled && !strings.Contains(err.Error(), "canceled") {
				t.Fatalf("canary cancellation error = %v", err)
			}
			entries, readErr := os.ReadDir(spec.CanaryRoot)
			if readErr != nil || len(entries) != 0 {
				t.Fatalf("canary cleanup = %v, entries = %v", readErr, entries)
			}
		})
	}
}

func TestLiveVerifierRejectsCanaryRootInsideDeclaredMountBeforeMutation(t *testing.T) {
	plan, _ := exactFixture()
	root := t.TempDir()
	plan.Mounts[0].Path = root
	verifier, err := filesystem.NewLiveVerifier(&collectorHost{}, &collectorRuntime{})
	if err != nil {
		t.Fatal(err)
	}
	spec := liveSpec(t, plan)
	spec.CanaryRoot = root

	_, err = verifier.Verify(t.Context(), spec)
	assertError(t, err, filesystem.InvalidObservation, "collector")
}

func TestNewLiveVerifierRejectsNilBoundaries(t *testing.T) {
	if _, err := filesystem.NewLiveVerifier(nil, &collectorRuntime{}); err == nil {
		t.Fatal("NewLiveVerifier(nil host) succeeded")
	}
	if _, err := filesystem.NewLiveVerifier(&collectorHost{}, nil); err == nil {
		t.Fatal("NewLiveVerifier(nil runtime) succeeded")
	}
}

func liveSpec(t *testing.T, planValue contract.Plan) filesystem.LiveSpec {
	t.Helper()
	return filesystem.LiveSpec{Runtime: compose.RuntimeSpec{
		Plan: planValue, DockerPath: "/usr/bin/docker", Workdir: "/boxes",
	}, CatPath: "/usr/bin/cat", TestPath: "/usr/bin/test", CanaryRoot: t.TempDir()}
}

type collectorHost struct {
	mounts        []lima.MountObservation
	inspectErr    error
	outputs       map[string]lima.Capture
	invokeErr     map[string]error
	canaryOutputs map[string]lima.Capture
	canaryErr     map[string]error
	inspectCalls  int
	calls         [][]string
}

func (h *collectorHost) InspectMounts(context.Context, lima.Spec) ([]lima.MountObservation, error) {
	h.inspectCalls++
	return append([]lima.MountObservation(nil), h.mounts...), h.inspectErr
}

func (h *collectorHost) Capture(_ context.Context, _ lima.Spec, _ string, argv []string) (lima.Capture, error) {
	h.calls = append(h.calls, append([]string(nil), argv...))
	key := "guest"
	if len(argv) >= 4 && argv[1] == "exec" {
		key = argv[3]
	}
	if argv[0] == "/usr/bin/test" || (len(argv) >= 5 && argv[4] == "/usr/bin/test") {
		return h.canaryOutputs[key], h.canaryErr[key]
	}
	return h.outputs[key], h.invokeErr[key]
}

type collectorRuntime struct {
	observation compose.RuntimeObservation
	err         error
	calls       int
}

func (r *collectorRuntime) Observe(context.Context, compose.RuntimeSpec) (compose.RuntimeObservation, error) {
	r.calls++
	return r.observation, r.err
}
