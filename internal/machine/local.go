package machine

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

// LocalObserverConfig is the complete read-only input to a live Machine
// observation. Paths and the router address are copied on construction.
type LocalObserverConfig struct {
	Paths         []PathRequirement
	RouterAddress string
}

// LocalObserver collects platform, filesystem, and temporary bind evidence
// from the current Machine. It never creates directories or retains a listener.
type LocalObserver struct {
	paths         []PathRequirement
	routerAddress string
}

func NewLocalObserver(config LocalObserverConfig) (*LocalObserver, error) {
	if len(config.Paths) == 0 {
		return nil, errors.New("local Machine observer path set is empty")
	}
	if config.RouterAddress == "" {
		return nil, errors.New("local Machine observer router address is empty")
	}
	return &LocalObserver{
		paths:         append([]PathRequirement(nil), config.Paths...),
		routerAddress: config.RouterAddress,
	}, nil
}

func (observer *LocalObserver) Observe(ctx context.Context) (Observation, error) {
	if observer == nil || len(observer.paths) == 0 || observer.routerAddress == "" {
		return Observation{}, errors.New("local Machine observer is incomplete")
	}
	if err := ctx.Err(); err != nil {
		return Observation{}, err
	}
	accelerator, err := observeAccelerator()
	if err != nil {
		return Observation{}, fmt.Errorf("observe Machine accelerator: %w", err)
	}
	result := Observation{
		Platform:      Platform{OS: runtimeOS, Architecture: runtimeArchitecture, Accelerator: accelerator},
		Paths:         make([]PathObservation, 0, len(observer.paths)),
		RouterAddress: observer.routerAddress,
	}
	for _, required := range observer.paths {
		if err := ctx.Err(); err != nil {
			return Observation{}, err
		}
		result.Paths = append(result.Paths, observePath(required))
	}

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp4", observer.routerAddress)
	if err != nil {
		result.RouterDetail = "temporary bind failed"
		return result, nil
	}
	result.RouterAvailable = true
	if err := listener.Close(); err != nil {
		return Observation{}, fmt.Errorf("release Machine router probe: %w", err)
	}
	return result, nil
}

func observePath(required PathRequirement) PathObservation {
	result := PathObservation{Name: required.Name, Path: required.Path}
	ancestor, targetExists, err := existingAncestor(required.Path)
	if err != nil {
		result.Detail = "existing parent is unavailable"
		return result
	}
	resolved, err := filepath.EvalSymlinks(ancestor)
	if err != nil || resolved != ancestor {
		result.Detail = "path ancestry is not canonical"
		return result
	}
	if targetExists {
		info, statErr := os.Lstat(required.Path)
		if statErr != nil || !info.IsDir() {
			result.Detail = "target is not a directory"
			return result
		}
	}
	if err := unix.Access(ancestor, unix.R_OK|unix.W_OK|unix.X_OK); err != nil {
		result.Detail = "existing parent is not readable, writable, and searchable"
		return result
	}
	var statistics unix.Statfs_t
	if err := unix.Statfs(ancestor, &statistics); err != nil {
		result.Detail = "filesystem capacity is unavailable"
		return result
	}
	result.Usable = true
	result.AvailableBytes = uint64(statistics.Bavail) * uint64(statistics.Bsize)
	return result
}

func existingAncestor(path string) (string, bool, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", false, errors.New("path is not clean and absolute")
	}
	current := path
	for {
		_, err := os.Lstat(current)
		if err == nil {
			return current, current == path, nil
		}
		if !errors.Is(err, syscall.ENOENT) {
			return "", false, err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false, err
		}
		current = parent
	}
}
