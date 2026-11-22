package up

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/generatedstate"
	"github.com/shoe-point-0/agent-box/internal/journal"
	"github.com/shoe-point-0/agent-box/internal/lima"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/state"
	"github.com/shoe-point-0/agent-box/internal/testutil"
	"github.com/shoe-point-0/agent-box/internal/toolchain"
)

func TestReconcileLimaResumesConfigAndInstanceWithoutDuplicateApply(t *testing.T) {
	fixture := newLimaReconcileFixture(t)
	authority := &memoryLimaAuthority{failConfig: true}
	runtime := &memoryLimaRuntime{spec: fixture.spec}
	writes := 0
	config := LimaConfig{
		Authority: authority, Runtime: runtime, Artifact: fixture.artifact, Spec: fixture.spec, Now: monotonicNow(),
		WriteConfig: func(path string, spec lima.Spec) error {
			writes++
			return lima.WriteConfig(path, spec)
		},
	}

	_, err := ReconcileLima(t.Context(), config, fixture.attempt("operation-one"), fixture.generated)
	if err == nil || !strings.Contains(err.Error(), "injected config persistence failure") {
		t.Fatalf("first reconciliation error = %v", err)
	}
	if writes != 1 || runtime.createCalls != 0 {
		t.Fatalf("config interruption writes=%d create-calls=%d", writes, runtime.createCalls)
	}

	authority.failInstance = true
	_, err = ReconcileLima(t.Context(), config, fixture.attempt("operation-two"), fixture.generated)
	if err == nil || !strings.Contains(err.Error(), "injected instance persistence failure") {
		t.Fatalf("second reconciliation error = %v", err)
	}
	if writes != 1 || runtime.createCalls != 1 || runtime.createMutations != 1 || runtime.startMutations != 0 {
		t.Fatalf("instance interruption writes=%d create=%d/%d start=%d", writes, runtime.createCalls, runtime.createMutations, runtime.startMutations)
	}

	result, err := ReconcileLima(t.Context(), config, fixture.attempt("operation-three"), fixture.generated)
	if err != nil {
		t.Fatal(err)
	}
	if result.Observation.State != lima.Running || result.Instance.Name != fixture.spec.RuntimeHost.InstanceName ||
		writes != 1 || runtime.createCalls != 2 || runtime.createMutations != 1 || runtime.startMutations != 1 {
		t.Fatalf("resumed result=%#v writes=%d create=%d/%d start=%d", result, writes, runtime.createCalls, runtime.createMutations, runtime.startMutations)
	}

	warm, err := ReconcileLima(t.Context(), config, fixture.attempt("operation-four"), fixture.generated)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(warm, result) || writes != 1 || runtime.createCalls != 2 || runtime.createMutations != 1 || runtime.startMutations != 1 {
		t.Fatalf("warm reconciliation mutated: result=%#v writes=%d create=%d/%d start=%d", warm, writes, runtime.createCalls, runtime.createMutations, runtime.startMutations)
	}
}

