package up

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/gitstore"
	"github.com/shoe-point-0/agent-box/internal/journal"
	"github.com/shoe-point-0/agent-box/internal/lima"
	plancompiler "github.com/shoe-point-0/agent-box/internal/plan"
	"github.com/shoe-point-0/agent-box/internal/toolchain"
	"github.com/shoe-point-0/agent-box/internal/worktree"
)

func TestReconcileFoundationRecoversOrderedConcreteStagesWithoutDuplicateMutation(t *testing.T) {
	fixture := newGitReconcileFixtureWithDriver(t, "lima-vz")
	canonical, err := plancompiler.CanonicalJSON(fixture.plan)
	if err != nil {
		t.Fatal(err)
	}
	boxRoot := filepath.Dir(fixture.workspaceRoot)
	attempt := PreparedAttempt{
		Operation: journal.Operation{ID: "operation-one", BoxID: fixture.plan.BoxID, Kind: "up", State: journal.StateRunning},
		Plan:      fixture.plan, CanonicalPlan: canonical, Generation: 1, Repository: fixture.facts,
		BoxRoot: boxRoot, WorkspaceRoot: fixture.workspaceRoot,
		PlanPath: filepath.Join(boxRoot, "plan.json"), LimaConfigPath: filepath.Join(boxRoot, "lima.yaml"),
	}
	prepareGeneratedApplicationRoot(t, attempt)

	imagePath := filepath.Join(t.TempDir(), "guest.img")
	if err := os.WriteFile(imagePath, []byte("guest image\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	artifact := toolchain.Identity{
		Component: toolchain.Lima, Version: lima.SupportedVersion,
		Digest: contract.Digest("sha256:" + strings.Repeat("c", 64)),
	}
	guest := toolchain.Identity{
		Component: toolchain.GuestImage, Version: "fixture",
		Digest: contract.Digest("sha256:" + strings.Repeat("d", 64)),
	}

	generatedAuthority := &memoryGeneratedAuthority{}
	gitAuthority := &memoryGitAuthority{}
	limaAuthority := &memoryLimaAuthority{}
	stores := &checkingMaterializer{
		Materializer: gitstore.Materializer{Git: fixture.client}, authority: gitAuthority,
	}
	worktrees := &interruptingWorktrees{Manager: worktree.Manager{
		Git: fixture.client, WorkspaceRoot: fixture.workspaceRoot,
	}}
	runtime := &memoryLimaRuntime{}
	runtimeSpecSet := false
	planWrites := 0
	buildCalls := 0
	interruptBuild := true
	config := FoundationConfig{
		Generated: GeneratedPlanConfig{
			Authority: generatedAuthority,
			WritePlan: func(path string, data []byte) error {
				planWrites++
				return writeImmutableFixture(path, data)
			},
		},
		Git: GitConfig{Authority: gitAuthority, Stores: stores, Worktrees: worktrees},
		BuildLima: func(_ context.Context, got PreparedAttempt, generated GeneratedPlanResult, git GitResult) (LimaConfig, error) {
			buildCalls++
			if got.PlanPath != attempt.PlanPath || generated.BoxRoot.Root.CanonicalPath != boxRoot ||
				git.Store.GitCommonDir != fixture.facts.GitStorePath || git.Worktree.CanonicalPath != fixture.facts.WorktreePath {
				return LimaConfig{}, errors.New("foundation dependency result drift")
			}
			for _, path := range []string{got.PlanPath, fixture.facts.GitStorePath, fixture.facts.WorktreePath} {
				if _, err := os.Lstat(path); err != nil {
					return LimaConfig{}, errors.New("Lima builder ran before dependencies converged")
				}
			}
			if interruptBuild {
				interruptBuild = false
				return LimaConfig{}, errors.New("injected interruption before Lima intent")
			}
			spec := lima.Spec{
				RuntimeHost: got.Plan.RuntimeHost, Mounts: got.Plan.Mounts, BoxID: got.Plan.BoxID,
				PlanDigest: got.Plan.PlanDigest, Generation: got.Generation,
				GuestImage: guest, ImagePath: imagePath,
			}
			var err error
			spec.PathEvidence, err = lima.RecordRuntimePaths(spec)
			if err != nil {
				return LimaConfig{}, err
			}
			if !runtimeSpecSet {
				runtime.spec = spec
				runtimeSpecSet = true
			} else if !reflect.DeepEqual(runtime.spec, spec) {
				return LimaConfig{}, errors.New("runtime Spec changed between attempts")
			}
			return LimaConfig{Authority: limaAuthority, Runtime: runtime, Artifact: artifact, Spec: spec}, nil
		},
	}

	if _, err := ReconcileFoundation(t.Context(), config, attempt); err == nil || !strings.Contains(err.Error(), "injected interruption") {
		t.Fatalf("interrupted foundation error = %v", err)
	}
	if generatedAuthority.value.BoxRoot == nil || gitAuthority.value.Store == nil || gitAuthority.value.Worktree == nil || limaAuthority.found {
		t.Fatalf("authority after interruption: generated=%#v git=%#v lima=%#v", generatedAuthority.value, gitAuthority.value, limaAuthority.value)
	}

	attempt.Operation.ID = "operation-two"
	second, err := ReconcileFoundation(t.Context(), config, attempt)
	if err != nil {
		t.Fatal(err)
	}
	attempt.Operation.ID = "operation-three"
	third, err := ReconcileFoundation(t.Context(), config, attempt)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(second, third) {
		t.Fatalf("warm foundation result changed: second=%#v third=%#v", second, third)
	}
	if planWrites != 1 || stores.calls != 1 || worktrees.ensures != 1 || worktrees.observes != 2 || buildCalls != 3 {
		t.Fatalf("foundation calls: plan=%d store=%d ensure=%d observe=%d build=%d", planWrites, stores.calls, worktrees.ensures, worktrees.observes, buildCalls)
	}
	if runtime.createMutations != 1 || runtime.startMutations != 1 {
		t.Fatalf("runtime mutations: create=%d start=%d", runtime.createMutations, runtime.startMutations)
	}
}
