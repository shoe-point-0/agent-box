package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/daemon"
	"github.com/shoe-point-0/agent-box/internal/daemonclient"
	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/journal"
	"github.com/shoe-point-0/agent-box/internal/machinegit"
	"github.com/shoe-point-0/agent-box/internal/paths"
	"github.com/shoe-point-0/agent-box/internal/state"
	"github.com/shoe-point-0/agent-box/internal/testutil"
)

func TestRunReportsVersion(t *testing.T) {
	t.Parallel()

	for _, argument := range []string{"--version", "version"} {
		argument := argument
		t.Run(argument, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			if code := run(context.Background(), []string{argument}, &stdout, &stderr); code != 0 {
				t.Fatalf("run(%q) code = %d, want 0; stderr = %q", argument, code, stderr.String())
			}
			if got := stdout.String(); !strings.HasPrefix(got, "aboxd devel (commit unknown, built unknown)\n") {
				t.Fatalf("run(%q) stdout = %q, want development build identity", argument, got)
			}
			if stderr.Len() != 0 {
				t.Fatalf("run(%q) stderr = %q, want empty", argument, stderr.String())
			}
		})
	}
}

func TestRunServesHandshakeAndShutsDown(t *testing.T) {
	root := testutil.TempDir(t)
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	runtimeDir := filepath.Join(root, "runtime")
	stateDatabase := filepath.Join(root, "state", "state.db")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stdout, stderr bytes.Buffer
	exit := make(chan int, 1)
	go func() {
		exit <- run(ctx, []string{"serve", "--runtime-dir", runtimeDir, "--state-db", stateDatabase}, &stdout, &stderr)
	}()

	socketPath := filepath.Join(runtimeDir, "aboxd.sock")
	waitForSocket(t, socketPath)
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://unix/v1/handshake", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set(daemon.HeaderAPIMin, "1")
	request.Header.Set(daemon.HeaderAPIMax, "1")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var handshake struct {
		APIVersion string `json:"apiVersion"`
	}
	if err := json.NewDecoder(response.Body).Decode(&handshake); err != nil {
		_ = response.Body.Close()
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || handshake.APIVersion != contract.DaemonAPIVersion {
		t.Fatalf("handshake status/version = %d/%q", response.StatusCode, handshake.APIVersion)
	}
	manifestPath := filepath.Join(root, "workspace", ".abox", "box.yaml")
	workspaceID, err := identity.Workspace(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	boxID, err := identity.Box(workspaceID, "TASK-REMOVE")
	if err != nil {
		t.Fatal(err)
	}
	daemonClient, err := daemonclient.New(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	failure, err := daemonClient.Remove(t.Context(), daemon.MutationRequest{
		WorkspaceID: workspaceID.String(), BoxID: boxID.String(), TaskKey: "TASK-REMOVE",
		ManifestPath: manifestPath, ManifestDigest: "sha256:manifest", Input: json.RawMessage(`{}`),
	}, "production-remove-missing-authority", func(daemonclient.Event) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if failure == nil || failure.Code != contract.CodeOwnershipConflict {
		t.Fatalf("production remove failure = %#v, want ownership conflict", failure)
	}

	upBoxID, err := identity.Box(workspaceID, "TASK-UP")
	if err != nil {
		t.Fatal(err)
	}
	failure, err = daemonClient.Up(t.Context(), daemon.MutationRequest{
		WorkspaceID: workspaceID.String(), BoxID: upBoxID.String(), TaskKey: "TASK-UP",
		ManifestPath: manifestPath, ManifestDigest: "sha256:manifest", Input: json.RawMessage(`{}`),
	}, "production-up-missing-installation", func(daemonclient.Event) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if failure == nil || failure.Code != contract.CodePrerequisiteFailed {
		t.Fatalf("production up failure = %#v, want prerequisite failure", failure)
	}

	// Release both test clients before measuring server shutdown.
	daemonClient.Close()
	transport.CloseIdleConnections()
	cancel()
	select {
	case code := <-exit:
		if code != contract.ExitOK {
			t.Fatalf("run() code = %d, want %d; stderr=%q", code, contract.ExitOK, stderr.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("daemon did not shut down")
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("daemon output stdout/stderr = %q/%q, want empty", stdout.String(), stderr.String())
	}
}

func TestProductionRemovalDispatchFailsClosedWithoutOwnershipAuthority(t *testing.T) {
	root := testutil.TempDir(t)
	store, err := state.Open(t.Context(), filepath.Join(root, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	operationJournal, err := journal.New(store)
	if err != nil {
		t.Fatal(err)
	}
	pathProvider, err := paths.New(root, filepath.Join(root, "cache"), filepath.Join(root, "logs"))
	if err != nil {
		t.Fatal(err)
	}
	runner := newProductionRemovalRunner(
		store, operationJournal, daemon.NewWriterGate(), pathProvider, machinegit.Client{},
		journal.Owner{InstanceID: "daemon-test", ProcessStartID: "process-test", BootID: "boot-test"}, root,
		filepath.Join(root, "installation.json"),
	)
	failure := runner.Run(t.Context(), journal.Operation{
		ID: "operation-remove", BoxID: "box-missing", Kind: "remove", State: journal.StateRunning,
	}, nil, func(string, any) error { return nil })
	if failure == nil || failure.Code != contract.CodeOwnershipConflict {
		t.Fatalf("production removal failure = %#v, want ownership conflict", failure)
	}
}

func TestProductionUpRunsLiveMachinePreflightBeforeBoxMutation(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("Linux engineering-lane refusal test")
	}
	root := testutil.TempDir(t)
	runtimeRoot := filepath.Join(root, "runtime")
	if err := os.Mkdir(runtimeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := state.Open(t.Context(), filepath.Join(root, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	operationJournal, err := journal.New(store)
	if err != nil {
		t.Fatal(err)
	}
	pathProvider, err := paths.New(root, filepath.Join(root, "cache"), filepath.Join(root, "logs"))
	if err != nil {
		t.Fatal(err)
	}
	workspaceID, err := identity.Workspace(filepath.Join(root, "fixture", ".abox", "box.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	boxID, err := identity.Box(workspaceID, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runner := newProductionUpRunner(
		store, operationJournal, pathProvider, machinegit.Client{},
		journal.Owner{InstanceID: "daemon-test", ProcessStartID: "process-test", BootID: "boot-test"},
		root, runtimeRoot, writeTestInstallation(t, root),
	)
	failure := runner.Run(t.Context(), journal.Operation{
		ID: "operation-up-preflight", BoxID: boxID.String(), Kind: "up", State: journal.StateRunning,
	}, json.RawMessage(`{}`), func(string, any) error { return nil })
	if failure == nil || failure.Code != contract.CodeMachineUnsupported {
		t.Fatalf("production up failure = %#v, want Machine unsupported", failure)
	}
	boxRoot, err := pathProvider.BoxRoot(boxID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(boxRoot); !os.IsNotExist(err) {
		t.Fatalf("Machine preflight failure left Box state at %q: %v", boxRoot, err)
	}
}

func writeTestInstallation(t *testing.T, root string) string {
	t.Helper()
	artifact := func(name, version string, executable bool) map[string]any {
		mode := os.FileMode(0o600)
		if executable {
			mode = 0o700
		}
		data := []byte("fixture-" + name + "\n")
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, data, mode); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		return map[string]any{"version": version, "digest": fmt.Sprintf("sha256:%x", digest), "path": path}
	}
	utilities := make(map[string]any)
	utilityPaths := make(map[string]string)
	for _, name := range []string{"cat", "sha256sum", "stat", "systemctl", "test"} {
		utilities[name] = artifact(name, "fixture-1", true)
		utilityPaths[name] = "/usr/bin/" + name
	}
	document := map[string]any{
		"schemaVersion": "abox.installation/v1alpha1",
		"artifacts": map[string]any{
			"lima": artifact("limactl", "2.0.0", true), "guestImage": artifact("guest-image", "fixture", false),
			"docker": artifact("docker", "fixture", true), "compose": artifact("compose", "fixture", true), "utilities": utilities,
		},
		"guest": map[string]any{
			"dockerPath": "/usr/bin/docker", "composePath": "/usr/libexec/docker/cli-plugins/docker-compose",
			"utilityPaths": utilityPaths, "workdir": "/tmp", "dockerSocket": "/var/run/docker.sock", "dockerService": "docker.service",
		},
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "installation.json")
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func waitForSocket(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if info, err := os.Lstat(path); err == nil && info.Mode().Type() == os.ModeSocket {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("socket %q did not appear", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
