package up

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/journal"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/state"
)

func TestReconcileComposeRetainsApplyAndFreshlyObservesWarmResources(t *testing.T) {
	attempt, spec, observation := composeReconcileFixture(t)
	authority := &memoryComposeAuthority{}
	runtime := &memoryComposeRuntime{authority: authority, observation: observation}
	config := ComposeApplyConfig{Authority: authority, Runtime: runtime, Now: monotonicNow()}

	first, err := ReconcileCompose(t.Context(), config, attempt, spec)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.applyCalls != 1 || runtime.applyMutations != 1 || runtime.observeCalls != 0 || authority.value.Records == nil {
		t.Fatalf("cold reconciliation: apply=%d/%d observe=%d authority=%#v", runtime.applyCalls, runtime.applyMutations, runtime.observeCalls, authority.value)
	}
	if !reflect.DeepEqual(first.Records, compose.RecordsFromObservation(observation)) {
		t.Fatalf("cold records = %#v", first.Records)
	}

	attempt.Operation.ID = "operation-compose-two"
	second, err := ReconcileCompose(t.Context(), config, attempt, spec)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.applyCalls != 1 || runtime.applyMutations != 1 || runtime.observeCalls != 1 ||
		authority.value.OperationID != attempt.Operation.ID || !reflect.DeepEqual(second, first) {
		t.Fatalf("warm reconciliation: result=%#v apply=%d/%d observe=%d authority=%#v", second, runtime.applyCalls, runtime.applyMutations, runtime.observeCalls, authority.value)
	}
}

func TestReconcileComposeRetriesIdempotentApplyAfterRecordPersistenceFailure(t *testing.T) {
	attempt, spec, observation := composeReconcileFixture(t)
	authority := &memoryComposeAuthority{failRecords: true}
	runtime := &memoryComposeRuntime{authority: authority, observation: observation}
	config := ComposeApplyConfig{Authority: authority, Runtime: runtime, Now: monotonicNow()}

	_, err := ReconcileCompose(t.Context(), config, attempt, spec)
	if err == nil || !strings.Contains(err.Error(), "injected resource persistence failure") {
		t.Fatalf("first reconciliation error = %v", err)
	}
	if !authority.found || authority.value.Records != nil || runtime.applyCalls != 1 || runtime.applyMutations != 1 {
		t.Fatalf("interrupted apply: authority=%#v calls=%d/%d", authority.value, runtime.applyCalls, runtime.applyMutations)
	}

	attempt.Operation.ID = "operation-compose-recovery"
	result, err := ReconcileCompose(t.Context(), config, attempt, spec)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.applyCalls != 2 || runtime.applyMutations != 1 || runtime.observeCalls != 0 ||
		result.Records.Resources[0].ID != observation.Containers[0].ID {
		t.Fatalf("recovered apply: result=%#v calls=%d/%d observe=%d", result, runtime.applyCalls, runtime.applyMutations, runtime.observeCalls)
	}
}

func TestReconcileComposeSeedsReadyAuthorityAndRefusesLiveIdentityDrift(t *testing.T) {
	attempt, spec, observation := composeReconcileFixture(t)
	records := compose.RecordsFromObservation(observation)
	attempt.Warm = true
	attempt.Current = state.RemovalSnapshot{
		BoxID: attempt.Plan.BoxID, Generation: attempt.Generation, PlanDigest: attempt.Plan.PlanDigest,
		ComposeSpec: spec, Compose: cloneRuntimeRecords(records),
	}
	authority := &memoryComposeAuthority{}
	runtime := &memoryComposeRuntime{authority: authority, observation: observation}
	config := ComposeApplyConfig{Authority: authority, Runtime: runtime}

	result, err := ReconcileCompose(t.Context(), config, attempt, spec)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.applyCalls != 0 || runtime.observeCalls != 1 || !reflect.DeepEqual(result.Records, records) {
		t.Fatalf("READY warm reconciliation: result=%#v apply=%d observe=%d", result, runtime.applyCalls, runtime.observeCalls)
	}

	runtime.observation.Containers[0].ID = "replacement-container"
	attempt.Operation.ID = "operation-compose-drift"
	_, err = ReconcileCompose(t.Context(), config, attempt, spec)
	if err == nil || !strings.Contains(err.Error(), "live runtime identities differ") {
		t.Fatalf("live identity drift error = %v", err)
	}
	if runtime.applyCalls != 0 || runtime.observeCalls != 2 {
		t.Fatalf("identity drift mutated runtime: apply=%d observe=%d", runtime.applyCalls, runtime.observeCalls)
	}
}

