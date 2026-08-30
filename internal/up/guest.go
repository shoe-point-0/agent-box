package up

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/shoe-point-0/agent-box/internal/guesttoolchain"
	"github.com/shoe-point-0/agent-box/internal/lima"
)

// GuestProvisioner verifies the pinned guest artifacts and runtime before any
// repository-owned Compose command may execute.
type GuestProvisioner interface {
	Provision(context.Context, guesttoolchain.Spec) (guesttoolchain.Observation, error)
}

var _ GuestProvisioner = (*guesttoolchain.Provisioner)(nil)

// GuestConfig binds one freshly reconciled runtime host to the pinned guest
// toolchain. Spec is rebuilt for every attempt rather than retaining Box state
// in a shared provisioner.
type GuestConfig struct {
	Provisioner GuestProvisioner
	Spec        guesttoolchain.Spec
}

// GuestResult carries the exact verified guest paths into Compose and
// filesystem adapter construction.
type GuestResult struct {
	Spec        guesttoolchain.Spec
	Observation guesttoolchain.Observation
}

// ReconcileGuest reruns the complete idempotent guest verification on cold and
// warm attempts. Successful history is never substituted for live tool and
// Docker observations.
func ReconcileGuest(ctx context.Context, config GuestConfig, attempt PreparedAttempt, foundation FoundationResult) (GuestResult, error) {
	if config.Provisioner == nil {
		return GuestResult{}, errors.New("reconcile guest toolchain: provisioner is required")
	}
	if attempt.Operation.ID == "" || attempt.Plan.BoxID != attempt.Operation.BoxID ||
		foundation.Lima.Observation.State != lima.Running ||
		!reflect.DeepEqual(config.Spec.RuntimeHost, foundation.Lima.Spec) {
		return GuestResult{}, errors.New("reconcile guest toolchain: fresh runtime-host identity is incomplete")
	}
	observation, err := config.Provisioner.Provision(ctx, config.Spec)
	if err != nil {
		return GuestResult{}, fmt.Errorf("reconcile guest toolchain: provision: %w", err)
	}
	return GuestResult{Spec: config.Spec, Observation: observation}, nil
}
