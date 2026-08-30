package state

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

var expectedTables = []string{
	"box_generations",
	"boxes",
	"compose_projects",
	"leases",
	"operation_events",
	"operations",
	"reconciliation_generated_authority",
	"reconciliation_git_authority",
	"removal_ownership_snapshots",
	"repositories",
	"runtime_hosts",
	"workspaces",
	"worktrees",
}

func TestOpenInitializesV1AndConfiguresEveryConnection(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "state")
	path := filepath.Join(root, "state.db")
	store, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	assertSchema(t, store.db)

	connections := make([]*sql.Conn, 0, maxOpenConnections)
	for range maxOpenConnections {
		connection, err := store.db.Conn(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		connections = append(connections, connection)
		assertPragmas(t, connection)
	}
	for _, connection := range connections {
		if err := connection.Close(); err != nil {
			t.Fatal(err)
		}
	}

	assertMode(t, root, 0o700)
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		assertMode(t, candidate, 0o600)
	}
}

func TestMigrateEverySchemaFixture(t *testing.T) {
	t.Parallel()
	fixtures, err := filepath.Glob("testdata/schema-v*.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) != CurrentSchemaVersion+1 {
		t.Fatalf("fixture count = %d, want %d", len(fixtures), CurrentSchemaVersion+1)
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(filepath.Base(fixture), func(t *testing.T) {
			t.Parallel()
			contents, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "state", "state.db")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(string(contents)); err != nil {
				_ = db.Close()
				t.Fatalf("load fixture: %v", err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}

			store, err := Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := store.Close(); err != nil {
					t.Error(err)
				}
			}()
			assertSchema(t, store.db)
		})
	}
}

func TestBoxIntentByIDReturnsPersistedDetachedOperationIdentity(t *testing.T) {
	store := openTestStore(t)
	if err := store.Update(t.Context(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO workspaces
			(id, manifest_path, manifest_digest, created_at, updated_at)
			VALUES ('workspace-intent', '/workspace/.abox/box.yaml', 'sha256:manifest', 1, 1)`); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO boxes
			(id, workspace_id, task_key, recorded_state, current_generation, created_at, updated_at)
			VALUES ('box-intent', 'workspace-intent', 'TASK-1', 'ready', 3, 1, 1)`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	intent, found, err := store.BoxIntentByID(t.Context(), "box-intent")
	if err != nil {
		t.Fatal(err)
	}
	if !found || intent.ID != "box-intent" || intent.WorkspaceID != "workspace-intent" ||
		intent.TaskKey != "TASK-1" || intent.ManifestPath != "/workspace/.abox/box.yaml" ||
		intent.ManifestDigest != "sha256:manifest" || intent.RecordedState != "ready" || intent.CurrentGeneration != 3 {
		t.Fatalf("BoxIntentByID() = %+v, found=%t", intent, found)
	}
	if _, found, err := store.BoxIntentByID(t.Context(), "missing"); err != nil || found {
		t.Fatalf("missing BoxIntentByID() found=%t error=%v", found, err)
	}
}

func TestOwnershipViolationRollsBackTransaction(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	seedBoxes(t, store)

	err := store.Update(t.Context(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO box_generations
			(box_id, generation, plan_digest, plan_path, recorded_state, created_at)
			VALUES ('box-a', 1, 'sha256:plan', '/plans/a', 'planned', 1)`); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO operation_events
			(box_id, operation_id, sequence, kind, occurred_at)
			VALUES ('box-b', 'operation-a', 1, 'started', 1)`)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
		t.Fatalf("ownership violation error = %v", err)
	}

	var generations int
	if err := store.View(t.Context(), func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT count(*) FROM box_generations WHERE box_id = 'box-a'`).Scan(&generations)
	}); err != nil {
		t.Fatal(err)
	}
	if generations != 0 {
		t.Fatalf("rolled-back generation count = %d, want 0", generations)
	}
}

func TestForeignKeyViolationRollsBackTransaction(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)

	err := store.Update(t.Context(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO workspaces
			(id, manifest_path, manifest_digest, created_at, updated_at)
			VALUES ('workspace-a', '/workspace/a/.abox/box.yaml', 'sha256:manifest', 1, 1)`); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO repositories
			(box_id, name, source_path, git_store_path, base_commit, branch)
			VALUES ('missing-box', 'repo', '/source', '/git/repo.git', 'deadbeef', 'main')`)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
		t.Fatalf("foreign-key violation error = %v", err)
	}

	var workspaces int
	if err := store.View(t.Context(), func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT count(*) FROM workspaces`).Scan(&workspaces)
	}); err != nil {
		t.Fatal(err)
	}
	if workspaces != 0 {
		t.Fatalf("rolled-back workspace count = %d, want 0", workspaces)
	}
}

