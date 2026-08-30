// Package up coordinates one daemon-owned Box reconciliation attempt.
package up

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/shoe-point-0/agent-box/internal/journal"
)

const (
	defaultLeaseTTL       = 30 * time.Second
	defaultRenewInterval  = 10 * time.Second
	defaultCleanupTimeout = 5 * time.Second
)

// LeaseJournal is the fenced mutation authority required by Coordinator.
type LeaseJournal interface {
	AcquireLease(context.Context, string, string, journal.Owner, time.Time, time.Duration) (journal.Lease, error)
	RenewLease(context.Context, journal.Lease, time.Time, time.Duration) (journal.Lease, error)
	ReleaseLease(context.Context, journal.Lease, time.Time) error
}

// Execute performs one already-journaled operation DAG. It must re-observe every
// external boundary and return only after READY has been durably published.
type Execute func(context.Context) error

// Config binds one persisted operation to its lease and reconcile execution.
type Config struct {
	Journal        LeaseJournal
	Operation      journal.Operation
	Owner          journal.Owner
	Execute        Execute
	LeaseTTL       time.Duration
	RenewInterval  time.Duration
	CleanupTimeout time.Duration
	Now            func() time.Time
}

// Coordinator owns the lease renewal goroutine and exact-fence release for one
// reconciliation attempt.
type Coordinator struct {
	journal        LeaseJournal
	operation      journal.Operation
	owner          journal.Owner
	execute        Execute
	leaseTTL       time.Duration
	renewInterval  time.Duration
	cleanupTimeout time.Duration
	now            func() time.Time
}

// NewCoordinator validates the complete ownership and timing contract.
func NewCoordinator(config Config) (*Coordinator, error) {
	if config.Journal == nil || config.Execute == nil {
		return nil, errors.New("configure Box up: journal and executor are required")
	}
	if config.Operation.ID == "" || config.Operation.BoxID == "" || config.Operation.Kind != "up" {
		return nil, errors.New("configure Box up: persisted up operation identity is invalid")
	}
	if config.Owner.InstanceID == "" || config.Owner.ProcessStartID == "" || config.Owner.BootID == "" {
		return nil, errors.New("configure Box up: daemon owner identity is incomplete")
	}
	leaseTTL := config.LeaseTTL
	if leaseTTL == 0 {
		leaseTTL = defaultLeaseTTL
	}
	renewInterval := config.RenewInterval
	if renewInterval == 0 {
		renewInterval = defaultRenewInterval
	}
	cleanupTimeout := config.CleanupTimeout
	if cleanupTimeout == 0 {
		cleanupTimeout = defaultCleanupTimeout
	}
	if leaseTTL <= 0 || renewInterval <= 0 || renewInterval >= leaseTTL || cleanupTimeout <= 0 {
		return nil, errors.New("configure Box up: lease and cleanup durations are invalid")
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &Coordinator{
		journal: config.Journal, operation: config.Operation, owner: config.Owner, execute: config.Execute,
		leaseTTL: leaseTTL, renewInterval: renewInterval, cleanupTimeout: cleanupTimeout, now: now,
	}, nil
}

type renewalResult struct {
	lease journal.Lease
	err   error
}

// Reconcile holds a live fenced lease for the complete DAG. Lease loss cancels
// the DAG and deliberately skips release because ownership is no longer proven.
func (c *Coordinator) Reconcile(ctx context.Context) error {
	lease, err := c.journal.AcquireLease(ctx, c.operation.BoxID, c.operation.ID, c.owner, c.now().UTC(), c.leaseTTL)
	if err != nil {
		return fmt.Errorf("reconcile Box: acquire lease: %w", err)
	}

	runContext, cancelRun := context.WithCancelCause(ctx)
	stopRenewal := make(chan struct{})
	renewalDone := make(chan renewalResult, 1)
	var stopOnce sync.Once
	stop := func() { stopOnce.Do(func() { close(stopRenewal) }) }
	go c.renew(runContext, cancelRun, stopRenewal, renewalDone, lease)

	executeErr := c.execute(runContext)
	stop()
	renewed := <-renewalDone
	cancelRun(nil)
	if renewed.err != nil {
		return errors.Join(executeErr, renewed.err)
	}

	cleanupContext, cancelCleanup := context.WithTimeout(context.WithoutCancel(ctx), c.cleanupTimeout)
	defer cancelCleanup()
	releaseErr := c.journal.ReleaseLease(cleanupContext, renewed.lease, c.now().UTC())
	if releaseErr != nil {
		releaseErr = fmt.Errorf("reconcile Box: release lease: %w", releaseErr)
	}
	return errors.Join(executeErr, releaseErr)
}

func (c *Coordinator) renew(ctx context.Context, cancel context.CancelCauseFunc, stop <-chan struct{}, done chan<- renewalResult, lease journal.Lease) {
	ticker := time.NewTicker(c.renewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			done <- renewalResult{lease: lease}
			return
		case <-ctx.Done():
			done <- renewalResult{lease: lease}
			return
		case <-ticker.C:
			renewed, err := c.journal.RenewLease(ctx, lease, c.now().UTC(), c.leaseTTL)
			if err != nil {
				renewErr := fmt.Errorf("reconcile Box: renew lease: %w", err)
				cancel(renewErr)
				done <- renewalResult{lease: lease, err: renewErr}
				return
			}
			lease = renewed
		}
	}
}
