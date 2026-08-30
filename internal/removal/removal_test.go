package removal_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/gitstore"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/publication"
	"github.com/shoe-point-0/agent-box/internal/removal"
	"github.com/shoe-point-0/agent-box/internal/repository"
	"github.com/shoe-point-0/agent-box/internal/worktree"
)

func TestWorktreeStepFeedsExactGitLossesIntoDefaultRefusal(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	observed := worktree.Observation{
		Repository: "repo", CanonicalPath: filepath.Join(root, "workspace", "repo"),
		CanonicalCommon: filepath.Join(root, "git", "repo.git"), Branch: "abox/task/repo",
		Commit: "2222222222222222222222222222222222222222", LockReason: "owned by Agent Box box-one",
		Registered: true, Dirty: true, ChangedPaths: []string{"tracked.txt", "untracked\n.txt"},
		IgnoredPaths:     []string{filepath.Join("cache", "ignored.bin")},
		WorktreeIdentity: ownership.FileIdentity{Device: 1, Inode: 2}, StoreIdentity: ownership.FileIdentity{Device: 1, Inode: 3},
	}
	worktrees := &stubWorktreeObserver{observation: observed}
	publications := stubPublicationObserver{statuses: []publication.ReferenceStatus{{
		Reference:          publication.Reference{Name: "refs/heads/" + observed.Branch, Commit: observed.Commit},
		UnpublishedCommits: []string{"1111111111111111111111111111111111111111", observed.Commit},
	}}}
	remover := &stubWorktreeRemover{}
	destination := repository.Destination{Kind: repository.DestinationLocal, URL: root}
	step, err := removal.NewWorktreeStep(removal.WorktreeStepConfig{
		Facts: repository.Facts{Name: "repo"}, Plan: contract.Plan{}, Store: gitstore.Record{},
		Destinations: []repository.Destination{destination}, Worktrees: worktrees,
		Publications: publications, Remover: remover,
	})
	if err != nil {
		t.Fatal(err)
	}
	log := []string{}
	coordinator, err := removal.New(removal.Config{
		Steps: []removal.Step{step}, Lease: fakeGuard{&log}, Quiescer: fakeQuiescer{&log}, Recorder: fakeRecorder{log: &log},
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := coordinator.Preview(t.Context(), "box-one")
	if err != nil {
		t.Fatal(err)
	}
	wantLosses := []removal.Loss{
		{Kind: removal.LossUnpublishedCommit, Value: "1111111111111111111111111111111111111111"},
		{Kind: removal.LossUnpublishedCommit, Value: observed.Commit},
		{Kind: removal.LossWorktreeChange, Value: filepath.Join(observed.CanonicalPath, "cache", "ignored.bin")},
		{Kind: removal.LossWorktreeChange, Value: filepath.Join(observed.CanonicalPath, "tracked.txt")},
		{Kind: removal.LossWorktreeChange, Value: filepath.Join(observed.CanonicalPath, "untracked\n.txt")},
	}
	if !slices.Equal(preview.Losses, wantLosses) {
		t.Fatalf("losses = %#v, want %#v", preview.Losses, wantLosses)
	}
	_, err = coordinator.Execute(t.Context(), preview, removal.Authorization{PreviewDigest: preview.Digest})
	if !errors.Is(err, removal.ErrSafetyRefusal) || remover.calls != 0 || len(log) != 0 {
		t.Fatalf("default Execute = %v, remover calls = %d, lifecycle = %v", err, remover.calls, log)
	}
}

func TestDefaultRemovalRefusesExactLossesWithoutLifecycleMutation(t *testing.T) {
	t.Parallel()
	log := []string{}
	steps := []*fakeStep{
		newStep("compose/project", removal.StageCompose, removal.Fact{Present: true, Evidence: evidence("compose-1"), Losses: []removal.Loss{{Kind: removal.LossVolume, Value: "volume-db"}}}, &log),
		newStep("worktree/repo", removal.StageWorktree, removal.Fact{Present: true, Evidence: evidence("inode-1"), Losses: []removal.Loss{
			{Kind: removal.LossUnpublishedCommit, Value: "1111111111111111111111111111111111111111"},
			{Kind: removal.LossWorktreeChange, Value: "ignored.cache"},
			{Kind: removal.LossWorktreeChange, Value: "untracked.txt"},
		}}, &log),
	}
	coordinator := newCoordinator(t, steps, &log, nil)
	preview, err := coordinator.Preview(t.Context(), "box-one")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Losses) != 4 || preview.Digest == "" {
		t.Fatalf("preview = %#v", preview)
	}
	if len(log) != 2 {
		t.Fatalf("preview log = %v", log)
	}
	log = log[:0]

	_, err = coordinator.Execute(t.Context(), preview, removal.Authorization{PreviewDigest: preview.Digest})
	if !errors.Is(err, removal.ErrSafetyRefusal) {
		t.Fatalf("Execute error = %v, want refusal", err)
	}
	var refusal *removal.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("error type = %T", err)
	}
	want := []string{"--yes", "--discard-worktree-changes", "--discard-unpublished-commits", "--delete-volumes"}
	if !slices.Equal(refusal.Missing, want) {
		t.Fatalf("missing = %v, want %v", refusal.Missing, want)
	}
	if len(log) != 0 {
		t.Fatalf("refusal caused lifecycle calls: %v", log)
	}
	for _, step := range steps {
		if step.removes != 0 {
			t.Fatalf("%s remove calls = %d", step.key, step.removes)
		}
	}
}

