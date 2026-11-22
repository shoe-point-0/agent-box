package state

import (
	"strings"
	"testing"
	"time"
)

func TestReconciliationAuthorityWriteEligibility(t *testing.T) {
	publication := readyGenerationFixture(t)
	boxID := publication.Plan.BoxID
	const (
		wrongOperation  = "running up operation is required"
		wrongGeneration = "generation is not current or next"
	)
	for _, tc := range []struct {
		name            string
		current, target int64
		kind, state     string
		otherBox        bool
		wantErr         string
	}{
		{"current generation", 1, 1, "up", "running", false, ""},
		{"next generation", 1, 2, "up", "running", false, ""},
		{"stale generation", 2, 1, "up", "running", false, wrongGeneration},
		{"skipped generation", 1, 3, "up", "running", false, wrongGeneration},
		{"finished up", 1, 1, "up", "failed", false, wrongOperation},
		{"running remove", 1, 1, "remove", "running", false, wrongOperation},
		{"another Box", 1, 1, "up", "running", true, "Box operation is absent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			store := openTestStore(t)
			seedReadyBox(t, store, publication.Plan)
			seedRunningUpOperation(t, store, boxID, "original")
			writers := recoveryWriters(t, store, publication)
			originalTime := time.Unix(1, 0)
			for _, writer := range writers {
				if err := writer.save("original", 1, originalTime); err != nil {
					t.Fatalf("seed %s authority: %v", writer.name, err)
				}
			}
			finishOperation(t, store, "original")
			if _, err := store.db.ExecContext(ctx, `UPDATE boxes SET current_generation = ? WHERE id = ?`, tc.current, boxID); err != nil {
				t.Fatal(err)
			}
			operationBox := boxID
			if tc.otherBox {
				operationBox = "other-box"
				if _, err := store.db.ExecContext(ctx, `INSERT INTO boxes
					(id, workspace_id, task_key, recorded_state, created_at, updated_at)
					VALUES (?, ?, 'OTHER', 'planning', 1, 1)`, operationBox, publication.Plan.WorkspaceID); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := store.db.ExecContext(ctx, `INSERT INTO operations
				(id, box_id, idempotency_key, kind, recorded_state, started_at)
				VALUES ('candidate', ?, 'candidate', ?, ?, 2)`, operationBox, tc.kind, tc.state); err != nil {
				t.Fatal(err)
			}

			for _, writer := range writers {
				t.Run(writer.name, func(t *testing.T) {
					table := "reconciliation_" + strings.ToLower(writer.name) + "_authority"
					err := writer.save("candidate", tc.target, time.Unix(3, 0))
					if tc.wantErr == "" {
						if err != nil {
							t.Fatal(err)
						}
						var op string
						if err := store.db.QueryRowContext(ctx, `SELECT operation_id FROM `+table+` WHERE box_id = ? AND generation = ?`, boxID, tc.target).Scan(&op); err != nil {
							t.Fatal(err)
						}
						if op != "candidate" {
							t.Fatalf("saved operation = %q, want candidate", op)
						}
						return
					}
					want := "save reconciliation " + writer.name + " authority: " + tc.wantErr
					if err == nil || err.Error() != want {
						t.Fatalf("save error = %v, want %q", err, want)
					}
					// A rejected retry must neither replace the old claim nor add a new one.
					var rows, original int
					query := `SELECT count(*), count(*) FILTER (WHERE generation = 1 AND operation_id = 'original' AND updated_at = ?)
						FROM ` + table + ` WHERE box_id = ?`
					if err := store.db.QueryRowContext(ctx, query, originalTime.UnixNano(), boxID).Scan(&rows, &original); err != nil {
						t.Fatal(err)
					}
					if rows != 1 || original != 1 {
						t.Fatalf("after rejected write: rows = %d, original claims = %d; want one original claim", rows, original)
					}
				})
			}
		})
	}
}

type recoveryWriter struct {
	name string
	save func(operationID string, generation int64, at time.Time) error
}

func recoveryWriters(t *testing.T, store *Store, publication ReadyGeneration) []recoveryWriter {
	t.Helper()
	ctx := t.Context()
	return []recoveryWriter{
		{"generated", func(op string, gen int64, at time.Time) error {
			return store.SaveReconciliationGeneratedAuthority(ctx, ReconciliationGeneratedAuthority{
				BoxID: publication.Plan.BoxID, Generation: gen, PlanDigest: publication.Plan.PlanDigest, OperationID: op,
			}, at)
		}},
		{"Git", func(op string, gen int64, at time.Time) error {
			a := gitAuthorityFixture(publication, op)
			a.Generation = gen
			return store.SaveReconciliationGitAuthority(ctx, a, at)
		}},
		{"Lima", func(op string, gen int64, at time.Time) error {
			a := limaAuthorityFixture(t, publication, op)
			a.Generation, a.Spec.Generation = gen, gen
			return store.SaveReconciliationLimaAuthority(ctx, a, at)
		}},
		{"Compose", func(op string, gen int64, at time.Time) error {
			a := composeAuthorityFixture(publication, op)
			a.Generation, a.Spec.Generation, a.Spec.RuntimeHost.Generation = gen, gen, gen
			return store.SaveReconciliationComposeAuthority(ctx, a, at)
		}},
	}
}
