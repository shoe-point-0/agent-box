package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/daemon"
	"github.com/shoe-point-0/agent-box/internal/daemonclient"
	"github.com/shoe-point-0/agent-box/internal/manifest"
)

type fakeDaemon struct {
	handshakeFailures []*contract.Failure
	handshakeErrors   []error
	handshakeDefault  error
	upFailure         *contract.Failure
	upError           error
	listOutput        contract.ListOutput
	listFailure       *contract.Failure
	listError         error
	events            []daemonclient.Event
	request           daemon.MutationRequest
	key               string
	closed            bool
	handshakes        int
	listCalls         int
}

func (f *fakeDaemon) Handshake(context.Context) (*contract.Failure, error) {
	index := f.handshakes
	f.handshakes++
	var failure *contract.Failure
	var err error
	if index < len(f.handshakeFailures) {
		failure = f.handshakeFailures[index]
	}
	if index < len(f.handshakeErrors) {
		err = f.handshakeErrors[index]
	} else {
		err = f.handshakeDefault
	}
	return failure, err
}

func (f *fakeDaemon) Up(_ context.Context, request daemon.MutationRequest, key string, consume func(daemonclient.Event) error) (*contract.Failure, error) {
	f.request, f.key = request, key
	for _, event := range f.events {
		if err := consume(event); err != nil {
			return nil, err
		}
	}
	return f.upFailure, f.upError
}

func (f *fakeDaemon) List(context.Context) (contract.ListOutput, *contract.Failure, error) {
	f.listCalls++
	return f.listOutput, f.listFailure, f.listError
}

func (f *fakeDaemon) Close() { f.closed = true }

func TestRunReportsVersion(t *testing.T) {
	t.Parallel()
	for _, argument := range []string{"--version", "version"} {
		var stdout, stderr bytes.Buffer
		if code := run(context.Background(), []string{argument}, &stdout, &stderr); code != 0 {
			t.Fatalf("run(%q) code = %d, want 0; stderr = %q", argument, code, stderr.String())
		}
		if got := stdout.String(); !strings.HasPrefix(got, "abox devel (commit unknown, built unknown)\n") {
			t.Fatalf("run(%q) stdout = %q, want development build identity", argument, got)
		}
		if stderr.Len() != 0 {
			t.Fatalf("run(%q) stderr = %q, want empty", argument, stderr.String())
		}
	}
}

func TestHelpExposesOnlyImplementedCommands(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("help code = %d", code)
	}
	for _, command := range []string{"init", "list", "up", "version", "help"} {
		if !strings.Contains(stdout.String(), "  "+command) {
			t.Errorf("help does not expose %q: %q", command, stdout.String())
		}
	}
	for _, unavailable := range []string{"plan", "status", "shell", "exec", "logs", "routes", "publish", "down", "rm", "doctor"} {
		if strings.Contains(stdout.String(), "  "+unavailable+" ") {
			t.Errorf("help claims unavailable command %q", unavailable)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("help stderr = %q", stderr.String())
	}
}

