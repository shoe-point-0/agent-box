// Package filesystem verifies the live mount boundary before a Box can become
// ready. It compares independently observed layers rather than trusting the
// generated Lima or Compose inputs that caused them.
package filesystem

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/lima"
)

const (
	maxMountInfoBytes = 1 << 20
	maxMountInfoLines = 8192
)

type ErrorKind string

const (
	InvalidObservation ErrorKind = "invalid-observation"
	MalformedMountInfo ErrorKind = "malformed-mountinfo"
	MountSetMismatch   ErrorKind = "mount-set-mismatch"
	ProbeFailed        ErrorKind = "probe-failed"
)

// Error intentionally reports only a layer and stable reason. Live paths and
// mountinfo may contain user or workload data and do not belong in diagnostics.
type Error struct {
	Kind   ErrorKind
	Layer  string
	Reason string
}

func (e *Error) Error() string {
	return fmt.Sprintf("verify filesystem %s: %s: %s", e.Layer, e.Kind, e.Reason)
}

// Observation contains four independent live views of the Plan's mount set.
// ContainerMountInfo is keyed by the immutable container ID from Docker
// inspect, never by a mutable name.
type Observation struct {
	Lima               []lima.MountObservation
	GuestMountInfo     []byte
	Docker             compose.RuntimeObservation
	ContainerMountInfo map[string][]byte
}

type expectedMount struct {
	path     string
	writable bool
}

// Verify requires exact mount membership and modes at the Lima config, guest
// kernel, Docker inspect, and container-kernel boundaries.
func Verify(plan contract.Plan, observed Observation) error {
	expected, byService, err := expectedMounts(plan.Mounts)
	if err != nil {
		return failure(InvalidObservation, "plan", err.Error())
	}
	if err := verifyLima(expected, observed.Lima); err != nil {
		return err
	}
	guest, err := parseMountInfo("guest-mountinfo", observed.GuestMountInfo)
	if err != nil {
		return err
	}
	if err := compareProjected("guest-mountinfo", expected, guest); err != nil {
		return err
	}
	return verifyContainers(byService, observed.Docker.Containers, observed.ContainerMountInfo)
}

func expectedMounts(planned []contract.MountPlan) ([]expectedMount, map[string][]expectedMount, error) {
	if len(planned) == 0 {
		return nil, nil, errors.New("mount set is empty")
	}
	expected := make([]expectedMount, 0, len(planned))
	byService := make(map[string][]expectedMount)
	seenPaths := make(map[string]struct{}, len(planned))
	for _, mount := range planned {
		if !cleanAbsolute(mount.Path) {
			return nil, nil, errors.New("mount path is invalid")
		}
		if _, duplicate := seenPaths[mount.Path]; duplicate {
			return nil, nil, errors.New("mount set contains a duplicate path")
		}
		seenPaths[mount.Path] = struct{}{}
		item := expectedMount{path: mount.Path, writable: mount.Writable}
		expected = append(expected, item)
		if len(mount.Services) == 0 {
			return nil, nil, errors.New("mount has no services")
		}
		seenServices := make(map[string]struct{}, len(mount.Services))
		for _, service := range mount.Services {
			if service == "" || strings.ContainsAny(service, "\x00\r\n") {
				return nil, nil, errors.New("mount service is invalid")
			}
			if _, duplicate := seenServices[service]; duplicate {
				return nil, nil, errors.New("mount contains a duplicate service")
			}
			seenServices[service] = struct{}{}
			byService[service] = append(byService[service], item)
		}
	}
	sortMounts(expected)
	for service := range byService {
		sortMounts(byService[service])
	}
	return expected, byService, nil
}

func verifyLima(expected []expectedMount, observed []lima.MountObservation) error {
	actual := make([]expectedMount, 0, len(observed))
	seen := make(map[string]struct{}, len(observed))
	for _, mount := range observed {
		if !cleanAbsolute(mount.Source) || mount.Destination != mount.Source {
			return failure(MountSetMismatch, "lima-config", "mount source or destination differs from Plan")
		}
		if _, duplicate := seen[mount.Source]; duplicate {
			return failure(MountSetMismatch, "lima-config", "mount set contains a duplicate")
		}
		seen[mount.Source] = struct{}{}
		actual = append(actual, expectedMount{path: mount.Source, writable: mount.Writable})
	}
	return compareExact("lima-config", expected, actual)
}

