package tcpallocator_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/journal"
	"github.com/shoe-point-0/agent-box/internal/state"
	"github.com/shoe-point-0/agent-box/internal/tcpallocator"
)

const testTimeout = 2 * time.Second

func TestConcurrentAllocationsAreUniqueAndProxyReachable(t *testing.T) {
	store := openStore(t)
	lease := acquireLease(t, store, "concurrent")
	echo := startEchoServer(t)
	dialer := newMappingDialer(map[string]string{"postgres": echo.Address()})
	first, last := availableRange(t, 12)
	managers := make([]*tcpallocator.Manager, 4)
	for index := range managers {
		managers[index] = newManager(t, store, dialer, first, last)
		defer closeManager(t, managers[index])
	}

	const count = 8
	allocations := make(chan tcpallocator.Allocation, count)
	errorsChannel := make(chan error, count)
	var workers sync.WaitGroup
	for index := range count {
		workers.Add(1)
		go func() {
			defer workers.Done()
			allocation, err := managers[index%len(managers)].Allocate(t.Context(), lease, tcpallocator.Endpoint{
				Name: fmt.Sprintf("database-%02d", index), Service: "postgres", TargetPort: 5432,
			})
			if err != nil {
				errorsChannel <- err
				return
			}
			allocations <- allocation
		}()
	}
	workers.Wait()
	close(allocations)
	close(errorsChannel)
	for err := range errorsChannel {
		t.Fatalf("Allocate() error = %v", err)
	}

	seen := make(map[int]struct{}, count)
	for allocation := range allocations {
		if allocation.Host != "127.0.0.1" || !allocation.Ready {
			t.Fatalf("allocation = %+v, want ready loopback endpoint", allocation)
		}
		if _, duplicate := seen[allocation.Port]; duplicate {
			t.Fatalf("Machine port %d was allocated twice", allocation.Port)
		}
		seen[allocation.Port] = struct{}{}
		assertProxyEcho(t, allocation, allocation.Name)
	}
	if len(seen) != count {
		t.Fatalf("unique allocations = %d, want %d", len(seen), count)
	}
}

func TestUnreachableTargetIsNeverPersistedOrPublished(t *testing.T) {
	store := openStore(t)
	lease := acquireLease(t, store, "unreachable")
	first, last := availableRange(t, 1)
	dialer := newMappingDialer(nil)
	manager := newManager(t, store, dialer, first, last)
	defer closeManager(t, manager)

	_, err := manager.Allocate(t.Context(), lease, tcpallocator.Endpoint{
		Name: "database", Service: "missing", TargetPort: 5432,
	})
	if !errors.Is(err, tcpallocator.ErrTargetUnreachable) {
		t.Fatalf("Allocate(unreachable) error = %v, want ErrTargetUnreachable", err)
	}
	recorded, err := manager.Recorded(t.Context(), lease.BoxID)
	if err != nil {
		t.Fatal(err)
	}
	if len(recorded) != 0 {
		t.Fatalf("unreachable allocation was persisted: %+v", recorded)
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(first)))
	if err != nil {
		t.Fatalf("failed allocation retained listener: %v", err)
	}
	_ = listener.Close()
}

func TestRecoveryRetainsStablePortWhileStoppedAndRefusesOccupation(t *testing.T) {
	store := openStore(t)
	lease := acquireLease(t, store, "recovery")
	echo := startEchoServer(t)
	dialer := newMappingDialer(map[string]string{"postgres": echo.Address()})
	first, last := availableRange(t, 2)

	manager := newManager(t, store, dialer, first, last)
	allocation, err := manager.Allocate(t.Context(), lease, tcpallocator.Endpoint{
		Name: "database", Service: "postgres", TargetPort: 5432,
	})
	if err != nil {
		t.Fatal(err)
	}
	closeManager(t, manager)

	blocker, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(allocation.Port)))
	if err != nil {
		t.Fatal(err)
	}
	restarted := newManager(t, store, dialer, first, last)
	_, err = restarted.Recover(t.Context(), lease)
	var occupied *tcpallocator.PortOccupiedError
	if !errors.As(err, &occupied) || occupied.Port != allocation.Port {
		t.Fatalf("Recover(occupied) error = %v, want port %d occupation", err, allocation.Port)
	}
	_ = blocker.Close()

	dialer.Set("postgres", "")
	recovered, err := restarted.Recover(t.Context(), lease)
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 1 || recovered[0].Port != allocation.Port || recovered[0].Ready {
		t.Fatalf("stopped recovery = %+v, want stable not-ready allocation", recovered)
	}
	assertConnects(t, recovered[0])

	dialer.Set("postgres", echo.Address())
	ready, err := restarted.Refresh(t.Context(), lease, "database")
	if err != nil || !ready.Ready || ready.Port != allocation.Port {
		t.Fatalf("Refresh() = %+v, %v", ready, err)
	}
	assertProxyEcho(t, ready, "recovered")
	closeManager(t, restarted)
}

