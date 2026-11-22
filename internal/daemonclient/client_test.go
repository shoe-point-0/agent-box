package daemonclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/daemon"
	"github.com/shoe-point-0/agent-box/internal/daemonclient"
	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/journal"
	"github.com/shoe-point-0/agent-box/internal/removal"
	"github.com/shoe-point-0/agent-box/internal/state"
	"github.com/shoe-point-0/agent-box/internal/testutil"
)

func TestClientNegotiatesAndStreamsRealDaemonOperation(t *testing.T) {
	root := testutil.TempDir(t)
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := state.Open(t.Context(), filepath.Join(root, "state", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close store: %v", err)
		}
	})
	operationJournal, err := journal.New(store)
	if err != nil {
		t.Fatal(err)
	}
	server, err := daemon.Start(daemon.Config{
		RuntimeDir: filepath.Join(root, "runtime"), Journal: operationJournal,
		Writers: daemon.NewWriterGate(),
		List: func(context.Context) (contract.ListOutput, error) {
			return contract.ListOutput{SchemaVersion: contract.ListSchemaVersion, Boxes: []contract.ListBox{}}, nil
		},
		Plan: func(context.Context, daemon.PlanRequest) (contract.PlanOutput, error) {
			return contract.PlanOutput{
				SchemaVersion: contract.PlanOutputSchemaVersion,
				Plan:          contract.Plan{SchemaVersion: contract.PlanSchemaVersion},
				Operations:    []contract.PlanOperation{},
			}, nil
		},
		Run: func(_ context.Context, _ journal.Operation, _ json.RawMessage, emit daemon.Emit) *contract.Failure {
			if err := emit("reconcile.started", map[string]string{"step": "test"}); err != nil {
				return &contract.Failure{Code: contract.CodeInternalInvariant}
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown() error = %v", err)
		}
	}()

	client, err := daemonclient.New(server.SocketPath())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if failure, err := client.Handshake(t.Context()); err != nil || failure != nil {
		t.Fatalf("Handshake() = %#v, %v", failure, err)
	}
	manifest := filepath.Join(root, "workspace", ".abox", "box.yaml")
	workspaceID, err := identity.Workspace(manifest)
	if err != nil {
		t.Fatal(err)
	}
	boxID, err := identity.Box(workspaceID, "TASK-1")
	if err != nil {
		t.Fatal(err)
	}
	planned, failure, err := client.Plan(t.Context(), daemon.PlanRequest{
		WorkspaceID: workspaceID.String(), BoxID: boxID.String(), TaskKey: "TASK-1", ManifestPath: manifest,
	})
	if err != nil || failure != nil || planned.SchemaVersion != contract.PlanOutputSchemaVersion || planned.Operations == nil {
		t.Fatalf("Plan() = %#v, %#v, %v", planned, failure, err)
	}
	var events []daemonclient.Event
	failure, err = client.Up(t.Context(), daemon.MutationRequest{
		WorkspaceID: workspaceID.String(), BoxID: boxID.String(), TaskKey: "TASK-1",
		ManifestPath: manifest, ManifestDigest: "sha256:manifest", Input: json.RawMessage(`{}`),
	}, "client-test-1", func(event daemonclient.Event) error {
		events = append(events, event)
		return nil
	})
	if err != nil || failure != nil {
		t.Fatalf("Up() = %#v, %v", failure, err)
	}
	if len(events) != 2 || events[0].Kind != "reconcile.started" || events[1].Kind != "operation.finished" {
		t.Fatalf("events = %#v", events)
	}
	for index, event := range events {
		if event.Sequence != int64(index+1) || event.OperationID == "" {
			t.Errorf("event %d = %#v", index, event)
		}
	}
}

func TestClientRemovalPreservesPreviewAuthorizationOverRealDaemon(t *testing.T) {
	root := testutil.TempDir(t)
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := state.Open(t.Context(), filepath.Join(root, "state", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	operationJournal, err := journal.New(store)
	if err != nil {
		t.Fatal(err)
	}
	step := &removalTestStep{present: true}
	coordinator, err := removal.New(removal.Config{
		Steps: []removal.Step{step}, Lease: removalTestLease{}, Quiescer: removalTestQuiescer{}, Recorder: removalTestRecorder{},
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := removal.OperationRunner{Assemble: func(context.Context, journal.Operation) (*removal.Coordinator, error) {
		return coordinator, nil
	}}
	server, err := daemon.Start(daemon.Config{
		RuntimeDir: filepath.Join(root, "runtime"), Journal: operationJournal, Writers: daemon.NewWriterGate(),
		List: func(context.Context) (contract.ListOutput, error) {
			return contract.ListOutput{SchemaVersion: contract.ListSchemaVersion, Boxes: []contract.ListBox{}}, nil
		},
		Plan: func(context.Context, daemon.PlanRequest) (contract.PlanOutput, error) {
			return contract.PlanOutput{SchemaVersion: contract.PlanOutputSchemaVersion, Plan: contract.Plan{SchemaVersion: contract.PlanSchemaVersion}, Operations: []contract.PlanOperation{}}, nil
		},
		Run: func(ctx context.Context, operation journal.Operation, input json.RawMessage, emit daemon.Emit) *contract.Failure {
			return runner.Run(ctx, operation, input, func(kind string, details any) error { return emit(kind, details) })
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})
	client, err := daemonclient.New(server.SocketPath())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	manifest := filepath.Join(root, "workspace", ".abox", "box.yaml")
	workspaceID, err := identity.Workspace(manifest)
	if err != nil {
		t.Fatal(err)
	}
	boxID, err := identity.Box(workspaceID, "TASK-REMOVE")
	if err != nil {
		t.Fatal(err)
	}
	request := daemon.MutationRequest{
		WorkspaceID: workspaceID.String(), BoxID: boxID.String(), TaskKey: "TASK-REMOVE", ManifestPath: manifest,
		ManifestDigest: "sha256:manifest", Input: json.RawMessage(`{}`),
	}
	var preview removal.Preview
	failure, err := client.Remove(t.Context(), request, "remove-preview", func(event daemonclient.Event) error {
		if event.Kind == "removal.preview" {
			return json.Unmarshal(event.Details, &preview)
		}
		if event.Kind == "operation.finished" {
			const want = `{"apiVersion":"abox.api/v1alpha1","errorCode":"ABOX_CLEANUP_REFUSED","state":"failed"}`
			if string(event.Details) != want {
				t.Errorf("journaled terminal details = %s, want %s", event.Details, want)
			}
		}
		return nil
	})
	if err != nil || failure == nil || failure.Code != contract.CodeCleanupRefused || preview.Digest == "" || step.removes != 0 {
		t.Fatalf("preview Remove() = failure %#v, error %v, preview %#v, removes %d", failure, err, preview, step.removes)
	}
	authorization, err := json.Marshal(removal.OperationInput{PreviewDigest: preview.Digest, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	request.Input = authorization
	var kinds []string
	failure, err = client.Remove(t.Context(), request, "remove-authorized", func(event daemonclient.Event) error {
		kinds = append(kinds, event.Kind)
		return nil
	})
	if err != nil || failure != nil || step.removes != 1 {
		t.Fatalf("authorized Remove() = failure %#v, error %v, removes %d", failure, err, step.removes)
	}
	if got := fmt.Sprint(kinds); got != "[removal.preview removal.completed operation.finished]" {
		t.Fatalf("authorized removal events = %s", got)
	}
}

type removalTestStep struct {
	present bool
	removes int
}

func (*removalTestStep) Key() string          { return "compose/test" }
func (*removalTestStep) Stage() removal.Stage { return removal.StageCompose }
func (s *removalTestStep) Observe(context.Context) (removal.Fact, error) {
	if !s.present {
		return removal.Fact{Losses: []removal.Loss{}}, nil
	}
	return removal.Fact{Present: true, Evidence: map[string]string{"id": "test-resource"}, Losses: []removal.Loss{}}, nil
}
func (s *removalTestStep) Remove(context.Context, removal.Fact) error {
	s.removes++
	s.present = false
	return nil
}

type removalTestLease struct{}

func (removalTestLease) Acquire(_ context.Context, boxID string) (removal.Lease, error) {
	return removal.Lease{BoxID: boxID, Token: "test-lease"}, nil
}
func (removalTestLease) Release(context.Context, removal.Lease) error { return nil }

type removalTestQuiescer struct{}

func (removalTestQuiescer) Quiesce(context.Context, removal.Lease) error { return nil }
func (removalTestQuiescer) Resume(context.Context, removal.Lease) error  { return nil }

type removalTestRecorder struct{}

func (removalTestRecorder) Record(context.Context, removal.Transition) error { return nil }

func TestClientListsStrictBoundedInventory(t *testing.T) {
	t.Parallel()
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/v1/boxes" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		writeTestJSON(writer, contract.ListOutput{SchemaVersion: contract.ListSchemaVersion, Boxes: []contract.ListBox{
			{ID: "box-a", WorkspaceID: "ws-a", TaskKey: "TASK-A", ManifestPath: "/work/a/.abox/box.yaml", State: "active", RecordedState: "ready", Drift: []string{}},
		}})
	})
	socket, stop := serveUnixHTTP(t, handler)
	defer stop()
	client, err := daemonclient.New(socket)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	output, failure, err := client.List(t.Context())
	if err != nil || failure != nil || len(output.Boxes) != 1 || output.Boxes[0].ID != "box-a" {
		t.Fatalf("List() = %#v, %#v, %v", output, failure, err)
	}
}

func TestClientRejectsInvalidListEnvelope(t *testing.T) {
	t.Parallel()
	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(writer, `{"schemaVersion":"abox.list/v1alpha1","boxes":[],"unknown":true}`)
	})
	socket, stop := serveUnixHTTP(t, handler)
	defer stop()
	client, err := daemonclient.New(socket)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_, _, err = client.List(t.Context())
	if !errors.Is(err, daemonclient.ErrProtocol) {
		t.Fatalf("List() error = %v, want protocol violation", err)
	}
}

func TestClientResumesStreamFromLastDurableSequence(t *testing.T) {
	const operationID = "op_0123456789ABCDEF"
	var eventRequests atomic.Int32
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get(daemon.HeaderAPIMin) != "1" || request.Header.Get(daemon.HeaderAPIMax) != "1" {
			t.Errorf("API headers = %q/%q", request.Header.Get(daemon.HeaderAPIMin), request.Header.Get(daemon.HeaderAPIMax))
		}
		switch request.URL.Path {
		case "/v1/handshake":
			writeTestJSON(writer, map[string]any{"apiVersion": contract.DaemonAPIVersion, "minimum": 1, "maximum": 1})
		case "/v1/boxes:up":
			if request.Header.Get(daemon.HeaderIdempotencyKey) != "resume-key" {
				t.Errorf("idempotency key = %q", request.Header.Get(daemon.HeaderIdempotencyKey))
			}
			writer.Header().Set(daemon.HeaderOperationID, operationID)
			writer.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = fmt.Fprintln(writer, eventLine(operationID, 1, "reconcile.started", `{"step":"one"}`))
		case "/v1/operations/" + operationID + "/events":
			eventRequests.Add(1)
			if request.URL.Query().Get("after") != "1" {
				t.Errorf("resume sequence = %q", request.URL.Query().Get("after"))
			}
			writer.Header().Set(daemon.HeaderOperationID, operationID)
			writer.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = fmt.Fprintln(writer, eventLine(operationID, 2, "operation.finished", `{"apiVersion":"abox.api/v1alpha1","state":"succeeded"}`))
		default:
			http.NotFound(writer, request)
		}
	})
	socket, stop := serveUnixHTTP(t, handler)
	defer stop()
	client, err := daemonclient.New(socket)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	var sequences []int64
	failure, err := client.Up(t.Context(), daemon.MutationRequest{}, "resume-key", func(event daemonclient.Event) error {
		sequences = append(sequences, event.Sequence)
		return nil
	})
	if err != nil || failure != nil {
		t.Fatalf("Up() = %#v, %v", failure, err)
	}
	if fmt.Sprint(sequences) != "[1 2]" || eventRequests.Load() != 1 {
		t.Fatalf("sequences/resumes = %v/%d", sequences, eventRequests.Load())
	}
}

