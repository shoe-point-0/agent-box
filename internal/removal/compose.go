package removal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strconv"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/ownership"
)

const ownedVolumePolicy = "delete-with-explicit-volume-authorization"

// ComposeRuntime is the concrete guest Compose lifecycle boundary consumed by
// removal. Observe is read-only; Remove performs its own fresh ownership proof.
type ComposeRuntime interface {
	Observe(context.Context, compose.RuntimeSpec) (compose.RuntimeObservation, error)
	Remove(context.Context, compose.RuntimeSpec, compose.RuntimeRecords) (compose.RuntimeObservation, error)
}

// ComposeStepConfig binds one immutable runtime specification and persisted
// resource inventory to removal policy.
type ComposeStepConfig struct {
	Spec    compose.RuntimeSpec
	Records compose.RuntimeRecords
	Runtime ComposeRuntime
}

// ComposeStep converts exact container, network, volume, label, and volume-loss
// observations into the digest-bound coordinator inventory.
type ComposeStep struct {
	spec    compose.RuntimeSpec
	records compose.RuntimeRecords
	runtime ComposeRuntime
}

// NewComposeStep rejects incomplete project identity or lifecycle wiring before
// any guest observation.
func NewComposeStep(config ComposeStepConfig) (*ComposeStep, error) {
	if config.Runtime == nil || config.Spec.Plan.Compose.ProjectName == "" ||
		config.Spec.Compiled.ProjectName != config.Spec.Plan.Compose.ProjectName {
		return nil, errors.New("configure Compose removal: runtime specification and adapter are required")
	}
	return &ComposeStep{spec: config.Spec, records: cloneRuntimeRecords(config.Records), runtime: config.Runtime}, nil
}

func (s *ComposeStep) Key() string { return "compose/" + s.spec.Plan.Compose.ProjectName }
func (*ComposeStep) Stage() Stage  { return StageCompose }

func (s *ComposeStep) Observe(ctx context.Context) (Fact, error) {
	_, fact, err := s.observe(ctx)
	return fact, err
}

// Remove closes the coordinator/leaf gap by reproducing the complete fact
// before the Runtime performs its own independent live ownership proof.
func (s *ComposeStep) Remove(ctx context.Context, expected Fact) error {
	_, actual, err := s.observe(ctx)
	if err != nil {
		return err
	}
	if !equalFact(actual, expected) {
		return ErrInventoryDrift
	}
	_, err = s.runtime.Remove(ctx, s.spec, s.records)
	return err
}

func (s *ComposeStep) observe(ctx context.Context) (compose.RuntimeObservation, Fact, error) {
	observed, err := s.runtime.Observe(ctx, s.spec)
	if err != nil {
		return compose.RuntimeObservation{}, Fact{}, fmt.Errorf("observe Compose removal: %w", err)
	}
	if observed.ProjectName != s.spec.Plan.Compose.ProjectName {
		return compose.RuntimeObservation{}, Fact{}, errors.New("observe Compose removal: project identity differs")
	}
	if err := compose.VerifyRemovalRecords(s.records, observed); err != nil {
		return compose.RuntimeObservation{}, Fact{}, fmt.Errorf("observe Compose removal ownership: %w", err)
	}
	if len(observed.Containers)+len(observed.Networks)+len(observed.Volumes) == 0 {
		return observed, Fact{Present: false}, nil
	}

	evidence := map[string]string{
		"project":      observed.ProjectName,
		"volumePolicy": ownedVolumePolicy,
	}
	for index, container := range observed.Containers {
		value, encodeErr := composeEvidence(container.ID, container.Name, "", container.Labels)
		if encodeErr != nil {
			return compose.RuntimeObservation{}, Fact{}, encodeErr
		}
		evidence["container."+strconv.Itoa(index)] = value
	}
	for index, network := range observed.Networks {
		value, encodeErr := composeEvidence(network.ID, network.Name, "", network.Labels)
		if encodeErr != nil {
			return compose.RuntimeObservation{}, Fact{}, encodeErr
		}
		evidence["network."+strconv.Itoa(index)] = value
	}
	losses := make([]Loss, 0, len(observed.Volumes))
	for index, volume := range observed.Volumes {
		value, encodeErr := composeEvidence(volume.ID, volume.Name, volume.Driver, volume.Labels)
		if encodeErr != nil {
			return compose.RuntimeObservation{}, Fact{}, encodeErr
		}
		evidence["volume."+strconv.Itoa(index)] = value
		losses = append(losses, Loss{Kind: LossVolume, Value: volume.ID})
	}
	return observed, Fact{Present: true, Evidence: evidence, Losses: losses}, nil
}

func composeEvidence(id, name, driver string, labels map[string]string) (string, error) {
	encoded, err := json.Marshal(struct {
		ID     string            `json:"id"`
		Name   string            `json:"name"`
		Driver string            `json:"driver,omitempty"`
		Labels map[string]string `json:"labels"`
	}{ID: id, Name: name, Driver: driver, Labels: labels})
	if err != nil {
		return "", fmt.Errorf("encode Compose removal evidence: %w", err)
	}
	return string(encoded), nil
}

func cloneRuntimeRecords(records compose.RuntimeRecords) compose.RuntimeRecords {
	cloned := compose.RuntimeRecords{
		Resources: make([]ownership.ComposeResourceRecord, len(records.Resources)),
		Volumes:   make([]ownership.VolumeRecord, len(records.Volumes)),
	}
	for index, record := range records.Resources {
		record.Labels = maps.Clone(record.Labels)
		cloned.Resources[index] = record
	}
	for index, record := range records.Volumes {
		record.Labels = maps.Clone(record.Labels)
		cloned.Volumes[index] = record
	}
	return cloned
}