func TestRemovalReleasesOnlySelectedBoxAllocation(t *testing.T) {
	store := openStore(t)
	selectedLease := acquireLease(t, store, "remove-selected")
	siblingLease := acquireLease(t, store, "remove-sibling")
	echo := startEchoServer(t)
	dialer := newMappingDialer(map[string]string{"postgres": echo.Address()})
	first, last := availableRange(t, 4)
	manager := newManager(t, store, dialer, first, last)
	defer closeManager(t, manager)

	firstAllocation, err := manager.Allocate(t.Context(), selectedLease, tcpallocator.Endpoint{
		Name: "database", Service: "postgres", TargetPort: 5432,
	})
	if err != nil {
		t.Fatal(err)
	}
	secondAllocation, err := manager.Allocate(t.Context(), siblingLease, tcpallocator.Endpoint{
		Name: "database", Service: "postgres", TargetPort: 5432,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(t.Context(), selectedLease, "database"); err != nil {
		t.Fatal(err)
	}

	replacement, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(firstAllocation.Port)))
	if err != nil {
		t.Fatalf("selected allocation port was not released: %v", err)
	}
	_ = replacement.Close()
	assertProxyEcho(t, secondAllocation, "still-active")
	selected, err := manager.Recorded(t.Context(), selectedLease.BoxID)
	if err != nil {
		t.Fatal(err)
	}
	sibling, err := manager.Recorded(t.Context(), siblingLease.BoxID)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 0 || len(sibling) != 1 || sibling[0].Name != "database" || sibling[0].Port != secondAllocation.Port {
		t.Fatalf("recorded allocations after selected removal: selected=%+v sibling=%+v", selected, sibling)
	}
}

func TestRangeExhaustionDoesNotMutateState(t *testing.T) {
	store := openStore(t)
	lease := acquireLease(t, store, "exhaustion")
	echo := startEchoServer(t)
	dialer := newMappingDialer(map[string]string{"postgres": echo.Address()})
	first, last, blockers := occupiedRange(t, 2)
	defer func() {
		for _, blocker := range blockers {
			_ = blocker.Close()
		}
	}()
	manager := newManager(t, store, dialer, first, last)
	defer closeManager(t, manager)

	_, err := manager.Allocate(t.Context(), lease, tcpallocator.Endpoint{
		Name: "database", Service: "postgres", TargetPort: 5432,
	})
	if !errors.Is(err, tcpallocator.ErrExhausted) {
		t.Fatalf("Allocate(exhausted) error = %v, want ErrExhausted", err)
	}
	recorded, err := manager.Recorded(t.Context(), lease.BoxID)
	if err != nil || len(recorded) != 0 {
		t.Fatalf("Recorded() = %+v, %v, want empty", recorded, err)
	}
}

