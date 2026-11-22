// Package journal persists daemon-owned mutation operations, their event streams,
// and fenced Box leases.
package journal

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/state"
)

const (
	StateRunning   = "running"
	StateSucceeded = contract.OperationSucceeded
	StateFailed    = contract.OperationFailed

	defaultPollInterval = 25 * time.Millisecond
	streamBuffer        = 16
	maxOpaqueBytes      = 512
)

var (
	ErrNotFound            = errors.New("journal record not found")
	ErrIdempotencyConflict = errors.New("idempotency key conflicts with its recorded operation")
	ErrOperationActive     = errors.New("a different mutation operation is already active")
	ErrLeaseHeld           = errors.New("box lease is held by another owner")
	ErrLeaseLost           = errors.New("box lease ownership was lost")
)

// Journal owns operation, event, and lease transactions over one state store.
type Journal struct {
	store        *state.Store
	pollInterval time.Duration
}

// New constructs a Journal. The Store remains owned by the caller.
func New(store *state.Store) (*Journal, error) {
	if store == nil {
		return nil, errors.New("create operation journal: state store is nil")
	}
	return &Journal{store: store, pollInterval: defaultPollInterval}, nil
}

// BeginRequest contains the immutable identity and request facts persisted before
// any operation side effect.
type BeginRequest struct {
	WorkspaceID    identity.WorkspaceID
	BoxID          identity.BoxID
	TaskKey        string
	ManifestPath   string
	ManifestDigest string
	OperationID    string
	IdempotencyKey string
	Kind           string
	RequestDigest  string
	StartedAt      time.Time
}

// Operation is one persisted daemon-owned mutation attempt.
type Operation struct {
	ID             string
	BoxID          string
	IdempotencyKey string
	Kind           string
	RequestDigest  string
	State          string
	StartedAt      time.Time
	FinishedAt     *time.Time
	ErrorCode      string
}