func verifyContainers(byService map[string][]expectedMount, containers []compose.ContainerObservation, mountInfo map[string][]byte) error {
	if len(containers) == 0 {
		return failure(MountSetMismatch, "docker-inspect", "project has no containers")
	}
	seenIDs := make(map[string]struct{}, len(containers))
	for _, container := range containers {
		if container.ID == "" || container.Service == "" {
			return failure(InvalidObservation, "docker-inspect", "container identity is incomplete")
		}
		if _, duplicate := seenIDs[container.ID]; duplicate {
			return failure(InvalidObservation, "docker-inspect", "container set contains a duplicate ID")
		}
		seenIDs[container.ID] = struct{}{}
		expected := byService[container.Service]
		actual := make([]expectedMount, 0, len(container.Mounts))
		for _, mount := range container.Mounts {
			if mount.Type != "bind" {
				continue
			}
			if !cleanAbsolute(mount.Source) || mount.Destination != mount.Source {
				return failure(MountSetMismatch, "docker-inspect", "bind source or destination differs from Plan")
			}
			actual = append(actual, expectedMount{path: mount.Source, writable: mount.ReadWrite})
		}
		if err := compareExact("docker-inspect", expected, actual); err != nil {
			return err
		}
		data, ok := mountInfo[container.ID]
		if !ok {
			return failure(MountSetMismatch, "container-mountinfo", "container observation is missing")
		}
		parsed, err := parseMountInfo("container-mountinfo", data)
		if err != nil {
			return err
		}
		if err := compareProjected("container-mountinfo", expected, parsed); err != nil {
			return err
		}
	}
	if len(mountInfo) != len(seenIDs) {
		return failure(MountSetMismatch, "container-mountinfo", "container observation set differs from Docker inspect")
	}
	for id := range mountInfo {
		if _, ok := seenIDs[id]; !ok {
			return failure(MountSetMismatch, "container-mountinfo", "container observation set differs from Docker inspect")
		}
	}
	return nil
}

func compareProjected(layer string, expected, all []expectedMount) error {
	actual := make([]expectedMount, 0, len(expected))
	for _, mount := range all {
		for _, root := range expected {
			if contained(root.path, mount.path) {
				actual = append(actual, mount)
				break
			}
		}
	}
	return compareExact(layer, expected, actual)
}

func compareExact(layer string, expected, actual []expectedMount) error {
	sortMounts(actual)
	if !slices.Equal(expected, actual) {
		return failure(MountSetMismatch, layer, "live mount set or mode differs from Plan")
	}
	return nil
}

func parseMountInfo(layer string, data []byte) ([]expectedMount, error) {
	if len(data) == 0 || len(data) > maxMountInfoBytes || bytes.IndexByte(data, 0) >= 0 {
		return nil, failure(MalformedMountInfo, layer, "mountinfo size or encoding is invalid")
	}
	lines := bytes.Split(bytes.TrimSuffix(data, []byte{'\n'}), []byte{'\n'})
	if len(lines) > maxMountInfoLines {
		return nil, failure(MalformedMountInfo, layer, "mountinfo contains too many records")
	}
	mounts := make([]expectedMount, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(string(line))
		separator := slices.Index(fields, "-")
		if len(fields) < 10 || separator < 6 || separator+3 > len(fields) {
			return nil, failure(MalformedMountInfo, layer, "mountinfo record is malformed")
		}
		path, ok := unescapeMountField(fields[4])
		if !ok || !cleanAbsolute(path) {
			return nil, failure(MalformedMountInfo, layer, "mountinfo path is invalid")
		}
		writable, ok := mountWritable(fields[5])
		if !ok {
			return nil, failure(MalformedMountInfo, layer, "mountinfo mode is invalid")
		}
		// Linux permits stacked mounts (for example autofs plus binfmt_misc).
		// Preserve every record: compareProjected still rejects ambiguous
		// membership inside the declared workspace mount boundary.
		mounts = append(mounts, expectedMount{path: path, writable: writable})
	}
	return mounts, nil
}

func unescapeMountField(value string) (string, bool) {
	var output strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] != '\\' {
			output.WriteByte(value[index])
			continue
		}
		if index+3 >= len(value) {
			return "", false
		}
		escape := value[index+1 : index+4]
		switch escape {
		case "040":
			output.WriteByte(' ')
		case "011":
			output.WriteByte('\t')
		case "012":
			output.WriteByte('\n')
		case "134":
			output.WriteByte('\\')
		default:
			return "", false
		}
		index += 3
	}
	return output.String(), true
}

func mountWritable(options string) (bool, bool) {
	readOnly := false
	readWrite := false
	for _, option := range strings.Split(options, ",") {
		switch option {
		case "ro":
			readOnly = true
		case "rw":
			readWrite = true
		}
	}
	if readOnly == readWrite {
		return false, false
	}
	return readWrite, true
}

func sortMounts(mounts []expectedMount) {
	slices.SortFunc(mounts, func(left, right expectedMount) int {
		if compared := strings.Compare(left.path, right.path); compared != 0 {
			return compared
		}
		if left.writable == right.writable {
			return 0
		}
		if !left.writable {
			return -1
		}
		return 1
	})
}

func cleanAbsolute(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path && !strings.ContainsAny(path, "\x00\r\n")
}

func contained(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func failure(kind ErrorKind, layer, reason string) *Error {
	return &Error{Kind: kind, Layer: layer, Reason: reason}
}
