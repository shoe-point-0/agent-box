package state

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/generatedstate"
	"github.com/shoe-point-0/agent-box/internal/gitstore"
	"github.com/shoe-point-0/agent-box/internal/lima"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/repository"
	"github.com/shoe-point-0/agent-box/internal/toolchain"
)

const removalSnapshotSchemaVersion = "abox.removal-ownership/v1alpha3"

// RepositoryRemovalRecord retains the trusted repository intent and private
// store identity required to reconstruct worktree and private-store steps.
type RepositoryRemovalRecord struct {
	Facts repository.Facts
	Store gitstore.Record
}

// RemovalSnapshot is the immutable ownership authority for one materialized
// generation. Every external fact must still be freshly re-observed before a
// destructive operation.
type RemovalSnapshot struct {
	BoxID               string
	Generation          int64
	PlanDigest          contract.Digest
	BoxRoot             generatedstate.Record
	Repositories        []RepositoryRemovalRecord
	LimaArtifact        toolchain.Identity
	LimaSpec            lima.Spec
	Lima                ownership.LimaInstanceRecord
	ComposeSpec         compose.RuntimeSpec
	Compose             compose.RuntimeRecords
	LimaPathEvidence    ownership.PathSetRecord
	ComposePathEvidence ownership.PathSetRecord
}

type removalSnapshotDocument struct {
	SchemaVersion string `json:"schemaVersion"`
	RemovalSnapshot
}