// Begin atomically establishes the Workspace and Box and creates one running
// operation. A duplicate idempotency key with identical immutable request facts
// reattaches to the recorded operation.
func (j *Journal) Begin(ctx context.Context, request BeginRequest) (operation Operation, created bool, err error) {
	if err := validateBegin(request); err != nil {
		return Operation{}, false, fmt.Errorf("begin operation: %w", err)
	}

	err = j.store.Update(ctx, func(tx *sql.Tx) error {
		now := request.StartedAt.UnixNano()
		if _, err := tx.ExecContext(ctx, `INSERT INTO workspaces
			(id, manifest_path, manifest_digest, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				manifest_digest = excluded.manifest_digest,
				updated_at = excluded.updated_at`,
			request.WorkspaceID.String(), request.ManifestPath, request.ManifestDigest, now, now); err != nil {
			return fmt.Errorf("establish workspace: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO boxes
			(id, workspace_id, task_key, recorded_state, created_at, updated_at)
			VALUES (?, ?, ?, 'planning', ?, ?)
			ON CONFLICT(workspace_id, task_key) DO NOTHING`,
			request.BoxID.String(), request.WorkspaceID.String(), request.TaskKey, now, now); err != nil {
			return fmt.Errorf("establish Box: %w", err)
		}

		var boxID string
		if err := tx.QueryRowContext(ctx,
			`SELECT id FROM boxes WHERE workspace_id = ? AND task_key = ?`,
			request.WorkspaceID.String(), request.TaskKey).Scan(&boxID); err != nil {
			return fmt.Errorf("read established Box: %w", err)
		}
		if boxID != request.BoxID.String() {
			return errors.New("deterministic Box identity conflicts with recorded state")
		}

		recorded, found, err := operationByIdempotencyKey(ctx, tx, boxID, request.IdempotencyKey)
		if err != nil {
			return err
		}
		if found {
			if recorded.Kind != request.Kind || recorded.RequestDigest != request.RequestDigest {
				return ErrIdempotencyConflict
			}
			operation = recorded
			return nil
		}

		active, found, err := activeOperation(ctx, tx, boxID)
		if err != nil {
			return err
		}
		if found {
			operation = active
			return ErrOperationActive
		}

		if _, err := tx.ExecContext(ctx, `INSERT INTO operations
			(id, box_id, idempotency_key, kind, request_digest, recorded_state, started_at)
			VALUES (?, ?, ?, ?, ?, 'running', ?)`,
			request.OperationID, boxID, request.IdempotencyKey, request.Kind,
			request.RequestDigest, now); err != nil {
			return fmt.Errorf("persist operation: %w", err)
		}
		operation = Operation{
			ID: request.OperationID, BoxID: boxID, IdempotencyKey: request.IdempotencyKey,
			Kind: request.Kind, RequestDigest: request.RequestDigest,
			State: StateRunning, StartedAt: request.StartedAt,
		}
		created = true
		return nil
	})
	if err != nil {
		return Operation{}, false, fmt.Errorf("begin operation: %w", err)
	}
	return operation, created, nil
}

// Complete commits the running operation's terminal state and last event together.
// Failure leaves both unpublished; an already completed operation cannot be finished again.
func (j *Journal) Complete(ctx context.Context, operationID string, result contract.OperationResult, finishedAt time.Time) error {
	if err := validateOpaque("operation ID", operationID); err != nil {
		return fmt.Errorf("complete operation: %w", err)
	}
	if err := result.Validate(); err != nil {
		return fmt.Errorf("complete operation: %w", err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode operation result: %w", err)
	}
	details, err := validateDetails(encoded)
	if err != nil {
		return fmt.Errorf("complete operation: %w", err)
	}
	err = j.store.Update(ctx, func(tx *sql.Tx) error {
		var boxID string
		err := tx.QueryRowContext(ctx, `UPDATE operations
			SET recorded_state = ?, finished_at = ?, error_code = NULLIF(?, '')
			WHERE id = ? AND recorded_state = 'running' RETURNING box_id`,
			result.State, finishedAt.UnixNano(), result.ErrorCode, operationID).Scan(&boxID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		_, err = appendEventTx(ctx, tx, boxID, operationID, contract.OperationFinished, details, finishedAt)
		return err
	})
	if err != nil {
		return fmt.Errorf("complete operation: %w", err)
	}
	return nil
}

// Event is a durable, monotonically sequenced operation event.
type Event struct {
	BoxID       string
	OperationID string
	Sequence    int64
	Kind        string
	Details     json.RawMessage
	OccurredAt  time.Time
}

// AppendEvent adds a nonterminal event to a running operation. Only Complete can finish it.
func (j *Journal) AppendEvent(ctx context.Context, operationID, kind string, details json.RawMessage, occurredAt time.Time) (event Event, err error) {
	if err := validateOpaque("operation ID", operationID); err != nil {
		return Event{}, fmt.Errorf("append operation event: %w", err)
	}
	if err := validateOpaque("event kind", kind); err != nil {
		return Event{}, fmt.Errorf("append operation event: %w", err)
	}
	if kind == contract.OperationFinished {
		return Event{}, errors.New("append operation event: terminal event requires completion")
	}
	details, err = validateDetails(details)
	if err != nil {
		return Event{}, fmt.Errorf("append operation event: %w", err)
	}

	err = j.store.Update(ctx, func(tx *sql.Tx) error {
		var boxID string
		if err := tx.QueryRowContext(ctx, `SELECT box_id FROM operations WHERE id = ? AND recorded_state = 'running'`, operationID).
			Scan(&boxID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("read event operation: %w", err)
		}
		var appendErr error
		event, appendErr = appendEventTx(ctx, tx, boxID, operationID, kind, details, occurredAt)
		return appendErr
	})
	if err != nil {
		return Event{}, fmt.Errorf("append operation event: %w", err)
	}
	return event, nil
}

func appendEventTx(ctx context.Context, tx *sql.Tx, boxID, operationID, kind string, details json.RawMessage, occurredAt time.Time) (Event, error) {
	var sequence int64
	err := tx.QueryRowContext(ctx, `INSERT INTO operation_events
		(box_id, operation_id, sequence, kind, details_json, occurred_at)
		SELECT ?, ?, COALESCE(MAX(sequence), 0) + 1, ?, ?, ? FROM operation_events WHERE operation_id = ?
		RETURNING sequence`, boxID, operationID, kind, string(details), occurredAt.UnixNano(), operationID).Scan(&sequence)
	if err != nil {
		return Event{}, fmt.Errorf("persist operation event: %w", err)
	}
	return Event{BoxID: boxID, OperationID: operationID, Sequence: sequence, Kind: kind, Details: details, OccurredAt: occurredAt}, nil
}

// Events returns durable events after a sequence in stable ascending order.
func (j *Journal) Events(ctx context.Context, operationID string, afterSequence int64) ([]Event, error) {
	if err := validateOpaque("operation ID", operationID); err != nil {
		return nil, fmt.Errorf("read operation events: %w", err)
	}
	if afterSequence < 0 {
		return nil, errors.New("read operation events: sequence is negative")
	}
	var events []Event
	err := j.store.View(ctx, func(tx *sql.Tx) (returnErr error) {
		rows, err := tx.QueryContext(ctx, `SELECT box_id, operation_id, sequence, kind, details_json, occurred_at
			FROM operation_events WHERE operation_id = ? AND sequence > ? ORDER BY sequence`,
			operationID, afterSequence)
		if err != nil {
			return fmt.Errorf("query operation events: %w", err)
		}
		defer func() { returnErr = errors.Join(returnErr, rows.Close()) }()
		for rows.Next() {
			var event Event
			var details string
			var occurredAt int64
			if err := rows.Scan(&event.BoxID, &event.OperationID, &event.Sequence,
				&event.Kind, &details, &occurredAt); err != nil {
				return fmt.Errorf("scan operation event: %w", err)
			}
			event.Details = json.RawMessage(details)
			event.OccurredAt = time.Unix(0, occurredAt).UTC()
			events = append(events, event)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("read operation events: %w", err)
	}
	return events, nil
}

// Operation returns one persisted operation by its stable identity. It is used
// by transports that must stop an attached event stream only after the durable
// terminal state is visible.
func (j *Journal) Operation(ctx context.Context, operationID string) (Operation, error) {
	if err := validateOpaque("operation ID", operationID); err != nil {
		return Operation{}, fmt.Errorf("read operation: %w", err)
	}
	var operation Operation
	var found bool
	err := j.store.View(ctx, func(tx *sql.Tx) error {
		var err error
		operation, found, err = scanOperation(tx.QueryRowContext(ctx, `SELECT id, box_id, idempotency_key, kind,
			request_digest, recorded_state, started_at, finished_at, COALESCE(error_code, '')
			FROM operations WHERE id = ?`, operationID))
		return err
	})
	if err != nil {
		return Operation{}, fmt.Errorf("read operation: %w", err)
	}
	if !found {
		return Operation{}, fmt.Errorf("read operation: %w", ErrNotFound)
	}
	return operation, nil
}

// StreamItem contains either one event or a terminal stream error.
type StreamItem struct {
	Event Event
	Err   error
}

// Subscribe replays persisted events and polls for new ones. Canceling ctx only
// detaches this subscriber; no operation cancellation capability is retained.
func (j *Journal) Subscribe(ctx context.Context, operationID string, afterSequence int64) <-chan StreamItem {
	items := make(chan StreamItem, streamBuffer)
	go func() {
		defer close(items)
		sequence := afterSequence
		for {
			events, err := j.Events(ctx, operationID, sequence)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					sendStreamItem(ctx, items, StreamItem{Err: err})
				}
				return
			}
			for _, event := range events {
				if !sendStreamItem(ctx, items, StreamItem{Event: event}) {
					return
				}
				sequence = event.Sequence
			}
			timer := time.NewTimer(j.pollInterval)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return
			case <-timer.C:
			}
		}
	}()
	return items
}

// Owner identifies one daemon incarnation without relying on PID identity.
type Owner struct {
	InstanceID     string
	ProcessStartID string
	BootID         string
}

// CurrentOwner creates a random daemon instance identity bound to this process
// start and the current boot/session identity.
func CurrentOwner() (Owner, error) {
	instanceID, err := randomID("daemon")
	if err != nil {
		return Owner{}, fmt.Errorf("create daemon owner: %w", err)
	}
	processStartID, bootID, err := currentProcessIdentity()
	if err != nil {
		return Owner{}, fmt.Errorf("create daemon owner: %w", err)
	}
	return Owner{InstanceID: instanceID, ProcessStartID: processStartID, BootID: bootID}, nil
}

// Lease is the current fenced right to mutate one Box.
type Lease struct {
	BoxID       string
	OperationID string
	Owner       Owner
	Fence       int64
	AcquiredAt  time.Time
	RenewedAt   time.Time
	ExpiresAt   time.Time
	ReleasedAt  *time.Time
}

// AcquireLease creates or reacquires one Box lease. An unexpired unreleased
// lease can only be re-read by its exact owner and operation.
func (j *Journal) AcquireLease(ctx context.Context, boxID, operationID string, owner Owner, now time.Time, ttl time.Duration) (lease Lease, err error) {
	if err := validateLeaseInput(boxID, operationID, owner, now, ttl); err != nil {
		return Lease{}, fmt.Errorf("acquire Box lease: %w", err)
	}
	expiresAt := now.Add(ttl)
	err = j.store.Update(ctx, func(tx *sql.Tx) error {
		current, found, err := readLease(ctx, tx, boxID)
		if err != nil {
			return err
		}
		if !found {
			if _, err := tx.ExecContext(ctx, `INSERT INTO leases
				(box_id, operation_id, owner_id, process_start_id, boot_id, fencing_token,
				 acquired_at, renewed_at, expires_at, released_at)
				VALUES (?, ?, ?, ?, ?, 1, ?, ?, ?, NULL)`,
				boxID, operationID, owner.InstanceID, owner.ProcessStartID, owner.BootID,
				now.UnixNano(), now.UnixNano(), expiresAt.UnixNano()); err != nil {
				return fmt.Errorf("persist Box lease: %w", err)
			}
			lease = Lease{BoxID: boxID, OperationID: operationID, Owner: owner, Fence: 1,
				AcquiredAt: now, RenewedAt: now, ExpiresAt: expiresAt}
			return nil
		}
		if current.ReleasedAt == nil && current.ExpiresAt.After(now) {
			if current.OperationID == operationID && current.Owner == owner {
				lease = current
				return nil
			}
			return ErrLeaseHeld
		}
		fence := current.Fence + 1
		if _, err := tx.ExecContext(ctx, `UPDATE leases SET
			operation_id = ?, owner_id = ?, process_start_id = ?, boot_id = ?,
			fencing_token = ?, acquired_at = ?, renewed_at = ?, expires_at = ?, released_at = NULL
			WHERE box_id = ?`, operationID, owner.InstanceID, owner.ProcessStartID,
			owner.BootID, fence, now.UnixNano(), now.UnixNano(), expiresAt.UnixNano(), boxID); err != nil {
			return fmt.Errorf("reacquire Box lease: %w", err)
		}
		lease = Lease{BoxID: boxID, OperationID: operationID, Owner: owner, Fence: fence,
			AcquiredAt: now, RenewedAt: now, ExpiresAt: expiresAt}
		return nil
	})
	if err != nil {
		return Lease{}, fmt.Errorf("acquire Box lease: %w", err)
	}
	return lease, nil
}

// RenewLease extends an exact live fenced lease.
func (j *Journal) RenewLease(ctx context.Context, lease Lease, now time.Time, ttl time.Duration) (Lease, error) {
	if err := validateLeaseInput(lease.BoxID, lease.OperationID, lease.Owner, now, ttl); err != nil || lease.Fence < 1 {
		if err == nil {
			err = errors.New("fencing token must be positive")
		}
		return Lease{}, fmt.Errorf("renew Box lease: %w", err)
	}
	expiresAt := now.Add(ttl)
	changed, err := j.execUpdate(ctx, `UPDATE leases SET renewed_at = ?, expires_at = ?
		WHERE box_id = ? AND operation_id = ? AND owner_id = ? AND process_start_id = ?
		AND boot_id = ? AND fencing_token = ? AND released_at IS NULL AND expires_at > ?`,
		now.UnixNano(), expiresAt.UnixNano(), lease.BoxID, lease.OperationID,
		lease.Owner.InstanceID, lease.Owner.ProcessStartID, lease.Owner.BootID,
		lease.Fence, now.UnixNano())
	if err != nil {
		return Lease{}, fmt.Errorf("renew Box lease: %w", err)
	}
	if changed == 0 {
		return Lease{}, fmt.Errorf("renew Box lease: %w", ErrLeaseLost)
	}
	lease.RenewedAt = now
	lease.ExpiresAt = expiresAt
	return lease, nil
}

// ReleaseLease relinquishes an exact live fenced lease while preserving its
// fence so the next owner receives a strictly greater token.
func (j *Journal) ReleaseLease(ctx context.Context, lease Lease, now time.Time) error {
	if err := validateOwner(lease.Owner); err != nil || lease.Fence < 1 {
		if err == nil {
			err = errors.New("fencing token must be positive")
		}
		return fmt.Errorf("release Box lease: %w", err)
	}
	changed, err := j.execUpdate(ctx, `UPDATE leases SET released_at = ?
		WHERE box_id = ? AND operation_id = ? AND owner_id = ? AND process_start_id = ?
		AND boot_id = ? AND fencing_token = ? AND released_at IS NULL AND expires_at > ?`,
		now.UnixNano(), lease.BoxID, lease.OperationID, lease.Owner.InstanceID,
		lease.Owner.ProcessStartID, lease.Owner.BootID, lease.Fence, now.UnixNano())
	if err != nil {
		return fmt.Errorf("release Box lease: %w", err)
	}
	if changed == 0 {
		return fmt.Errorf("release Box lease: %w", ErrLeaseLost)
	}
	return nil
}

func (j *Journal) execUpdate(ctx context.Context, query string, arguments ...any) (int64, error) {
	var changed int64
	err := j.store.Update(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, query, arguments...)
		if err != nil {
			return err
		}
		changed, err = result.RowsAffected()
		return err
	})
	return changed, err
}

func operationByIdempotencyKey(ctx context.Context, tx *sql.Tx, boxID, key string) (Operation, bool, error) {
	return scanOperation(tx.QueryRowContext(ctx, `SELECT id, box_id, idempotency_key, kind,
		request_digest, recorded_state, started_at, finished_at, COALESCE(error_code, '')
		FROM operations WHERE box_id = ? AND idempotency_key = ?`, boxID, key))
}

func activeOperation(ctx context.Context, tx *sql.Tx, boxID string) (Operation, bool, error) {
	return scanOperation(tx.QueryRowContext(ctx, `SELECT id, box_id, idempotency_key, kind,
		request_digest, recorded_state, started_at, finished_at, COALESCE(error_code, '')
		FROM operations WHERE box_id = ? AND recorded_state = 'running'`, boxID))
}

func scanOperation(row *sql.Row) (Operation, bool, error) {
	var operation Operation
	var startedAt int64
	var finishedAt sql.NullInt64
	err := row.Scan(&operation.ID, &operation.BoxID, &operation.IdempotencyKey,
		&operation.Kind, &operation.RequestDigest, &operation.State, &startedAt,
		&finishedAt, &operation.ErrorCode)
	if errors.Is(err, sql.ErrNoRows) {
		return Operation{}, false, nil
	}
	if err != nil {
		return Operation{}, false, fmt.Errorf("read operation: %w", err)
	}
	operation.StartedAt = time.Unix(0, startedAt).UTC()
	if finishedAt.Valid {
		value := time.Unix(0, finishedAt.Int64).UTC()
		operation.FinishedAt = &value
	}
	return operation, true, nil
}

func readLease(ctx context.Context, tx *sql.Tx, boxID string) (Lease, bool, error) {
	var lease Lease
	var acquiredAt, renewedAt, expiresAt int64
	var releasedAt sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT box_id, operation_id, owner_id, process_start_id,
		boot_id, fencing_token, acquired_at, renewed_at, expires_at, released_at
		FROM leases WHERE box_id = ?`, boxID).Scan(&lease.BoxID, &lease.OperationID,
		&lease.Owner.InstanceID, &lease.Owner.ProcessStartID, &lease.Owner.BootID,
		&lease.Fence, &acquiredAt, &renewedAt, &expiresAt, &releasedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Lease{}, false, nil
	}
	if err != nil {
		return Lease{}, false, fmt.Errorf("read Box lease: %w", err)
	}
	lease.AcquiredAt = time.Unix(0, acquiredAt).UTC()
	lease.RenewedAt = time.Unix(0, renewedAt).UTC()
	lease.ExpiresAt = time.Unix(0, expiresAt).UTC()
	if releasedAt.Valid {
		value := time.Unix(0, releasedAt.Int64).UTC()
		lease.ReleasedAt = &value
	}
	return lease, true, nil
}

func validateBegin(request BeginRequest) error {
	workspaceID, err := identity.Workspace(request.ManifestPath)
	if err != nil {
		return err
	}
	if workspaceID != request.WorkspaceID {
		return errors.New("workspace ID does not match manifest path")
	}
	boxID, err := identity.Box(request.WorkspaceID, request.TaskKey)
	if err != nil {
		return err
	}
	if boxID != request.BoxID {
		return errors.New("box ID does not match Workspace and task")
	}
	for name, value := range map[string]string{
		"manifest digest": request.ManifestDigest,
		"operation ID":    request.OperationID,
		"idempotency key": request.IdempotencyKey,
		"operation kind":  request.Kind,
		"request digest":  request.RequestDigest,
	} {
		if err := validateOpaque(name, value); err != nil {
			return err
		}
	}
	return nil
}

func validateLeaseInput(boxID, operationID string, owner Owner, now time.Time, ttl time.Duration) error {
	if err := validateOpaque("Box ID", boxID); err != nil {
		return err
	}
	if err := validateOpaque("operation ID", operationID); err != nil {
		return err
	}
	if err := validateOwner(owner); err != nil {
		return err
	}
	if ttl <= 0 {
		return errors.New("lease TTL must be positive")
	}
	if !now.Add(ttl).After(now) {
		return errors.New("lease TTL overflows timestamp")
	}
	return nil
}

func validateOwner(owner Owner) error {
	for name, value := range map[string]string{
		"daemon instance ID": owner.InstanceID,
		"process start ID":   owner.ProcessStartID,
		"boot ID":            owner.BootID,
	} {
		if err := validateOpaque(name, value); err != nil {
			return err
		}
	}
	return nil
}

func validateOpaque(name, value string) error {
	if value == "" {
		return fmt.Errorf("%s is empty", name)
	}
	if len(value) > maxOpaqueBytes {
		return fmt.Errorf("%s exceeds %d bytes", name, maxOpaqueBytes)
	}
	if strings.IndexByte(value, 0) >= 0 {
		return fmt.Errorf("%s contains NUL", name)
	}
	return nil
}

func validateDetails(details json.RawMessage) (json.RawMessage, error) {
	if len(details) == 0 {
		return json.RawMessage("{}"), nil
	}
	var document any
	if err := json.Unmarshal(details, &document); err != nil {
		return nil, fmt.Errorf("event details are invalid JSON: %w", err)
	}
	if _, ok := document.(map[string]any); !ok {
		return nil, errors.New("event details must be a JSON object")
	}
	if sensitiveField(document) {
		return nil, errors.New("event details contain a sensitive field")
	}
	canonical, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode event details: %w", err)
	}
	return canonical, nil
}

func sensitiveField(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.NewReplacer("-", "_", ".", "_").Replace(key))
			switch normalized {
			case "authorization", "credential", "credentials", "password", "private_key", "secret", "token", "api_key":
				return true
			}
			if sensitiveField(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if sensitiveField(child) {
				return true
			}
		}
	}
	return false
}