func TestInitCreatesDeterministicStrictManifestWithoutDaemonOrBoxState(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	deps := dependencies{
		getwd: func() (string, error) { return root, nil },
		userPaths: func() (daemon.UserPaths, error) {
			panic("init contacted the daemon path provider")
		},
		newClient: func(string) (daemonAPI, error) {
			panic("init constructed a daemon client")
		},
		startDaemon: func(daemon.UserPaths) (<-chan error, error) {
			panic("init started the daemon")
		},
	}
	var stdout, stderr bytes.Buffer
	if code := runWith(t.Context(), []string{"init"}, &stdout, &stderr, deps); code != contract.ExitOK {
		t.Fatalf("init code = %d; stderr = %q", code, stderr.String())
	}
	manifestPath := filepath.Join(root, ".abox", "box.yaml")
	if got, want := stdout.String(), "created "+manifestPath+"\n"; got != want {
		t.Fatalf("init stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("init stderr = %q, want empty", stderr.String())
	}
	contents, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(contents), starterManifest; got != want {
		t.Fatalf("starter manifest differs:\n%s", got)
	}
	info, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("manifest mode = %04o, want 0600", got)
	}
	loader, err := manifest.NewLoader([]string{root}, manifest.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := loader.Load(manifestPath)
	if err != nil {
		t.Fatalf("strict loader rejected starter manifest: %v", err)
	}
	if got := workspace.Spec.Repositories[0].CanonicalLocal; got != root {
		t.Fatalf("starter repository = %q, want %q", got, root)
	}
	rootEntries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	manifestEntries, err := os.ReadDir(filepath.Join(root, ".abox"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rootEntries) != 1 || rootEntries[0].Name() != ".abox" || len(manifestEntries) != 1 || manifestEntries[0].Name() != "box.yaml" {
		t.Fatalf("init created unexpected filesystem state: root=%v .abox=%v", rootEntries, manifestEntries)
	}
}

func TestInitPreservesExistingManifestUnlessForced(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	directory := filepath.Join(root, ".abox")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "box.yaml")
	original := []byte("existing manifest\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	deps := dependencies{getwd: func() (string, error) { return root, nil }}
	var stdout, stderr bytes.Buffer
	if code := runWith(t.Context(), []string{"init"}, &stdout, &stderr, deps); code != contract.ExitConflict {
		t.Fatalf("init existing code = %d, want %d; stderr=%q", code, contract.ExitConflict, stderr.String())
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(contents, original) {
		t.Fatalf("non-force init replaced existing manifest with %q", contents)
	}
	stdout.Reset()
	stderr.Reset()
	if code := runWith(t.Context(), []string{"init", "--force"}, &stdout, &stderr, deps); code != contract.ExitOK {
		t.Fatalf("forced init code = %d; stderr=%q", code, stderr.String())
	}
	contents, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(contents); got != starterManifest {
		t.Fatalf("forced init content differs:\n%s", got)
	}
}

func TestInitRefusesSymlinkedManifestDirectory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(root, ".abox")); err != nil {
		t.Fatal(err)
	}
	deps := dependencies{getwd: func() (string, error) { return root, nil }}
	var stdout, stderr bytes.Buffer
	if code := runWith(t.Context(), []string{"init", "--force"}, &stdout, &stderr, deps); code != contract.ExitPrerequisite {
		t.Fatalf("symlink init code = %d, want %d; stderr=%q", code, contract.ExitPrerequisite, stderr.String())
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("symlink target was mutated: %v", entries)
	}
}

func TestInitHelpAndArguments(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"init", "--force", "--force"}, {"init", "extra"}, {"init", "--help", "--force"}} {
		var stdout, stderr bytes.Buffer
		if code := runWith(t.Context(), args, &stdout, &stderr, dependencies{}); code != contract.ExitUsage {
			t.Errorf("runWith(%v) code = %d, want %d", args, code, contract.ExitUsage)
		}
	}
	var stdout, stderr bytes.Buffer
	if code := runWith(t.Context(), []string{"init", "--help"}, &stdout, &stderr, dependencies{}); code != contract.ExitOK {
		t.Fatalf("init help code = %d", code)
	}
	if !strings.Contains(stdout.String(), "--force") || stderr.Len() != 0 {
		t.Fatalf("init help stdout/stderr = %q/%q", stdout.String(), stderr.String())
	}
}

func TestListRendersDeterministicHumanInventory(t *testing.T) {
	t.Parallel()
	client := &fakeDaemon{listOutput: listFixture()}
	var stdout, stderr bytes.Buffer
	code := runWith(t.Context(), []string{"list"}, &stdout, &stderr, testDependencies(t.TempDir(), client))
	if code != contract.ExitOK || stderr.Len() != 0 {
		t.Fatalf("list code/stderr = %d/%q", code, stderr.String())
	}
	want := "BOX           STATE     TASK    WORKSPACE    DRIFT\n" +
		"box-active    active    TASK-A  ws-active    -\n" +
		"box-degraded  degraded  TASK-D  ws-degraded  runtime host is running while Compose is stopped\n" +
		"box-stale     stale     TASK-X  ws-stale     runtime host record is missing for a materialized generation\n" +
		"box-stopped   stopped   TASK-S  ws-stopped   -\n"
	if got := stdout.String(); got != want {
		t.Fatalf("human list = %q, want %q", got, want)
	}
	if client.listCalls != 1 || !client.closed {
		t.Fatalf("list calls/closed = %d/%v", client.listCalls, client.closed)
	}
}

func TestListJSONValidatesAgainstVersionedSchema(t *testing.T) {
	t.Parallel()
	client := &fakeDaemon{listOutput: listFixture()}
	var stdout, stderr bytes.Buffer
	code := runWith(t.Context(), []string{"list", "--json"}, &stdout, &stderr, testDependencies(t.TempDir(), client))
	if code != contract.ExitOK || stderr.Len() != 0 {
		t.Fatalf("JSON list code/stderr = %d/%q", code, stderr.String())
	}
	validateJSONLine(t, compileCLISchema(t, "list"), bytes.TrimSpace(stdout.Bytes()))
	var output contract.ListOutput
	decodeStrictTest(t, bytes.TrimSpace(stdout.Bytes()), &output)
	if len(output.Boxes) != 4 || output.SchemaVersion != contract.ListSchemaVersion {
		t.Fatalf("JSON list output = %#v", output)
	}
}

