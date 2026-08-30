// Package tcpallocator owns stable Machine-loopback TCP listeners and proxies
// their streams to Box targets selected by service name and port.
package tcpallocator

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/journal"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/state"
)

const loopbackHost = "127.0.0.1"

var (
	// ErrExhausted means every configured Machine port is already bound or retained.
	ErrExhausted = errors.New("TCP allocation range exhausted")
	// ErrNotFound means the selected allocation is neither retained nor active.
	ErrNotFound = errors.New("TCP allocation not found")
	// ErrConflict means a stable allocation exists for different target intent.
	ErrConflict = errors.New("TCP allocation target conflicts with retained state")
	// ErrLeaseLost means the supplied fenced Box lease is not currently active.
	ErrLeaseLost = errors.New("TCP allocation Box lease was lost")
	// ErrTargetUnreachable means the target could not be reached before publication.
	ErrTargetUnreachable = errors.New("TCP allocation target is unreachable")
)

// PortOccupiedError reports a recorded port that another listener owns after a
// daemon restart. Recovery never silently moves a stable endpoint.
type PortOccupiedError struct{ Port int }

func (e *PortOccupiedError) Error() string {
	return fmt.Sprintf("recover TCP allocation: recorded Machine port %d is occupied", e.Port)
}

// Range is the inclusive unprivileged Machine port allocation range.
type Range struct {
	First int
	Last  int
}

// Target is the immutable Box endpoint behind one Machine listener.
type Target struct {
	BoxID   string
	Service string
	Port    int
}

// Dialer opens one stream to a live Box target. A Lima-backed implementation
// can own the guest transport without changing Machine listener ownership.
type Dialer interface {
	DialContext(context.Context, Target) (net.Conn, error)
}

// DialerFunc adapts a function to Dialer.
type DialerFunc func(context.Context, Target) (net.Conn, error)

func (f DialerFunc) DialContext(ctx context.Context, target Target) (net.Conn, error) {
	return f(ctx, target)
}

// Endpoint is one named Plan endpoint to allocate for a Box.
type Endpoint struct {
	Name       string
	Service    string
	TargetPort int
}

// Allocation is a context-safe projection. Ready is true only after a fresh
// target connection succeeded while the exact Machine listener remained bound.
type Allocation struct {
	BoxID      string
	Name       string
	Service    string
	TargetPort int
	Host       string
	Port       int
	Ready      bool
}

// Context returns the endpoint projection accepted by context generation.
func (a Allocation) Context() contract.PortContext {
	return contract.PortContext{Name: a.Name, Host: a.Host, Port: a.Port, Ready: a.Ready}
}

// Config defines bounded network behavior.
type Config struct {
	Range        Range
	ProbeTimeout time.Duration
	DialTimeout  time.Duration
}

// Manager owns every listener, accepted connection, and proxy goroutine it
// creates. Closing a Manager releases sockets but deliberately preserves rows
// so a successor daemon can recover the exact ports.
type Manager struct {
	store *state.Store
	dial  Dialer
	cfg   Config
	now   func() time.Time

	mu     sync.Mutex
	active map[allocationKey]*activeAllocation
	closed bool
	wg     sync.WaitGroup
}

type allocationKey struct {
	boxID string
	name  string
}

type activeAllocation struct {
	allocation  Allocation
	target      Target
	listener    net.Listener
	listenerID  string
	leaseID     string
	connections map[net.Conn]struct{}
}

// New constructs an empty daemon listener registry. The Store and Dialer remain
// caller-owned; Manager.Close must run before either dependency is released.
func New(store *state.Store, dialer Dialer, cfg Config) (*Manager, error) {
	if store == nil || dialer == nil {
		return nil, errors.New("create TCP allocator: state store and target dialer are required")
	}
	if cfg.Range.First < 1024 || cfg.Range.Last > 65535 || cfg.Range.First > cfg.Range.Last {
		return nil, errors.New("create TCP allocator: range must be an ordered unprivileged port range")
	}
	if cfg.ProbeTimeout <= 0 || cfg.DialTimeout <= 0 {
		return nil, errors.New("create TCP allocator: probe and dial timeouts must be positive")
	}
	return &Manager{
		store: store, dial: dialer, cfg: cfg, now: time.Now,
		active: make(map[allocationKey]*activeAllocation),
	}, nil
}

