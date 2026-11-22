package filesystem_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/filesystem"
	"github.com/shoe-point-0/agent-box/internal/lima"
)

func TestVerifyAcceptsExactIndependentMountObservations(t *testing.T) {
	t.Parallel()
	plan, observed := exactFixture()
	if err := filesystem.Verify(plan, observed); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestVerifyRejectsDriftAtEveryLayer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		layer string
		drift func(*filesystem.Observation)
	}{
		{"lima-extra", "lima-config", func(value *filesystem.Observation) {
			value.Lima = append(value.Lima, lima.MountObservation{Source: "/private", Destination: "/private", Writable: true})
		}},
		{"lima-remap", "lima-config", func(value *filesystem.Observation) { value.Lima[0].Destination = "/elsewhere" }},
		{"guest-missing", "guest-mountinfo", func(value *filesystem.Observation) {
			value.GuestMountInfo = mountInfo(mount("/", true), mount("/boxes/worktree", true))
		}},
		{"guest-nested", "guest-mountinfo", func(value *filesystem.Observation) {
			value.GuestMountInfo = append(value.GuestMountInfo, mountInfoLine("/boxes/worktree/private", true)...)
		}},
		{"docker-extra-bind", "docker-inspect", func(value *filesystem.Observation) {
			value.Docker.Containers[0].Mounts = append(value.Docker.Containers[0].Mounts, compose.MountObservation{
				Type: "bind", Source: "/private", Destination: "/private", ReadWrite: true,
			})
		}},
		{"docker-mode", "docker-inspect", func(value *filesystem.Observation) { value.Docker.Containers[0].Mounts[0].ReadWrite = false }},
		{"container-missing", "container-mountinfo", func(value *filesystem.Observation) { delete(value.ContainerMountInfo, "container-1") }},
		{"container-nested", "container-mountinfo", func(value *filesystem.Observation) {
			value.ContainerMountInfo["container-1"] = append(value.ContainerMountInfo["container-1"], mountInfoLine("/boxes/git/private", true)...)
		}},
		{"container-extra-observation", "container-mountinfo", func(value *filesystem.Observation) {
			value.ContainerMountInfo["foreign"] = mountInfo(mount("/", true))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			plan, observed := exactFixture()
			test.drift(&observed)
			err := filesystem.Verify(plan, observed)
			assertError(t, err, filesystem.MountSetMismatch, test.layer)
		})
	}
}

