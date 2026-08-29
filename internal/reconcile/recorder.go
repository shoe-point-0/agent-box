package reconcile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// AppendEventFunc adapts the durable operation journal without coupling the DAG
// package to its concrete storage implementation.
type AppendEventFunc func(context.Context, string, string, json.RawMessage, time.Time) error

// EventRecorder stores reconcile transitions in one journal operation stream.
type EventRecorder struct {
	operationID string
	appendEvent AppendEventFunc
	now         func() time.Time
}

// NewEventRecorder creates a persistent recorder. now is injected so tests and
// callers can provide a stable UTC clock boundary.
func NewEventRecorder(operationID string, appendEvent AppendEventFunc, now func() time.Time) (*EventRecorder, error) {
	if operationID == "" {
		return nil, errors.New("create reconcile event recorder: operation ID is empty")
	}
	if appendEvent == nil {
		return nil, errors.New("create reconcile event recorder: append event function is nil")
	}
	if now == nil {
		return nil, errors.New("create reconcile event recorder: clock is nil")
	}
	return &EventRecorder{operationID: operationID, appendEvent: appendEvent, now: now}, nil
}

// Record persists only stable transition metadata. Operation-owned provider
// state is intentionally never serialized here.
func (r *EventRecorder) Record(ctx context.Context, transition Transition) error {
	details, err := json.Marshal(struct {
		Key     string `json:"operationKey"`
		Created bool   `json:"created"`
	}{Key: transition.Key, Created: transition.Created})
	if err != nil {
		return fmt.Errorf("encode reconcile event: %w", err)
	}
	if err := r.appendEvent(ctx, r.operationID, "reconcile."+transition.Kind, details, r.now().UTC()); err != nil {
		return fmt.Errorf("append reconcile event: %w", err)
	}
	return nil
}
