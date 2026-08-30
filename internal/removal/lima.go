package removal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	"github.com/shoe-point-0/agent-box/internal/lima"
	"github.com/shoe-point-0/agent-box/internal/ownership"
)

// LimaRuntime is the ownership-verifying runtime-host boundary consumed by
// removal. ObserveRemoval is read-only; Delete repeats the proof immediately
// before invoking limactl.
type LimaRuntime interface {
	ObserveRemoval(context.Context, lima.Spec, ownership.LimaInstanceRecord) (lima.Observation, error)
	Delete(context.Context, lima.Spec, ownership.LimaInstanceRecord) (lima.Observation, error)
}

// LimaStepConfig binds one immutable runtime-host specification and persisted
// ownership record to its concrete Lima adapter.
type LimaStepConfig struct {
	Spec    lima.Spec
	Record  ownership.LimaInstanceRecord
	Runtime LimaRuntime
}

// LimaStep converts exact live instance state and metadata into the
// digest-bound coordinator inventory.
type LimaStep struct {
	spec    lima.Spec
	record  ownership.LimaInstanceRecord
	runtime LimaRuntime
}

// NewLimaStep rejects incomplete or inconsistent runtime-host identity before
// any Lima observation.
func NewLimaStep(config LimaStepConfig) (*LimaStep, error) {
	name := config.Spec.RuntimeHost.InstanceName
	if config.Runtime == nil || name == "" || config.Spec.BoxID == "" || config.Record.Name != name ||
		config.Record.Metadata[ownership.BoxIDFact] != config.Spec.BoxID || len(config.Record.Metadata) == 0 {
		return nil, errors.New("configure Lima removal: runtime specification, ownership record, and adapter are required")
	}
	record := config.Record
	record.Metadata = maps.Clone(config.Record.Metadata)
	return &LimaStep{spec: config.Spec, record: record, runtime: config.Runtime}, nil
}

func (s *LimaStep) Key() string { return "runtime-host/" + s.spec.RuntimeHost.InstanceName }
func (*LimaStep) Stage() Stage  { return StageRuntimeHost }

func (s *LimaStep) Observe(ctx context.Context) (Fact, error) {
	_, fact, err := s.observe(ctx)
	return fact, err
}

// Remove closes the coordinator/leaf gap by reproducing the complete live fact
// before Delete performs its own independent ownership and config proof.
func (s *LimaStep) Remove(ctx context.Context, expected Fact) error {
	_, actual, err := s.observe(ctx)
	if err != nil {
		return err
	}
	if !equalFact(actual, expected) {
		return ErrInventoryDrift
	}
	_, err = s.runtime.Delete(ctx, s.spec, s.record)
	return err
}

func (s *LimaStep) observe(ctx context.Context) (lima.Observation, Fact, error) {
	observed, err := s.runtime.ObserveRemoval(ctx, s.spec, s.record)
	if err != nil {
		return lima.Observation{}, Fact{}, fmt.Errorf("observe Lima removal: %w", err)
	}
	if observed.Name != s.spec.RuntimeHost.InstanceName {
		return lima.Observation{}, Fact{}, errors.New("observe Lima removal: instance identity differs")
	}
	if observed.State == lima.Absent {
		return observed, Fact{Present: false}, nil
	}
	if observed.State != lima.Running && observed.State != lima.Stopped {
		return lima.Observation{}, Fact{}, errors.New("observe Lima removal: instance state is not safely removable")
	}
	metadata, err := json.Marshal(observed.Metadata)
	if err != nil {
		return lima.Observation{}, Fact{}, fmt.Errorf("encode Lima removal metadata: %w", err)
	}
	return observed, Fact{Present: true, Evidence: map[string]string{
		"name": observed.Name, "state": string(observed.State), "limaStatus": observed.LimaStatus,
		"limaVersion": observed.LimaVersion, "metadata": string(metadata),
	}}, nil
}
