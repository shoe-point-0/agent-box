package journal

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/state"
)

func TestBeginReattachesOnlyAnIdenticalRequest(t *testing.T) {
	t.Parallel()
	journal, _ := openTestJournal(t)
	request := testBeginRequest(t, filepath.Join(t.TempDir(), "workspace", ".abox", "box.yaml"), "operation-a")

	first, created, err := journal.Begin(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !created || first.ID != request.OperationID {
		t.Fatalf("first Begin() = (%+v, %t), want newly created %q", first, created, request.OperationID)
	}

	duplicate := request
	duplicate.OperationID = "operation-ignored"
	attached, created, err := journal.Begin(t.Context(), duplicate)
	if err != nil {
		t.Fatal(err)
	}
	if created || attached.ID != first.ID {
		t.Fatalf("duplicate Begin() = (%+v, %t), want attached %q", attached, created, first.ID)
	}

	conflict := request
	conflict.RequestDigest = "sha256:different"
	if _, _, err := journal.Begin(t.Context(), conflict); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting Begin() error = %v, want ErrIdempotencyConflict", err)
	}

	active := request
	active.OperationID = "operation-b"
	active.IdempotencyKey = "request-b"
	active.RequestDigest = "sha256:request-b"
	if _, _, err := journal.Begin(t.Context(), active); !errors.Is(err, ErrOperationActive) {
		t.Fatalf("second active Begin() error = %v, want ErrOperationActive", err)
	}
}

func TestConcurrentProcessesEstablishOneBoxAndOperation(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-process integration test")
	}
	root := t.TempDir()
	databasePath := filepath.Join(root, "state", "state.db")
	manifestPath := filepath.Join(root, "workspace", ".abox", "box.yaml")
	store, err := state.Open(t.Context(), databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	const processCount = 8
	gate := filepath.Join(root, "start")
	commands := make([]*exec.Cmd, 0, processCount)
	resultPaths := make([]string, 0, processCount)
	for index := range processCount {
		resultPath := filepath.Join(root, fmt.Sprintf("result-%d", index))
		command := helperCommand(t, map[string]string{
			"ABOX_JOURNAL_HELPER":    "begin",
			"ABOX_JOURNAL_DATABASE":  databasePath,
			"ABOX_JOURNAL_MANIFEST":  manifestPath,
			"ABOX_JOURNAL_OPERATION": fmt.Sprintf("operation-%d", index),
			"ABOX_JOURNAL_RESULT":    resultPath,
			"ABOX_JOURNAL_GATE":      gate,
		})
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		commands = append(commands, command)
		resultPaths = append(resultPaths, resultPath)
	}
	if err := os.WriteFile(gate, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, command := range commands {
		if output, err := waitCommand(command); err != nil {
			t.Fatalf("journal helper failed: %v\n%s", err, output)
		}
	}

	var operationIDs []string
	for _, resultPath := range resultPaths {
		contents, err := os.ReadFile(resultPath)
		if err != nil {
			t.Fatal(err)
		}
		operationIDs = append(operationIDs, strings.TrimSpace(string(contents)))
	}
	for _, operationID := range operationIDs[1:] {
		if operationID != operationIDs[0] {
			t.Fatalf("process operation IDs = %v, want one identity", operationIDs)
		}
	}

	store, err = state.Open(t.Context(), databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	}()
	var boxes, operations int
	if err := store.View(t.Context(), func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(t.Context(), `SELECT count(*) FROM boxes`).Scan(&boxes); err != nil {
			return err
		}
		return tx.QueryRowContext(t.Context(), `SELECT count(*) FROM operations WHERE recorded_state = 'running'`).Scan(&operations)
	}); err != nil {
		t.Fatal(err)
	}
	if boxes != 1 || operations != 1 {
		t.Fatalf("persisted boxes/active operations = %d/%d, want 1/1", boxes, operations)
	}
}

func TestDeadProcessLeaseExpiresWithoutLiveLeaseTheft(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-process integration test")
	}
	root := t.TempDir()
	databasePath := filepath.Join(root, "state", "state.db")
	manifestPath := filepath.Join(root, "workspace", ".abox", "box.yaml")
	store, err := state.Open(t.Context(), databasePath)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	request := testBeginRequest(t, manifestPath, "operation-a")
	operation, _, err := journal.Begin(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	base := time.Unix(1_000, 0).UTC()
	first := runLeaseHelper(t, root, databasePath, operation.BoxID, operation.ID, "owner-a", base, 10*time.Second)
	if first != "1" {
		t.Fatalf("first lease fence = %q, want 1", first)
	}
	liveContender := runLeaseHelper(t, root, databasePath, operation.BoxID, operation.ID, "owner-b", base.Add(9*time.Second), 10*time.Second)
	if liveContender != "held" {
		t.Fatalf("live contender result = %q, want held", liveContender)
	}
	recovered := runLeaseHelper(t, root, databasePath, operation.BoxID, operation.ID, "owner-b", base.Add(10*time.Second), 10*time.Second)
	if recovered != "2" {
		t.Fatalf("expired lease recovery fence = %q, want 2", recovered)
	}
}