func TestListEmptyAndArguments(t *testing.T) {
	t.Parallel()
	client := &fakeDaemon{listOutput: contract.ListOutput{SchemaVersion: contract.ListSchemaVersion, Boxes: []contract.ListBox{}}}
	var stdout, stderr bytes.Buffer
	if code := runWith(t.Context(), []string{"list"}, &stdout, &stderr, testDependencies(t.TempDir(), client)); code != contract.ExitOK {
		t.Fatalf("empty list code = %d; stderr=%q", code, stderr.String())
	}
	if got, want := stdout.String(), "No Boxes.\n"; got != want {
		t.Fatalf("empty list output = %q, want %q", got, want)
	}
	for _, args := range [][]string{{"list", "--json", "--json"}, {"list", "extra"}, {"list", "--help", "--json"}} {
		stdout.Reset()
		stderr.Reset()
		if code := runWith(t.Context(), args, &stdout, &stderr, dependencies{}); code != contract.ExitUsage {
			t.Errorf("runWith(%v) code = %d, want %d", args, code, contract.ExitUsage)
		}
	}
	stdout.Reset()
	stderr.Reset()
	if code := runWith(t.Context(), []string{"list", "--help"}, &stdout, &stderr, dependencies{}); code != contract.ExitOK || !strings.Contains(stdout.String(), "--json") || stderr.Len() != 0 {
		t.Fatalf("list help code/stdout/stderr = %d/%q/%q", code, stdout.String(), stderr.String())
	}
}

func TestUpDiscoversManifestAndRendersHumanProgressOnStderr(t *testing.T) {
	t.Parallel()
	root, nested, manifest := manifestFixture(t)
	client := &fakeDaemon{events: []daemonclient.Event{testEvent("reconcile.started", 1)}, upFailure: testFailure(contract.CodeFeatureNotSupported)}
	deps := testDependencies(nested, client)
	var stdout, stderr bytes.Buffer
	code := runWith(t.Context(), []string{"up", "TASK-1"}, &stdout, &stderr, deps)
	if code != contract.ExitUsage {
		t.Fatalf("up code = %d, want %d; stderr=%q", code, contract.ExitUsage, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("human stdout = %q, want empty", stdout.String())
	}
	if got, want := stderr.String(), "abox: reconcile.started\nabox: ABOX_FEATURE_NOT_SUPPORTED: test failure\n"; got != want {
		t.Fatalf("human stderr = %q, want %q", got, want)
	}
	if client.request.ManifestPath != manifest || client.request.TaskKey != "TASK-1" ||
		!strings.HasPrefix(client.request.ManifestDigest, "sha256:") || client.request.WorkspaceID == "" || client.request.BoxID == "" {
		t.Fatalf("request = %#v; fixture root=%q", client.request, root)
	}
	if !strings.HasPrefix(client.key, "cli_") || !client.closed {
		t.Fatalf("idempotency/close = %q/%v", client.key, client.closed)
	}
}

func TestUpJSONIsSchemaValidNDJSONAndKeepsStderrEmpty(t *testing.T) {
	t.Parallel()
	_, cwd, _ := manifestFixture(t)
	client := &fakeDaemon{events: []daemonclient.Event{
		testEvent("reconcile.started", 1),
		testEvent("operation.finished", 2),
	}, upFailure: testFailure(contract.CodeServiceNotReady)}
	var stdout, stderr bytes.Buffer
	code := runWith(t.Context(), []string{"up", "TASK-2", "--json"}, &stdout, &stderr, testDependencies(cwd, client))
	if code != contract.ExitReadiness {
		t.Fatalf("JSON up code = %d, want %d", code, contract.ExitReadiness)
	}
	if stderr.Len() != 0 {
		t.Fatalf("JSON stderr = %q, want empty", stderr.String())
	}
	lines := bytes.Split(bytes.TrimSpace(stdout.Bytes()), []byte("\n"))
	if len(lines) != 3 {
		t.Fatalf("NDJSON line count = %d, want 3; output=%q", len(lines), stdout.String())
	}
	eventSchema := compileCLISchema(t, "event")
	for _, line := range lines[:2] {
		validateJSONLine(t, eventSchema, line)
		var event contract.Event
		decodeStrictTest(t, line, &event)
		if event.SchemaVersion != contract.EventSchemaVersion {
			t.Fatalf("event schema version = %q", event.SchemaVersion)
		}
	}
	errorSchema := compileCLISchema(t, "error")
	validateJSONLine(t, errorSchema, lines[2])
	var failure contract.Failure
	decodeStrictTest(t, lines[2], &failure)
	if failure.Code != contract.CodeServiceNotReady {
		t.Fatalf("JSON failure code = %q", failure.Code)
	}
}

func TestStableExitCodeMapping(t *testing.T) {
	t.Parallel()
	_, cwd, _ := manifestFixture(t)
	tests := []struct {
		name string
		code contract.ErrorCode
		exit int
	}{
		{"usage", contract.CodeCLIUsage, contract.ExitUsage},
		{"transport", contract.CodeDaemonUnavailable, contract.ExitInfrastructure},
		{"not-found", contract.CodeNotFound, contract.ExitNotFound},
		{"safety", contract.CodeCleanupRefused, contract.ExitSafety},
		{"readiness", contract.CodeRouteNotReady, contract.ExitReadiness},
		{"compatibility", contract.CodeAPIIncompatible, contract.ExitCompatibility},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &fakeDaemon{upFailure: testFailure(test.code)}
			var stdout, stderr bytes.Buffer
			if got := runWith(t.Context(), []string{"up", "TASK"}, &stdout, &stderr, testDependencies(cwd, client)); got != test.exit {
				t.Fatalf("exit = %d, want %d; stderr=%q", got, test.exit, stderr.String())
			}
		})
	}
}