func composeReconcileFixture(t *testing.T) (PreparedAttempt, compose.RuntimeSpec, compose.RuntimeObservation) {
	t.Helper()
	boxID := "compose-a1b2c3d4"
	planDigest := contract.Digest("sha256:" + strings.Repeat("a", 64))
	plan := contract.Plan{
		BoxID: boxID, PlanDigest: planDigest,
		Compose: contract.ComposePlan{ProjectName: boxID},
	}
	attempt := PreparedAttempt{
		Operation:  journal.Operation{ID: "operation-compose-one", BoxID: boxID, Kind: "up", State: journal.StateRunning},
		Plan:       plan,
		Generation: 1,
		BoxRoot:    t.TempDir(),
	}
	spec := compose.RuntimeSpec{Plan: plan, Generation: 1, Workdir: attempt.BoxRoot}
	labels := map[string]string{
		"com.docker.compose.project": boxID,
		ownership.ComposeBoxIDFact:   boxID,
		"dev.abox.plan":              string(planDigest),
		"dev.abox.generation":        "1",
	}
	observation := compose.RuntimeObservation{
		ProjectName: boxID,
		Containers:  []compose.ContainerObservation{{ID: "container-one", Service: "workspace", Labels: cloneStringMap(labels)}},
		Networks:    []compose.NetworkObservation{{ID: "network-one", Labels: cloneStringMap(labels)}},
		Volumes:     []compose.VolumeObservation{{ID: "volume-one", Labels: cloneStringMap(labels)}},
	}
	return attempt, spec, observation
}

type memoryComposeAuthority struct {
	value       state.ReconciliationComposeAuthority
	found       bool
	failRecords bool
}

func (s *memoryComposeAuthority) ReconciliationComposeAuthorityFor(_ context.Context, boxID string, generation int64) (state.ReconciliationComposeAuthority, bool, error) {
	if !s.found {
		return state.ReconciliationComposeAuthority{}, false, nil
	}
	if s.value.BoxID != boxID || s.value.Generation != generation {
		return state.ReconciliationComposeAuthority{}, false, errors.New("unexpected authority lookup")
	}
	return s.value, true, nil
}

func (s *memoryComposeAuthority) SaveReconciliationComposeAuthority(_ context.Context, authority state.ReconciliationComposeAuthority, _ time.Time) error {
	if s.found {
		existing, next := s.value, authority
		existing.OperationID, next.OperationID = "", ""
		existing.Records, next.Records = nil, nil
		if !reflect.DeepEqual(existing, next) {
			return errors.New("authority identity drift")
		}
	}
	if s.failRecords && authority.Records != nil {
		s.failRecords = false
		return errors.New("injected resource persistence failure")
	}
	s.value, s.found = authority, true
	return nil
}

type memoryComposeRuntime struct {
	authority                  *memoryComposeAuthority
	observation                compose.RuntimeObservation
	created                    bool
	applyCalls, applyMutations int
	observeCalls               int
}

func (r *memoryComposeRuntime) Apply(_ context.Context, _ compose.RuntimeSpec) (compose.RuntimeObservation, error) {
	r.applyCalls++
	if !r.authority.found || r.authority.value.Records != nil {
		return compose.RuntimeObservation{}, errors.New("Compose apply preceded durable intent")
	}
	if !r.created {
		r.created = true
		r.applyMutations++
	}
	return r.observation, nil
}

func (r *memoryComposeRuntime) Observe(_ context.Context, _ compose.RuntimeSpec) (compose.RuntimeObservation, error) {
	r.observeCalls++
	return r.observation, nil
}
