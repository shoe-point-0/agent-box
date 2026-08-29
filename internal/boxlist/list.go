// Package boxlist builds the read-only Box inventory from persisted identity
// and the latest cached component observations.
package boxlist

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/state"
)

const (
	StateActive   = "active"
	StateStopped  = "stopped"
	StateDegraded = "degraded"
	StateStale    = "stale"
)

type recordSource interface {
	BoxRecords(context.Context) ([]state.BoxRecord, error)
}

// Service owns no mutation capability; it can only aggregate snapshots from
// the state store. Runtime adapters can refresh those observations elsewhere.
type Service struct {
	source recordSource
}

func New(source *state.Store) (*Service, error) {
	if source == nil {
		return nil, errors.New("create Box list service: state store is nil")
	}
	return &Service{source: source}, nil
}

// List returns Boxes in stable identity order as provided by the transactional
// state snapshot.
func (s *Service) List(ctx context.Context) (contract.ListOutput, error) {
	records, err := s.source.BoxRecords(ctx)
	if err != nil {
		return contract.ListOutput{}, fmt.Errorf("list Boxes: %w", err)
	}
	boxes := make([]contract.ListBox, 0, len(records))
	for _, record := range records {
		observed, drift := summarize(record)
		boxes = append(boxes, contract.ListBox{
			ID: record.ID, WorkspaceID: record.WorkspaceID, TaskKey: record.TaskKey,
			ManifestPath: record.ManifestPath, State: observed,
			RecordedState: record.RecordedState, Drift: drift,
		})
	}
	return contract.ListOutput{SchemaVersion: contract.ListSchemaVersion, Boxes: boxes}, nil
}

func summarize(record state.BoxRecord) (string, []string) {
	var observed string
	drift := make([]string, 0, 2)
	runtimeState := normalized(record.RuntimeRecordedState)
	composeState := normalized(record.ComposeRecordedState)

	switch runtimeState {
	case "":
		if record.CurrentGeneration > 0 {
			observed = StateStale
			drift = append(drift, "runtime host record is missing for a materialized generation")
		} else {
			observed = StateStopped
		}
	case "stopped", "exited":
		if composeState == "" || composeState == "stopped" || composeState == "exited" {
			observed = StateStopped
		} else {
			observed = StateDegraded
			drift = append(drift, fmt.Sprintf("runtime host is %s while Compose is %s", runtimeState, composeState))
		}
	case "running":
		switch composeState {
		case "ready", "running", "healthy":
			observed = StateActive
		case "":
			observed = StateDegraded
			drift = append(drift, "Compose project record is missing for a running runtime host")
		default:
			observed = StateDegraded
			drift = append(drift, fmt.Sprintf("runtime host is running while Compose is %s", composeState))
		}
	default:
		observed = StateDegraded
		if composeState == "" {
			drift = append(drift, fmt.Sprintf("runtime host observation is %s", runtimeState))
		} else {
			drift = append(drift, fmt.Sprintf("runtime host is %s while Compose is %s", runtimeState, composeState))
		}
	}

	if expected := expectedState(record.RecordedState); expected != "" && expected != observed {
		drift = append(drift, fmt.Sprintf("recorded Box state %s differs from observed %s", strings.ToLower(record.RecordedState), observed))
	}
	return observed, drift
}

func normalized(value *string) string {
	if value == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(*value))
}

func expectedState(recorded string) string {
	switch strings.ToLower(strings.TrimSpace(recorded)) {
	case "ready", "running", "active":
		return StateActive
	case "planning", "planned", "stopped":
		return StateStopped
	case "degraded", "failed":
		return StateDegraded
	case "stale":
		return StateStale
	default:
		return ""
	}
}
