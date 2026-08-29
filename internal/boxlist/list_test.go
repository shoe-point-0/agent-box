package boxlist

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/state"
)

func TestListDeterministicallySummarizesObservedAndRecordedState(t *testing.T) {
	t.Parallel()
	store := openStore(t)
	seedInventory(t, store)
	service, err := New(store)
	if err != nil {
		t.Fatal(err)
	}

	before, err := store.BoxRecords(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	output, err := service.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	after, err := store.BoxRecords(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("list changed persisted records: before=%#v after=%#v", before, after)
	}
	if output.SchemaVersion != "abox.list/v1alpha1" {
		t.Fatalf("schema version = %q", output.SchemaVersion)
	}
	if len(output.Boxes) != 4 {
		t.Fatalf("Box count = %d, want 4", len(output.Boxes))
	}
	gotStates := make([]string, 0, len(output.Boxes))
	gotIDs := make([]string, 0, len(output.Boxes))
	for _, box := range output.Boxes {
		gotIDs = append(gotIDs, box.ID)
		gotStates = append(gotStates, box.State)
		if box.WorkspaceID == "" || box.TaskKey == "" || box.ManifestPath == "" || box.RecordedState == "" || box.Drift == nil {
			t.Errorf("incomplete Box output: %#v", box)
		}
	}
	if want := []string{"box-active", "box-degraded", "box-stale", "box-stopped"}; !reflect.DeepEqual(gotIDs, want) {
		t.Fatalf("IDs = %v, want %v", gotIDs, want)
	}
	if want := []string{StateActive, StateDegraded, StateStale, StateStopped}; !reflect.DeepEqual(gotStates, want) {
		t.Fatalf("states = %v, want %v", gotStates, want)
	}
	if len(output.Boxes[0].Drift) != 0 {
		t.Fatalf("active drift = %v, want empty", output.Boxes[0].Drift)
	}
	if want := []string{
		"runtime host is running while Compose is stopped",
		"recorded Box state ready differs from observed degraded",
	}; !reflect.DeepEqual(output.Boxes[1].Drift, want) {
		t.Fatalf("degraded drift = %v, want %v", output.Boxes[1].Drift, want)
	}
	if want := []string{
		"runtime host record is missing for a materialized generation",
		"recorded Box state ready differs from observed stale",
	}; !reflect.DeepEqual(output.Boxes[2].Drift, want) {
		t.Fatalf("stale drift = %v, want %v", output.Boxes[2].Drift, want)
	}
}

func TestListEmptyUsesJSONArray(t *testing.T) {
	t.Parallel()
	store := openStore(t)
	service, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	output, err := service.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if output.Boxes == nil || len(output.Boxes) != 0 {
		t.Fatalf("empty Boxes = %#v, want non-nil empty slice", output.Boxes)
	}
}

func openStore(t *testing.T) *state.Store {
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
	return store
}

func seedInventory(t *testing.T, store *state.Store) {
	t.Helper()
	err := store.Update(t.Context(), func(tx *sql.Tx) error {
		statements := []string{
			`INSERT INTO workspaces (id, manifest_path, manifest_digest, created_at, updated_at) VALUES
			 ('ws-active', '/work/active/.abox/box.yaml', 'sha256:active', 1, 1),
			 ('ws-degraded', '/work/degraded/.abox/box.yaml', 'sha256:degraded', 1, 1),
			 ('ws-stale', '/work/stale/.abox/box.yaml', 'sha256:stale', 1, 1),
			 ('ws-stopped', '/work/stopped/.abox/box.yaml', 'sha256:stopped', 1, 1)`,
			`INSERT INTO boxes (id, workspace_id, task_key, recorded_state, current_generation, created_at, updated_at) VALUES
			 ('box-active', 'ws-active', 'TASK-A', 'ready', 1, 1, 1),
			 ('box-degraded', 'ws-degraded', 'TASK-D', 'ready', 1, 1, 1),
			 ('box-stale', 'ws-stale', 'TASK-X', 'ready', 1, 1, 1),
			 ('box-stopped', 'ws-stopped', 'TASK-S', 'stopped', 1, 1, 1)`,
			`INSERT INTO runtime_hosts (box_id, driver, instance_name, config_digest, recorded_state, observed_at) VALUES
			 ('box-active', 'lima', 'box-active', 'sha256:a', 'running', 1),
			 ('box-degraded', 'lima', 'box-degraded', 'sha256:d', 'running', 1),
			 ('box-stopped', 'lima', 'box-stopped', 'sha256:s', 'stopped', 1)`,
			`INSERT INTO compose_projects (box_id, project_name, config_digest, recorded_state, observed_at) VALUES
			 ('box-active', 'box-active', 'sha256:a', 'ready', 1),
			 ('box-degraded', 'box-degraded', 'sha256:d', 'stopped', 1),
			 ('box-stopped', 'box-stopped', 'sha256:s', 'stopped', 1)`,
		}
		for _, statement := range statements {
			if _, err := tx.ExecContext(t.Context(), statement); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
