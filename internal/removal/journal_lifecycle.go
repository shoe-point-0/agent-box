package removal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/shoe-point-0/agent-box/internal/journal"
)

const defaultLeaseTTL = 30 * time.Second

// LeaseJournal is the durable fenced-lease projection required by removal.
// The concrete operation journal remains the owner of lease transactions.
type LeaseJournal interface {
	AcquireLease(context.Context, string, string, journal.Owner, time.Time, time.Duration) (journal.Lease, error)
	RenewLease(context.Context, journal.Lease, time.Time, time.Duration) (journal.Lease, error)
	ReleaseLease(context.Context, journal.Lease, time.Time) error
}

// JournalLeaseGuardConfig binds removal to one already-running daemon operation.
type JournalLeaseGuardConfig struct {
	Journal   LeaseJournal
	Operation journal.Operation
	Owner     journal.Owner
	TTL       time.Duration
	Now       func() time.Time
}

// JournalLeaseGuard holds and renews one fenced Box lease for the complete
// removal execution. Lease loss cancels the execution context before another
// destructive step can begin.
type JournalLeaseGuard struct {
	journal     LeaseJournal
	operationID string
	boxID       string
	owner       journal.Owner
	ttl         time.Duration
	now         func() time.Time

	mu     sync.Mutex
	active *leaseLifecycle
}

type leaseLifecycle struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
	done   chan struct{}

	mu      sync.Mutex
	lease   journal.Lease
	failure error
}

func (l Lease) executionContext(parent context.Context) context.Context {
	if l.state == nil {
		return parent
	}
	return l.state.ctx
}

// NewJournalLeaseGuard validates operation ownership before any lease write.
func NewJournalLeaseGuard(config JournalLeaseGuardConfig) (*JournalLeaseGuard, error) {
	if config.Journal == nil || config.Operation.ID == "" || config.Operation.BoxID == "" ||
		config.Operation.State != journal.StateRunning || config.Owner.InstanceID == "" ||
		config.Owner.ProcessStartID == "" || config.Owner.BootID == "" {
		return nil, errors.New("configure removal lease: running operation, journal, and owner are required")
	}
	ttl := config.TTL
	if ttl == 0 {
		ttl = defaultLeaseTTL
	}
	if ttl <= 0 {
		return nil, errors.New("configure removal lease: TTL must be positive")
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &JournalLeaseGuard{
		journal: config.Journal, operationID: config.Operation.ID, boxID: config.Operation.BoxID,
		owner: config.Owner, ttl: ttl, now: now,
	}, nil
}

func (g *JournalLeaseGuard) Acquire(ctx context.Context, boxID string) (Lease, error) {
	if boxID != g.boxID {
		return Lease{}, errors.New("acquire removal lease: Box differs from operation")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active != nil {
		return Lease{}, errors.New("acquire removal lease: guard already holds a lease")
	}

	persisted, err := g.journal.AcquireLease(ctx, boxID, g.operationID, g.owner, g.now().UTC(), g.ttl)
	if err != nil {
		return Lease{}, err
	}
	executionContext, cancel := context.WithCancelCause(ctx)
	lifecycle := &leaseLifecycle{
		ctx: executionContext, cancel: cancel, done: make(chan struct{}), lease: persisted,
	}
	g.active = lifecycle
	go g.renew(lifecycle)
	return Lease{
		BoxID: boxID,
		Token: g.operationID + ":" + strconv.FormatInt(persisted.Fence, 10),
		state: lifecycle,
	}, nil
}

func (g *JournalLeaseGuard) Release(ctx context.Context, lease Lease) error {
	g.mu.Lock()
	lifecycle := g.active
	if lifecycle == nil || lease.state != lifecycle || lease.BoxID != g.boxID {
		g.mu.Unlock()
		return errors.New("release removal lease: lease is not held by this guard")
	}
	g.active = nil
	g.mu.Unlock()

	lifecycle.cancel(nil)
	<-lifecycle.done
	persisted, renewalErr := lifecycle.snapshot()
	releaseErr := g.journal.ReleaseLease(ctx, persisted, g.now().UTC())
	return errors.Join(renewalErr, releaseErr)
}

func (g *JournalLeaseGuard) renew(lifecycle *leaseLifecycle) {
	defer close(lifecycle.done)
	interval := g.ttl / 3
	if interval <= 0 {
		interval = time.Nanosecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-lifecycle.ctx.Done():
			return
		case <-ticker.C:
			current, _ := lifecycle.snapshot()
			renewed, err := g.journal.RenewLease(lifecycle.ctx, current, g.now().UTC(), g.ttl)
			if err != nil {
				lifecycle.fail(fmt.Errorf("renew removal lease: %w", err))
				return
			}
			lifecycle.mu.Lock()
			lifecycle.lease = renewed
			lifecycle.mu.Unlock()
		}
	}
}

func (l *leaseLifecycle) snapshot() (journal.Lease, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.lease, l.failure
}

func (l *leaseLifecycle) fail(err error) {
	l.mu.Lock()
	if l.failure == nil {
		l.failure = err
	}
	l.mu.Unlock()
	l.cancel(err)
}

// AppendEventFunc adapts the durable operation journal without exposing it to
// the policy coordinator.
type AppendEventFunc func(context.Context, string, string, json.RawMessage, time.Time) error

// EventRecorder persists removal transitions in the owning operation stream.
type EventRecorder struct {
	operationID string
	appendEvent AppendEventFunc
	now         func() time.Time
}

// NewEventRecorder creates an operation-scoped durable removal recorder.
func NewEventRecorder(operationID string, appendEvent AppendEventFunc, now func() time.Time) (*EventRecorder, error) {
	if operationID == "" || appendEvent == nil {
		return nil, errors.New("configure removal recorder: operation ID and append function are required")
	}
	if now == nil {
		now = time.Now
	}
	return &EventRecorder{operationID: operationID, appendEvent: appendEvent, now: now}, nil
}

func (r *EventRecorder) Record(ctx context.Context, transition Transition) error {
	switch transition.Kind {
	case TransitionRemoveStarted, TransitionRemoveSucceeded, TransitionRemoveFailed:
	default:
		return errors.New("record removal transition: invalid kind")
	}
	if !validOpaque(transition.Resource) {
		return errors.New("record removal transition: invalid resource")
	}
	details, err := json.Marshal(struct {
		Resource string `json:"resource"`
	}{Resource: transition.Resource})
	if err != nil {
		return fmt.Errorf("encode removal transition: %w", err)
	}
	if err := r.appendEvent(ctx, r.operationID, "removal."+transition.Kind, details, r.now().UTC()); err != nil {
		return fmt.Errorf("append removal transition: %w", err)
	}
	return nil
}