// Allocate returns the stable allocation for endpoint. It binds the Machine
// listener first, proves target reachability, persists under the exact live
// lease, and only then starts serving and returns the endpoint.
func (m *Manager) Allocate(ctx context.Context, lease journal.Lease, endpoint Endpoint) (Allocation, error) {
	if err := validateRequest(lease, endpoint); err != nil {
		return Allocation{}, fmt.Errorf("allocate TCP endpoint: %w", err)
	}
	key := allocationKey{boxID: lease.BoxID, name: endpoint.Name}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return Allocation{}, errors.New("allocate TCP endpoint: allocator is closed")
	}
	if active := m.active[key]; active != nil {
		if active.target.Service != endpoint.Service || active.target.Port != endpoint.TargetPort {
			return Allocation{}, ErrConflict
		}
		if err := m.adoptLocked(ctx, lease, active); err != nil {
			return Allocation{}, err
		}
		ready := m.probe(ctx, active.target) == nil
		if err := m.setReady(ctx, lease, key, active.listenerID, ready); err != nil {
			return Allocation{}, err
		}
		active.allocation.Ready = ready
		if !ready {
			return Allocation{}, ErrTargetUnreachable
		}
		return active.allocation, nil
	}

	recorded, found, err := m.readOne(ctx, key)
	if err != nil {
		return Allocation{}, err
	}
	if found {
		if recorded.Service != endpoint.Service || recorded.TargetPort != endpoint.TargetPort {
			return Allocation{}, ErrConflict
		}
		active, err := m.recoverRecordLocked(ctx, lease, recorded, true)
		if err != nil {
			return Allocation{}, err
		}
		return active.allocation, nil
	}

	target := Target{BoxID: lease.BoxID, Service: endpoint.Service, Port: endpoint.TargetPort}
	for port := m.cfg.Range.First; port <= m.cfg.Range.Last; port++ {
		listener, err := net.Listen("tcp4", net.JoinHostPort(loopbackHost, strconv.Itoa(port)))
		if err != nil {
			continue
		}
		if err := m.probe(ctx, target); err != nil {
			_ = listener.Close()
			return Allocation{}, fmt.Errorf("allocate TCP endpoint %q: %w: %v", endpoint.Name, ErrTargetUnreachable, err)
		}
		listenerID, err := randomListenerID()
		if err != nil {
			_ = listener.Close()
			return Allocation{}, err
		}
		allocation := Allocation{
			BoxID: lease.BoxID, Name: endpoint.Name, Service: endpoint.Service,
			TargetPort: endpoint.TargetPort, Host: loopbackHost, Port: port, Ready: true,
		}
		reserved, err := m.insert(ctx, lease, allocation, listenerID)
		if err != nil {
			_ = listener.Close()
			return Allocation{}, err
		}
		if !reserved {
			_ = listener.Close()
			continue
		}
		active := &activeAllocation{
			allocation: allocation, target: target, listener: listener,
			listenerID: listenerID, leaseID: leaseIdentity(lease),
			connections: make(map[net.Conn]struct{}),
		}
		m.activateLocked(key, active)
		return allocation, nil
	}
	return Allocation{}, ErrExhausted
}

