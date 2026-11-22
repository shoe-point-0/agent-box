package doctor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/shoe-point-0/agent-box/internal/daemon"
	"github.com/shoe-point-0/agent-box/internal/machinegit"
	"github.com/shoe-point-0/agent-box/internal/toolchain"
)

const (
	httpRouterAddress = "127.0.0.1:7777"
	probeTimeout      = 3 * time.Second
)

// LocalObserver inspects only existing Machine state. It deliberately has no
// listener, daemon-start, state-write, filesystem-probe, or runtime-mutation
// dependency.
type LocalObserver struct {
	WorkingDirectory string
	Git              *machinegit.DiagnosticClient
	Tools            *toolchain.Diagnostics
	configuration    *Check
}

func NewLocalObserver(workingDirectory string) (*LocalObserver, error) {
	if !filepath.IsAbs(workingDirectory) || filepath.Clean(workingDirectory) != workingDirectory {
		return nil, errors.New("doctor working directory must be clean and absolute")
	}
	// An invalid environment is an observable prerequisite, not an internal
	// failure. Do not construct either tool client until validation succeeds.
	home, _ := os.UserHomeDir()
	path := os.Getenv("PATH")
	tools, err := toolchain.NewDiagnostics(path, home, workingDirectory)
	if err != nil {
		check := Check{Evidence: EvidenceTested, Status: StatusFail}
		switch {
		case errors.Is(err, toolchain.ErrDiagnosticPath):
			check.Result = "environment.path"
			check.Observed = "PATH is unset or contains an empty, relative, non-clean, or control-character entry"
			check.Required = "PATH containing only non-empty, clean absolute directories without control characters"
			check.Remediation = "set PATH to absolute directories; remove empty or relative entries, trailing slashes (except /), dot segments, and control characters; preserve spaces in directory names, then rerun abox doctor"
		case errors.Is(err, toolchain.ErrDiagnosticHome):
			check.Result = "environment.home"
			check.Observed = "HOME is unset or is not a clean absolute directory without control characters"
			check.Required = "a non-empty, clean absolute HOME directory without control characters"
			check.Remediation = "set HOME to your absolute home directory without trailing slashes (except /), dot segments, or control characters, then rerun abox doctor"
		default:
			return nil, err
		}
		return &LocalObserver{WorkingDirectory: workingDirectory, configuration: &check}, nil
	}
	var git *machinegit.DiagnosticClient
	if explicit := os.Getenv("HERMES_CRUSHER_REAL_GIT"); explicit != "" {
		git, _ = machinegit.NewDefaultDiagnosticClientAt(explicit, path, home, workingDirectory)
	} else {
		git, _ = machinegit.NewDefaultDiagnosticClient(path, home, workingDirectory)
	}
	return &LocalObserver{WorkingDirectory: workingDirectory, Git: git, Tools: tools}, nil
}

