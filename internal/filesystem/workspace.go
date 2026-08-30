package filesystem

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io/fs"
	"path/filepath"
	"strconv"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/lima"
)

const (
	workspaceShellPath = "/bin/sh"
	maxProbeDataBytes  = 256

	observeProbeScript = `if [ ! -e "$1" ]; then
	printf 'A\n'
	exit 0
fi
if [ ! -f "$1" ] || [ -L "$1" ]; then
	printf 'N\n'
	exit 0
fi
printf 'F %s\n' "$(stat -c '%a' -- "$1")"
cat -- "$1"`
	writeProbeScript = `umask 077
set -C
printf '%s' "$2" > "$1"
chmod "$3" "$1"`
	renameProbeScript = `mv -- "$1" "$2"`
	removeProbeScript = `rm -- "$1"`
	watchProbeScript  = `umask 077
directory=${1%/*}
target=${1##*/}
ready=$2
handshake=watch-handshake
fifo=/tmp/abox-watch-$$
watcher=
producer=
cleanup() {
	if [ -n "$producer" ]; then kill "$producer" 2>/dev/null || :; fi
	if [ -n "$watcher" ]; then kill "$watcher" 2>/dev/null || :; fi
	rm -f -- "$fifo" "$directory/$handshake" "$ready"
}
trap cleanup EXIT HUP INT TERM
mkfifo -m 600 "$fifo"
exec 3<>"$fifo"
inotifyd - "$directory":ncw >"$fifo" &
watcher=$!
(
	while :; do
		: >"$directory/$handshake"
		sleep 0.01
	done
) &
producer=$!
tab=$(printf '	')
while IFS="$tab" read -r events watched subfile <&3; do
	if [ "$subfile" = "$handshake" ]; then break; fi
done
kill "$producer" 2>/dev/null || :
wait "$producer" 2>/dev/null || :
producer=
rm -f -- "$directory/$handshake"
: >"$ready"
while IFS="$tab" read -r events watched subfile <&3; do
	if [ "$subfile" != "$target" ]; then continue; fi
	case "$events" in *n*|*c*|*w*) exit 0 ;; esac
done
exit 1`
)

// WorkspaceCommandHost is the bounded guest-command boundary implemented by
// lima.Adapter. Probe commands execute inside an immutable Docker container ID.
type WorkspaceCommandHost interface {
	Capture(context.Context, lima.Spec, string, []string) (lima.Capture, error)
}

// WorkspaceAdapter performs the probe's fixed command set in a workspace
// container. Scripts are constants and all runtime values are positional argv;
// no path or probe data is interpolated into shell source.
type WorkspaceAdapter struct {
	host    WorkspaceCommandHost
	runtime compose.RuntimeSpec
}

func NewWorkspaceAdapter(host WorkspaceCommandHost, runtime compose.RuntimeSpec) (*WorkspaceAdapter, error) {
	if host == nil {
		return nil, failure(InvalidObservation, "workspace-command", "runtime boundary is nil")
	}
	if !cleanAbsolute(runtime.DockerPath) || !cleanAbsolute(runtime.Workdir) {
		return nil, failure(InvalidObservation, "workspace-command", "runtime paths are invalid")
	}
	return &WorkspaceAdapter{host: host, runtime: runtime}, nil
}

func (a *WorkspaceAdapter) Observe(ctx context.Context, containerID, path string) (ProbeFile, error) {
	if !safeContainerID(containerID) || !validProbePath(path) {
		return ProbeFile{}, failure(InvalidObservation, "workspace-observe", "probe target is invalid")
	}
	capture, err := a.capture(ctx, containerID, "observe", observeProbeScript, path)
	if err != nil {
		return ProbeFile{}, err
	}
	return decodeProbeFile(capture.Stdout)
}

func (a *WorkspaceAdapter) Write(ctx context.Context, containerID, path string, data []byte, mode fs.FileMode) error {
	if !safeContainerID(containerID) || !validProbePath(path) || !validProbeData(data) || (mode.Perm() != 0o600 && mode.Perm() != 0o640) {
		return failure(InvalidObservation, "workspace-write", "probe request is invalid")
	}
	return a.mutate(ctx, containerID, "write", writeProbeScript, path, string(data), modeString(mode))
}

func (a *WorkspaceAdapter) Rename(ctx context.Context, containerID, oldPath, newPath string) error {
	if !safeContainerID(containerID) || !validProbePath(oldPath) || !validProbePath(newPath) || filepath.Dir(oldPath) != filepath.Dir(newPath) {
		return failure(InvalidObservation, "workspace-rename", "probe request is invalid")
	}
	return a.mutate(ctx, containerID, "rename", renameProbeScript, oldPath, newPath)
}

