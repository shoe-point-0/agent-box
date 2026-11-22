package machine

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/testutil"
)

// Filesystem and loopback observations are live; neither requires host virtualization.
func testAccelerator() (string, error) { return "test-accelerator", nil }

func TestLocalObserverCollectsLivePathsPlatformAndRouterAvailability(t *testing.T) {
	root := testutil.TempDir(t)
	missing := filepath.Join(root, "not-created", "state")
	address := availableAddress(t)
	configured := []PathRequirement{
		{Name: "existing", Path: root, MinFreeBytes: 1},
		{Name: "missing", Path: missing, MinFreeBytes: 1},
	}
	observer, err := NewLocalObserver(LocalObserverConfig{Paths: configured, RouterAddress: address})
	if err != nil {
		t.Fatal(err)
	}
	configured[0].Path = "/changed-after-construction"

	observation, err := observer.observe(t.Context(), testAccelerator)
	if err != nil {
		t.Fatal(err)
	}
	if observation.Platform.OS != runtime.GOOS || observation.Platform.Architecture != runtime.GOARCH || observation.Platform.Accelerator != "test-accelerator" {
		t.Fatalf("platform = %#v", observation.Platform)
	}
	if observation.RouterAddress != address || !observation.RouterAvailable || observation.RouterDetail != "" {
		t.Fatalf("router observation = %#v", observation)
	}
	if len(observation.Paths) != 2 {
		t.Fatalf("paths = %#v", observation.Paths)
	}
	for _, observed := range observation.Paths {
		if !observed.Usable || observed.AvailableBytes == 0 || observed.Detail != "" {
			t.Errorf("path observation = %#v", observed)
		}
	}
	if _, err := os.Lstat(missing); !os.IsNotExist(err) {
		t.Fatalf("observer created missing path: %v", err)
	}
}

func TestLocalObserverReportsOccupiedRouterWithoutRetainingAProbe(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	observer, err := NewLocalObserver(LocalObserverConfig{
		Paths: []PathRequirement{{Name: "state", Path: testutil.TempDir(t), MinFreeBytes: 1}}, RouterAddress: address,
	})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := observer.observe(t.Context(), testAccelerator)
	if err != nil {
		t.Fatal(err)
	}
	if observation.RouterAvailable || observation.RouterDetail == "" {
		t.Fatalf("router observation = %#v", observation)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	rebound, err := net.Listen("tcp4", address)
	if err != nil {
		t.Fatalf("observer retained router probe: %v", err)
	}
	_ = rebound.Close()
}

func TestLocalObserverRejectsSymlinkedPathAncestry(t *testing.T) {
	root := testutil.TempDir(t)
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	observer, err := NewLocalObserver(LocalObserverConfig{
		Paths:         []PathRequirement{{Name: "state", Path: filepath.Join(link, "state"), MinFreeBytes: 1}},
		RouterAddress: availableAddress(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := observer.observe(t.Context(), testAccelerator)
	if err != nil {
		t.Fatal(err)
	}
	if len(observation.Paths) != 1 || observation.Paths[0].Usable || observation.Paths[0].Detail == "" {
		t.Fatalf("symlinked path observation = %#v", observation.Paths)
	}
}

func TestLocalObserverPropagatesUnavailableAccelerator(t *testing.T) {
	observer, err := NewLocalObserver(LocalObserverConfig{
		Paths: []PathRequirement{{Name: "state", Path: testutil.TempDir(t)}}, RouterAddress: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = observer.observe(t.Context(), func() (string, error) { return "", os.ErrNotExist })
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("observe with missing accelerator: got %v, want missing probe error", err)
	}
}

func availableAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}
