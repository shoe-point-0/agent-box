package journal

import (
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/shoe-point-0/agent-box/internal/contract"
)

func TestCompleteRollsBackEitherWriteAndCanRetry(t *testing.T) {
	for _, tc := range []struct {
		name, trigger string
		outcome       *contract.Failure
	}{
		{"state update", `BEFORE UPDATE OF recorded_state ON operations`, nil},
		{"event insert", `BEFORE INSERT ON operation_events`, &contract.Failure{Code: contract.CodeCleanupRefused}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			j, store := openTestJournal(t)
			request := testBeginRequest(t, filepath.Join(t.TempDir(), "box.yaml"), "operation-a")
			op, _, err := j.Begin(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := j.AppendEvent(ctx, op.ID, "progress", nil, op.StartedAt); err != nil {
				t.Fatal(err)
			}
			exec := func(query string) {
				t.Helper()
				if err := store.Update(ctx, func(tx *sql.Tx) error {
					_, err := tx.ExecContext(ctx, query)
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
			exec(`CREATE TRIGGER fail_write ` + tc.trigger + ` BEGIN SELECT RAISE(ABORT, 'injected failure'); END`)
			result := contract.NewOperationResult(tc.outcome)
			finish := op.StartedAt.Add(time.Second)
			if err := j.Complete(ctx, op.ID, result, finish); err == nil {
				t.Fatal("completion succeeded despite injected failure")
			}
			stored, err := j.Operation(ctx, op.ID)
			if err != nil || stored.State != StateRunning || stored.ErrorCode != "" || stored.FinishedAt != nil {
				t.Fatalf("operation after rollback = %+v, %v", stored, err)
			}
			events, err := j.Events(ctx, op.ID, 0)
			if err != nil || len(events) != 1 || events[0].Kind != "progress" || events[0].Sequence != 1 {
				t.Fatalf("events after rollback = %+v, %v", events, err)
			}

			exec(`DROP TRIGGER fail_write`)
			if err := j.Complete(ctx, op.ID, result, finish); err != nil {
				t.Fatal(err)
			}
			if err := j.Complete(ctx, op.ID, result, finish.Add(time.Second)); err == nil {
				t.Fatal("completed operation accepted a second completion")
			}
			if _, err := j.AppendEvent(ctx, op.ID, "late progress", nil, finish); err == nil {
				t.Fatal("completed operation accepted a subsequent event")
			}
			stored, err = j.Operation(ctx, op.ID)
			if err != nil || stored.State != result.State || stored.ErrorCode != string(result.ErrorCode) ||
				stored.FinishedAt == nil || !stored.FinishedAt.Equal(finish) {
				t.Fatalf("completed operation = %+v, %v", stored, err)
			}
			events, err = j.Events(ctx, op.ID, 1)
			if err != nil || len(events) != 1 || events[0].Kind != contract.OperationFinished ||
				events[0].Sequence != 2 || !events[0].OccurredAt.Equal(finish) {
				t.Fatalf("completion events = %+v, %v", events, err)
			}
			var got contract.OperationResult
			if err := json.Unmarshal(events[0].Details, &got); err != nil || got != result {
				t.Fatalf("terminal details = %+v, %v; want %+v", got, err, result)
			}
		})
	}
}

func TestOnlyValidCompletionCanPublishTerminalEvent(t *testing.T) {
	ctx := t.Context()
	j, _ := openTestJournal(t)
	request := testBeginRequest(t, filepath.Join(t.TempDir(), "box.yaml"), "operation-a")
	op, _, err := j.Begin(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	result := contract.NewOperationResult(nil)
	if _, err := j.AppendEvent(ctx, op.ID, contract.OperationFinished, json.RawMessage(`{"state":"succeeded"}`), op.StartedAt); err == nil {
		t.Fatal("AppendEvent accepted the reserved terminal event")
	}
	if err := j.Complete(ctx, "missing", result, op.StartedAt); !errors.Is(err, ErrNotFound) {
		t.Fatalf("complete unknown operation = %v; want ErrNotFound", err)
	}
	result.ErrorCode = contract.CodeInternalInvariant
	if err := j.Complete(ctx, op.ID, result, op.StartedAt); err == nil {
		t.Fatal("invalid terminal result was accepted")
	}
	if events, err := j.Events(ctx, op.ID, 0); err != nil || len(events) != 0 {
		t.Fatalf("events after invalid completion = %+v, %v", events, err)
	}
	if got, err := j.Operation(ctx, op.ID); err != nil || got.State != StateRunning {
		t.Fatalf("operation after invalid completion = %+v, %v", got, err)
	}
}