func TestClientRemoveResumesStreamFromLastDurableSequence(t *testing.T) {
	const operationID = "op_0123456789ABCDEF"
	const details = `{"state": "failed", "errorCode": "ABOX_CLEANUP_REFUSED", "apiVersion": "abox.api/v1alpha1"}`
	var eventRequests atomic.Int32
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/boxes:remove":
			if request.Header.Get(daemon.HeaderIdempotencyKey) != "remove-resume-key" {
				t.Errorf("idempotency key = %q", request.Header.Get(daemon.HeaderIdempotencyKey))
			}
			writer.Header().Set(daemon.HeaderOperationID, operationID)
			writer.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = fmt.Fprintln(writer, eventLine(operationID, 1, "removal.preview", `{"digest":"sha256:preview"}`))
		case "/v1/operations/" + operationID + "/events":
			eventRequests.Add(1)
			if request.URL.Query().Get("after") != "1" {
				t.Errorf("resume sequence = %q", request.URL.Query().Get("after"))
			}
			writer.Header().Set(daemon.HeaderOperationID, operationID)
			writer.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = fmt.Fprintln(writer, eventLine(operationID, 2, "operation.finished", details))
		default:
			http.NotFound(writer, request)
		}
	})
	socket, stop := serveUnixHTTP(t, handler)
	defer stop()
	client, err := daemonclient.New(socket)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	var sequences []int64
	failure, err := client.Remove(t.Context(), daemon.MutationRequest{}, "remove-resume-key", func(event daemonclient.Event) error {
		sequences = append(sequences, event.Sequence)
		if event.Sequence == 2 && string(event.Details) != details {
			t.Errorf("resumed terminal details = %s, want original %s", event.Details, details)
		}
		return nil
	})
	want := contract.Failure{SchemaVersion: contract.ErrorSchemaVersion, Code: contract.CodeCleanupRefused, Message: "operation failed"}
	if err != nil || failure == nil || *failure != want {
		t.Fatalf("Remove() = %#v, %v; want %#v, nil", failure, err, want)
	}
	if fmt.Sprint(sequences) != "[1 2]" || eventRequests.Load() != 1 {
		t.Fatalf("sequences/resumes = %v/%d", sequences, eventRequests.Load())
	}
}