func TestFailedMigrationIsAtomic(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state", "state.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE workspaces (wrong TEXT); PRAGMA user_version = 0`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := Open(t.Context(), path); err == nil {
		t.Fatal("Open() succeeded with conflicting v0 schema")
	}

	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("schema version after failed migration = %d, want 0", version)
	}
	var boxes int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = 'boxes'`).Scan(&boxes); err != nil {
		t.Fatal(err)
	}
	if boxes != 0 {
		t.Fatal("partial migration left boxes table behind")
	}
}

func TestOpenRejectsNewerSchema(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state", "state.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version = ` + strconv.Itoa(CurrentSchemaVersion+1)); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = Open(t.Context(), path)
	if !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("Open() error = %v, want ErrSchemaTooNew", err)
	}
}

func TestOpenRejectsCorruptionWithoutReplacingInput(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state", "state.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	contents := []byte("not a sqlite database")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Open(t.Context(), path); err == nil {
		t.Fatal("Open() succeeded with corrupt database")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(contents) {
		t.Fatalf("corrupt input was replaced: got %q", got)
	}
}

func TestOpenRejectsUnsafePaths(t *testing.T) {
	t.Parallel()
	for name, path := range map[string]string{
		"relative": "state.db",
		"unclean":  t.TempDir() + string(filepath.Separator) + "state/../state.db",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Open(t.Context(), path); err == nil {
				t.Fatalf("Open(%q) succeeded", path)
			}
		})
	}

	symlinkRoot := t.TempDir()
	target := filepath.Join(symlinkRoot, "target")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(symlinkRoot, "state.db")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(t.Context(), link); err == nil {
		t.Fatal("Open() followed a database symlink")
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "state", "state.db"))
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

func seedBoxes(t *testing.T, store *Store) {
	t.Helper()
	if err := store.Update(t.Context(), func(tx *sql.Tx) error {
		statements := []string{
			`INSERT INTO workspaces (id, manifest_path, manifest_digest, created_at, updated_at)
			 VALUES ('workspace-a', '/workspace/a/.abox/box.yaml', 'sha256:a', 1, 1)`,
			`INSERT INTO workspaces (id, manifest_path, manifest_digest, created_at, updated_at)
			 VALUES ('workspace-b', '/workspace/b/.abox/box.yaml', 'sha256:b', 1, 1)`,
			`INSERT INTO boxes (id, workspace_id, task_key, recorded_state, created_at, updated_at)
			 VALUES ('box-a', 'workspace-a', 'task-a', 'planned', 1, 1)`,
			`INSERT INTO boxes (id, workspace_id, task_key, recorded_state, created_at, updated_at)
			 VALUES ('box-b', 'workspace-b', 'task-b', 'planned', 1, 1)`,
			`INSERT INTO operations (id, box_id, idempotency_key, kind, recorded_state, started_at)
			 VALUES ('operation-a', 'box-a', 'key-a', 'up', 'running', 1)`,
		}
		for _, statement := range statements {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func assertSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != CurrentSchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, CurrentSchemaVersion)
	}

	rows, err := db.Query(`SELECT name FROM sqlite_schema
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Error(err)
		}
	}()
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(tables, expectedTables) {
		t.Fatalf("tables = %v, want %v", tables, expectedTables)
	}
	for _, deferred := range []string{"routes", "port_allocations", "setup_stamps"} {
		if slices.Contains(tables, deferred) {
			t.Fatalf("deferred table %q exists in v1", deferred)
		}
	}
}

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func assertPragmas(t *testing.T, connection queryRower) {
	t.Helper()
	for pragma, want := range map[string]string{
		"foreign_keys": "1",
		"journal_mode": "wal",
		"busy_timeout": strconv.Itoa(busyTimeoutMillis),
		"synchronous":  "2",
	} {
		var got string
		if err := connection.QueryRowContext(t.Context(), "PRAGMA "+pragma).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if !strings.EqualFold(got, want) {
			t.Fatalf("PRAGMA %s = %q, want %q", pragma, got, want)
		}
	}
}

func assertMode(t *testing.T, path string, want fs.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode of %s = %04o, want %04o", filepath.Base(path), got, want)
	}
}
