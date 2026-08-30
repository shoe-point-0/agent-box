//go:build linux

package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/journal"
	"github.com/shoe-point-0/agent-box/internal/state"
)

func TestUnixServerSecurityHandshakeAndLifecycle(t *testing.T) {
	server, client, runtimeDir := startTestServer(t, func(_ context.Context, _ journal.Operation, _ json.RawMessage, _ Emit) *contract.Failure {
		return nil
	})

	assertPath(t, runtimeDir, os.ModeDir, 0o700)
	assertPath(t, server.SocketPath(), os.ModeSocket, 0o600)
	response := doRequest(t, client, http.MethodGet, "/v1/handshake", "", "", true)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("handshake status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	var handshake struct {
		APIVersion string `json:"apiVersion"`
		Minimum    int    `json:"minimum"`
		Maximum    int    `json:"maximum"`
	}
	decodeJSON(t, response.Body, &handshake)
	if handshake.APIVersion != contract.DaemonAPIVersion || handshake.Minimum != 1 || handshake.Maximum != 1 {
		t.Fatalf("handshake = %+v", handshake)
	}
	response = doRequest(t, client, http.MethodGet, "/v1/boxes", "", "", true)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	var inventory contract.ListOutput
	decodeJSON(t, response.Body, &inventory)
	if inventory.SchemaVersion != contract.ListSchemaVersion || inventory.Boxes == nil || len(inventory.Boxes) != 0 {
		t.Fatalf("empty list = %#v", inventory)
	}

	response = doRequest(t, client, http.MethodGet, "/v1/handshake", "", "", false)
	if response.StatusCode != http.StatusUpgradeRequired {
		t.Fatalf("incompatible handshake status = %d, want %d", response.StatusCode, http.StatusUpgradeRequired)
	}
	var failure contract.Failure
	decodeJSON(t, response.Body, &failure)
	if failure.Code != contract.CodeAPIIncompatible || failure.ExitCode() != contract.ExitCompatibility {
		t.Fatalf("incompatible failure = %+v", failure)
	}

	store, operationJournal := openJournal(t)
	defer func() { _ = store.Close() }()
	if _, err := Start(Config{RuntimeDir: runtimeDir, Journal: operationJournal, Writers: NewWriterGate(), Run: func(context.Context, journal.Operation, json.RawMessage, Emit) *contract.Failure { return nil }, List: emptyList, Plan: emptyPlan}); err == nil {
		t.Fatal("second Start() acquired the daemon runtime")
	}

	shutdownServer(t, server)
	if _, err := os.Lstat(server.SocketPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket after shutdown error = %v, want not exist", err)
	}
	if _, err := os.Stat(filepath.Join(runtimeDir, "aboxd.lock")); err != nil {
		t.Fatalf("persistent lock path: %v", err)
	}
	if err := server.Shutdown(t.Context()); err != nil {
		t.Fatalf("idempotent Shutdown(): %v", err)
	}
}

