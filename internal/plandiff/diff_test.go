package plandiff_test

import (
	"encoding/json"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/plandiff"
)

var resultSink plandiff.Result

func TestClassifyCoversReconcileBehaviorTable(t *testing.T) {
	tests := []struct {
		name        string
		fact        plandiff.Fact
		wantClass   plandiff.Class
		wantReason  plandiff.Reason
		wantChanges int
	}{
		{"route or readiness configuration", plandiff.Fact{Kind: plandiff.KindRouteOrReadiness, Resource: "route/api"}, plandiff.ClassInPlace, plandiff.ReasonRouteChanged, 1},
		{"Compose service image or build configuration", plandiff.Fact{Kind: plandiff.KindCompose, Resource: "compose"}, plandiff.ClassInPlace, plandiff.ReasonComposeChanged, 1},
		{"setup job input or command", plandiff.Fact{Kind: plandiff.KindSetupJob, Resource: "setup-job/install"}, plandiff.ClassInPlace, plandiff.ReasonSetupJobChanged, 1},
		{"worktree edits", plandiff.Fact{Kind: plandiff.KindWorktreeEdit, Resource: "repository/repo"}, plandiff.ClassNoOp, plandiff.ReasonObservedStateOnly, 0},
		{"VM CPU or memory", plandiff.Fact{Kind: plandiff.KindRuntimeResources, Resource: "runtime-host"}, plandiff.ClassRestart, plandiff.ReasonRuntimeResourcesChanged, 1},
		{"VM disk increase", plandiff.Fact{Kind: plandiff.KindRuntimeDiskGrow, Resource: "runtime-host"}, plandiff.ClassGrowth, plandiff.ReasonRuntimeDiskIncrease, 1},
		{"VM disk shrink", plandiff.Fact{Kind: plandiff.KindRuntimeDiskShrink, Resource: "runtime-host"}, plandiff.ClassConflict, plandiff.ReasonRuntimeDiskShrink, 1},
		{"base ref or branch replacement clean", plandiff.Fact{Kind: plandiff.KindRepositoryCheckout, Resource: "repository/repo"}, plandiff.ClassInPlace, plandiff.ReasonRepositoryCheckoutChanged, 1},
		{"base ref or branch replacement dirty", plandiff.Fact{Kind: plandiff.KindRepositoryCheckout, Resource: "repository/repo", Dirty: true}, plandiff.ClassConflict, plandiff.ReasonDirtyCheckoutReplacement, 1},
		{"repository removal clean", plandiff.Fact{Kind: plandiff.KindRepositoryRemoval, Resource: "repository/repo"}, plandiff.ClassRemoval, plandiff.ReasonRepositoryRemoved, 1},
		{"repository removal dirty", plandiff.Fact{Kind: plandiff.KindRepositoryRemoval, Resource: "repository/repo", Dirty: true}, plandiff.ClassConflict, plandiff.ReasonDirtyRepositoryRemoval, 1},
		{"secret reference changed", plandiff.Fact{Kind: plandiff.KindSecretReference, Resource: "secret/TOKEN"}, plandiff.ClassRestart, plandiff.ReasonSecretReferenceChanged, 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := plandiff.Classify([]plandiff.Fact{test.fact})
			if got.Classification != test.wantClass || got.Reason != mapResultReason(test.wantClass, test.wantReason) || len(got.Changes) != test.wantChanges {
				t.Fatalf("Classify() = %#v, want class %q and %d changes", got, test.wantClass, test.wantChanges)
			}
			if test.wantChanges > 0 && (got.Changes[0].Reason != test.wantReason || got.Changes[0].Description == "") {
				t.Fatalf("Classify() changes = %#v, want reason %q and human description", got.Changes, test.wantReason)
			}
		})
	}
}

