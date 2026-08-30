package guesttoolchain_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/guesttoolchain"
	"github.com/shoe-point-0/agent-box/internal/lima"
	"github.com/shoe-point-0/agent-box/internal/process"
	"github.com/shoe-point-0/agent-box/internal/toolchain"
)

func TestProvisionFreshAndWarmIsVerifiedAndIdempotent(t *testing.T) {
	t.Parallel()
	spec, _, guestDigests := fixtureSpec(t)
	invoker := newFakeInvoker(spec, guestDigests)
	provisioner, err := guesttoolchain.New(invoker)
	if err != nil {
		t.Fatal(err)
	}

	for run := 0; run < 2; run++ {
		observation, err := provisioner.Provision(t.Context(), spec)
		if err != nil {
			t.Fatalf("Provision() run %d error = %v", run+1, err)
		}
		if observation.DockerVersion != "28.3.3" || observation.ComposeVersion != "2.39.1" ||
			observation.CgroupVersion != "2" || observation.StorageDriver != "overlay2" ||
			observation.DockerRootDir != "/var/lib/docker" || observation.SocketMode != "660" ||
			!slices.Contains(observation.NetworkDrivers, "bridge") {
			t.Fatalf("Provision() run %d observation = %#v", run+1, observation)
		}
	}

	calls := invoker.callsCopy()
	if countCommand(calls, spec.UtilityPaths["systemctl"]) != 2 {
		t.Fatalf("systemctl calls = %d, want 2", countCommand(calls, spec.UtilityPaths["systemctl"]))
	}
	wantStart := []string{spec.UtilityPaths["systemctl"], "enable", "--now", "docker.service"}
	for _, call := range calls {
		if call[0] == spec.UtilityPaths["systemctl"] && !slices.Equal(call, wantStart) {
			t.Fatalf("systemctl argv = %q, want %q", call, wantStart)
		}
	}
	firstStart := slices.IndexFunc(calls, func(call []string) bool { return call[0] == spec.UtilityPaths["systemctl"] })
	if firstStart != 7 { // five digests followed by Docker and Compose versions
		t.Fatalf("first mutation call index = %d, want 7; calls=%q", firstStart, calls)
	}
}

func TestGuestDigestMismatchStopsBeforeVersionOrMutation(t *testing.T) {
	t.Parallel()
	spec, _, guestDigests := fixtureSpec(t)
	guestDigests[spec.DockerPath] = strings.Repeat("0", 64)
	invoker := newFakeInvoker(spec, guestDigests)
	provisioner, _ := guesttoolchain.New(invoker)

	_, err := provisioner.Provision(t.Context(), spec)
	assertLayer(t, err, guesttoolchain.ArtifactLayer)
	calls := invoker.callsCopy()
	if countCommand(calls, spec.DockerPath) != 0 || countCommand(calls, spec.UtilityPaths["systemctl"]) != 0 {
		t.Fatalf("commands continued after digest mismatch: %q", calls)
	}
}

func TestChangedMachineArtifactStopsBeforeGuestInvocation(t *testing.T) {
	t.Parallel()
	spec, machinePaths, guestDigests := fixtureSpec(t)
	if err := os.WriteFile(machinePaths["docker"], []byte("changed"), 0o700); err != nil {
		t.Fatal(err)
	}
	invoker := newFakeInvoker(spec, guestDigests)
	provisioner, _ := guesttoolchain.New(invoker)

	_, err := provisioner.Provision(t.Context(), spec)
	assertLayer(t, err, guesttoolchain.ArtifactLayer)
	var prerequisite *toolchain.PrerequisiteError
	if !errors.As(err, &prerequisite) || prerequisite.Reason != toolchain.DigestMismatch {
		t.Fatalf("error = %v, want nested digest mismatch", err)
	}
	if calls := invoker.callsCopy(); len(calls) != 0 {
		t.Fatalf("guest calls after Machine artifact changed = %q", calls)
	}
}

func TestWrongVersionStopsBeforeDaemonStart(t *testing.T) {
	t.Parallel()
	spec, _, guestDigests := fixtureSpec(t)
	invoker := newFakeInvoker(spec, guestDigests)
	invoker.dockerVersion = "Docker version 99.0.0, build bad\n"
	provisioner, _ := guesttoolchain.New(invoker)

	_, err := provisioner.Provision(t.Context(), spec)
	assertLayer(t, err, guesttoolchain.VersionLayer)
	if countCommand(invoker.callsCopy(), spec.UtilityPaths["systemctl"]) != 0 {
		t.Fatal("daemon start reached after version mismatch")
	}
}