func TestRuntimeRefusesSymlinksAndStaleSocketPaths(t *testing.T) {
	store, operationJournal := openJournal(t)
	defer func() { _ = store.Close() }()
	runner := func(context.Context, journal.Operation, json.RawMessage, Emit) *contract.Failure { return nil }

	parent := t.TempDir()
	if err := os.Chmod(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(parent, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(parent, "runtime-link")
	if err := os.Symlink(target, symlink); err != nil {
		t.Fatal(err)
	}
	if _, err := Start(Config{RuntimeDir: symlink, Journal: operationJournal, Writers: NewWriterGate(), Run: runner, List: emptyList, Plan: emptyPlan}); err == nil {
		t.Fatal("Start() accepted a symlink runtime directory")
	}

	runtimeDir := filepath.Join(parent, "runtime")
	if err := os.Mkdir(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	stale := runtimeSocketPath(runtimeDir)
	if err := os.WriteFile(stale, []byte("not a socket"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Start(Config{RuntimeDir: runtimeDir, Journal: operationJournal, Writers: NewWriterGate(), Run: runner, List: emptyList, Plan: emptyPlan}); err == nil || !strings.Contains(err.Error(), "refusing stale or active endpoint") {
		t.Fatalf("Start() stale-path error = %v", err)
	}
	contents, err := os.ReadFile(stale)
	if err != nil || string(contents) != "not a socket" {
		t.Fatalf("stale path was mutated: contents=%q error=%v", contents, err)
	}
}

func TestPeerListenerRejectsMismatchedKernelUID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peer.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	listener.SetUnlinkOnClose(true)
	wrapped := newPeerListener(listener, uint32(os.Geteuid()+1))
	acceptDone := make(chan error, 1)
	go func() {
		_, err := wrapped.Accept()
		acceptDone <- err
	}()

	connection, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	_ = connection.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := bufio.NewReader(connection).ReadByte(); !errors.Is(err, io.EOF) {
		t.Fatalf("mismatched peer read error = %v, want EOF", err)
	}
	_ = connection.Close()
	_ = wrapped.Close()
	select {
	case err := <-acceptDone:
		if err == nil {
			t.Fatal("Accept() returned an unauthorized connection")
		}
	case <-time.After(time.Second):
		t.Fatal("peer listener did not stop")
	}
}

func TestMutationStartsOnceReattachesAndResumesNDJSON(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var executions atomic.Int32
	runner := func(ctx context.Context, _ journal.Operation, input json.RawMessage, emit Emit) *contract.Failure {
		executions.Add(1)
		if err := emit("operation.started", map[string]any{"phase": "apply"}); err != nil {
			return &contract.Failure{Code: contract.CodeInternalInvariant}
		}
		close(started)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return &contract.Failure{Code: contract.CodeInfrastructureApplyFailed}
		}
	}
	server, client, _ := startTestServer(t, runner)
	defer shutdownServer(t, server)
	body := testMutationBody(t, json.RawMessage(`{"generation":1}`))

	first := doRequest(t, client, http.MethodPost, "/v1/boxes:up", body, "request-1", true)
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first mutation status = %d", first.StatusCode)
	}
	firstOperationID := first.Header.Get(HeaderOperationID)
	firstDecoder := json.NewDecoder(first.Body)
	firstStarted := decodeEvent(t, firstDecoder)
	if firstStarted.APIVersion != contract.DaemonAPIVersion || firstStarted.Kind != "operation.started" || firstStarted.Sequence != 1 {
		t.Fatalf("first event = %+v", firstStarted)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("operation did not start")
	}

	attached := doRequest(t, client, http.MethodPost, "/v1/boxes:up", body, "request-1", true)
	if attached.StatusCode != http.StatusOK || attached.Header.Get(HeaderOperationID) != firstOperationID {
		t.Fatalf("attached response status/operation = %d/%q, want 200/%q", attached.StatusCode, attached.Header.Get(HeaderOperationID), firstOperationID)
	}
	attachedDecoder := json.NewDecoder(attached.Body)
	attachedStarted := decodeEvent(t, attachedDecoder)
	if attachedStarted.Sequence != firstStarted.Sequence || attachedStarted.OperationID != firstOperationID {
		t.Fatalf("attached first event = %+v", attachedStarted)
	}
	if executions.Load() != 1 {
		t.Fatalf("operation executions = %d, want 1", executions.Load())
	}

	close(release)
	firstFinished := decodeEvent(t, firstDecoder)
	attachedFinished := decodeEvent(t, attachedDecoder)
	if firstFinished.Kind != "operation.finished" || attachedFinished.Sequence != firstFinished.Sequence {
		t.Fatalf("terminal events = %+v / %+v", firstFinished, attachedFinished)
	}
	var result operationResult
	if err := json.Unmarshal(firstFinished.Details, &result); err != nil {
		t.Fatal(err)
	}
	if result.APIVersion != contract.DaemonAPIVersion || result.State != journal.StateSucceeded || result.ErrorCode != "" {
		t.Fatalf("terminal result = %+v", result)
	}
	assertStreamEOF(t, firstDecoder)
	assertStreamEOF(t, attachedDecoder)
	_ = first.Body.Close()
	_ = attached.Body.Close()

	resumed := doRequest(t, client, http.MethodGet, "/v1/operations/"+firstOperationID+"/events?after=1", "", "", true)
	if resumed.StatusCode != http.StatusOK {
		t.Fatalf("resume status = %d", resumed.StatusCode)
	}
	resumeDecoder := json.NewDecoder(resumed.Body)
	resumeEvent := decodeEvent(t, resumeDecoder)
	if resumeEvent.Sequence != firstFinished.Sequence || resumeEvent.Kind != "operation.finished" {
		t.Fatalf("resumed event = %+v", resumeEvent)
	}
	assertStreamEOF(t, resumeDecoder)
	_ = resumed.Body.Close()

	conflictingBody := mutationWithInput(t, body, json.RawMessage(`{"generation":2}`))
	conflict := doRequest(t, client, http.MethodPost, "/v1/boxes:up", conflictingBody, "request-1", true)
	if conflict.StatusCode != http.StatusConflict {
		t.Fatalf("idempotency conflict status = %d, want %d", conflict.StatusCode, http.StatusConflict)
	}
	var failure contract.Failure
	decodeJSON(t, conflict.Body, &failure)
	if failure.Code != contract.CodeOperationConflict {
		t.Fatalf("idempotency failure = %+v", failure)
	}
}

func TestMutationDigestPreservesLargeJSONIntegerIdentity(t *testing.T) {
	base := MutationRequest{
		WorkspaceID: "ws_aaaaaaaa", BoxID: "task-aaaaaaaa", TaskKey: "TASK",
		ManifestPath: "/workspace/.abox/box.yaml", ManifestDigest: "sha256:manifest",
	}
	first := base
	first.Input = json.RawMessage(`{"value":9007199254740992}`)
	second := base
	second.Input = json.RawMessage(`{"value":9007199254740993}`)
	firstDigest, err := mutationDigest(first)
	if err != nil {
		t.Fatal(err)
	}
	secondDigest, err := mutationDigest(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstDigest == secondDigest {
		t.Fatalf("large integer request digests collided: %q", firstDigest)
	}
}

func TestRestartedDaemonRefusesUnownedRunningOperation(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	store, operationJournal := openJournalAt(t, filepath.Join(root, "state", "state.db"))
	defer func() { _ = store.Close() }()
	body := testMutationBody(t, json.RawMessage(`{"generation":1}`))
	var mutation MutationRequest
	if err := json.Unmarshal([]byte(body), &mutation); err != nil {
		t.Fatal(err)
	}
	digest, err := mutationDigest(mutation)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = operationJournal.Begin(t.Context(), journal.BeginRequest{
		WorkspaceID: identity.WorkspaceID(mutation.WorkspaceID), BoxID: identity.BoxID(mutation.BoxID),
		TaskKey: mutation.TaskKey, ManifestPath: mutation.ManifestPath, ManifestDigest: mutation.ManifestDigest,
		OperationID: "operation-from-dead-daemon", IdempotencyKey: "restart-1", Kind: "up",
		RequestDigest: digest, StartedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var executions atomic.Int32
	server, err := Start(Config{
		RuntimeDir: filepath.Join(root, "runtime"), Journal: operationJournal,
		Writers: NewWriterGate(),
		List:    emptyList, Plan: emptyPlan,
		Run: func(context.Context, journal.Operation, json.RawMessage, Emit) *contract.Failure {
			executions.Add(1)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer shutdownServer(t, server)
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", server.SocketPath())
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	response := doRequest(t, client, http.MethodPost, "/v1/boxes:up", body, "restart-1", true)
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("orphan attachment status = %d, want %d", response.StatusCode, http.StatusConflict)
	}
	var failure contract.Failure
	decodeJSON(t, response.Body, &failure)
	if failure.Code != contract.CodeOperationConflict || !failure.Retryable || executions.Load() != 0 {
		t.Fatalf("orphan attachment failure/executions = %+v/%d", failure, executions.Load())
	}
}

func TestClientDetachDoesNotCancelOperationAndShutdownDoes(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	runner := func(ctx context.Context, _ journal.Operation, _ json.RawMessage, emit Emit) *contract.Failure {
		if err := emit("operation.started", map[string]any{"phase": "wait"}); err != nil {
			return &contract.Failure{Code: contract.CodeInternalInvariant}
		}
		close(started)
		<-ctx.Done()
		close(canceled)
		return &contract.Failure{Code: contract.CodeInfrastructureApplyFailed}
	}
	server, client, _ := startTestServer(t, runner)
	requestContext, detach := context.WithCancel(t.Context())
	request := newRequest(t, requestContext, http.MethodPost, "/v1/boxes:up", testMutationBody(t, json.RawMessage(`{}`)), "detach-1", true)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(response.Body)
	_ = decodeEvent(t, decoder)
	<-started
	detach()
	_ = response.Body.Close()
	select {
	case <-canceled:
		t.Fatal("client detach canceled daemon-owned operation")
	case <-time.After(75 * time.Millisecond):
	}

	shutdownServer(t, server)
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("daemon shutdown did not cancel active operation")
	}
}

func TestShutdownTimeoutRetainsLockUntilRetryCompletes(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	runner := func(_ context.Context, _ journal.Operation, _ json.RawMessage, emit Emit) *contract.Failure {
		if err := emit("operation.started", map[string]any{"phase": "uninterruptible"}); err != nil {
			return &contract.Failure{Code: contract.CodeInternalInvariant}
		}
		close(started)
		<-release
		return nil
	}
	server, client, runtimeDir := startTestServer(t, runner)
	response := doRequest(t, client, http.MethodPost, "/v1/boxes:up", testMutationBody(t, json.RawMessage(`{}`)), "timeout-1", true)
	decoder := json.NewDecoder(response.Body)
	_ = decodeEvent(t, decoder)
	<-started

	shortContext, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := server.Shutdown(shortContext); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timed-out Shutdown() error = %v, want deadline exceeded", err)
	}
	assertPath(t, server.SocketPath(), os.ModeSocket, 0o600)
	store, operationJournal := openJournal(t)
	defer func() { _ = store.Close() }()
	if _, err := Start(Config{RuntimeDir: runtimeDir, Journal: operationJournal, Writers: NewWriterGate(), Run: runner, List: emptyList, Plan: emptyPlan}); err == nil {
		t.Fatal("second daemon acquired lock after incomplete shutdown")
	}

	close(release)
	_ = decodeEvent(t, decoder)
	assertStreamEOF(t, decoder)
	_ = response.Body.Close()
	shutdownServer(t, server)
	if _, err := os.Lstat(server.SocketPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket after shutdown retry error = %v, want not exist", err)
	}
}

func emptyList(context.Context) (contract.ListOutput, error) {
	return contract.ListOutput{SchemaVersion: contract.ListSchemaVersion, Boxes: []contract.ListBox{}}, nil
}

func emptyPlan(context.Context, PlanRequest) (contract.PlanOutput, error) {
	return contract.PlanOutput{
		SchemaVersion: contract.PlanOutputSchemaVersion,
		Plan:          contract.Plan{SchemaVersion: contract.PlanSchemaVersion},
		Operations:    []contract.PlanOperation{},
	}, nil
}

func startTestServer(t *testing.T, runner Runner) (*Server, *http.Client, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	runtimeDir := filepath.Join(root, "runtime")
	store, operationJournal := openJournalAt(t, filepath.Join(root, "state", "state.db"))
	server, err := Start(Config{RuntimeDir: runtimeDir, Journal: operationJournal, Writers: NewWriterGate(), Run: runner, List: emptyList, Plan: emptyPlan})
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		shutdownServer(t, server)
		if err := store.Close(); err != nil {
			t.Errorf("close state store: %v", err)
		}
	})
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", server.SocketPath())
		},
	}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	t.Cleanup(transport.CloseIdleConnections)
	return server, client, runtimeDir
}

func openJournal(t *testing.T) (*state.Store, *journal.Journal) {
	t.Helper()
	return openJournalAt(t, filepath.Join(t.TempDir(), "state", "state.db"))
}

func openJournalAt(t *testing.T, path string) (*state.Store, *journal.Journal) {
	t.Helper()
	store, err := state.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	operationJournal, err := journal.New(store)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	return store, operationJournal
}

func testMutationBody(t *testing.T, input json.RawMessage) string {
	t.Helper()
	manifestPath := filepath.Join(t.TempDir(), "workspace", ".abox", "box.yaml")
	workspaceID, err := identity.Workspace(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	boxID, err := identity.Box(workspaceID, "TASK-1")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(MutationRequest{
		WorkspaceID: workspaceID.String(), BoxID: boxID.String(), TaskKey: "TASK-1",
		ManifestPath: manifestPath, ManifestDigest: "sha256:manifest", Input: input,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func mutationWithInput(t *testing.T, body string, input json.RawMessage) string {
	t.Helper()
	var request MutationRequest
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		t.Fatal(err)
	}
	request.Input = input
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func doRequest(t *testing.T, client *http.Client, method, path, body, key string, compatible bool) *http.Response {
	t.Helper()
	request := newRequest(t, t.Context(), method, path, body, key, compatible)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func newRequest(t *testing.T, ctx context.Context, method, path, body, key string, compatible bool) *http.Request {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, method, "http://unix"+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if compatible {
		request.Header.Set(HeaderAPIMin, "1")
		request.Header.Set(HeaderAPIMax, "1")
	} else {
		request.Header.Set(HeaderAPIMin, "2")
		request.Header.Set(HeaderAPIMax, "3")
	}
	if key != "" {
		request.Header.Set(HeaderIdempotencyKey, key)
	}
	return request
}

func decodeEvent(t *testing.T, decoder *json.Decoder) streamEvent {
	t.Helper()
	var event streamEvent
	if err := decoder.Decode(&event); err != nil {
		t.Fatal(err)
	}
	return event
}

func assertStreamEOF(t *testing.T, decoder *json.Decoder) {
	t.Helper()
	var event streamEvent
	if err := decoder.Decode(&event); !errors.Is(err, io.EOF) {
		t.Fatalf("stream terminal decode error = %v, want EOF; event=%+v", err, event)
	}
}

func decodeJSON(t *testing.T, body io.ReadCloser, value any) {
	t.Helper()
	defer func() { _ = body.Close() }()
	if err := json.NewDecoder(body).Decode(value); err != nil {
		t.Fatal(err)
	}
}

func assertPath(t *testing.T, path string, kind os.FileMode, permissions os.FileMode) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Type() != kind || info.Mode().Perm() != permissions {
		t.Fatalf("path %q mode = %v, want type %v permissions %04o", path, info.Mode(), kind, permissions)
	}
	status, ok := info.Sys().(*syscall.Stat_t)
	if !ok || status.Uid != uint32(os.Geteuid()) {
		t.Fatalf("path %q UID is not daemon UID %d", path, os.Geteuid())
	}
}

func shutdownServer(t *testing.T, server *Server) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown(): %v", err)
	}
}