func TestVerifyAllowsNonBindContainerMountsOutsidePlanProjection(t *testing.T) {
	t.Parallel()
	plan, observed := exactFixture()
	observed.Docker.Containers[0].Mounts = append(observed.Docker.Containers[0].Mounts, compose.MountObservation{
		Type: "volume", Source: "cache-volume", Destination: "/cache", ReadWrite: true,
	})
	observed.ContainerMountInfo["container-1"] = append(observed.ContainerMountInfo["container-1"], mountInfoLine("/cache", true)...)
	if err := filesystem.Verify(plan, observed); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestVerifyDecodesKernelMountInfoEscapes(t *testing.T) {
	t.Parallel()
	plan, observed := exactFixture()
	plan.Mounts[1].Path = "/boxes/work tree"
	observed.Lima[1].Source = "/boxes/work tree"
	observed.Lima[1].Destination = "/boxes/work tree"
	observed.Docker.Containers[0].Mounts[1].Source = "/boxes/work tree"
	observed.Docker.Containers[0].Mounts[1].Destination = "/boxes/work tree"
	info := mountInfo(mount("/", true), mount("/boxes/git", true), mount("/boxes/work\\040tree", true))
	observed.GuestMountInfo = append([]byte(nil), info...)
	observed.ContainerMountInfo["container-1"] = append([]byte(nil), info...)
	if err := filesystem.Verify(plan, observed); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestVerifyRejectsMalformedOrUnboundedMountInfoWithoutLeakingInput(t *testing.T) {
	t.Parallel()
	plan, observed := exactFixture()
	secret := "do-not-report-this-path"
	observed.GuestMountInfo = []byte("malformed " + secret + "\n")
	err := filesystem.Verify(plan, observed)
	assertError(t, err, filesystem.MalformedMountInfo, "guest-mountinfo")
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked mountinfo content: %v", err)
	}

	_, observed = exactFixture()
	observed.GuestMountInfo = make([]byte, (1<<20)+1)
	err = filesystem.Verify(plan, observed)
	assertError(t, err, filesystem.MalformedMountInfo, "guest-mountinfo")
}

func TestObservationFromLimaInspectReturnsDefensiveMountCopy(t *testing.T) {
	t.Parallel()
	// An absent observation has no config and therefore authorizes no mounts.
	observation := lima.Observation{Name: "box", State: lima.Absent}
	if mounts := observation.Mounts(); mounts != nil {
		t.Fatalf("Mounts() = %#v, want nil", mounts)
	}
}

func exactFixture() (contract.Plan, filesystem.Observation) {
	plan := contract.Plan{Mounts: []contract.MountPlan{
		{Path: "/boxes/git", Services: []string{"workspace"}, Writable: true},
		{Path: "/boxes/worktree", Services: []string{"workspace"}, Writable: true},
	}}
	limaMounts := []lima.MountObservation{
		{Source: "/boxes/git", Destination: "/boxes/git", Writable: true},
		{Source: "/boxes/worktree", Destination: "/boxes/worktree", Writable: true},
	}
	container := compose.ContainerObservation{
		ID: "container-1", Service: "workspace", Mounts: []compose.MountObservation{
			{Type: "bind", Source: "/boxes/git", Destination: "/boxes/git", ReadWrite: true},
			{Type: "bind", Source: "/boxes/worktree", Destination: "/boxes/worktree", ReadWrite: true},
		},
	}
	info := mountInfo(mount("/", true), mount("/boxes/git", true), mount("/boxes/worktree", true))
	return plan, filesystem.Observation{
		Lima: limaMounts, GuestMountInfo: append([]byte(nil), info...),
		Docker:             compose.RuntimeObservation{Containers: []compose.ContainerObservation{container}},
		ContainerMountInfo: map[string][]byte{"container-1": append([]byte(nil), info...)},
	}
}

type fixtureMount struct {
	path     string
	writable bool
}

func mount(path string, writable bool) fixtureMount {
	return fixtureMount{path: path, writable: writable}
}

func mountInfo(mounts ...fixtureMount) []byte {
	var result []byte
	for _, item := range mounts {
		result = append(result, mountInfoLine(item.path, item.writable)...)
	}
	return result
}

func mountInfoLine(path string, writable bool) []byte {
	mode := "ro"
	if writable {
		mode = "rw"
	}
	return []byte("42 31 0:40 / " + path + " " + mode + ",relatime - virtiofs mount0 " + mode + "\n")
}

func assertError(t *testing.T, err error, kind filesystem.ErrorKind, layer string) {
	t.Helper()
	var verificationError *filesystem.Error
	if !errors.As(err, &verificationError) || verificationError.Kind != kind || verificationError.Layer != layer {
		t.Fatalf("error = %v, want kind %q layer %q", err, kind, layer)
	}
}

func TestVerifyAllowsStackedSystemMountsButRejectsStackedWorkspaceMounts(t *testing.T) {
	for _, layer := range []string{"guest-mountinfo", "container-mountinfo"} {
		t.Run(layer, func(t *testing.T) {
			plan, observed := exactFixture()
			stacked := []byte("40 27 0:34 / /proc/sys/fs/binfmt_misc rw,relatime - autofs systemd-1 rw\n217 40 0:42 / /proc/sys/fs/binfmt_misc rw,nosuid,nodev,noexec - binfmt_misc binfmt_misc rw\n")
			appendRecords := func(data []byte) {
				if layer == "guest-mountinfo" {
					observed.GuestMountInfo = append(observed.GuestMountInfo, data...)
				} else {
					observed.ContainerMountInfo["container-1"] = append(observed.ContainerMountInfo["container-1"], data...)
				}
			}
			appendRecords(stacked)
			if err := filesystem.Verify(plan, observed); err != nil {
				t.Fatalf("ordinary stacked system mounts rejected: %v", err)
			}
			appendRecords(mountInfoLine("/boxes/worktree", true))
			assertError(t, filesystem.Verify(plan, observed), filesystem.MountSetMismatch, layer)
		})
	}
}
