package tcpallocator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/shoe-point-0/agent-box/internal/journal"
)

type storedAllocation struct {
	BoxID       string
	Name        string
	Service     string
	TargetPort  int
	MachinePort int
	ListenerID  string
	OperationID string
	LeaseFence  int64
	Ready       bool
}

// Recorded returns the stable database projection for diagnostics and recovery.
// It does not imply that a listener or target is live.
func (m *Manager) Recorded(ctx context.Context, boxID string) ([]Allocation, error) {
	if boxID == "" {
		return nil, errors.New("read TCP allocations: Box ID is empty")
	}
	records, err := m.readBox(ctx, boxID)
	if err != nil {
		return nil, err
	}
	allocations := make([]Allocation, 0, len(records))
	for _, record := range records {
		allocations = append(allocations, Allocation{
			BoxID: record.BoxID, Name: record.Name, Service: record.Service,
			TargetPort: record.TargetPort, Host: loopbackHost,
			Port: record.MachinePort, Ready: record.Ready,
		})
	}
	return allocations, nil
}

func (m *Manager) readOne(ctx context.Context, key allocationKey) (record storedAllocation, found bool, err error) {
	err = m.store.View(ctx, func(tx *sql.Tx) error {
		var ready int
		err := tx.QueryRowContext(ctx, `SELECT
			box_id, name, service, target_port, machine_port, listener_id,
			operation_id, lease_fence, ready
			FROM port_allocations WHERE box_id = ? AND name = ?`, key.boxID, key.name).Scan(
			&record.BoxID, &record.Name, &record.Service, &record.TargetPort,
			&record.MachinePort, &record.ListenerID, &record.OperationID,
			&record.LeaseFence, &ready,
		)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("query TCP allocation: %w", err)
		}
		record.Ready = ready == 1
		found = true
		return nil
	})
	if err != nil {
		return storedAllocation{}, false, fmt.Errorf("read TCP allocation: %w", err)
	}
	return record, found, nil
}