func TestClientRejectsUnknownEventFields(t *testing.T) {
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set(daemon.HeaderOperationID, "op_ABCDEF")
		_, _ = fmt.Fprintln(writer, `{"apiVersion":"abox.api/v1alpha1","operationId":"op_ABCDEF","sequence":1,"kind":"operation.finished","details":{"apiVersion":"abox.api/v1alpha1","state":"succeeded"},"occurredAt":"2026-08-29T22:00:00Z","unknown":true}`)
	})
	socket, stop := serveUnixHTTP(t, handler)
	defer stop()
	client, err := daemonclient.New(socket)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_, err = client.Up(t.Context(), daemon.MutationRequest{}, "key", func(daemonclient.Event) error { return nil })
	if !errors.Is(err, daemonclient.ErrProtocol) {
		t.Fatalf("Up() error = %v, want protocol violation", err)
	}
}

func TestClientRejectsSequenceGap(t *testing.T) {
	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set(daemon.HeaderOperationID, "op_ABCDEF")
		_, _ = fmt.Fprintln(writer, eventLine("op_ABCDEF", 2, "operation.finished", `{"apiVersion":"abox.api/v1alpha1","state":"succeeded"}`))
	})
	socket, stop := serveUnixHTTP(t, handler)
	defer stop()
	client, err := daemonclient.New(socket)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	consumed := false
	_, err = client.Up(t.Context(), daemon.MutationRequest{}, "key", func(daemonclient.Event) error {
		consumed = true
		return nil
	})
	if !errors.Is(err, daemonclient.ErrProtocol) || consumed {
		t.Fatalf("Up() error/consumed = %v/%v, want protocol violation before output", err, consumed)
	}
}