func TestLeaseRenewReleaseAndFenceRejectStaleOwner(t *testing.T) {
	t.Parallel()
	journal, _ := openTestJournal(t)
	request := testBeginRequest(t, filepath.Join(t.TempDir(), "workspace", ".abox", "box.yaml"), "operation-a")
	operation, _, err := journal.Begin(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	ownerA := Owner{InstanceID: "daemon-a", ProcessStartID: "process-a", BootID: "boot-a"}
	ownerB := Owner{InstanceID: "daemon-b", ProcessStartID: "process-b", BootID: "boot-a"}
	now := time.Unix(2_000, 0).UTC()
	leaseA, err := journal.AcquireLease(t.Context(), operation.BoxID, operation.ID, ownerA, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	leaseA, err = journal.RenewLease(t.Context(), leaseA, now.Add(10*time.Second), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.ReleaseLease(t.Context(), leaseA, now.Add(20*time.Second)); err != nil {
		t.Fatal(err)
	}
	leaseB, err := journal.AcquireLease(t.Context(), operation.BoxID, operation.ID, ownerB, now.Add(21*time.Second), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if leaseB.Fence != leaseA.Fence+1 {
		t.Fatalf("successor fence = %d, want %d", leaseB.Fence, leaseA.Fence+1)
	}
	if _, err := journal.RenewLease(t.Context(), leaseA, now.Add(22*time.Second), time.Minute); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale renewal error = %v, want ErrLeaseLost", err)
	}
	if err := journal.ReleaseLease(t.Context(), leaseA, now.Add(22*time.Second)); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale release error = %v, want ErrLeaseLost", err)
	}
}

func TestEventsSurviveRestartInStableConcurrentSequence(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	databasePath := filepath.Join(root, "state", "state.db")
	store, err := state.Open(t.Context(), databasePath)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	request := testBeginRequest(t, filepath.Join(root, "workspace", ".abox", "box.yaml"), "operation-a")
	operation, _, err := journal.Begin(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}

	const eventCount = 32
	var group sync.WaitGroup
	errorsChannel := make(chan error, eventCount)
	for index := range eventCount {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			_, err := journal.AppendEvent(t.Context(), operation.ID, "progress",
				json.RawMessage(fmt.Sprintf(`{"step":%d}`, index)), time.Unix(int64(index+1), 0).UTC())
			errorsChannel <- err
		}(index)
	}
	group.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = state.Open(t.Context(), databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	}()
	journal, err = New(store)
	if err != nil {
		t.Fatal(err)
	}
	events, err := journal.Events(t.Context(), operation.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != eventCount {
		t.Fatalf("event count after restart = %d, want %d", len(events), eventCount)
	}
	for index, event := range events {
		if event.Sequence != int64(index+1) {
			t.Fatalf("event sequence[%d] = %d, want %d", index, event.Sequence, index+1)
		}
	}
}

func TestSubscriberDetachDoesNotCancelOperation(t *testing.T) {
	t.Parallel()
	journal, _ := openTestJournal(t)
	request := testBeginRequest(t, filepath.Join(t.TempDir(), "workspace", ".abox", "box.yaml"), "operation-a")
	operation, _, err := journal.Begin(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}

	subscriberContext, detach := context.WithCancel(t.Context())
	stream := journal.Subscribe(subscriberContext, operation.ID, 0)
	if _, err := journal.AppendEvent(t.Context(), operation.ID, "started", nil, time.Unix(1, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	select {
	case item := <-stream:
		if item.Err != nil || item.Event.Sequence != 1 {
			t.Fatalf("first stream item = %+v", item)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive persisted event")
	}
	detach()
	select {
	case _, open := <-stream:
		if open {
			t.Fatal("subscriber stream remained open after detach")
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not detach")
	}

	if _, err := journal.AppendEvent(t.Context(), operation.ID, "continued", nil, time.Unix(2, 0).UTC()); err != nil {
		t.Fatalf("append after subscriber detach: %v", err)
	}
	if err := journal.Finish(t.Context(), operation.ID, StateSucceeded, "", time.Unix(3, 0).UTC()); err != nil {
		t.Fatalf("finish after subscriber detach: %v", err)
	}
	events, err := journal.Events(t.Context(), operation.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := []int64{events[0].Sequence, events[1].Sequence}; !slices.Equal(got, []int64{1, 2}) {
		t.Fatalf("event sequence after detach = %v, want [1 2]", got)
	}
}

func TestEventDetailsRejectSensitiveFields(t *testing.T) {
	t.Parallel()
	journal, _ := openTestJournal(t)
	request := testBeginRequest(t, filepath.Join(t.TempDir(), "workspace", ".abox", "box.yaml"), "operation-a")
	operation, _, err := journal.Begin(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.AppendEvent(t.Context(), operation.ID, "unsafe",
		json.RawMessage(`{"nested":{"api_key":"do-not-persist"}}`), time.Now()); err == nil {
		t.Fatal("AppendEvent() accepted a sensitive field")
	}
}

func TestCurrentOwnerUsesRandomInstanceAndProcessBootIdentity(t *testing.T) {
	first, err := CurrentOwner()
	if err != nil {
		t.Fatal(err)
	}
	second, err := CurrentOwner()
	if err != nil {
		t.Fatal(err)
	}
	if first.InstanceID == second.InstanceID {
		t.Fatal("daemon instance IDs are not random per incarnation")
	}
	if first.ProcessStartID == "" || first.BootID == "" {
		t.Fatalf("owner identity is incomplete: %+v", first)
	}
	if first.ProcessStartID != second.ProcessStartID || first.BootID != second.BootID {
		t.Fatalf("same process owner evidence changed: first=%+v second=%+v", first, second)
	}
}

func TestProcessHelper(t *testing.T) {
	action := os.Getenv("ABOX_JOURNAL_HELPER")
	if action == "" {
		return
	}
	if gate := os.Getenv("ABOX_JOURNAL_GATE"); gate != "" {
		deadline := time.Now().Add(5 * time.Second)
		for {
			if _, err := os.Stat(gate); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("timed out waiting for process gate")
			}
			time.Sleep(time.Millisecond)
		}
	}
	store, err := state.Open(t.Context(), os.Getenv("ABOX_JOURNAL_DATABASE"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	}()
	journal, err := New(store)
	if err != nil {
		t.Fatal(err)
	}

	var result string
	switch action {
	case "begin":
		request := testBeginRequest(t, os.Getenv("ABOX_JOURNAL_MANIFEST"), os.Getenv("ABOX_JOURNAL_OPERATION"))
		operation, _, err := journal.Begin(t.Context(), request)
		if err != nil {
			t.Fatal(err)
		}
		result = operation.ID
	case "lease":
		nowNanoseconds, err := strconv.ParseInt(os.Getenv("ABOX_JOURNAL_NOW"), 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		ttlNanoseconds, err := strconv.ParseInt(os.Getenv("ABOX_JOURNAL_TTL"), 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		ownerID := os.Getenv("ABOX_JOURNAL_OWNER")
		owner := Owner{InstanceID: ownerID, ProcessStartID: "process-" + ownerID, BootID: "boot-test"}
		lease, err := journal.AcquireLease(t.Context(), os.Getenv("ABOX_JOURNAL_BOX"),
			os.Getenv("ABOX_JOURNAL_OPERATION"), owner, time.Unix(0, nowNanoseconds).UTC(), time.Duration(ttlNanoseconds))
		if errors.Is(err, ErrLeaseHeld) {
			result = "held"
		} else if err != nil {
			t.Fatal(err)
		} else {
			result = strconv.FormatInt(lease.Fence, 10)
		}
	default:
		t.Fatalf("unknown helper action %q", action)
	}
	if err := os.WriteFile(os.Getenv("ABOX_JOURNAL_RESULT"), []byte(result+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func openTestJournal(t *testing.T) (*Journal, *state.Store) {
	t.Helper()
	store, err := state.Open(t.Context(), filepath.Join(t.TempDir(), "state", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	journal, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	return journal, store
}

func testBeginRequest(t *testing.T, manifestPath, operationID string) BeginRequest {
	t.Helper()
	workspaceID, err := identity.Workspace(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	boxID, err := identity.Box(workspaceID, "TASK-1")
	if err != nil {
		t.Fatal(err)
	}
	return BeginRequest{
		WorkspaceID: workspaceID, BoxID: boxID, TaskKey: "TASK-1",
		ManifestPath: manifestPath, ManifestDigest: "sha256:manifest",
		OperationID: operationID, IdempotencyKey: "request-a", Kind: "up",
		RequestDigest: "sha256:request-a", StartedAt: time.Unix(100, 0).UTC(),
	}
}

func helperCommand(t *testing.T, environment map[string]string) *exec.Cmd {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestProcessHelper$")
	command.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	for key, value := range environment {
		command.Env = append(command.Env, key+"="+value)
	}
	return command
}

func waitCommand(command *exec.Cmd) ([]byte, error) {
	err := command.Wait()
	return nil, err
}

func runLeaseHelper(t *testing.T, root, databasePath, boxID, operationID, owner string, now time.Time, ttl time.Duration) string {
	t.Helper()
	resultPath := filepath.Join(root, "lease-"+owner+"-"+strconv.FormatInt(now.UnixNano(), 10))
	command := helperCommand(t, map[string]string{
		"ABOX_JOURNAL_HELPER":    "lease",
		"ABOX_JOURNAL_DATABASE":  databasePath,
		"ABOX_JOURNAL_BOX":       boxID,
		"ABOX_JOURNAL_OPERATION": operationID,
		"ABOX_JOURNAL_OWNER":     owner,
		"ABOX_JOURNAL_NOW":       strconv.FormatInt(now.UnixNano(), 10),
		"ABOX_JOURNAL_TTL":       strconv.FormatInt(int64(ttl), 10),
		"ABOX_JOURNAL_RESULT":    resultPath,
	})
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("lease helper failed: %v\n%s", err, output)
	}
	contents, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(contents))
}