// SaveRemovalSnapshot atomically binds complete removal ownership to the
// current retained Plan. A generation's authority is immutable: an identical
// retry is accepted, but different ownership facts are refused.
func (s *Store) SaveRemovalSnapshot(ctx context.Context, snapshot RemovalSnapshot) error {
	normalized := cloneRemovalSnapshot(snapshot)
	slices.SortFunc(normalized.Repositories, func(left, right RepositoryRemovalRecord) int {
		return strings.Compare(left.Facts.Name, right.Facts.Name)
	})
	if err := validateRemovalSnapshot(normalized); err != nil {
		return err
	}
	encoded, err := json.Marshal(removalSnapshotDocument{
		SchemaVersion:   removalSnapshotSchemaVersion,
		RemovalSnapshot: normalized,
	})
	if err != nil {
		return fmt.Errorf("encode removal ownership snapshot: %w", err)
	}

	err = s.Update(ctx, func(tx *sql.Tx) error {
		var generation int64
		var planDigest string
		err := tx.QueryRowContext(ctx, `SELECT b.current_generation, g.plan_digest
			FROM boxes AS b
			JOIN box_generations AS g
			  ON g.box_id = b.id AND g.generation = b.current_generation
			WHERE b.id = ?`, normalized.BoxID).Scan(&generation, &planDigest)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("save removal ownership snapshot: current Plan is absent")
		}
		if err != nil {
			return fmt.Errorf("query current Plan for removal ownership: %w", err)
		}
		if generation != normalized.Generation || planDigest != string(normalized.PlanDigest) {
			return errors.New("save removal ownership snapshot: current Plan differs")
		}

		var existing []byte
		err = tx.QueryRowContext(ctx, `SELECT snapshot_json
			FROM removal_ownership_snapshots WHERE box_id = ? AND generation = ?`,
			normalized.BoxID, normalized.Generation).Scan(&existing)
		if err == nil {
			if bytes.Equal(existing, encoded) {
				return nil
			}
			return errors.New("save removal ownership snapshot: generation ownership is immutable")
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("query removal ownership snapshot: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO removal_ownership_snapshots
			(box_id, generation, plan_digest, snapshot_json) VALUES (?, ?, ?, ?)`,
			normalized.BoxID, normalized.Generation, normalized.PlanDigest, string(encoded)); err != nil {
			return fmt.Errorf("insert removal ownership snapshot: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

// CurrentRemovalSnapshot loads the ownership authority for the Box's exact
// current generation in one read transaction. Missing authority is reported as
// found=false and must fail closed at the removal integration boundary.
func (s *Store) CurrentRemovalSnapshot(ctx context.Context, boxID string) (snapshot RemovalSnapshot, found bool, err error) {
	err = s.View(ctx, func(tx *sql.Tx) error {
		var encoded []byte
		var generation int64
		var planDigest string
		err := tx.QueryRowContext(ctx, `SELECT s.generation, s.plan_digest, s.snapshot_json
			FROM boxes AS b
			JOIN removal_ownership_snapshots AS s
			  ON s.box_id = b.id AND s.generation = b.current_generation
			WHERE b.id = ?`, boxID).Scan(&generation, &planDigest, &encoded)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("query current removal ownership snapshot: %w", err)
		}
		decoded, err := decodeRemovalSnapshot(encoded)
		if err != nil {
			return err
		}
		if decoded.BoxID != boxID || decoded.Generation != generation || string(decoded.PlanDigest) != planDigest {
			return errors.New("read current removal ownership snapshot: row identity differs from payload")
		}
		snapshot = decoded
		found = true
		return nil
	})
	if err != nil {
		return RemovalSnapshot{}, false, err
	}
	return snapshot, found, nil
}

func decodeRemovalSnapshot(encoded []byte) (RemovalSnapshot, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var document removalSnapshotDocument
	if err := decoder.Decode(&document); err != nil {
		return RemovalSnapshot{}, fmt.Errorf("decode removal ownership snapshot: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return RemovalSnapshot{}, errors.New("decode removal ownership snapshot: trailing content")
	}
	if document.SchemaVersion != removalSnapshotSchemaVersion {
		return RemovalSnapshot{}, errors.New("decode removal ownership snapshot: unsupported schema version")
	}
	if err := validateRemovalSnapshot(document.RemovalSnapshot); err != nil {
		return RemovalSnapshot{}, fmt.Errorf("decode removal ownership snapshot: %w", err)
	}
	return document.RemovalSnapshot, nil
}

func validateRemovalSnapshot(snapshot RemovalSnapshot) error {
	if invalidFact(snapshot.BoxID) || snapshot.Generation < 1 || invalidFact(string(snapshot.PlanDigest)) {
		return errors.New("save removal ownership snapshot: Box generation identity is incomplete")
	}
	if snapshot.BoxRoot.BoxID != snapshot.BoxID || !validPathRecord(snapshot.BoxRoot.Root) {
		return errors.New("save removal ownership snapshot: Box-root ownership is incomplete")
	}
	if len(snapshot.Repositories) == 0 {
		return errors.New("save removal ownership snapshot: repository ownership is absent")
	}
	for index, record := range snapshot.Repositories {
		if index > 0 && snapshot.Repositories[index-1].Facts.Name == record.Facts.Name {
			return errors.New("save removal ownership snapshot: repository ownership is duplicated")
		}
		facts, store := record.Facts, record.Store
		if invalidFact(facts.Name) || invalidFact(facts.SourceRoot) || invalidFact(facts.BaseCommit) ||
			invalidFact(facts.Branch) || invalidFact(facts.WorktreePath) || invalidFact(facts.GitStorePath) ||
			facts.PublishDestination.Kind != repository.DestinationLocal || invalidFact(facts.PublishDestination.URL) ||
			store.Repository != facts.Name || store.GitCommonDir != facts.GitStorePath ||
			store.BaseCommit != facts.BaseCommit || store.PublishDestination != facts.PublishDestination ||
			store.Identity == (ownership.FileIdentity{}) {
			return errors.New("save removal ownership snapshot: repository ownership is incomplete")
		}
	}
	generation := strconv.FormatInt(snapshot.Generation, 10)
	if snapshot.Lima.Name != snapshot.BoxID || snapshot.Lima.Metadata[ownership.BoxIDFact] != snapshot.BoxID ||
		snapshot.Lima.Metadata["dev.abox.plan-digest"] != string(snapshot.PlanDigest) ||
		snapshot.Lima.Metadata["dev.abox.generation"] != generation {
		return errors.New("save removal ownership snapshot: Lima ownership is incomplete")
	}
	if err := validateLimaRemovalSpec(snapshot); err != nil {
		return err
	}
	if err := validateComposeRemovalSpec(snapshot); err != nil {
		return err
	}
	if len(snapshot.Compose.Resources) == 0 {
		return errors.New("save removal ownership snapshot: Compose ownership is absent")
	}
	project := snapshot.ComposeSpec.Plan.Compose.ProjectName
	for _, record := range snapshot.Compose.Resources {
		if invalidFact(record.Kind) || invalidFact(record.ID) ||
			!validComposeRemovalLabels(record.Labels, snapshot.BoxID, project, string(snapshot.PlanDigest), generation) {
			return errors.New("save removal ownership snapshot: Compose resource ownership is incomplete")
		}
	}
	for _, record := range snapshot.Compose.Volumes {
		if invalidFact(record.ID) ||
			!validComposeRemovalLabels(record.Labels, snapshot.BoxID, project, string(snapshot.PlanDigest), generation) {
			return errors.New("save removal ownership snapshot: Compose volume ownership is incomplete")
		}
	}
	if !validPathSet(snapshot.LimaPathEvidence) || !validPathSet(snapshot.ComposePathEvidence) {
		return errors.New("save removal ownership snapshot: runtime path ownership is incomplete")
	}
	return nil
}

func validComposeRemovalLabels(labels map[string]string, boxID, project, planDigest, generation string) bool {
	return labels[ownership.ComposeBoxIDFact] == boxID &&
		labels["com.docker.compose.project"] == project &&
		labels["dev.abox.plan"] == planDigest && labels["dev.abox.generation"] == generation
}

func validateLimaRemovalSpec(snapshot RemovalSnapshot) error {
	spec := snapshot.LimaSpec
	if !validToolchainIdentity(snapshot.LimaArtifact, toolchain.Lima) || snapshot.LimaArtifact.Version != lima.SupportedVersion ||
		spec.BoxID != snapshot.BoxID || spec.RuntimeHost.InstanceName != snapshot.BoxID ||
		spec.PlanDigest != snapshot.PlanDigest || spec.Generation != snapshot.Generation ||
		!reflect.DeepEqual(spec.RuntimeHost, snapshot.ComposeSpec.Plan.RuntimeHost) ||
		!reflect.DeepEqual(spec.Mounts, snapshot.ComposeSpec.Plan.Mounts) ||
		!reflect.DeepEqual(spec.PathEvidence, snapshot.LimaPathEvidence) ||
		!validToolchainIdentity(spec.GuestImage, toolchain.GuestImage) || !cleanAbsolutePath(spec.ImagePath) {
		return errors.New("save removal ownership snapshot: Lima removal specification is incomplete")
	}
	return nil
}

func validateComposeRemovalSpec(snapshot RemovalSnapshot) error {
	spec := snapshot.ComposeSpec
	compiled := spec.Compiled
	if spec.Plan.BoxID != snapshot.BoxID || spec.Plan.PlanDigest != snapshot.PlanDigest ||
		spec.Generation != snapshot.Generation || !reflect.DeepEqual(spec.RuntimeHost, snapshot.LimaSpec) ||
		!reflect.DeepEqual(spec.PathEvidence, snapshot.ComposePathEvidence) ||
		compiled.ProjectName != spec.Plan.Compose.ProjectName || compiled.ProjectName == "" ||
		!cleanAbsolutePath(spec.DockerPath) || !cleanAbsolutePath(spec.Workdir) ||
		!cleanAbsolutePath(compiled.RootPath) || !cleanAbsolutePath(compiled.OverridePath) ||
		!cleanAbsolutePath(compiled.ResolvedModelPath) || len(compiled.ResolvedModel) == 0 {
		return errors.New("save removal ownership snapshot: Compose removal specification is incomplete")
	}
	digest := sha256.Sum256(compiled.ResolvedModel)
	if compiled.ResolvedModelDigest != contract.Digest(fmt.Sprintf("sha256:%x", digest)) {
		return errors.New("save removal ownership snapshot: Compose resolved-model identity differs")
	}
	return nil
}

func cleanAbsolutePath(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path && !strings.ContainsRune(path, '\x00')
}

func validToolchainIdentity(identity toolchain.Identity, component toolchain.Component) bool {
	digest := string(identity.Digest)
	return identity.Component == component && identity.Name == "" && !invalidFact(identity.Version) &&
		len(digest) == len("sha256:")+sha256.Size*2 && strings.HasPrefix(digest, "sha256:") &&
		strings.Trim(digest[len("sha256:"):], "0123456789abcdef") == ""
}

func validPathRecord(record ownership.PathRecord) bool {
	return filepath.IsAbs(record.CanonicalPath) && filepath.Clean(record.CanonicalPath) == record.CanonicalPath &&
		record.Identity != (ownership.FileIdentity{})
}

func validPathSet(record ownership.PathSetRecord) bool {
	if len(record.Paths) == 0 {
		return false
	}
	for index, chain := range record.Paths {
		if !filepath.IsAbs(chain.CanonicalPath) || filepath.Clean(chain.CanonicalPath) != chain.CanonicalPath || len(chain.Ancestors) == 0 {
			return false
		}
		if index > 0 && record.Paths[index-1].CanonicalPath >= chain.CanonicalPath {
			return false
		}
		if chain.Ancestors[len(chain.Ancestors)-1].CanonicalPath != chain.CanonicalPath {
			return false
		}
		for _, ancestor := range chain.Ancestors {
			if !validPathRecord(ancestor) {
				return false
			}
		}
	}
	return true
}

func invalidFact(value string) bool {
	return value == "" || strings.ContainsRune(value, '\x00')
}

func cloneRemovalSnapshot(snapshot RemovalSnapshot) RemovalSnapshot {
	cloned := snapshot
	cloned.Repositories = slices.Clone(snapshot.Repositories)
	cloned.LimaSpec = cloneLimaSpec(snapshot.LimaSpec)
	cloned.Lima.Metadata = maps.Clone(snapshot.Lima.Metadata)
	cloned.ComposeSpec = cloneComposeSpec(snapshot.ComposeSpec)
	cloned.Compose.Resources = make([]ownership.ComposeResourceRecord, len(snapshot.Compose.Resources))
	for index, record := range snapshot.Compose.Resources {
		record.Labels = maps.Clone(record.Labels)
		cloned.Compose.Resources[index] = record
	}
	cloned.Compose.Volumes = make([]ownership.VolumeRecord, len(snapshot.Compose.Volumes))
	for index, record := range snapshot.Compose.Volumes {
		record.Labels = maps.Clone(record.Labels)
		cloned.Compose.Volumes[index] = record
	}
	cloned.LimaPathEvidence = clonePathSet(snapshot.LimaPathEvidence)
	cloned.ComposePathEvidence = clonePathSet(snapshot.ComposePathEvidence)
	return cloned
}

func cloneLimaSpec(spec lima.Spec) lima.Spec {
	cloned := spec
	cloned.Mounts = slices.Clone(spec.Mounts)
	for index := range cloned.Mounts {
		cloned.Mounts[index].Services = slices.Clone(spec.Mounts[index].Services)
	}
	cloned.PathEvidence = clonePathSet(spec.PathEvidence)
	return cloned
}

func cloneComposeSpec(spec compose.RuntimeSpec) compose.RuntimeSpec {
	cloned := spec
	cloned.RuntimeHost = cloneLimaSpec(spec.RuntimeHost)
	cloned.Plan.Repositories = slices.Clone(spec.Plan.Repositories)
	cloned.Plan.Compose.Includes = slices.Clone(spec.Plan.Compose.Includes)
	cloned.Plan.Compose.Profiles = slices.Clone(spec.Plan.Compose.Profiles)
	cloned.Plan.Mounts = slices.Clone(spec.Plan.Mounts)
	for index := range cloned.Plan.Mounts {
		cloned.Plan.Mounts[index].Services = slices.Clone(spec.Plan.Mounts[index].Services)
	}
	cloned.Compiled.ResolvedModel = slices.Clone(spec.Compiled.ResolvedModel)
	cloned.PathEvidence = clonePathSet(spec.PathEvidence)
	return cloned
}

func clonePathSet(record ownership.PathSetRecord) ownership.PathSetRecord {
	cloned := ownership.PathSetRecord{Paths: make([]ownership.PathChainRecord, len(record.Paths))}
	for index, chain := range record.Paths {
		chain.Ancestors = slices.Clone(chain.Ancestors)
		cloned.Paths[index] = chain
	}
	return cloned
}