func TestCompareExecutablePlanChanges(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*contract.Plan)
		observed    plandiff.Observation
		wantClass   plandiff.Class
		wantReason  plandiff.Reason
		wantChanges int
	}{
		{
			name: "no-op with dirty worktree observation",
			mutate: func(plan *contract.Plan) {
				plan.PlanDigest = digest('a')
			},
			observed:  plandiff.Observation{Repositories: []plandiff.RepositoryObservation{{Name: "repo", Dirty: true}}},
			wantClass: plandiff.ClassNoOp, wantReason: plandiff.ReasonPlanDigestsMatch,
		},
		{
			name: "Compose configuration",
			mutate: func(plan *contract.Plan) {
				plan.Compose.Includes[0].Digest = digest('b')
			},
			wantClass: plandiff.ClassInPlace, wantReason: plandiff.ReasonComposeChanged, wantChanges: 1,
		},
		{
			name: "CPU or memory",
			mutate: func(plan *contract.Plan) {
				plan.RuntimeHost.MemoryBytes++
			},
			wantClass: plandiff.ClassRestart, wantReason: plandiff.ReasonRuntimeResourcesChanged, wantChanges: 1,
		},
		{
			name: "disk increase",
			mutate: func(plan *contract.Plan) {
				plan.RuntimeHost.DiskBytes++
			},
			wantClass: plandiff.ClassGrowth, wantReason: plandiff.ReasonRuntimeDiskIncrease, wantChanges: 1,
		},
		{
			name: "disk shrink",
			mutate: func(plan *contract.Plan) {
				plan.RuntimeHost.DiskBytes--
			},
			wantClass: plandiff.ClassConflict, wantReason: plandiff.ReasonRuntimeDiskShrink, wantChanges: 1,
		},
		{
			name: "dirty base replacement",
			mutate: func(plan *contract.Plan) {
				plan.Repositories[0].BaseCommit = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
			},
			observed:  plandiff.Observation{Repositories: []plandiff.RepositoryObservation{{Name: "repo", Dirty: true}}},
			wantClass: plandiff.ClassConflict, wantReason: plandiff.ReasonDirtyCheckoutReplacement, wantChanges: 1,
		},
		{
			name: "dirty branch replacement",
			mutate: func(plan *contract.Plan) {
				plan.Repositories[0].Branch = "abox/TASK-1/replacement"
			},
			observed:  plandiff.Observation{Repositories: []plandiff.RepositoryObservation{{Name: "repo", Dirty: true}}},
			wantClass: plandiff.ClassConflict, wantReason: plandiff.ReasonDirtyCheckoutReplacement, wantChanges: 1,
		},
		{
			name: "clean repository removal",
			mutate: func(plan *contract.Plan) {
				plan.Repositories = nil
			},
			wantClass: plandiff.ClassRemoval, wantReason: plandiff.ReasonRepositoryRemoved, wantChanges: 1,
		},
		{
			name: "dirty repository removal",
			mutate: func(plan *contract.Plan) {
				plan.Repositories = nil
			},
			observed:  plandiff.Observation{Repositories: []plandiff.RepositoryObservation{{Name: "repo", Dirty: true}}},
			wantClass: plandiff.ClassConflict, wantReason: plandiff.ReasonDirtyRepositoryRemoval, wantChanges: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := basePlan()
			desired := clonePlan(t, current)
			test.mutate(&desired)
			if desired.PlanDigest == current.PlanDigest && !reflect.DeepEqual(current, desired) {
				desired.PlanDigest = digest('f')
			}

			got := plandiff.Compare(&current, &desired, test.observed)
			if got.Classification != test.wantClass || len(got.Changes) != test.wantChanges {
				t.Fatalf("Compare() = %#v, want class %q with %d changes", got, test.wantClass, test.wantChanges)
			}
			if test.wantClass == plandiff.ClassNoOp {
				if got.Reason != test.wantReason {
					t.Fatalf("Compare() reason = %q, want %q", got.Reason, test.wantReason)
				}
				return
			}
			if got.Changes[0].Reason != test.wantReason || got.Changes[0].Description == "" {
				t.Fatalf("Compare() changes = %#v, want reason %q and human description", got.Changes, test.wantReason)
			}
		})
	}
}

func TestCompareConflictDoesNotMutateInputs(t *testing.T) {
	current := basePlan()
	desired := clonePlan(t, current)
	desired.RuntimeHost.DiskBytes--
	desired.Repositories[0].Branch = "abox/TASK-1/replacement"
	desired.PlanDigest = digest('f')
	observed := plandiff.Observation{Repositories: []plandiff.RepositoryObservation{{Name: "repo", Dirty: true}}}

	currentBefore := mustJSON(t, current)
	desiredBefore := mustJSON(t, desired)
	observedBefore := mustJSON(t, observed)
	got := plandiff.Compare(&current, &desired, observed)
	if got.Classification != plandiff.ClassConflict {
		t.Fatalf("Compare() class = %q, want conflict", got.Classification)
	}
	if string(mustJSON(t, current)) != string(currentBefore) || string(mustJSON(t, desired)) != string(desiredBefore) ||
		string(mustJSON(t, observed)) != string(observedBefore) {
		t.Fatal("Compare() mutated Plan or observation input")
	}
}