func TestRuntimePrerequisiteDiagnostics(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*fakeInvoker)
	}{
		{"cgroup-v1", func(invoker *fakeInvoker) { invoker.info.CgroupVersion = "1" }},
		{"wrong-storage", func(invoker *fakeInvoker) { invoker.info.Driver = "vfs" }},
		{"no-bridge", func(invoker *fakeInvoker) { invoker.info.Plugins.Network = []string{"host", "null"} }},
		{"rootless", func(invoker *fakeInvoker) { invoker.info.SecurityOptions = []string{"name=rootless"} }},
		{"world-socket", func(invoker *fakeInvoker) { invoker.socketStat = "0:0:666:socket\n" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec, _, guestDigests := fixtureSpec(t)
			invoker := newFakeInvoker(spec, guestDigests)
			test.mutate(invoker)
			provisioner, _ := guesttoolchain.New(invoker)
			_, err := provisioner.Provision(t.Context(), spec)
			assertLayer(t, err, guesttoolchain.RuntimeLayer)
		})
	}
}

func TestCancellationIdentifiesCanceledLayer(t *testing.T) {
	t.Parallel()
	spec, _, guestDigests := fixtureSpec(t)
	invoker := newFakeInvoker(spec, guestDigests)
	invoker.failPath = spec.UtilityPaths["systemctl"]
	invoker.failErr = &process.CancellationError{Cause: context.Canceled}
	provisioner, _ := guesttoolchain.New(invoker)

	_, err := provisioner.Provision(t.Context(), spec)
	assertLayer(t, err, guesttoolchain.CanceledLayer)
}

func TestInvalidContractNeverInvokesGuest(t *testing.T) {
	t.Parallel()
	spec, _, guestDigests := fixtureSpec(t)
	invoker := newFakeInvoker(spec, guestDigests)
	provisioner, _ := guesttoolchain.New(invoker)
	spec.UtilityPaths["systemctl"] = "relative/systemctl"

	_, err := provisioner.Provision(t.Context(), spec)
	assertLayer(t, err, guesttoolchain.ArtifactLayer)
	if len(invoker.callsCopy()) != 0 {
		t.Fatal("invalid contract reached guest")
	}
}

func fixtureSpec(t *testing.T) (guesttoolchain.Spec, map[string]string, map[string]string) {
	t.Helper()
	directory := t.TempDir()
	type fixture struct {
		key       string
		component toolchain.Component
		name      string
		version   string
		mode      fs.FileMode
	}
	fixtures := []fixture{
		{"lima", toolchain.Lima, "", lima.SupportedVersion, 0o700},
		{"guest-image", toolchain.GuestImage, "", "ubuntu-24.04", 0o600},
		{"docker", toolchain.Docker, "", "28.3.3", 0o700},
		{"compose", toolchain.Compose, "", "2.39.1", 0o700},
		{"sha256sum", toolchain.Utility, "sha256sum", "9.4", 0o700},
		{"stat", toolchain.Utility, "stat", "9.4", 0o700},
		{"systemctl", toolchain.Utility, "systemctl", "255", 0o700},
	}
	identities := make(map[string]toolchain.Identity, len(fixtures))
	entries := make([]toolchain.Entry, 0, len(fixtures))
	machinePaths := make(map[string]string, len(fixtures))
	for _, fixture := range fixtures {
		content := []byte("fixture-" + fixture.key)
		path := filepath.Join(directory, fixture.key)
		if err := os.WriteFile(path, content, fixture.mode); err != nil {
			t.Fatal(err)
		}
		identity := toolchain.Identity{Component: fixture.component, Name: fixture.name, Version: fixture.version, Digest: digest(content)}
		identities[fixture.key] = identity
		machinePaths[fixture.key] = path
		entries = append(entries, toolchain.Entry{Component: fixture.component, Name: fixture.name, Candidate: toolchain.Candidate{Identity: identity, Path: path}})
	}
	provider, err := toolchain.NewPreinstalledProvider(entries)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := toolchain.Verify(t.Context(), toolchain.Specification{
		Lima: identities["lima"], GuestImage: identities["guest-image"], Docker: identities["docker"], Compose: identities["compose"],
		Utilities: []toolchain.Identity{identities["sha256sum"], identities["stat"], identities["systemctl"]},
	}, provider)
	if err != nil {
		t.Fatal(err)
	}
	spec := guesttoolchain.Spec{
		Toolchain: verified, DockerPath: "/usr/bin/docker", ComposePath: "/usr/libexec/docker/cli-plugins/docker-compose",
		UtilityPaths: map[string]string{"sha256sum": "/usr/bin/sha256sum", "stat": "/usr/bin/stat", "systemctl": "/usr/bin/systemctl"},
		Workdir:      "/", DockerSocket: "/var/run/docker.sock", DockerService: "docker.service",
	}
	guestDigests := map[string]string{
		spec.DockerPath: digestHex([]byte("fixture-docker")), spec.ComposePath: digestHex([]byte("fixture-compose")),
		spec.UtilityPaths["sha256sum"]: digestHex([]byte("fixture-sha256sum")),
		spec.UtilityPaths["stat"]:      digestHex([]byte("fixture-stat")),
		spec.UtilityPaths["systemctl"]: digestHex([]byte("fixture-systemctl")),
	}
	return spec, machinePaths, guestDigests
}