func TestUsageFailureInJSONModeIsParseable(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	code := runWith(t.Context(), []string{"up", "--json", "bad task"}, &stdout, &stderr, dependencies{})
	if code != contract.ExitUsage || stderr.Len() != 0 {
		t.Fatalf("usage code/stderr = %d/%q", code, stderr.String())
	}
	validateJSONLine(t, compileCLISchema(t, "error"), bytes.TrimSpace(stdout.Bytes()))
}

func TestConnectStartsMissingDaemonAndWaitsForHandshake(t *testing.T) {
	client := &fakeDaemon{handshakeErrors: []error{daemonclient.ErrUnavailable, daemonclient.ErrUnavailable, nil}}
	starts := 0
	deps := testDependencies(t.TempDir(), client)
	deps.startDaemon = func(daemon.UserPaths) (<-chan error, error) {
		starts++
		return make(chan error), nil
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	connected, connectFailure := connect(ctx, daemon.UserPaths{Socket: "/tmp/test.sock"}, deps)
	if connectFailure != nil || connected != client || starts != 1 || client.handshakes != 3 {
		t.Fatalf("connect = %T, failure=%v starts=%d handshakes=%d", connected, connectFailure, starts, client.handshakes)
	}
	connected.Close()
}

func TestConnectReportsEarlyLauncherExitWithoutFullStartupWait(t *testing.T) {
	client := &fakeDaemon{handshakeErrors: []error{daemonclient.ErrUnavailable}, handshakeDefault: daemonclient.ErrUnavailable}
	deps := testDependencies(t.TempDir(), client)
	done := make(chan error, 1)
	done <- errors.New("daemon exited")
	close(done)
	deps.startDaemon = func(daemon.UserPaths) (<-chan error, error) { return done, nil }
	started := time.Now()
	connected, connectFailure := connect(t.Context(), daemon.UserPaths{Socket: "/tmp/test.sock"}, deps)
	if connected != nil || connectFailure == nil || connectFailure.Code != contract.CodeDaemonUnavailable {
		t.Fatalf("connect = %T, failure=%#v", connected, connectFailure)
	}
	if elapsed := time.Since(started); elapsed >= startupBudget {
		t.Fatalf("launcher exit reported after %s, want before full %s budget", elapsed, startupBudget)
	}
	if !strings.Contains(connectFailure.Message, "recovery may be required") {
		t.Fatalf("launcher failure message = %q", connectFailure.Message)
	}
}

func TestResolveManifestRejectsMissingAndOversizedFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if _, _, got := resolveManifest(root, ""); got == nil || got.Code != contract.CodeManifestInvalid {
		t.Fatalf("missing manifest failure = %#v", got)
	}
	path := filepath.Join(root, "large.yaml")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), maxManifest+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, got := resolveManifest(root, path); got == nil || got.Code != contract.CodeManifestInvalid {
		t.Fatalf("oversized manifest failure = %#v", got)
	}
}