// Recover rebinds every recorded allocation for lease.BoxID to its exact port.
// A stopped target leaves the listener retained and Ready false; an occupied
// recorded port fails closed instead of changing the context-visible endpoint.
func (m *Manager) Recover(ctx context.Context, lease journal.Lease) ([]Allocation, error) {
	if err := validateLease(lease); err != nil {
		return nil, fmt.Errorf("recover TCP allocations: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, errors.New("recover TCP allocations: allocator is closed")
	}
	records, err := m.readBox(ctx, lease.BoxID)
	if err != nil {
		return nil, err
	}
	result := make([]Allocation, 0, len(records))
	for _, record := range records {
		key := allocationKey{boxID: record.BoxID, name: record.Name}
		if active := m.active[key]; active != nil {
			if err := m.adoptLocked(ctx, lease, active); err != nil {
				return nil, err
			}
			result = append(result, active.allocation)
			continue
		}
		active, err := m.recoverRecordLocked(ctx, lease, record, false)
		if err != nil {
			return nil, err
		}
		result = append(result, active.allocation)
	}
	return result, nil
}

// Refresh probes one retained target and atomically updates publication state.
func (m *Manager) Refresh(ctx context.Context, lease journal.Lease, name string) (Allocation, error) {
	if err := validateName(name); err != nil {
		return Allocation{}, fmt.Errorf("refresh TCP allocation: %w", err)
	}
	key := allocationKey{boxID: lease.BoxID, name: name}
	m.mu.Lock()
	defer m.mu.Unlock()
	active := m.active[key]
	if active == nil {
		return Allocation{}, ErrNotFound
	}
	if err := m.adoptLocked(ctx, lease, active); err != nil {
		return Allocation{}, err
	}
	ready := m.probe(ctx, active.target) == nil
	if err := m.setReady(ctx, lease, key, active.listenerID, ready); err != nil {
		return Allocation{}, err
	}
	active.allocation.Ready = ready
	return active.allocation, nil
}

// Remove releases only boxID/name after adopting the exact listener under the
// current fenced lease and proving its retained identity.
func (m *Manager) Remove(ctx context.Context, lease journal.Lease, name string) error {
	if err := validateName(name); err != nil {
		return fmt.Errorf("remove TCP allocation: %w", err)
	}
	key := allocationKey{boxID: lease.BoxID, name: name}
	m.mu.Lock()
	defer m.mu.Unlock()
	active := m.active[key]
	if active == nil {
		return ErrNotFound
	}
	if err := m.adoptLocked(ctx, lease, active); err != nil {
		return err
	}
	recorded := ownership.ListenerRecord{
		Network: "tcp4", Address: active.listener.Addr().String(),
		ListenerID: active.listenerID, LeaseID: active.leaseID,
	}
	live := ownership.ListenerObservation{
		Network: "tcp4", Address: active.listener.Addr().String(),
		ListenerID: active.listenerID, LeaseID: leaseIdentity(lease), LeaseActive: true,
	}
	if _, err := ownership.ProveListener(recorded, live); err != nil {
		return fmt.Errorf("remove TCP allocation: %w", err)
	}
	removed, err := m.delete(ctx, lease, key, active.listenerID)
	if err != nil {
		return err
	}
	if !removed {
		return ErrNotFound
	}
	m.deactivateLocked(key, active)
	return nil
}

// Close releases all process-owned sockets and waits for proxy goroutines. It
// does not remove persistent allocations.
func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	var closeErr error
	for key, active := range m.active {
		closeErr = errors.Join(closeErr, closeActive(active))
		delete(m.active, key)
	}
	m.mu.Unlock()
	m.wg.Wait()
	return closeErr
}

func (m *Manager) recoverRecordLocked(ctx context.Context, lease journal.Lease, record storedAllocation, requireReady bool) (*activeAllocation, error) {
	listener, err := net.Listen("tcp4", net.JoinHostPort(loopbackHost, strconv.Itoa(record.MachinePort)))
	if err != nil {
		return nil, &PortOccupiedError{Port: record.MachinePort}
	}
	target := Target{BoxID: record.BoxID, Service: record.Service, Port: record.TargetPort}
	ready := m.probe(ctx, target) == nil
	if requireReady && !ready {
		_ = listener.Close()
		return nil, ErrTargetUnreachable
	}
	listenerID, err := randomListenerID()
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	if err := m.recoverPersist(ctx, lease, record, listenerID, ready); err != nil {
		_ = listener.Close()
		return nil, err
	}
	active := &activeAllocation{
		allocation: Allocation{
			BoxID: record.BoxID, Name: record.Name, Service: record.Service,
			TargetPort: record.TargetPort, Host: loopbackHost, Port: record.MachinePort, Ready: ready,
		},
		target: target, listener: listener, listenerID: listenerID,
		leaseID: leaseIdentity(lease), connections: make(map[net.Conn]struct{}),
	}
	m.activateLocked(allocationKey{boxID: record.BoxID, name: record.Name}, active)
	return active, nil
}

