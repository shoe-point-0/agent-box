package ownership_test

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/ownership"
)

func TestPathProofConsumerRemovesOnlyOwnedTarget(t *testing.T) {
	t.Parallel()

	root := canonicalTempRoot(t)
	owned := makeDirectory(t, filepath.Join(root, "owned"))
	unrelated := makeDirectory(t, filepath.Join(root, "unrelated"))
	if err := os.WriteFile(filepath.Join(unrelated, "keep"), []byte("unrelated\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	proof, err := ownership.ProvePath(root, pathRecord(t, owned))
	if err != nil {
		t.Fatalf("ProvePath() error = %v", err)
	}
	if err := removeTree(proof); err != nil {
		t.Fatalf("proof-consuming remove error = %v", err)
	}
	if _, err := os.Stat(owned); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned target still exists or cannot be checked: %v", err)
	}
	if content, err := os.ReadFile(filepath.Join(unrelated, "keep")); err != nil || string(content) != "unrelated\n" {
		t.Fatalf("unrelated resource changed: content = %q, error = %v", content, err)
	}
}

func TestPathProofRejectsForgedStaleAndSymlinkTargets(t *testing.T) {
	t.Parallel()

	root := canonicalTempRoot(t)
	owned := makeDirectory(t, filepath.Join(root, "owned"))
	outsideRoot := canonicalTempRoot(t)
	outside := makeDirectory(t, filepath.Join(outsideRoot, "outside"))

	stale := pathRecord(t, owned)
	stale.Identity.Inode++
	_, err := ownership.ProvePath(root, stale)
	assertRefused(t, err)

	forgedPath := makeDirectory(t, filepath.Join(root, "forged"))
	forged := pathRecord(t, forgedPath)
	forged.Identity = pathRecord(t, owned).Identity
	_, err = ownership.ProvePath(root, forged)
	assertRefused(t, err)

	_, err = ownership.ProvePath(root, pathRecord(t, outside))
	assertRefused(t, err)
	_, err = ownership.ProvePath(root, pathRecord(t, root))
	assertRefused(t, err)

	link := filepath.Join(root, "owned-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	_, err = ownership.ProvePath(root, pathRecord(t, link))
	assertRefused(t, err)

	parentLink := filepath.Join(root, "parent-link")
	if err := os.Symlink(outsideRoot, parentLink); err != nil {
		t.Fatal(err)
	}
	_, err = ownership.ProvePath(root, pathRecord(t, filepath.Join(parentLink, "outside")))
	assertRefused(t, err)

	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("rejected external target was changed: %v", err)
	}
}

func TestPathSetRecordsAndRevalidatesEveryAncestor(t *testing.T) {
	t.Parallel()

	root := canonicalTempRoot(t)
	first := makeDirectory(t, filepath.Join(root, "workspace", "first"))
	second := filepath.Join(root, "generated", "compose.yaml")
	if err := os.MkdirAll(filepath.Dir(second), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	record, err := ownership.RecordPathSet([]string{second, first})
	if err != nil {
		t.Fatal(err)
	}
	if err := ownership.VerifyPathSet([]string{first, second}, record); err != nil {
		t.Fatalf("VerifyPathSet() error = %v", err)
	}
	if len(record.Paths) != 2 || record.Paths[0].CanonicalPath != second || record.Paths[1].CanonicalPath != first {
		t.Fatalf("recorded set is not sorted and exact: %+v", record.Paths)
	}
	for _, chain := range record.Paths {
		if len(chain.Ancestors) < 2 || chain.Ancestors[0].CanonicalPath != string(filepath.Separator) ||
			chain.Ancestors[len(chain.Ancestors)-1].CanonicalPath != chain.CanonicalPath {
			t.Fatalf("ancestor chain is incomplete: %+v", chain)
		}
	}
}

func TestPathSetRefusesSetDriftAndConcurrentSubstitution(t *testing.T) {
	t.Parallel()

	t.Run("membership", func(t *testing.T) {
		root := canonicalTempRoot(t)
		first := makeDirectory(t, filepath.Join(root, "first"))
		second := makeDirectory(t, filepath.Join(root, "second"))
		record, err := ownership.RecordPathSet([]string{first})
		if err != nil {
			t.Fatal(err)
		}
		for _, expected := range [][]string{{first, second}, {second}, {first, first}} {
			if err := ownership.VerifyPathSet(expected, record); !errors.Is(err, ownership.ErrNotOwned) {
				t.Fatalf("VerifyPathSet(%q) error = %v, want ownership refusal", expected, err)
			}
		}
	})

	t.Run("renamed-ancestor", func(t *testing.T) {
		root := canonicalTempRoot(t)
		parent := makeDirectory(t, filepath.Join(root, "workspace"))
		target := makeDirectory(t, filepath.Join(parent, "repo"))
		record, err := ownership.RecordPathSet([]string{target})
		if err != nil {
			t.Fatal(err)
		}
		moved := filepath.Join(root, "workspace-original")
		if err := os.Rename(parent, moved); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(target, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := ownership.VerifyPathSet([]string{target}, record); !errors.Is(err, ownership.ErrNotOwned) {
			t.Fatalf("VerifyPathSet() error = %v, want ancestor-substitution refusal", err)
		}
		if _, err := os.Stat(filepath.Join(moved, "repo")); err != nil {
			t.Fatalf("original target changed after refusal: %v", err)
		}
	})

	t.Run("symlink-substitution", func(t *testing.T) {
		root := canonicalTempRoot(t)
		parent := makeDirectory(t, filepath.Join(root, "generated"))
		target := filepath.Join(parent, "compose.yaml")
		if err := os.WriteFile(target, []byte("original\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		record, err := ownership.RecordPathSet([]string{target})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(parent, parent+"-original"); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(parent+"-original", parent); err != nil {
			t.Fatal(err)
		}
		if err := ownership.VerifyPathSet([]string{target}, record); !errors.Is(err, ownership.ErrNotOwned) {
			t.Fatalf("VerifyPathSet() error = %v, want symlink-substitution refusal", err)
		}
	})
}

func TestRecordPathSetRejectsUnsafeInputs(t *testing.T) {
	t.Parallel()
	root := canonicalTempRoot(t)
	target := makeDirectory(t, filepath.Join(root, "target"))
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	for _, paths := range [][]string{nil, {"relative"}, {target, target}, {link}} {
		if _, err := ownership.RecordPathSet(paths); err == nil {
			t.Fatalf("RecordPathSet(%q) unexpectedly succeeded", paths)
		}
	}
}

func TestGitWorktreeProofRequiresPathRegistrationAndLock(t *testing.T) {
	t.Parallel()

	root := canonicalTempRoot(t)
	worktree := makeDirectory(t, filepath.Join(root, "workspace", "repo"))
	commonDir := makeDirectory(t, filepath.Join(root, "git", "repo.git"))
	pathProof, err := ownership.ProvePath(root, pathRecord(t, worktree))
	if err != nil {
		t.Fatal(err)
	}
	recorded := ownership.GitWorktreeRecord{
		CanonicalPath: worktree, CanonicalCommonDir: commonDir, LockReason: "abox:task-a1b2c3d4",
	}
	live := ownership.GitWorktreeObservation{
		CanonicalPath: worktree, CanonicalCommonDir: commonDir, LockReason: recorded.LockReason,
	}
	proof, err := ownership.ProveGitWorktree(pathProof, recorded, live)
	if err != nil || proof.Path() != worktree {
		t.Fatalf("ProveGitWorktree() = %#v, %v", proof, err)
	}

	tests := map[string]ownership.GitWorktreeObservation{
		"forged registration": {CanonicalPath: filepath.Join(root, "workspace", "other"), CanonicalCommonDir: commonDir, LockReason: recorded.LockReason},
		"wrong common dir":    {CanonicalPath: worktree, CanonicalCommonDir: filepath.Join(root, "git", "other.git"), LockReason: recorded.LockReason},
		"forged lock":         {CanonicalPath: worktree, CanonicalCommonDir: commonDir, LockReason: "abox:other-a1b2c3d4"},
		"unlocked":            {CanonicalPath: worktree, CanonicalCommonDir: commonDir},
	}
	for name, observation := range tests {
		t.Run(name, func(t *testing.T) {
			_, proofErr := ownership.ProveGitWorktree(pathProof, recorded, observation)
			assertRefused(t, proofErr)
		})
	}
}

func TestRuntimeProofsRequireOpaqueIdentityAndIndependentFacts(t *testing.T) {
	t.Parallel()

	labels := map[string]string{ownership.ComposeBoxIDFact: "task-a1b2c3d4", "dev.abox.workspace-id": "ws_a1b2c3d4"}
	composeRecord := ownership.ComposeResourceRecord{Kind: "container", ID: "sha256:opaque", Labels: labels}
	composeLive := ownership.ComposeResourceObservation{Kind: "container", ID: "sha256:opaque", Labels: labels}
	composeProof, err := ownership.ProveComposeResource(composeRecord, composeLive)
	if err != nil || composeProof.Kind() != "container" || composeProof.ID() != "sha256:opaque" {
		t.Fatalf("ProveComposeResource() = %#v, %v", composeProof, err)
	}

	for name, mutate := range map[string]func(*ownership.ComposeResourceObservation){
		"stale ID": func(live *ownership.ComposeResourceObservation) { live.ID = "sha256:replacement" },
		"forged label": func(live *ownership.ComposeResourceObservation) {
			live.Labels = map[string]string{ownership.ComposeBoxIDFact: "task-a1b2c3d4", "dev.abox.workspace-id": "ws_other234"}
		},
		"missing label": func(live *ownership.ComposeResourceObservation) {
			live.Labels = map[string]string{ownership.ComposeBoxIDFact: "task-a1b2c3d4"}
		},
		"forged kind": func(live *ownership.ComposeResourceObservation) { live.Kind = "network" },
	} {
		t.Run(name, func(t *testing.T) {
			live := composeLive
			mutate(&live)
			_, proofErr := ownership.ProveComposeResource(composeRecord, live)
			assertRefused(t, proofErr)
		})
	}

	volumeRecord := ownership.VolumeRecord{ID: "volume-opaque-id", Labels: labels}
	volumeProof, err := ownership.ProveVolume(volumeRecord, ownership.VolumeObservation{ID: volumeRecord.ID, Labels: labels})
	if err != nil || volumeProof.ID() != volumeRecord.ID {
		t.Fatalf("ProveVolume() = %#v, %v", volumeProof, err)
	}
	_, err = ownership.ProveVolume(volumeRecord, ownership.VolumeObservation{
		ID: volumeRecord.ID, Labels: map[string]string{ownership.ComposeBoxIDFact: "forged"},
	})
	assertRefused(t, err)
	_, err = ownership.ProveVolume(volumeRecord, ownership.VolumeObservation{
		ID: "stale-volume-id", Labels: labels,
	})
	assertRefused(t, err)
	_, err = ownership.ProveVolume(
		ownership.VolumeRecord{ID: "volume-opaque-id", Labels: map[string]string{"com.docker.compose.project": "looks-owned"}},
		ownership.VolumeObservation{ID: "volume-opaque-id", Labels: map[string]string{"com.docker.compose.project": "looks-owned"}},
	)
	assertRefused(t, err)
}

func TestLimaProofRejectsForgedNameMetadataAndStaleRecord(t *testing.T) {
	t.Parallel()

	recorded := ownership.LimaInstanceRecord{
		Name: "task-a1b2c3d4", Metadata: map[string]string{ownership.BoxIDFact: "task-a1b2c3d4", "generation": "7"},
	}
	live := ownership.LimaInstanceObservation(recorded)
	proof, err := ownership.ProveLimaInstance(recorded, live)
	if err != nil || proof.Name() != recorded.Name {
		t.Fatalf("ProveLimaInstance() = %#v, %v", proof, err)
	}

	_, err = ownership.ProveLimaInstance(recorded, ownership.LimaInstanceObservation{
		Name: "task-a1b2c3d4-forged", Metadata: recorded.Metadata,
	})
	assertRefused(t, err)
	_, err = ownership.ProveLimaInstance(recorded, ownership.LimaInstanceObservation{
		Name: recorded.Name, Metadata: map[string]string{ownership.BoxIDFact: recorded.Name, "generation": "8"},
	})
	assertRefused(t, err)
	_, err = ownership.ProveLimaInstance(
		ownership.LimaInstanceRecord{Name: recorded.Name},
		ownership.LimaInstanceObservation{Name: recorded.Name},
	)
	assertRefused(t, err)
}

func TestSocketProofRequiresLiveSocketPath(t *testing.T) {
	t.Parallel()

	root := canonicalTempRoot(t)
	socketPath := filepath.Join(root, "daemon.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()

	pathProof, err := ownership.ProvePath(root, pathRecord(t, socketPath))
	if err != nil {
		t.Fatal(err)
	}
	socketProof, err := ownership.ProveSocket(pathProof)
	if err != nil || socketProof.Path() != socketPath {
		t.Fatalf("ProveSocket() = %#v, %v", socketProof, err)
	}

	regular := filepath.Join(root, "not-a-socket")
	if err := os.WriteFile(regular, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	regularProof, err := ownership.ProvePath(root, pathRecord(t, regular))
	if err != nil {
		t.Fatal(err)
	}
	_, err = ownership.ProveSocket(regularProof)
	assertRefused(t, err)
}

func TestListenerProofRequiresBoundIdentityAndActiveLease(t *testing.T) {
	t.Parallel()

	recorded := ownership.ListenerRecord{
		Network: "tcp4", Address: "127.0.0.1:27777", ListenerID: "listener-opaque", LeaseID: "lease-opaque",
	}
	live := ownership.ListenerObservation{
		Network: recorded.Network, Address: recorded.Address, ListenerID: recorded.ListenerID,
		LeaseID: recorded.LeaseID, LeaseActive: true,
	}
	proof, err := ownership.ProveListener(recorded, live)
	if err != nil || proof.Address() != recorded.Address || proof.ListenerID() != recorded.ListenerID {
		t.Fatalf("ProveListener() = %#v, %v", proof, err)
	}

	for name, mutate := range map[string]func(*ownership.ListenerObservation){
		"rebound address": func(observation *ownership.ListenerObservation) { observation.Address = "127.0.0.1:27778" },
		"replacement ID":  func(observation *ownership.ListenerObservation) { observation.ListenerID = "replacement" },
		"stale lease":     func(observation *ownership.ListenerObservation) { observation.LeaseID = "new-lease" },
		"inactive lease":  func(observation *ownership.ListenerObservation) { observation.LeaseActive = false },
	} {
		t.Run(name, func(t *testing.T) {
			observation := live
			mutate(&observation)
			_, proofErr := ownership.ProveListener(recorded, observation)
			assertRefused(t, proofErr)
		})
	}
}

func TestProofErrorsAreInspectableAndZeroProofsAuthorizeNothing(t *testing.T) {
	t.Parallel()

	_, err := ownership.ProveVolume(
		ownership.VolumeRecord{ID: "recorded", Labels: map[string]string{ownership.ComposeBoxIDFact: "box"}},
		ownership.VolumeObservation{ID: "live", Labels: map[string]string{ownership.ComposeBoxIDFact: "box"}},
	)
	if !errors.Is(err, ownership.ErrNotOwned) {
		t.Fatalf("error = %v, want ErrNotOwned", err)
	}
	var proofError *ownership.ProofError
	if !errors.As(err, &proofError) || proofError.Resource != "volume" || proofError.Fact == "" {
		t.Fatalf("error = %#v, want inspectable volume ProofError", err)
	}

	if (ownership.PathProof{}).Path() != "" || (ownership.GitWorktreeProof{}).Path() != "" ||
		(ownership.LimaInstanceProof{}).Name() != "" || (ownership.ComposeResourceProof{}).ID() != "" ||
		(ownership.VolumeProof{}).ID() != "" || (ownership.SocketProof{}).Path() != "" ||
		(ownership.ListenerProof{}).ListenerID() != "" {
		t.Fatal("a zero proof exposed an adapter target")
	}
}

// removeTree models the leaf adapter contract: raw paths are not accepted.
func removeTree(proof ownership.PathProof) error { return os.RemoveAll(proof.Path()) }

func canonicalTempRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func makeDirectory(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func pathRecord(t *testing.T, path string) ownership.PathRecord {
	t.Helper()
	identity, err := ownership.InspectPath(path)
	if err != nil {
		t.Fatal(err)
	}
	return ownership.PathRecord{CanonicalPath: path, Identity: identity}
}

func assertRefused(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ownership.ErrNotOwned) {
		t.Fatalf("error = %v, want ErrNotOwned", err)
	}
}