func TestStartDaemonLaunchesPathExecutableWithPrivateLog(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "started")
	executable := filepath.Join(root, "aboxd")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$ABOX_START_MARKER\"\n"
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root)
	t.Setenv("ABOX_START_MARKER", marker)
	paths := daemon.UserPaths{
		RuntimeDir: filepath.Join(root, "runtime"), StateDatabase: filepath.Join(root, "state.db"),
		LogFile: filepath.Join(root, "logs", "aboxd.log"), Socket: filepath.Join(root, "runtime", "aboxd.sock"),
	}
	done, err := startDaemon(paths)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		contents, err := os.ReadFile(marker)
		if err == nil {
			want := "serve\n--runtime-dir\n" + paths.RuntimeDir + "\n--state-db\n" + paths.StateDatabase + "\n"
			if string(contents) != want {
				t.Fatalf("daemon argv = %q, want %q", contents, want)
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) || time.Now().After(deadline) {
			t.Fatalf("wait for daemon marker: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	for path, want := range map[string]os.FileMode{filepath.Dir(paths.LogFile): 0o700, paths.LogFile: 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("mode %s = %04o, want %04o", path, got, want)
		}
	}
	if err := <-done; err != nil {
		t.Fatalf("daemon launcher wait = %v", err)
	}
}

func listFixture() contract.ListOutput {
	return contract.ListOutput{SchemaVersion: contract.ListSchemaVersion, Boxes: []contract.ListBox{
		{ID: "box-active", WorkspaceID: "ws-active", TaskKey: "TASK-A", ManifestPath: "/work/active/.abox/box.yaml", State: "active", RecordedState: "ready", Drift: []string{}},
		{ID: "box-degraded", WorkspaceID: "ws-degraded", TaskKey: "TASK-D", ManifestPath: "/work/degraded/.abox/box.yaml", State: "degraded", RecordedState: "ready", Drift: []string{"runtime host is running while Compose is stopped"}},
		{ID: "box-stale", WorkspaceID: "ws-stale", TaskKey: "TASK-X", ManifestPath: "/work/stale/.abox/box.yaml", State: "stale", RecordedState: "ready", Drift: []string{"runtime host record is missing for a materialized generation"}},
		{ID: "box-stopped", WorkspaceID: "ws-stopped", TaskKey: "TASK-S", ManifestPath: "/work/stopped/.abox/box.yaml", State: "stopped", RecordedState: "stopped", Drift: []string{}},
	}}
}

func testDependencies(cwd string, client daemonAPI) dependencies {
	return dependencies{
		getwd:     func() (string, error) { return cwd, nil },
		userPaths: func() (daemon.UserPaths, error) { return daemon.UserPaths{Socket: "/test/aboxd.sock"}, nil },
		newClient: func(string) (daemonAPI, error) { return client, nil },
		startDaemon: func(daemon.UserPaths) (<-chan error, error) {
			return nil, errors.New("unexpected daemon start")
		},
		random: strings.NewReader("0123456789abcdef"),
	}
}

func manifestFixture(t *testing.T) (root, nested, manifest string) {
	t.Helper()
	root = t.TempDir()
	manifest = filepath.Join(root, ".abox", "box.yaml")
	if err := os.MkdirAll(filepath.Dir(manifest), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte("apiVersion: abox.dev/v1alpha1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifest = canonical
	nested = filepath.Join(root, "one", "two")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	return root, nested, manifest
}

func testEvent(kind string, sequence int64) daemonclient.Event {
	details := json.RawMessage(`{"apiVersion":"abox.api/v1alpha1","state":"failed","errorCode":"ABOX_SERVICE_NOT_READY"}`)
	if kind != "operation.finished" {
		details = json.RawMessage(`{"step":"test"}`)
	}
	return daemonclient.Event{
		APIVersion: contract.DaemonAPIVersion, OperationID: "op_0123456789ABCDEF",
		Sequence: sequence, Kind: kind, Details: details, OccurredAt: time.Date(2026, 8, 29, 22, 0, 0, 0, time.UTC),
	}
}

func testFailure(code contract.ErrorCode) *contract.Failure {
	return &contract.Failure{SchemaVersion: contract.ErrorSchemaVersion, Code: code, Message: "test failure", Retryable: false}
}

func compileCLISchema(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	path := filepath.Join("..", "..", "schemas", "v1alpha1", name+".schema.json")
	schema, err := jsonschema.NewCompiler().Compile(path)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func validateJSONLine(t *testing.T, schema *jsonschema.Schema, line []byte) {
	t.Helper()
	var document any
	if err := json.Unmarshal(line, &document); err != nil {
		t.Fatalf("invalid JSON %q: %v", line, err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("JSON does not validate: %v\n%s", err, line)
	}
}

func decodeStrictTest(t *testing.T, data []byte, destination any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
}