func (m *Manager) activateLocked(key allocationKey, active *activeAllocation) {
	m.active[key] = active
	m.wg.Add(1)
	go m.serve(key, active)
}

func (m *Manager) deactivateLocked(key allocationKey, active *activeAllocation) {
	delete(m.active, key)
	_ = closeActive(active)
}

func closeActive(active *activeAllocation) error {
	err := closeNetwork(active.listener)
	for connection := range active.connections {
		err = errors.Join(err, closeNetwork(connection))
	}
	return err
}

func closeNetwork(closer io.Closer) error {
	err := closer.Close()
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

func (m *Manager) serve(key allocationKey, active *activeAllocation) {
	defer m.wg.Done()
	for {
		incoming, err := active.listener.Accept()
		if err != nil {
			return
		}
		m.mu.Lock()
		if m.closed || m.active[key] != active {
			m.mu.Unlock()
			_ = incoming.Close()
			return
		}
		active.connections[incoming] = struct{}{}
		m.wg.Add(1)
		m.mu.Unlock()
		go m.proxy(key, active, incoming)
	}
}

func (m *Manager) proxy(key allocationKey, active *activeAllocation, incoming net.Conn) {
	defer m.wg.Done()
	defer m.forgetConnection(active, incoming)
	ctx, cancel := context.WithTimeout(context.Background(), m.cfg.DialTimeout)
	outgoing, err := m.dial.DialContext(ctx, active.target)
	cancel()
	if err != nil {
		return
	}
	if !m.rememberConnection(key, active, outgoing) {
		_ = outgoing.Close()
		return
	}
	defer m.forgetConnection(active, outgoing)

	copyDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(outgoing, incoming)
		if closer, ok := outgoing.(interface{ CloseWrite() error }); ok {
			_ = closer.CloseWrite()
		}
		close(copyDone)
	}()
	_, _ = io.Copy(incoming, outgoing)
	if closer, ok := incoming.(interface{ CloseWrite() error }); ok {
		_ = closer.CloseWrite()
	}
	<-copyDone
}

func (m *Manager) rememberConnection(key allocationKey, active *activeAllocation, connection net.Conn) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.active[key] != active {
		return false
	}
	active.connections[connection] = struct{}{}
	return true
}

func (m *Manager) forgetConnection(active *activeAllocation, connection net.Conn) {
	_ = closeNetwork(connection)
	m.mu.Lock()
	delete(active.connections, connection)
	m.mu.Unlock()
}

func (m *Manager) probe(ctx context.Context, target Target) error {
	probeContext, cancel := context.WithTimeout(ctx, m.cfg.ProbeTimeout)
	defer cancel()
	connection, err := m.dial.DialContext(probeContext, target)
	if err != nil {
		return err
	}
	return connection.Close()
}

func validateRequest(lease journal.Lease, endpoint Endpoint) error {
	if err := validateLease(lease); err != nil {
		return err
	}
	if err := validateName(endpoint.Name); err != nil {
		return err
	}
	if err := validateName(endpoint.Service); err != nil {
		return errors.New("service name is invalid")
	}
	if endpoint.TargetPort < 1 || endpoint.TargetPort > 65535 {
		return errors.New("target port must be between 1 and 65535")
	}
	return nil
}

func validateLease(lease journal.Lease) error {
	values := []string{
		lease.BoxID, lease.OperationID, lease.Owner.InstanceID,
		lease.Owner.ProcessStartID, lease.Owner.BootID,
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" || strings.IndexByte(value, 0) >= 0 {
			return errors.New("box lease identity is incomplete")
		}
	}
	if lease.Fence < 1 {
		return errors.New("box lease fence must be positive")
	}
	return nil
}

func validateName(name string) error {
	if strings.TrimSpace(name) == "" || strings.IndexByte(name, 0) >= 0 {
		return errors.New("endpoint name is invalid")
	}
	return nil
}

func leaseIdentity(lease journal.Lease) string {
	return lease.OperationID + "/" + strconv.FormatInt(lease.Fence, 10)
}

func randomListenerID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("create TCP listener identity: %w", err)
	}
	return "listener-" + hex.EncodeToString(bytes[:]), nil
}