func TestCompareNoOpAllocations(t *testing.T) {
	current := basePlan()
	desired := clonePlan(t, current)
	allocations := testing.AllocsPerRun(1000, func() {
		resultSink = plandiff.Compare(&current, &desired, plandiff.Observation{})
	})
	if allocations != 0 {
		t.Fatalf("no-op Compare() allocations = %v, want 0", allocations)
	}
}

func TestClassifyGoldenAndOrderingProperty(t *testing.T) {
	facts := []plandiff.Fact{
		{Kind: plandiff.KindRouteOrReadiness, Resource: "route/api"},
		{Kind: plandiff.KindRuntimeDiskGrow, Resource: "runtime-host"},
		{Kind: plandiff.KindRuntimeResources, Resource: "runtime-host"},
		{Kind: plandiff.KindSecretReference, Resource: "secret/TOKEN"},
		{Kind: plandiff.KindSetupJob, Resource: "setup-job/install"},
	}
	var first []byte
	for iteration := range 100 {
		random := rand.New(rand.NewPCG(uint64(iteration+1), uint64(iteration+101)))
		random.Shuffle(len(facts), func(left, right int) { facts[left], facts[right] = facts[right], facts[left] })
		encoded, err := json.MarshalIndent(plandiff.Classify(facts), "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		encoded = append(encoded, '\n')
		if iteration == 0 {
			first = encoded
			golden, err := os.ReadFile(filepath.Join("testdata", "diff.golden.json"))
			if err != nil {
				t.Fatal(err)
			}
			if string(encoded) != string(golden) {
				t.Fatalf("diff golden mismatch\nwant: %s\n got: %s", golden, encoded)
			}
			continue
		}
		if string(encoded) != string(first) {
			t.Fatalf("iteration %d changed output\nfirst: %s\n got: %s", iteration, first, encoded)
		}
	}
}

func BenchmarkCompareNoOp(b *testing.B) {
	current := basePlan()
	desired := clonePlan(b, current)
	b.ReportAllocs()
	for range b.N {
		resultSink = plandiff.Compare(&current, &desired, plandiff.Observation{})
	}
}

func mapResultReason(class plandiff.Class, changeReason plandiff.Reason) plandiff.Reason {
	if class == plandiff.ClassNoOp {
		return changeReason
	}
	return plandiff.ReasonChangesRequired
}

func basePlan() contract.Plan {
	return contract.Plan{
		SchemaVersion: contract.PlanSchemaVersion,
		WorkspaceID:   "ws_test",
		BoxID:         "task-test",
		TaskKey:       "TASK-1",
		Manifest:      contract.Artifact{Path: "/workspace/.abox/box.yaml", Digest: digest('1')},
		Repositories: []contract.PlanRepository{{
			Name: "repo", SourcePath: "/source/repo", BaseCommit: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Branch: "abox/TASK-1/repo", WorktreePath: "/boxes/task/workspace/repo", GitCommonDir: "/boxes/task/git/repo.git",
		}},
		RuntimeHost:      contract.RuntimeHostPlan{Driver: "lima-qemu", InstanceName: "abox-task-test", CPUMillicores: 2_000, MemoryBytes: 4 << 30, DiskBytes: 20 << 30},
		Compose:          contract.ComposePlan{ProjectName: "abox-task-test", Includes: []contract.Artifact{{Path: "/boxes/task/workspace/repo/compose.yaml", Digest: digest('2')}}, Profiles: []string{"development"}},
		WorkspaceService: contract.WorkspaceServicePlan{Name: "workspace", Repository: "repo", WorkingDirectory: "repo"},
		Mounts:           []contract.MountPlan{{Path: "/boxes/task/workspace/repo", Services: []string{"workspace"}, Writable: true}},
		Policy:           contract.PolicyPlan{GitMetadata: "read-write", DockerSocket: "deny", MachineMounts: "declared-only", NetworkEgress: "allow"},
		Tools:            contract.ToolVersions{AgentBox: "0.1.0", Git: "2.51.0", Lima: "2.0.0", Docker: "28.3.3", Compose: "2.39.1"},
		PlanDigest:       digest('a'),
	}
}

func clonePlan(t testing.TB, source contract.Plan) contract.Plan {
	t.Helper()
	encoded := mustJSON(t, source)
	var clone contract.Plan
	if err := json.Unmarshal(encoded, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func mustJSON(t testing.TB, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func digest(character byte) contract.Digest {
	value := make([]byte, 64)
	for index := range value {
		value[index] = character
	}
	return contract.Digest("sha256:" + string(value))
}