func TestReconcileLimaRefusesConfigReplacementBeforeRuntimeCall(t *testing.T) {
	fixture := newLimaReconcileFixture(t)
	authority := &memoryLimaAuthority{}
	runtime := &memoryLimaRuntime{spec: fixture.spec}
	config := LimaConfig{Authority: authority, Runtime: runtime, Artifact: fixture.artifact, Spec: fixture.spec}
	if _, err := ReconcileLima(t.Context(), config, fixture.attempt("operation-one"), fixture.generated); err != nil {
		t.Fatal(err)
	}
	calls := runtime.totalCalls()

	moved := fixture.configPath + ".owned"
	if err := os.Rename(fixture.configPath, moved); err != nil {
		t.Fatal(err)
	}
	foreign := []byte("preserve: foreign\n")
	if err := os.WriteFile(fixture.configPath, foreign, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ReconcileLima(t.Context(), config, fixture.attempt("operation-two"), fixture.generated)
	if err == nil || !strings.Contains(err.Error(), "prove generated config") {
		t.Fatalf("replacement error = %v", err)
	}
	if runtime.totalCalls() != calls {
		t.Fatal("config replacement reached the runtime boundary")
	}
	if got, readErr := os.ReadFile(fixture.configPath); readErr != nil || !reflect.DeepEqual(got, foreign) {
		t.Fatalf("foreign config changed: %q, %v", got, readErr)
	}
}

type limaReconcileFixture struct {
	plan       contract.Plan
	spec       lima.Spec
	artifact   toolchain.Identity
	boxRoot    string
	configPath string
	generated  GeneratedPlanResult
}

func newLimaReconcileFixture(t *testing.T) limaReconcileFixture {
	t.Helper()
	root := testutil.TempDir(t)
	boxesRoot := filepath.Join(root, "application", "boxes")
	boxRoot := filepath.Join(boxesRoot, "task-a1b2c3d4")
	mount := filepath.Join(boxRoot, "workspace", "repo")
	if err := os.MkdirAll(mount, 0o700); err != nil {
		t.Fatal(err)
	}
	image := filepath.Join(root, "guest.img")
	if err := os.WriteFile(image, []byte("guest image\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	planDigest := contract.Digest("sha256:" + strings.Repeat("a", 64))
	guestDigest := contract.Digest("sha256:" + strings.Repeat("b", 64))
	artifact := toolchain.Identity{Component: toolchain.Lima, Version: lima.SupportedVersion, Digest: contract.Digest("sha256:" + strings.Repeat("c", 64))}
	plan := contract.Plan{
		BoxID: "task-a1b2c3d4", PlanDigest: planDigest,
		RuntimeHost: contract.RuntimeHostPlan{Driver: "lima-vz", InstanceName: "task-a1b2c3d4", CPUMillicores: 2000, MemoryBytes: 4 << 30, DiskBytes: 20 << 30},
		Mounts:      []contract.MountPlan{{Path: mount, Writable: true}},
	}
	spec := lima.Spec{
		RuntimeHost: plan.RuntimeHost, Mounts: plan.Mounts, BoxID: plan.BoxID, PlanDigest: plan.PlanDigest, Generation: 1,
		GuestImage: toolchain.Identity{Component: toolchain.GuestImage, Version: "fixture", Digest: guestDigest}, ImagePath: image,
	}
	var err error
	spec.PathEvidence, err = lima.RecordRuntimePaths(spec)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := ownership.InspectPath(boxRoot)
	if err != nil {
		t.Fatal(err)
	}
	generated := GeneratedPlanResult{BoxRoot: generatedstate.Record{
		BoxID: plan.BoxID, Root: ownership.PathRecord{CanonicalPath: boxRoot, Identity: identity},
	}}
	return limaReconcileFixture{
		plan: plan, spec: spec, artifact: artifact, boxRoot: boxRoot,
		configPath: filepath.Join(boxRoot, "lima.yaml"), generated: generated,
	}
}

func (f limaReconcileFixture) attempt(operationID string) PreparedAttempt {
	return PreparedAttempt{
		Operation: journal.Operation{ID: operationID, BoxID: f.plan.BoxID, Kind: "up", State: journal.StateRunning},
		Plan:      f.plan, Generation: 1, BoxRoot: f.boxRoot, LimaConfigPath: f.configPath,
	}
}

type memoryLimaAuthority struct {
	value                    state.ReconciliationLimaAuthority
	found                    bool
	failConfig, failInstance bool
}

func (s *memoryLimaAuthority) ReconciliationLimaAuthorityFor(_ context.Context, boxID string, generation int64) (state.ReconciliationLimaAuthority, bool, error) {
	if !s.found {
		return state.ReconciliationLimaAuthority{}, false, nil
	}
	if s.value.BoxID != boxID || s.value.Generation != generation {
		return state.ReconciliationLimaAuthority{}, false, errors.New("unexpected Lima authority lookup")
	}
	return s.value, true, nil
}

func (s *memoryLimaAuthority) SaveReconciliationLimaAuthority(_ context.Context, authority state.ReconciliationLimaAuthority, _ time.Time) error {
	if s.failConfig && authority.ConfigEvidence != nil && (!s.found || s.value.ConfigEvidence == nil) {
		s.failConfig = false
		return errors.New("injected config persistence failure")
	}
	if s.failInstance && authority.Instance != nil && (!s.found || s.value.Instance == nil) {
		s.failInstance = false
		return errors.New("injected instance persistence failure")
	}
	if s.found {
		existing, next := s.value, authority
		existing.OperationID, next.OperationID = "", ""
		existing.ConfigEvidence, existing.Instance, next.ConfigEvidence, next.Instance = nil, nil, nil, nil
		if !reflect.DeepEqual(existing, next) {
			return errors.New("Lima authority identity drift")
		}
	}
	s.value, s.found = authority, true
	return nil
}

type memoryLimaRuntime struct {
	spec                         lima.Spec
	state                        lima.State
	createCalls, createMutations int
	inspectCalls                 int
	startCalls, startMutations   int
}

func (r *memoryLimaRuntime) Inspect(_ context.Context, spec lima.Spec) (lima.Observation, error) {
	r.inspectCalls++
	if !reflect.DeepEqual(spec, r.spec) {
		return lima.Observation{}, errors.New("runtime Spec drift")
	}
	return r.observation(), nil
}

func (r *memoryLimaRuntime) Create(_ context.Context, spec lima.Spec, path string, evidence ownership.PathSetRecord) (lima.Observation, error) {
	r.createCalls++
	if !reflect.DeepEqual(spec, r.spec) || path == "" {
		return lima.Observation{}, errors.New("runtime create identity drift")
	}
	if err := ownership.VerifyPathSet([]string{path}, evidence); err != nil {
		return lima.Observation{}, err
	}
	if r.state == lima.Absent || r.state == "" {
		r.state = lima.Stopped
		r.createMutations++
	}
	return r.observation(), nil
}

func (r *memoryLimaRuntime) Start(_ context.Context, spec lima.Spec) (lima.Observation, error) {
	r.startCalls++
	if !reflect.DeepEqual(spec, r.spec) || r.state == lima.Absent || r.state == "" {
		return lima.Observation{}, errors.New("runtime host is absent")
	}
	if r.state != lima.Running {
		r.state = lima.Running
		r.startMutations++
	}
	return r.observation(), nil
}

func (r *memoryLimaRuntime) observation() lima.Observation {
	state := r.state
	if state == "" {
		state = lima.Absent
	}
	metadata := map[string]string(nil)
	if state != lima.Absent {
		metadata = map[string]string{
			ownership.BoxIDFact:    r.spec.BoxID,
			"dev.abox.plan-digest": string(r.spec.PlanDigest),
			"dev.abox.generation":  "1",
		}
	}
	return lima.Observation{Name: r.spec.RuntimeHost.InstanceName, State: state, Metadata: metadata, LimaVersion: lima.SupportedVersion}
}

func (r *memoryLimaRuntime) totalCalls() int {
	return r.createCalls + r.inspectCalls + r.startCalls
}