func (a *WorkspaceAdapter) Remove(ctx context.Context, containerID, path string) error {
	if !safeContainerID(containerID) || !validProbePath(path) {
		return failure(InvalidObservation, "workspace-remove", "probe request is invalid")
	}
	return a.mutate(ctx, containerID, "remove", removeProbeScript, path)
}

// Watch blocks until the workspace container's native inotify watcher receives
// a create, modify, or close-write event for target. The ready file is created
// only after an in-container handshake proves the watch is active.
func (a *WorkspaceAdapter) Watch(ctx context.Context, containerID, target, ready string) error {
	if !safeContainerID(containerID) || !validProbePath(target) || !validProbePath(ready) ||
		filepath.Dir(target) != filepath.Dir(ready) || filepath.Base(target) != watchTargetName || filepath.Base(ready) != watchReadyName {
		return failure(InvalidObservation, "workspace-watch", "probe request is invalid")
	}
	return a.mutate(ctx, containerID, "watch", watchProbeScript, target, ready)
}

func (a *WorkspaceAdapter) mutate(ctx context.Context, containerID, operation, script string, arguments ...string) error {
	capture, err := a.capture(ctx, containerID, operation, script, arguments...)
	if err != nil {
		return err
	}
	if len(capture.Stdout) != 0 {
		return failure(ProbeFailed, "workspace-"+operation, "workspace probe output is invalid")
	}
	return nil
}

func (a *WorkspaceAdapter) capture(ctx context.Context, containerID, operation, script string, arguments ...string) (lima.Capture, error) {
	argv := []string{a.runtime.DockerPath, "exec", "--", containerID, workspaceShellPath, "-eu", "-c", script, "abox-probe"}
	argv = append(argv, arguments...)
	capture, err := a.host.Capture(ctx, a.runtime.RuntimeHost, a.runtime.Workdir, argv)
	if err != nil {
		return lima.Capture{}, workspaceCommandFailure(ctx, operation, err)
	}
	if capture.StdoutTruncated || capture.StderrTruncated || len(bytes.TrimSpace(capture.Stderr)) != 0 || len(capture.Stdout) > maxProbeDataBytes+32 {
		return lima.Capture{}, failure(ProbeFailed, "workspace-"+operation, "workspace probe output is invalid")
	}
	return capture, nil
}

func decodeProbeFile(data []byte) (ProbeFile, error) {
	if bytes.Equal(data, []byte("A\n")) {
		return ProbeFile{}, nil
	}
	if bytes.Equal(data, []byte("N\n")) {
		return ProbeFile{Exists: true}, nil
	}
	header, content, found := bytes.Cut(data, []byte{'\n'})
	if !found || !bytes.HasPrefix(header, []byte("F ")) || len(content) > maxProbeDataBytes {
		return ProbeFile{}, failure(ProbeFailed, "workspace-observe", "workspace probe output is malformed")
	}
	mode, err := strconv.ParseUint(string(bytes.TrimPrefix(header, []byte("F "))), 8, 12)
	if err != nil || mode > 0o777 {
		return ProbeFile{}, failure(ProbeFailed, "workspace-observe", "workspace probe output is malformed")
	}
	return ProbeFile{Exists: true, Regular: true, Mode: fs.FileMode(mode), Data: append([]byte(nil), content...)}, nil
}

func validProbePath(path string) bool {
	if !cleanAbsolute(path) || filepath.Base(path) == "." || filepath.Base(path) == string(filepath.Separator) {
		return false
	}
	nonceDirectory := filepath.Dir(path)
	if filepath.Base(filepath.Dir(nonceDirectory)) != probeDirectory {
		return false
	}
	nonce := filepath.Base(nonceDirectory)
	decoded, err := hex.DecodeString(nonce)
	return err == nil && len(decoded) == probeNonceBytes
}

func validProbeData(data []byte) bool {
	return len(data) > 0 && len(data) <= maxProbeDataBytes && !bytes.ContainsAny(data, "\x00\r\n")
}

func modeString(mode fs.FileMode) string {
	return "0" + strconv.FormatUint(uint64(mode.Perm()), 8)
}

func workspaceCommandFailure(ctx context.Context, operation string, err error) error {
	var limaError *lima.Error
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		(errors.As(err, &limaError) && limaError.Kind == lima.Canceled) {
		return failure(ProbeFailed, "workspace-"+operation, "workspace probe was canceled")
	}
	return failure(ProbeFailed, "workspace-"+operation, "workspace probe command failed")
}

var _ WorkspaceProbe = (*WorkspaceAdapter)(nil)