func (observer *LocalObserver) Observe(ctx context.Context, request Request) ([]Check, error) {
	if observer == nil || !filepath.IsAbs(observer.WorkingDirectory) || (observer.Tools == nil && observer.configuration == nil) {
		return nil, errors.New("local doctor observer is incomplete")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	checks := []Check{observer.platform(), observer.ownership()}
	if observer.configuration != nil {
		checks = append(checks, *observer.configuration)
	}
	paths, err := daemon.DefaultUserPaths()
	if err != nil {
		checks = append(checks, unknown("disk.runtime", "user-scoped runtime path unavailable", "canonical user-scoped runtime path", "correct the home and XDG path configuration, then rerun abox doctor"))
		checks = append(checks, unknown("disk.state", "user-scoped state path unavailable", "canonical user-scoped state path", "correct the home and XDG path configuration, then rerun abox doctor"))
	} else {
		checks = append(checks, observer.disk("disk.runtime", paths.RuntimeDir), observer.disk("disk.state", filepath.Dir(paths.StateDatabase)))
		checks = append(checks, installationMetadata(filepath.Join(filepath.Dir(paths.StateDatabase), "installation.json")))
	}

	git := observer.gitVersion(ctx)
	checks = append(checks, git)
	checks = append(checks,
		observer.tool(ctx, "tool.lima", toolchain.ProbeLimaVersion, "limactl version ", "install Lima using https://lima-vm.io/docs/installation/ and place limactl on PATH; runtime provisioning also requires installation.json (see docs/getting-started.md)"),
		observer.tool(ctx, "tool.docker", toolchain.ProbeDockerVersion, "Docker version ", "install the pinned Docker client and place docker on PATH"),
		observer.tool(ctx, "tool.compose", toolchain.ProbeComposeVersion, "", "install the pinned Docker Compose plugin"),
		unknown("tool.artifact-integrity", "version probes do not establish artifact identity or SHA-256 integrity", "complete pinned toolchain identities and matching SHA-256 digests", "run Machine preflight with the installed toolchain specification"),
	)
	checks = append(checks, observer.relativeWorktrees(ctx, git), observer.port(), observer.runtime(ctx))

	if request.Mode != ModeMachine {
		checks = append(checks,
			unknown("box.mounts", "no retained live mount evidence was read", "exact Plan, Lima, guest, Docker, and container mount sets", "start or reconcile the Box, then inspect its retained readiness evidence"),
			unknown("box.ownership", "no independently observed runtime ownership facts were read", "matching recorded and live Box identities", "start or reconcile the Box, then inspect its retained ownership evidence"),
			unknown("box.ports", "no retained Box port allocation evidence was read", "live loopback allocations with matching daemon leases", "start or reconcile the Box, then inspect its retained routing evidence"),
			unknown("box.runtime", "no existing daemon or runtime was contacted", "live runtime host and service state for "+request.TaskKey, "run abox up for the Box or inspect it while aboxd is already running"),
		)
	}
	if request.Mode == ModeFilesystem {
		checks = append(checks,
			warning("filesystem.probe", "doctor did not create a canary in a worktree", "retained bidirectional bytes, mode, rename, deletion, and cleanup evidence", "reconcile the Box to rerun the readiness probe; doctor never mutates a worktree"),
			warning("filesystem.watcher", "native watcher delivery was not retested", "retained native watcher evidence for the active mount transport", "use a polling watcher until the Box readiness probe records native delivery"),
		)
	}
	return checks, nil
}

func (observer *LocalObserver) platform() Check {
	observed := runtime.GOOS + "/" + runtime.GOARCH
	if runtime.GOOS == "linux" {
		return warning("machine.platform", observed+" engineering lane; production up currently rejects Linux", "production preflight currently requires darwin/arm64 with VZ", "Linux supports build/control-plane testing, not production provisioning; installing QEMU does not unlock abox up (see docs/getting-started.md)")
	}
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		return warning("machine.platform", observed+"; VZ availability not inferred", "darwin/arm64 with an observed supported VZ accelerator", "run the real Darwin hardware gate before claiming runtime support")
	}
	return Check{Result: "machine.platform", Evidence: EvidenceUnsupported, Status: StatusFail, Observed: observed, Required: "linux engineering lane or darwin/arm64 product lane", Remediation: "use a supported Machine platform"}
}

func (observer *LocalObserver) ownership() Check {
	return Check{
		Result: "machine.ownership", Evidence: EvidenceTested, Status: StatusPass,
		Observed: fmt.Sprintf("uid=%d gid=%d", os.Geteuid(), os.Getegid()), Required: "non-root user-scoped execution identity",
	}
}

func (observer *LocalObserver) disk(result, target string) Check {
	ancestor, exists, err := existingAncestor(target)
	if err != nil {
		return unknown(result, "filesystem capacity unavailable", "readable containing filesystem for "+target, "make the existing parent searchable and rerun abox doctor")
	}
	var statistics unix.Statfs_t
	if err := unix.Statfs(ancestor, &statistics); err != nil {
		return unknown(result, "statfs unavailable for existing parent", "readable filesystem capacity for "+target, "make the existing parent searchable and rerun abox doctor")
	}
	available := uint64(statistics.Bavail) * uint64(statistics.Bsize)
	observed := fmt.Sprintf("path=%s existingAncestor=%s availableBytes=%d", target, ancestor, available)
	if !exists {
		return warning(result, observed, "existing usable path with an explicit free-space budget", "the preflight owner must create the path only as part of an authorized operation")
	}
	return Check{Result: result, Evidence: EvidenceTested, Status: StatusPass, Observed: observed, Required: "existing readable path and filesystem capacity"}
}

func (observer *LocalObserver) gitVersion(ctx context.Context) Check {
	if observer.configuration != nil {
		return observer.skippedTool("tool.git")
	}
	const remediation = "install the supported Git version and place it on PATH"
	if observer.Git == nil {
		return unavailableTool("tool.git", remediation)
	}
	probeContext, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	result, err := observer.Git.Observe(probeContext, machinegit.ProbeVersion)
	return versionCheck("tool.git", result.Stdout, result.Stderr, result.StdoutTruncated || result.StderrTruncated || err != nil, "git version ", remediation)
}

func (observer *LocalObserver) tool(ctx context.Context, name string, probe toolchain.DiagnosticProbe, prefix, remediation string) Check {
	if observer.configuration != nil {
		return observer.skippedTool(name)
	}
	probeContext, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	result, err := observer.Tools.Observe(probeContext, probe)
	return versionCheck(name, result.Stdout, result.Stderr, result.StdoutTruncated || result.StderrTruncated || err != nil, prefix, remediation)
}

func (observer *LocalObserver) skippedTool(result string) Check {
	return unknown(result, "probe skipped because the diagnostic environment is invalid", observer.configuration.Required, observer.configuration.Remediation)
}

