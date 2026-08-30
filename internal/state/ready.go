package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/gitstore"
	"github.com/shoe-point-0/agent-box/internal/plan"
	"github.com/shoe-point-0/agent-box/internal/repository"
)

// ReadyRepository is the freshly observed repository state committed with a
// READY generation. Facts and Store must exactly match RemovalSnapshot.
type ReadyRepository struct {
	Facts         repository.Facts
	Store         gitstore.Record
	CurrentCommit string
	Dirty         bool
}

// ReadyGeneration is the complete durable publication payload for one live Box
// generation. External resources and generated files must already have passed
// their independent live readiness checks before this transaction is called.
type ReadyGeneration struct {
	Plan                contract.Plan
	Generation          int64
	PlanPath            string
	ContextPath         string
	ContextDigest       contract.Digest
	RuntimeConfigDigest contract.Digest
	Repositories        []ReadyRepository
	RemovalSnapshot     RemovalSnapshot
	ObservedAt          time.Time
	PublishedAt         time.Time
}

// PublishReady atomically commits the current generation, cached live component
// observations, context identity, complete removal authority, and READY state.
// Repeating the exact publication is idempotent; generations cannot be skipped,
// replaced, or rebound to different immutable evidence.
func (s *Store) PublishReady(ctx context.Context, publication ReadyGeneration) error {
	snapshot, encodedSnapshot, err := prepareReadyGeneration(publication)
	if err != nil {
		return err
	}
	publication.RemovalSnapshot = snapshot

	return s.Update(ctx, func(tx *sql.Tx) error {
		current, err := verifyReadyBox(ctx, tx, publication)
		if err != nil {
			return err
		}
		if publication.Generation != current && publication.Generation != current+1 {
			return errors.New("publish READY generation: generation is not current or next")
		}
		if current == 0 && publication.Generation != 1 {
			return errors.New("publish READY generation: first generation must be one")
		}

		if _, err := tx.ExecContext(ctx, `INSERT INTO box_generations
			(box_id, generation, plan_digest, plan_path, context_path, context_digest, recorded_state, created_at)
			VALUES (?, ?, ?, ?, ?, ?, 'ready', ?)
			ON CONFLICT(box_id, generation) DO NOTHING`,
			publication.Plan.BoxID, publication.Generation, publication.Plan.PlanDigest,
			publication.PlanPath, publication.ContextPath, publication.ContextDigest,
			publication.PublishedAt.UnixNano()); err != nil {
			return fmt.Errorf("publish READY generation: insert generation: %w", err)
		}
		if err := verifyReadyGenerationRow(ctx, tx, publication); err != nil {
			return err
		}

		for _, item := range publication.Repositories {
			if err := publishReadyRepository(ctx, tx, publication.Plan.BoxID, item, publication.ObservedAt); err != nil {
				return err
			}
		}
		if err := publishReadyRuntime(ctx, tx, publication); err != nil {
			return err
		}
		if err := saveRemovalSnapshotTx(ctx, tx, snapshot, encodedSnapshot); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE boxes
			SET recorded_state = 'ready', current_generation = ?, updated_at = ?
			WHERE id = ? AND current_generation IN (?, ?)`, publication.Generation,
			publication.PublishedAt.UnixNano(), publication.Plan.BoxID, current, publication.Generation)
		if err != nil {
			return fmt.Errorf("publish READY generation: update Box: %w", err)
		}
		changed, err := result.RowsAffected()
		if err != nil || changed != 1 {
			return fmt.Errorf("publish READY generation: update Box postcondition: %w", errors.Join(err, errors.New("box changed concurrently")))
		}
		return nil
	})
}

func prepareReadyGeneration(publication ReadyGeneration) (RemovalSnapshot, []byte, error) {
	if _, err := plan.CanonicalJSON(publication.Plan); err != nil {
		return RemovalSnapshot{}, nil, fmt.Errorf("publish READY generation: invalid Plan: %w", err)
	}
	if publication.Generation < 1 || publication.Generation != publication.RemovalSnapshot.Generation ||
		publication.Plan.BoxID != publication.RemovalSnapshot.BoxID || publication.Plan.PlanDigest != publication.RemovalSnapshot.PlanDigest {
		return RemovalSnapshot{}, nil, errors.New("publish READY generation: generation identity is inconsistent")
	}
	if !cleanAbsoluteFile(publication.PlanPath) || !cleanAbsoluteFile(publication.ContextPath) ||
		publication.PlanPath == publication.ContextPath || filepath.Dir(publication.PlanPath) != filepath.Dir(publication.ContextPath) {
		return RemovalSnapshot{}, nil, errors.New("publish READY generation: generated paths are invalid")
	}
	if !validSHA256(publication.ContextDigest) || !validSHA256(publication.RuntimeConfigDigest) ||
		publication.ObservedAt.IsZero() || publication.PublishedAt.IsZero() || publication.PublishedAt.Before(publication.ObservedAt) {
		return RemovalSnapshot{}, nil, errors.New("publish READY generation: observation evidence is incomplete")
	}
	if len(publication.Repositories) != len(publication.RemovalSnapshot.Repositories) || len(publication.Repositories) != len(publication.Plan.Repositories) {
		return RemovalSnapshot{}, nil, errors.New("publish READY generation: repository observations are incomplete")
	}
	snapshot, encoded, err := prepareRemovalSnapshot(publication.RemovalSnapshot)
	if err != nil {
		return RemovalSnapshot{}, nil, err
	}
	for index, item := range publication.Repositories {
		if !reflect.DeepEqual(item.Facts, snapshot.Repositories[index].Facts) || !reflect.DeepEqual(item.Store, snapshot.Repositories[index].Store) ||
			!validCommit(item.CurrentCommit) {
			return RemovalSnapshot{}, nil, errors.New("publish READY generation: repository observation differs from removal authority")
		}
	}
	return snapshot, encoded, nil
}

func verifyReadyBox(ctx context.Context, tx *sql.Tx, publication ReadyGeneration) (int64, error) {
	var workspaceID, taskKey, manifestPath, manifestDigest string
	var current int64
	err := tx.QueryRowContext(ctx, `SELECT b.workspace_id, b.task_key, b.current_generation,
		w.manifest_path, w.manifest_digest
		FROM boxes AS b JOIN workspaces AS w ON w.id = b.workspace_id WHERE b.id = ?`,
		publication.Plan.BoxID).Scan(&workspaceID, &taskKey, &current, &manifestPath, &manifestDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errors.New("publish READY generation: Box is absent")
	}
	if err != nil {
		return 0, fmt.Errorf("publish READY generation: query Box: %w", err)
	}
	if workspaceID != publication.Plan.WorkspaceID || taskKey != publication.Plan.TaskKey ||
		manifestPath != publication.Plan.Manifest.Path || manifestDigest != string(publication.Plan.Manifest.Digest) {
		return 0, errors.New("publish READY generation: persisted Box identity differs from Plan")
	}
	return current, nil
}

func verifyReadyGenerationRow(ctx context.Context, tx *sql.Tx, publication ReadyGeneration) error {
	var planDigest, planPath, contextPath, contextDigest, recordedState string
	err := tx.QueryRowContext(ctx, `SELECT plan_digest, plan_path, context_path, context_digest, recorded_state
		FROM box_generations WHERE box_id = ? AND generation = ?`, publication.Plan.BoxID, publication.Generation).
		Scan(&planDigest, &planPath, &contextPath, &contextDigest, &recordedState)
	if err != nil {
		return fmt.Errorf("publish READY generation: query generation: %w", err)
	}
	if planDigest != string(publication.Plan.PlanDigest) || planPath != publication.PlanPath ||
		contextPath != publication.ContextPath || contextDigest != string(publication.ContextDigest) || recordedState != "ready" {
		return errors.New("publish READY generation: generation evidence is immutable")
	}
	return nil
}

func publishReadyRepository(ctx context.Context, tx *sql.Tx, boxID string, item ReadyRepository, observedAt time.Time) error {
	facts := item.Facts
	if _, err := tx.ExecContext(ctx, `INSERT INTO repositories
		(box_id, name, source_path, git_store_path, base_commit, branch, publish_destination)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(box_id, name) DO UPDATE SET source_path = excluded.source_path,
		git_store_path = excluded.git_store_path, base_commit = excluded.base_commit,
		branch = excluded.branch, publish_destination = excluded.publish_destination`,
		boxID, facts.Name, facts.SourceRoot, facts.GitStorePath, facts.BaseCommit,
		facts.Branch, facts.PublishDestination.URL); err != nil {
		return fmt.Errorf("publish READY generation: cache repository: %w", err)
	}
	dirty := 0
	if item.Dirty {
		dirty = 1
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO worktrees
		(box_id, repository_name, path, git_common_dir, current_commit, branch, recorded_dirty, observed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(box_id, repository_name) DO UPDATE SET path = excluded.path,
		git_common_dir = excluded.git_common_dir, current_commit = excluded.current_commit,
		branch = excluded.branch, recorded_dirty = excluded.recorded_dirty, observed_at = excluded.observed_at`,
		boxID, facts.Name, facts.WorktreePath, item.Store.GitCommonDir, item.CurrentCommit,
		facts.Branch, dirty, observedAt.UnixNano()); err != nil {
		return fmt.Errorf("publish READY generation: cache worktree: %w", err)
	}
	return nil
}

func publishReadyRuntime(ctx context.Context, tx *sql.Tx, publication ReadyGeneration) error {
	plan := publication.Plan
	observedAt := publication.ObservedAt.UnixNano()
	if _, err := tx.ExecContext(ctx, `INSERT INTO runtime_hosts
		(box_id, driver, instance_name, config_digest, recorded_state, observed_at)
		VALUES (?, ?, ?, ?, 'running', ?)
		ON CONFLICT(box_id) DO UPDATE SET driver = excluded.driver, instance_name = excluded.instance_name,
		config_digest = excluded.config_digest, recorded_state = excluded.recorded_state, observed_at = excluded.observed_at`,
		plan.BoxID, plan.RuntimeHost.Driver, plan.RuntimeHost.InstanceName,
		publication.RuntimeConfigDigest, observedAt); err != nil {
		return fmt.Errorf("publish READY generation: cache runtime host: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO compose_projects
		(box_id, project_name, config_digest, recorded_state, observed_at)
		VALUES (?, ?, ?, 'ready', ?)
		ON CONFLICT(box_id) DO UPDATE SET project_name = excluded.project_name,
		config_digest = excluded.config_digest, recorded_state = excluded.recorded_state, observed_at = excluded.observed_at`,
		plan.BoxID, plan.Compose.ProjectName, publication.RemovalSnapshot.ComposeSpec.Compiled.ResolvedModelDigest,
		observedAt); err != nil {
		return fmt.Errorf("publish READY generation: cache Compose project: %w", err)
	}
	return nil
}

func cleanAbsoluteFile(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path && !strings.ContainsRune(path, '\x00')
}

func validSHA256(digest contract.Digest) bool {
	value := string(digest)
	return len(value) == len("sha256:")+64 && strings.HasPrefix(value, "sha256:") &&
		strings.Trim(value[len("sha256:"):], "0123456789abcdef") == ""
}

func validCommit(commit string) bool {
	return len(commit) == 40 && strings.Trim(commit, "0123456789abcdef") == ""
}