func openStore(t *testing.T) *state.Store {
	t.Helper()
	store, err := state.Open(t.Context(), filepath.Join(t.TempDir(), "state", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	return store
}

func acquireLease(t *testing.T, store *state.Store, suffix string) journal.Lease {
	t.Helper()
	manifestPath := filepath.Join(t.TempDir(), suffix, ".abox", "box.yaml")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o700); err != nil {
		t.Fatal(err)
	}
	workspaceID, err := identity.Workspace(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	boxID, err := identity.Box(workspaceID, "TASK-"+suffix)
	if err != nil {
		t.Fatal(err)
	}
	operationID := "operation-" + suffix
	operations, err := journal.New(store)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	operation, _, err := operations.Begin(t.Context(), journal.BeginRequest{
		WorkspaceID: workspaceID, BoxID: boxID, TaskKey: "TASK-" + suffix,
		ManifestPath: manifestPath, ManifestDigest: "sha256:manifest-" + suffix,
		OperationID: operationID, IdempotencyKey: "key-" + suffix, Kind: "up",
		RequestDigest: "sha256:request-" + suffix, StartedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := operations.AcquireLease(t.Context(), operation.BoxID, operation.ID, journal.Owner{
		InstanceID: "daemon-" + suffix, ProcessStartID: "process-" + suffix, BootID: "boot-test",
	}, now, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return lease
}

func newManager(t *testing.T, store *state.Store, dialer tcpallocator.Dialer, first, last int) *tcpallocator.Manager {
	t.Helper()
	manager, err := tcpallocator.New(store, dialer, tcpallocator.Config{
		Range:        tcpallocator.Range{First: first, Last: last},
		ProbeTimeout: testTimeout, DialTimeout: testTimeout,
	})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func closeManager(t *testing.T, manager *tcpallocator.Manager) {
	t.Helper()
	if err := manager.Close(); err != nil {
		t.Errorf("Manager.Close() error = %v", err)
	}
}

func availableRange(t *testing.T, width int) (int, int) {
	t.Helper()
	first, last, listeners := occupiedRange(t, width)
	for _, listener := range listeners {
		if err := listener.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return first, last
}

func occupiedRange(t *testing.T, width int) (int, int, []net.Listener) {
	t.Helper()
	for first := 20000; first+width-1 <= 60000; first += width {
		listeners := make([]net.Listener, 0, width)
		available := true
		for port := first; port < first+width; port++ {
			listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
			if err != nil {
				available = false
				break
			}
			listeners = append(listeners, listener)
		}
		if available {
			return first, first + width - 1, listeners
		}
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}
	t.Fatal("no contiguous loopback test port range is available")
	return 0, 0, nil
}

type mappingDialer struct {
	mu        sync.RWMutex
	addresses map[string]string
}

func newMappingDialer(addresses map[string]string) *mappingDialer {
	copyOfAddresses := make(map[string]string, len(addresses))
	for service, address := range addresses {
		copyOfAddresses[service] = address
	}
	return &mappingDialer{addresses: copyOfAddresses}
}

func (d *mappingDialer) Set(service, address string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if address == "" {
		delete(d.addresses, service)
		return
	}
	d.addresses[service] = address
}

func (d *mappingDialer) DialContext(ctx context.Context, target tcpallocator.Target) (net.Conn, error) {
	d.mu.RLock()
	address := d.addresses[target.Service]
	d.mu.RUnlock()
	if address == "" {
		return nil, errors.New("target unavailable")
	}
	return (&net.Dialer{}).DialContext(ctx, "tcp4", address)
}

type echoServer struct {
	listener net.Listener
	once     sync.Once
	wg       sync.WaitGroup
}

func startEchoServer(t *testing.T) *echoServer {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &echoServer{listener: listener}
	server.wg.Add(1)
	go func() {
		defer server.wg.Done()
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			server.wg.Add(1)
			go func() {
				defer server.wg.Done()
				defer func() { _ = connection.Close() }()
				_, _ = io.Copy(connection, connection)
			}()
		}
	}()
	t.Cleanup(server.Close)
	return server
}

func (s *echoServer) Address() string { return s.listener.Addr().String() }

func (s *echoServer) Close() {
	s.once.Do(func() { _ = s.listener.Close() })
	s.wg.Wait()
}

func assertProxyEcho(t *testing.T, allocation tcpallocator.Allocation, payload string) {
	t.Helper()
	connection, err := net.DialTimeout("tcp4", net.JoinHostPort(allocation.Host, strconv.Itoa(allocation.Port)), testTimeout)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()
	if err := connection.SetDeadline(time.Now().Add(testTimeout)); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Write([]byte(payload)); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(connection, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != payload {
		t.Fatalf("proxy echo = %q, want %q", got, payload)
	}
}

func assertConnects(t *testing.T, allocation tcpallocator.Allocation) {
	t.Helper()
	connection, err := net.DialTimeout("tcp4", net.JoinHostPort(allocation.Host, strconv.Itoa(allocation.Port)), testTimeout)
	if err != nil {
		t.Fatalf("retained stopped listener is not bound: %v", err)
	}
	_ = connection.Close()
}