func versionCheck(result string, stdout, stderr []byte, failed bool, prefix, remediation string) Check {
	if failed {
		return unavailableTool(result, remediation)
	}
	value := string(bytes.TrimSpace(append(append([]byte(nil), stdout...), stderr...)))
	value = strings.TrimSpace(strings.TrimPrefix(value, prefix))
	if result == "tool.docker" {
		value = strings.TrimSuffix(strings.Split(value, ",")[0], ",")
	}
	if value == "" || len(value) > 256 || strings.ContainsAny(value, "\x00\r\n") {
		return Check{Result: result, Evidence: EvidenceUnsupported, Status: StatusFail, Observed: "invalid version output", Required: "one bounded version value", Remediation: remediation}
	}
	return Check{Result: result, Evidence: EvidenceTested, Status: StatusPass, Observed: value, Required: "supported version; artifact digest is verified separately by Machine preflight"}
}

func unavailableTool(result, remediation string) Check {
	return Check{Result: result, Evidence: EvidenceUnsupported, Status: StatusFail, Observed: "unavailable", Required: "supported executable with bounded version output", Remediation: remediation}
}

func (observer *LocalObserver) relativeWorktrees(ctx context.Context, git Check) Check {
	if observer.configuration != nil {
		return observer.skippedTool("git.relative-worktrees")
	}
	if git.Status != StatusPass || observer.Git == nil {
		return unknown("git.relative-worktrees", "Git capability was not observable", "worktree.useRelativePaths capability on every Box Git client", "install supported Git or keep invariant absolute worktree paths")
	}
	probeContext, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	result, _ := observer.Git.Observe(probeContext, machinegit.ProbeRelativeWorktrees)
	output := string(append(append([]byte(nil), result.Stdout...), result.Stderr...))
	if strings.Contains(output, "--relative-paths") {
		return Check{Result: "git.relative-worktrees", Evidence: EvidenceTested, Status: StatusPass, Observed: "git worktree add exposes --relative-paths", Required: "relative-worktree capability on every Box Git client"}
	}
	return Check{Result: "git.relative-worktrees", Evidence: EvidenceUnsupported, Status: StatusAdvisory, Observed: "git worktree add does not expose --relative-paths", Required: "relative-worktree capability on every Box Git client", Remediation: "keep invariant absolute paths; never rewrite .git pointer files manually"}
}

func (observer *LocalObserver) port() Check {
	return unknown("port.http-router", httpRouterAddress+" was not bound or contacted", "availability proven by the preflight owner immediately before bind", "run Machine preflight; doctor does not bind, connect to, or reserve ports")
}

func (observer *LocalObserver) runtime(ctx context.Context) Check {
	if observer.configuration != nil {
		return observer.skippedTool("runtime.docker")
	}
	probeContext, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	result, err := observer.Tools.Observe(probeContext, toolchain.ProbeDockerRuntime)
	value := string(bytes.TrimSpace(append(append([]byte(nil), result.Stdout...), result.Stderr...)))
	if err != nil || result.StdoutTruncated || result.StderrTruncated || value == "" || len(value) > 256 || strings.ContainsAny(value, "\x00\r\n") {
		return warning("runtime.docker", "host Docker runtime was not observable; this does not establish Box guest readiness", "optional host client/server runtime identity; Box Docker runs inside its Lima guest", "a host Docker daemon is not a substitute for the Box guest; configure the verified runtime installation before abox up (see docs/getting-started.md)")
	}
	return Check{Result: "runtime.docker", Evidence: EvidenceTested, Status: StatusPass, Observed: value, Required: "readable client/server runtime identity"}
}

// Doctor cannot know a running daemon's --installation override. Report only
// the default metadata location, without claiming content or guest readiness.
func installationMetadata(path string) Check {
	const required = "verified runtime artifacts and a matching guest image, configured through installation.json or aboxd --installation"
	const remediation = "see docs/getting-started.md for the installation contract; installing limactl alone is insufficient"
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return unknown("runtime.installation", "default installation metadata unavailable at "+path+"; daemon overrides were not inspected", required, remediation)
	}
	return unknown("runtime.installation", "default installation metadata present at "+path+"; contents and artifact digests were not verified", required, remediation)
}

func existingAncestor(path string) (ancestor string, exists bool, err error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", false, errors.New("path is not clean and absolute")
	}
	current := path
	for {
		info, statErr := os.Lstat(current)
		if statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return "", false, errors.New("path ancestor is a symlink")
			}
			return current, current == path, nil
		}
		if !errors.Is(statErr, syscall.ENOENT) {
			return "", false, statErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false, statErr
		}
		current = parent
	}
}

func warning(result, observed, required, remediation string) Check {
	return Check{Result: result, Evidence: EvidenceWarning, Status: StatusAdvisory, Observed: observed, Required: required, Remediation: remediation}
}

func unknown(result, observed, required, remediation string) Check {
	return Check{Result: result, Evidence: EvidenceUnknown, Status: StatusAdvisory, Observed: observed, Required: required, Remediation: remediation}
}