func sendStreamItem(ctx context.Context, items chan<- StreamItem, item StreamItem) bool {
	select {
	case items <- item:
		return true
	case <-ctx.Done():
		return false
	}
}

func randomID(prefix string) (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return prefix + "-" + hex.EncodeToString(bytes[:]), nil
}

var processSessionStart = time.Now().UTC().Format(time.RFC3339Nano)

func currentProcessIdentity() (processStartID, bootID string, err error) {
	if runtime.GOOS != "linux" {
		return "pid:" + strconv.Itoa(os.Getpid()) + ":started:" + processSessionStart,
			"session:" + processSessionStart, nil
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", "", fmt.Errorf("read Linux boot ID: %w", err)
	}
	stat, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return "", "", fmt.Errorf("read Linux process start: %w", err)
	}
	closing := strings.LastIndexByte(string(stat), ')')
	if closing < 0 {
		return "", "", errors.New("parse Linux process start: missing command delimiter")
	}
	fields := strings.Fields(string(stat)[closing+1:])
	if len(fields) <= 19 {
		return "", "", errors.New("parse Linux process start: incomplete stat record")
	}
	bootID = strings.TrimSpace(string(boot))
	if bootID == "" {
		return "", "", errors.New("read Linux boot ID: empty value")
	}
	return "pid:" + strconv.Itoa(os.Getpid()) + ":ticks:" + fields[19], bootID, nil
}
