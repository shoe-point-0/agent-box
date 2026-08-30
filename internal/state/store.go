// Package state owns Agent Box's local transactional cache.
//
// Rows describing Git, Lima, and Compose are recorded observations. Callers
// must re-observe external resources before mutation; database presence is not
// an ownership proof or evidence that a resource still exists.
package state

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

const (
	CurrentSchemaVersion = 3
	busyTimeoutMillis    = 5_000
	maxOpenConnections   = 8
)

var (
	// ErrSchemaTooNew means the database was written by an incompatible newer binary.
	ErrSchemaTooNew = errors.New("state schema is newer than this binary")

	//go:embed migrations/*.sql
	migrationFiles embed.FS
)

type migration struct {
	version int
	name    string
	sql     string
}

// Store is the owner of one state database and its connection pool.
type Store struct {
	db   *sql.DB
	path string
}

// Open initializes or migrates a state database and verifies its durability
// contract before returning it to callers.
func Open(ctx context.Context, path string) (_ *Store, returnErr error) {
	if err := prepareDatabaseFile(path); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dataSourceName(path))
	if err != nil {
		return nil, fmt.Errorf("open state database: %w", err)
	}
	db.SetMaxOpenConns(maxOpenConnections)
	db.SetMaxIdleConns(maxOpenConnections)

	store := &Store{db: db, path: path}
	defer func() {
		if returnErr != nil {
			_ = db.Close()
		}
	}()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("connect state database: %w", err)
	}
	if err := checkIntegrity(ctx, db); err != nil {
		return nil, err
	}
	if err := migrate(ctx, db); err != nil {
		return nil, err
	}
	if err := verifyConnection(ctx, db); err != nil {
		return nil, err
	}
	if err := restrictDatabaseFiles(path); err != nil {
		return nil, err
	}
	return store, nil
}

// Path returns the canonical path supplied when the Store was opened.
func (s *Store) Path() string { return s.path }

// Close releases the connection pool. Transactions remain caller-bounded by context.
func (s *Store) Close() error { return s.db.Close() }

// View executes fn in a read transaction.
func (s *Store) View(ctx context.Context, fn func(*sql.Tx) error) error {
	return transact(ctx, s.db, &sql.TxOptions{ReadOnly: true}, fn)
}

// Update executes fn in an immediate write transaction and commits before returning.
func (s *Store) Update(ctx context.Context, fn func(*sql.Tx) error) error {
	return transact(ctx, s.db, &sql.TxOptions{Isolation: sql.LevelSerializable}, fn)
}

func transact(ctx context.Context, db *sql.DB, options *sql.TxOptions, fn func(*sql.Tx) error) error {
	if fn == nil {
		return errors.New("state transaction callback is nil")
	}
	tx, err := db.BeginTx(ctx, options)
	if err != nil {
		return fmt.Errorf("begin state transaction: %w", err)
	}
	if err := fn(tx); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return errors.Join(err, fmt.Errorf("rollback state transaction: %w", rollbackErr))
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit state transaction: %w", err)
	}
	return nil
}

func prepareDatabaseFile(path string) error {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("state database path must be a clean absolute path")
	}
	if strings.IndexByte(path, 0) >= 0 {
		return errors.New("state database path contains NUL")
	}

	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	if err := os.Chmod(parent, 0o700); err != nil {
		return fmt.Errorf("restrict state directory: %w", err)
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("state database path is not a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect state database: %w", err)
	}

	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("create state database: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("restrict state database: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close state database: %w", err)
	}
	return nil
}

func dataSourceName(path string) string {
	query := url.Values{}
	query.Set("_busy_timeout", strconv.Itoa(busyTimeoutMillis))
	query.Set("_foreign_keys", "on")
	query.Set("_journal_mode", "wal")
	query.Set("_synchronous", "full")
	query.Set("_txlock", "immediate")
	return (&url.URL{Scheme: "file", Path: path, RawQuery: query.Encode()}).String()
}

func checkIntegrity(ctx context.Context, db *sql.DB) error {
	var result string
	if err := db.QueryRowContext(ctx, "PRAGMA quick_check(1)").Scan(&result); err != nil {
		return fmt.Errorf("check state database integrity: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("check state database integrity: %s", result)
	}
	return nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read state schema version: %w", err)
	}
	if version > CurrentSchemaVersion {
		return fmt.Errorf("%w: database version %d, supported version %d", ErrSchemaTooNew, version, CurrentSchemaVersion)
	}

	for _, item := range migrations {
		if item.version <= version {
			continue
		}
		err := transact(ctx, db, &sql.TxOptions{Isolation: sql.LevelSerializable}, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, item.sql); err != nil {
				return fmt.Errorf("apply state migration %s: %w", item.name, err)
			}
			if _, err := tx.ExecContext(ctx, "PRAGMA user_version = "+strconv.Itoa(item.version)); err != nil {
				return fmt.Errorf("record state migration %s: %w", item.name, err)
			}
			return nil
		})
		if err != nil {
			return err
		}
		version = item.version
	}
	if version != CurrentSchemaVersion {
		return fmt.Errorf("state migrations ended at version %d, expected %d", version, CurrentSchemaVersion)
	}
	return nil
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read state migrations: %w", err)
	}
	migrations := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		separator := strings.IndexByte(entry.Name(), '_')
		if separator < 1 {
			return nil, fmt.Errorf("invalid state migration name %q", entry.Name())
		}
		version, err := strconv.Atoi(entry.Name()[:separator])
		if err != nil || version < 1 {
			return nil, fmt.Errorf("invalid state migration name %q", entry.Name())
		}
		contents, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read state migration %q: %w", entry.Name(), err)
		}
		migrations = append(migrations, migration{version: version, name: entry.Name(), sql: string(contents)})
	}
	for index, item := range migrations {
		expected := index + 1
		if item.version != expected {
			return nil, fmt.Errorf("state migration sequence has version %d, expected %d", item.version, expected)
		}
	}
	if len(migrations) != CurrentSchemaVersion {
		return nil, fmt.Errorf("state migration count is %d, expected %d", len(migrations), CurrentSchemaVersion)
	}
	return migrations, nil
}

func verifyConnection(ctx context.Context, db *sql.DB) error {
	checks := []struct {
		pragma string
		want   string
	}{
		{pragma: "foreign_keys", want: "1"},
		{pragma: "journal_mode", want: "wal"},
		{pragma: "busy_timeout", want: strconv.Itoa(busyTimeoutMillis)},
		{pragma: "synchronous", want: "2"},
	}
	for _, check := range checks {
		var got string
		if err := db.QueryRowContext(ctx, "PRAGMA "+check.pragma).Scan(&got); err != nil {
			return fmt.Errorf("verify state pragma %s: %w", check.pragma, err)
		}
		if !strings.EqualFold(got, check.want) {
			return fmt.Errorf("verify state pragma %s: got %q, want %q", check.pragma, got, check.want)
		}
	}
	return nil
}

func restrictDatabaseFiles(path string) error {
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		info, err := os.Lstat(candidate)
		if errors.Is(err, os.ErrNotExist) && candidate != path {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect state file %q: %w", filepath.Base(candidate), err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("state file %q is not regular", filepath.Base(candidate))
		}
		if err := os.Chmod(candidate, 0o600); err != nil {
			return fmt.Errorf("restrict state file %q: %w", filepath.Base(candidate), err)
		}
	}
	return nil
}