func (m *Manager) readBox(ctx context.Context, boxID string) (records []storedAllocation, err error) {
	err = m.store.View(ctx, func(tx *sql.Tx) (returnErr error) {
		rows, err := tx.QueryContext(ctx, `SELECT
			box_id, name, service, target_port, machine_port, listener_id,
			operation_id, lease_fence, ready
			FROM port_allocations WHERE box_id = ? ORDER BY name`, boxID)
		if err != nil {
			return fmt.Errorf("query Box TCP allocations: %w", err)
		}
		defer func() { returnErr = errors.Join(returnErr, rows.Close()) }()
		for rows.Next() {
			var record storedAllocation
			var ready int
			if err := rows.Scan(
				&record.BoxID, &record.Name, &record.Service, &record.TargetPort,
				&record.MachinePort, &record.ListenerID, &record.OperationID,
				&record.LeaseFence, &ready,
			); err != nil {
				return fmt.Errorf("scan Box TCP allocation: %w", err)
			}
			record.Ready = ready == 1
			records = append(records, record)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("read Box TCP allocations: %w", err)
	}
	return records, nil
}

func (m *Manager) insert(ctx context.Context, lease journal.Lease, allocation Allocation, listenerID string) (reserved bool, err error) {
	err = m.store.Update(ctx, func(tx *sql.Tx) error {
		now := m.now().UTC()
		if err := verifyLease(ctx, tx, lease, now.UnixNano()); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRowContext(ctx,
			`SELECT count(*) FROM port_allocations WHERE machine_port = ?`, allocation.Port,
		).Scan(&count); err != nil {
			return fmt.Errorf("check retained Machine port: %w", err)
		}
		if count != 0 {
			return nil
		}
		if err := tx.QueryRowContext(ctx,
			`SELECT count(*) FROM port_allocations WHERE box_id = ? AND name = ?`,
			allocation.BoxID, allocation.Name,
		).Scan(&count); err != nil {
			return fmt.Errorf("check retained TCP endpoint: %w", err)
		}
		if count != 0 {
			return ErrConflict
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO port_allocations
			(box_id, name, service, target_port, machine_port, listener_id,
			 operation_id, lease_fence, ready, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?)`,
			allocation.BoxID, allocation.Name, allocation.Service, allocation.TargetPort,
			allocation.Port, listenerID, lease.OperationID, lease.Fence, now.UnixNano(),
		); err != nil {
			return fmt.Errorf("persist TCP allocation: %w", err)
		}
		reserved = true
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("allocate TCP endpoint: %w", err)
	}
	return reserved, nil
}

func (m *Manager) recoverPersist(ctx context.Context, lease journal.Lease, record storedAllocation, listenerID string, ready bool) error {
	return m.store.Update(ctx, func(tx *sql.Tx) error {
		now := m.now().UTC()
		if err := verifyLease(ctx, tx, lease, now.UnixNano()); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE port_allocations SET
			listener_id = ?, operation_id = ?, lease_fence = ?, ready = ?, updated_at = ?
			WHERE box_id = ? AND name = ? AND service = ? AND target_port = ? AND machine_port = ?`,
			listenerID, lease.OperationID, lease.Fence, boolInteger(ready), now.UnixNano(),
			record.BoxID, record.Name, record.Service, record.TargetPort, record.MachinePort,
		)
		if err != nil {
			return fmt.Errorf("persist recovered TCP allocation: %w", err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return ErrConflict
		}
		return nil
	})
}

func (m *Manager) adoptLocked(ctx context.Context, lease journal.Lease, active *activeAllocation) error {
	if err := validateLease(lease); err != nil {
		return fmt.Errorf("adopt TCP allocation: %w", err)
	}
	err := m.store.Update(ctx, func(tx *sql.Tx) error {
		now := m.now().UTC()
		if err := verifyLease(ctx, tx, lease, now.UnixNano()); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE port_allocations SET
			operation_id = ?, lease_fence = ?, updated_at = ?
			WHERE box_id = ? AND name = ? AND listener_id = ?`,
			lease.OperationID, lease.Fence, now.UnixNano(), active.allocation.BoxID,
			active.allocation.Name, active.listenerID,
		)
		if err != nil {
			return fmt.Errorf("persist TCP listener adoption: %w", err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("adopt TCP allocation: %w", err)
	}
	active.leaseID = leaseIdentity(lease)
	return nil
}

func (m *Manager) setReady(ctx context.Context, lease journal.Lease, key allocationKey, listenerID string, ready bool) error {
	return m.store.Update(ctx, func(tx *sql.Tx) error {
		now := m.now().UTC()
		if err := verifyLease(ctx, tx, lease, now.UnixNano()); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE port_allocations
			SET ready = ?, updated_at = ?
			WHERE box_id = ? AND name = ? AND listener_id = ?`,
			boolInteger(ready), now.UnixNano(), key.boxID, key.name, listenerID,
		)
		if err != nil {
			return fmt.Errorf("persist TCP readiness: %w", err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return ErrNotFound
		}
		return nil
	})
}

func (m *Manager) delete(ctx context.Context, lease journal.Lease, key allocationKey, listenerID string) (removed bool, err error) {
	err = m.store.Update(ctx, func(tx *sql.Tx) error {
		if err := verifyLease(ctx, tx, lease, m.now().UTC().UnixNano()); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM port_allocations
			WHERE box_id = ? AND name = ? AND listener_id = ?
			  AND operation_id = ? AND lease_fence = ?`,
			key.boxID, key.name, listenerID, lease.OperationID, lease.Fence,
		)
		if err != nil {
			return fmt.Errorf("delete TCP allocation: %w", err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		removed = changed == 1
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("remove TCP allocation: %w", err)
	}
	return removed, nil
}

func verifyLease(ctx context.Context, tx *sql.Tx, lease journal.Lease, now int64) error {
	var count int
	err := tx.QueryRowContext(ctx, `SELECT count(*) FROM leases
		WHERE box_id = ? AND operation_id = ? AND owner_id = ?
		  AND process_start_id = ? AND boot_id = ? AND fencing_token = ?
		  AND released_at IS NULL AND expires_at > ?`,
		lease.BoxID, lease.OperationID, lease.Owner.InstanceID,
		lease.Owner.ProcessStartID, lease.Owner.BootID, lease.Fence, now,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("verify TCP allocation Box lease: %w", err)
	}
	if count != 1 {
		return ErrLeaseLost
	}
	return nil
}

func boolInteger(value bool) int {
	if value {
		return 1
	}
	return 0
}