func digest(content []byte) contract.Digest { return contract.Digest("sha256:" + digestHex(content)) }

func digestHex(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("%x", sum[:])
}

func assertLayer(t *testing.T, err error, want guesttoolchain.Layer) {
	t.Helper()
	var toolchainErr *guesttoolchain.Error
	if !errors.As(err, &toolchainErr) || toolchainErr.Layer != want {
		t.Fatalf("error = %v, want guest toolchain layer %s", err, want)
	}
}

type fakeInvoker struct {
	mu             sync.Mutex
	calls          [][]string
	guestDigests   map[string]string
	dockerVersion  string
	composeVersion string
	info           fakeDockerInfo
	socketStat     string
	failPath       string
	failErr        error
}

type fakeDockerInfo struct {
	ServerVersion   string   `json:"ServerVersion"`
	Driver          string   `json:"Driver"`
	CgroupVersion   string   `json:"CgroupVersion"`
	DockerRootDir   string   `json:"DockerRootDir"`
	SecurityOptions []string `json:"SecurityOptions"`
	Plugins         struct {
		Network []string `json:"Network"`
	} `json:"Plugins"`
}

func newFakeInvoker(spec guesttoolchain.Spec, guestDigests map[string]string) *fakeInvoker {
	invoker := &fakeInvoker{
		guestDigests: guestDigests, dockerVersion: "Docker version 28.3.3, build fixture\n", composeVersion: "2.39.1\n",
		socketStat: "0:0:660:socket\n",
		info:       fakeDockerInfo{ServerVersion: "28.3.3", Driver: "overlay2", CgroupVersion: "2", DockerRootDir: "/var/lib/docker"},
	}
	invoker.info.Plugins.Network = []string{"bridge", "host", "null", "overlay"}
	return invoker
}

func (f *fakeInvoker) Invoke(_ context.Context, _ lima.Spec, _ string, argv []string, _, _ io.Writer) (process.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	call := append([]string(nil), argv...)
	f.calls = append(f.calls, call)
	result := process.Result{ExitCode: 0}
	if len(argv) == 0 {
		return result, errors.New("empty argv")
	}
	if argv[0] == f.failPath {
		return result, f.failErr
	}
	switch {
	case strings.HasSuffix(argv[0], "/sha256sum"):
		path := argv[len(argv)-1]
		result.Stdout = []byte(f.guestDigests[path] + "  " + path + "\n")
	case strings.HasSuffix(argv[0], "/docker") && slices.Equal(argv[1:], []string{"--version"}):
		result.Stdout = []byte(f.dockerVersion)
	case strings.HasSuffix(argv[0], "/docker") && slices.Equal(argv[1:], []string{"compose", "version", "--short"}):
		result.Stdout = []byte(f.composeVersion)
	case strings.HasSuffix(argv[0], "/docker-compose"):
		result.Stdout = []byte(f.composeVersion)
	case strings.HasSuffix(argv[0], "/systemctl"):
	case strings.HasSuffix(argv[0], "/docker") && len(argv) > 1 && argv[1] == "info":
		result.Stdout, _ = json.Marshal(f.info)
		result.Stdout = append(result.Stdout, '\n')
	case strings.HasSuffix(argv[0], "/stat"):
		result.Stdout = []byte(f.socketStat)
	default:
		return result, fmt.Errorf("unexpected argv %q", argv)
	}
	return result, nil
}

func (f *fakeInvoker) callsCopy() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	output := make([][]string, len(f.calls))
	for index := range f.calls {
		output[index] = append([]string(nil), f.calls[index]...)
	}
	return output
}

func countCommand(calls [][]string, command string) int {
	count := 0
	for _, call := range calls {
		if len(call) > 0 && call[0] == command {
			count++
		}
	}
	return count
}