func TestClientValidatesTerminalDetailsBeforeOutput(t *testing.T) {
	const v1 = `{"apiVersion":"abox.api/v1alpha1",`
	for _, test := range []struct {
		name, details, trailing, why string
	}{
		{"missing version", `{}`, "", "invalid terminal event"},
		{"bad version", `{"apiVersion":"v2","state":"succeeded"}`, "", "invalid terminal event"},
		{"unknown state", v1 + `"state":"running"}`, "", "terminal event has invalid state"},
		{"successful with code", v1 + `"state":"succeeded","errorCode":"ABOX_CLEANUP_REFUSED"}`, "", "successful terminal event contains an error"},
		{"failed without code", v1 + `"state":"failed"}`, "", "terminal event contains an unknown error"},
		{"failed with unknown code", v1 + `"state":"failed","errorCode":"ABOX_UNKNOWN"}`, "", "terminal event contains an unknown error"},
		{"unknown field", v1 + `"state":"succeeded","extra":true}`, "", "invalid terminal event"},
		{"trailing JSON", v1 + `"state":"succeeded"}`, ` {}`, "decode event: multiple JSON values"},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set(daemon.HeaderOperationID, "op_ABCDEF")
				_, _ = fmt.Fprintln(writer, eventLine("op_ABCDEF", 1, "operation.finished", test.details)+test.trailing)
			})
			socket, stop := serveUnixHTTP(t, handler)
			defer stop()
			client, err := daemonclient.New(socket)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			failure, err := client.Up(t.Context(), daemon.MutationRequest{}, "key", func(daemonclient.Event) error {
				t.Error("consumed invalid terminal event")
				return nil
			})
			if failure != nil || !errors.Is(err, daemonclient.ErrProtocol) || err.Error() != daemonclient.ErrProtocol.Error()+": "+test.why {
				t.Fatalf("Up() = %#v, %v; want protocol violation: %s", failure, err, test.why)
			}
		})
	}
}

func serveUnixHTTP(t *testing.T, handler http.Handler) (string, func()) {
	t.Helper()
	path := filepath.Join(testutil.TempDir(t), "server.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}
	go func() { _ = server.Serve(listener) }()
	return path, func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}
}

func eventLine(operationID string, sequence int64, kind, details string) string {
	return fmt.Sprintf(`{"apiVersion":"%s","operationId":"%s","sequence":%d,"kind":"%s","details":%s,"occurredAt":"2026-08-29T22:00:00Z"}`,
		contract.DaemonAPIVersion, operationID, sequence, kind, details)
}

func writeTestJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}