func TestExplicitRemovalDeletesOnlySelectedTemporaryRootInOrder(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	selected := filepath.Join(root, "boxes", "selected")
	unrelated := filepath.Join(root, "boxes", "unrelated", "keep.txt")
	if err := os.MkdirAll(selected, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(selected, "dirty.txt"), []byte("discard me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(unrelated), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unrelated, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}

	log := []string{}
	compose := newStep("compose/selected", removal.StageCompose, removal.Fact{Present: true, Evidence: evidence("labels-selected"), Losses: []removal.Loss{{Kind: removal.LossVolume, Value: "selected-db"}}}, &log)
	worktree := newStep("worktree/selected", removal.StageWorktree, removal.Fact{Present: true, Evidence: map[string]string{
		"branch": "abox/task/repo", "commonDir": filepath.Join(selected, "git", "repo.git"), "identity": "device:1/inode:2", "lock": "owned by Agent Box box-selected",
	}, Losses: []removal.Loss{{Kind: removal.LossWorktreeChange, Value: "dirty.txt"}}}, &log)
	worktree.removeFn = func() error { return os.RemoveAll(selected) }
	coordinator := newCoordinator(t, []*fakeStep{compose, worktree}, &log, nil)
	preview, err := coordinator.Preview(t.Context(), "box-selected")
	if err != nil {
		t.Fatal(err)
	}
	log = log[:0]

	result, err := coordinator.Execute(t.Context(), preview, authorizeAll(preview))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(result.Completed, []string{"compose/selected", "worktree/selected"}) || len(result.Remaining) != 0 {
		t.Fatalf("result = %#v", result)
	}
	wantOrder := []string{
		"lease.acquire", "writers.quiesce", "compose/selected.observe", "worktree/selected.observe",
		"compose/selected.observe", "event.remove-started.compose/selected", "compose/selected.remove", "event.remove-succeeded.compose/selected",
		"worktree/selected.observe", "event.remove-started.worktree/selected", "worktree/selected.remove", "event.remove-succeeded.worktree/selected",
		"writers.resume", "lease.release",
	}
	if !reflect.DeepEqual(log, wantOrder) {
		t.Fatalf("execution order:\n got %v\nwant %v", log, wantOrder)
	}
	if _, err := os.Stat(selected); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("selected root remains: %v", err)
	}
	contents, err := os.ReadFile(unrelated)
	if err != nil || string(contents) != "keep me" {
		t.Fatalf("unrelated file = %q, %v", contents, err)
	}
}

func TestDriftAfterPreviewOrBetweenStepsRefusesBeforeAffectedMutation(t *testing.T) {
	t.Parallel()
	t.Run("after-preview", func(t *testing.T) {
		log := []string{}
		step := newStep("worktree/repo", removal.StageWorktree, removal.Fact{Present: true, Evidence: evidence("inode-1"), Losses: []removal.Loss{}}, &log)
		coordinator := newCoordinator(t, []*fakeStep{step}, &log, nil)
		preview, err := coordinator.Preview(t.Context(), "box-drift")
		if err != nil {
			t.Fatal(err)
		}
		step.fact.Evidence["identity"] = "inode-2"
		log = log[:0]
		_, err = coordinator.Execute(t.Context(), preview, authorizeAll(preview))
		if !errors.Is(err, removal.ErrInventoryDrift) {
			t.Fatalf("error = %v, want drift", err)
		}
		if step.removes != 0 {
			t.Fatal("drifted step was removed")
		}
		if !slices.Equal(log, []string{"lease.acquire", "writers.quiesce", "worktree/repo.observe", "writers.resume", "lease.release"}) {
			t.Fatalf("drift lifecycle = %v", log)
		}
	})

	t.Run("between-steps", func(t *testing.T) {
		log := []string{}
		later := newStep("worktree/repo", removal.StageWorktree, removal.Fact{Present: true, Evidence: evidence("inode-1"), Losses: []removal.Loss{}}, &log)
		first := newStep("compose/project", removal.StageCompose, removal.Fact{Present: true, Evidence: evidence("labels-1"), Losses: []removal.Loss{}}, &log)
		first.removeFn = func() error { later.fact.Evidence["identity"] = "inode-2"; return nil }
		coordinator := newCoordinator(t, []*fakeStep{first, later}, &log, nil)
		preview, err := coordinator.Preview(t.Context(), "box-drift")
		if err != nil {
			t.Fatal(err)
		}
		log = log[:0]
		result, err := coordinator.Execute(t.Context(), preview, authorizeAll(preview))
		if !errors.Is(err, removal.ErrInventoryDrift) {
			t.Fatalf("error = %v, want drift", err)
		}
		if !slices.Equal(result.Completed, []string{"compose/project"}) || !slices.Equal(result.Remaining, []string{"worktree/repo"}) {
			t.Fatalf("result = %#v", result)
		}
		if first.removes != 1 || later.removes != 0 {
			t.Fatalf("remove calls = %d/%d", first.removes, later.removes)
		}
	})
}

func TestInterruptedRemovalFreshPreviewConvergesAndRetainsProgress(t *testing.T) {
	t.Parallel()
	log := []string{}
	first := newStep("compose/project", removal.StageCompose, present("compose"), &log)
	second := newStep("runtime/host", removal.StageRuntimeHost, present("lima"), &log)
	third := newStep("worktree/repo", removal.StageWorktree, present("git"), &log)
	second.removeErr = errors.New("injected interruption")
	coordinator := newCoordinator(t, []*fakeStep{first, second, third}, &log, nil)
	preview, err := coordinator.Preview(t.Context(), "box-resume")
	if err != nil {
		t.Fatal(err)
	}

	result, err := coordinator.Execute(t.Context(), preview, authorizeAll(preview))
	if err == nil {
		t.Fatal("Execute succeeded despite injected interruption")
	}
	var execution *removal.ExecutionError
	if !errors.As(err, &execution) || execution.Resource != "runtime/host" {
		t.Fatalf("error = %v", err)
	}
	if !slices.Equal(result.Completed, []string{"compose/project"}) || !slices.Equal(result.Remaining, []string{"runtime/host", "worktree/repo"}) {
		t.Fatalf("partial result = %#v", result)
	}
	if first.fact.Present || !second.fact.Present || !third.fact.Present {
		t.Fatalf("facts after failure = %#v %#v %#v", first.fact, second.fact, third.fact)
	}

	second.removeErr = nil
	fresh, err := coordinator.Preview(t.Context(), "box-resume")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Digest == preview.Digest {
		t.Fatal("partial cleanup did not change confirmation digest")
	}
	result, err = coordinator.Execute(t.Context(), fresh, authorizeAll(fresh))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(result.Completed, []string{"runtime/host", "worktree/repo"}) {
		t.Fatalf("retry result = %#v", result)
	}
	if first.removes != 1 || second.removes != 2 || third.removes != 1 {
		t.Fatalf("remove calls = %d/%d/%d", first.removes, second.removes, third.removes)
	}
}

func TestRecorderFailureBeforeSideEffectPreservesResource(t *testing.T) {
	t.Parallel()
	log := []string{}
	step := newStep("compose/project", removal.StageCompose, present("compose"), &log)
	recorderErr := errors.New("journal unavailable")
	coordinator := newCoordinator(t, []*fakeStep{step}, &log, recorderErr)
	preview, err := coordinator.Preview(t.Context(), "box-journal")
	if err != nil {
		t.Fatal(err)
	}
	_, err = coordinator.Execute(t.Context(), preview, authorizeAll(preview))
	if !errors.Is(err, recorderErr) {
		t.Fatalf("error = %v", err)
	}
	if step.removes != 0 || !step.fact.Present {
		t.Fatal("resource mutated after before-event failure")
	}
}

func TestConfigurationAndTamperedPreviewFailClosed(t *testing.T) {
	t.Parallel()
	log := []string{}
	late := newStep("worktree/repo", removal.StageWorktree, present("git"), &log)
	early := newStep("compose/project", removal.StageCompose, present("compose"), &log)
	if _, err := removal.New(config([]*fakeStep{late, early}, &log, nil)); err == nil {
		t.Fatal("out-of-order steps accepted")
	}
	coordinator := newCoordinator(t, []*fakeStep{early}, &log, nil)
	preview, err := coordinator.Preview(t.Context(), "box-tamper")
	if err != nil {
		t.Fatal(err)
	}
	preview.Resources[0].Evidence["identity"] = "forged"
	if _, err := coordinator.Execute(t.Context(), preview, authorizeAll(preview)); !errors.Is(err, removal.ErrInventoryDrift) {
		t.Fatalf("tampered preview error = %v", err)
	}
}

type fakeStep struct {
	key       string
	stage     removal.Stage
	fact      removal.Fact
	log       *[]string
	removes   int
	removeErr error
	removeFn  func() error
}

func newStep(key string, stage removal.Stage, fact removal.Fact, log *[]string) *fakeStep {
	return &fakeStep{key: key, stage: stage, fact: cloneFact(fact), log: log}
}
func (s *fakeStep) Key() string          { return s.key }
func (s *fakeStep) Stage() removal.Stage { return s.stage }
func (s *fakeStep) Observe(context.Context) (removal.Fact, error) {
	*s.log = append(*s.log, s.key+".observe")
	return cloneFact(s.fact), nil
}
func (s *fakeStep) Remove(_ context.Context, _ removal.Fact) error {
	*s.log = append(*s.log, s.key+".remove")
	s.removes++
	if s.removeErr != nil {
		return s.removeErr
	}
	if s.removeFn != nil {
		if err := s.removeFn(); err != nil {
			return err
		}
	}
	s.fact = removal.Fact{Present: false}
	return nil
}

type fakeGuard struct{ log *[]string }

func (g fakeGuard) Acquire(_ context.Context, boxID string) (removal.Lease, error) {
	*g.log = append(*g.log, "lease.acquire")
	return removal.Lease{BoxID: boxID, Token: "fence-1"}, nil
}
func (g fakeGuard) Release(_ context.Context, _ removal.Lease) error {
	*g.log = append(*g.log, "lease.release")
	return nil
}

type fakeQuiescer struct{ log *[]string }

func (q fakeQuiescer) Quiesce(_ context.Context, _ removal.Lease) error {
	*q.log = append(*q.log, "writers.quiesce")
	return nil
}
func (q fakeQuiescer) Resume(_ context.Context, _ removal.Lease) error {
	*q.log = append(*q.log, "writers.resume")
	return nil
}

type fakeRecorder struct {
	log  *[]string
	fail error
}

func (r fakeRecorder) Record(_ context.Context, transition removal.Transition) error {
	*r.log = append(*r.log, "event."+transition.Kind+"."+transition.Resource)
	if transition.Kind == removal.TransitionRemoveStarted {
		return r.fail
	}
	return nil
}

func newCoordinator(t *testing.T, steps []*fakeStep, log *[]string, recorderErr error) *removal.Coordinator {
	t.Helper()
	coordinator, err := removal.New(config(steps, log, recorderErr))
	if err != nil {
		t.Fatal(err)
	}
	return coordinator
}
func config(steps []*fakeStep, log *[]string, recorderErr error) removal.Config {
	converted := make([]removal.Step, len(steps))
	for index, step := range steps {
		converted[index] = step
	}
	return removal.Config{Steps: converted, Lease: fakeGuard{log}, Quiescer: fakeQuiescer{log}, Recorder: fakeRecorder{log, recorderErr}}
}
func authorizeAll(preview removal.Preview) removal.Authorization {
	return removal.Authorization{PreviewDigest: preview.Digest, Yes: true, DiscardWorktreeChanges: true, DiscardUnpublishedCommits: true, DeleteVolumes: true}
}
func evidence(identity string) map[string]string { return map[string]string{"identity": identity} }
func present(identity string) removal.Fact {
	return removal.Fact{Present: true, Evidence: evidence(identity), Losses: []removal.Loss{}}
}
func cloneFact(fact removal.Fact) removal.Fact {
	clone := removal.Fact{Present: fact.Present, Evidence: map[string]string{}, Losses: slices.Clone(fact.Losses)}
	for key, value := range fact.Evidence {
		clone.Evidence[key] = value
	}
	if len(clone.Evidence) == 0 {
		clone.Evidence = nil
	}
	return clone
}

type stubWorktreeObserver struct {
	observation worktree.Observation
}

func (s *stubWorktreeObserver) Observe(context.Context, repository.Facts, contract.Plan, gitstore.Record) (worktree.Observation, error) {
	return s.observation, nil
}

type stubPublicationObserver struct {
	statuses []publication.ReferenceStatus
}

func (s stubPublicationObserver) Check(context.Context, gitstore.Record, []publication.Reference, []repository.Destination) ([]publication.ReferenceStatus, error) {
	return s.statuses, nil
}

type stubWorktreeRemover struct {
	calls int
}

func (s *stubWorktreeRemover) Remove(context.Context, repository.Facts, contract.Plan, gitstore.Record, worktree.Observation) error {
	s.calls++
	return nil
}
