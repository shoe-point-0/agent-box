package up

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/guesttoolchain"
	"github.com/shoe-point-0/agent-box/internal/journal"
	"github.com/shoe-point-0/agent-box/internal/lima"
)

func TestReconcileGuestReverifiesFreshRuntimeOnEveryAttempt(t *testing.T) {
	runtimeHost := lima.Spec{
		RuntimeHost: contract.RuntimeHostPlan{InstanceName: "box-runtime"},
		BoxID:       "box-runtime",
	}
	attempt := PreparedAttempt{
		Operation: journal.Operation{ID: "operation-guest", BoxID: "box-runtime"},
		Plan:      contract.Plan{BoxID: "box-runtime"},
	}
	foundation := FoundationResult{Lima: LimaResult{Spec: runtimeHost, Observation: lima.Observation{State: lima.Running}}}
	spec := guesttoolchain.Spec{RuntimeHost: runtimeHost, DockerPath: "/usr/bin/docker"}
	provisioner := &recordingGuestProvisioner{observation: guesttoolchain.Observation{DockerVersion: "29.6.1"}}

	for range 2 {
		result, err := ReconcileGuest(t.Context(), GuestConfig{Provisioner: provisioner, Spec: spec}, attempt, foundation)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(result.Spec, spec) || result.Observation.DockerVersion != "29.6.1" {
			t.Fatalf("guest result = %#v", result)
		}
	}
	if provisioner.calls != 2 {
		t.Fatalf("guest provision calls = %d, want 2", provisioner.calls)
	}
}

func TestReconcileGuestRefusesDriftAndPropagatesProvisionFailure(t *testing.T) {
	runtimeHost := lima.Spec{RuntimeHost: contract.RuntimeHostPlan{InstanceName: "box-runtime"}, BoxID: "box-runtime"}
	attempt := PreparedAttempt{Operation: journal.Operation{ID: "operation-guest", BoxID: "box-runtime"}, Plan: contract.Plan{BoxID: "box-runtime"}}
	foundation := FoundationResult{Lima: LimaResult{Spec: runtimeHost, Observation: lima.Observation{State: lima.Running}}}
	provisioner := &recordingGuestProvisioner{}

	drifted := guesttoolchain.Spec{RuntimeHost: runtimeHost}
	drifted.RuntimeHost.RuntimeHost.InstanceName = "foreign-runtime"
	if _, err := ReconcileGuest(t.Context(), GuestConfig{Provisioner: provisioner, Spec: drifted}, attempt, foundation); err == nil || provisioner.calls != 0 {
		t.Fatalf("drift error = %v, provision calls = %d", err, provisioner.calls)
	}

	wantErr := errors.New("injected guest failure")
	provisioner.err = wantErr
	if _, err := ReconcileGuest(t.Context(), GuestConfig{Provisioner: provisioner, Spec: guesttoolchain.Spec{RuntimeHost: runtimeHost}}, attempt, foundation); !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "provision") {
		t.Fatalf("provision error = %v", err)
	}
}

type recordingGuestProvisioner struct {
	calls       int
	observation guesttoolchain.Observation
	err         error
}

func (p *recordingGuestProvisioner) Provision(_ context.Context, _ guesttoolchain.Spec) (guesttoolchain.Observation, error) {
	p.calls++
	return p.observation, p.err
}
