//go:build linux || darwin

package daemon

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const (
	runtimeMode = 0o700
	privateMode = 0o600
)

type fileIdentity struct {
	device uint64
	inode  uint64
}

type preparedRuntime struct {
	socketPath string
	lock       *os.File
}

func prepareRuntime(runtimeDir string) (_ preparedRuntime, returnErr error) {
	if runtimeDir == "" || !filepath.IsAbs(runtimeDir) || filepath.Clean(runtimeDir) != runtimeDir {
		return preparedRuntime{}, errors.New("runtime directory must be a clean absolute path")
	}
	if filepath.Base(runtimeDir) == string(filepath.Separator) {
		return preparedRuntime{}, errors.New("runtime directory cannot be the filesystem root")
	}
	unix.Umask(0o077)

	parent := filepath.Dir(runtimeDir)
	if err := verifyOwnedDirectory(parent, false); err != nil {
		return preparedRuntime{}, fmt.Errorf("verify runtime parent: %w", err)
	}
	if err := unix.Mkdir(runtimeDir, runtimeMode); err != nil && !errors.Is(err, unix.EEXIST) {
		return preparedRuntime{}, fmt.Errorf("create runtime directory: %w", err)
	}
	if err := verifyOwnedDirectory(runtimeDir, true); err != nil {
		return preparedRuntime{}, fmt.Errorf("verify runtime directory: %w", err)
	}

	lockPath := filepath.Join(runtimeDir, "aboxd.lock")
	fd, err := unix.Open(lockPath, unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW, privateMode)
	if err != nil {
		return preparedRuntime{}, fmt.Errorf("open daemon lock: %w", err)
	}
	lock := os.NewFile(uintptr(fd), lockPath)
	defer func() {
		if returnErr != nil {
			_ = lock.Close()
		}
	}()
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return preparedRuntime{}, fmt.Errorf("acquire daemon lock: %w", err)
	}
	if err := verifyPrivateFileFD(fd); err != nil {
		return preparedRuntime{}, fmt.Errorf("verify daemon lock: %w", err)
	}
	if err := lock.Chmod(privateMode); err != nil {
		return preparedRuntime{}, fmt.Errorf("restrict daemon lock: %w", err)
	}

	socketPath := runtimeSocketPath(runtimeDir)
	if _, err := os.Lstat(socketPath); err == nil {
		return preparedRuntime{}, errors.New("socket path already exists; refusing stale or active endpoint")
	} else if !errors.Is(err, os.ErrNotExist) {
		return preparedRuntime{}, fmt.Errorf("inspect socket path: %w", err)
	}
	return preparedRuntime{socketPath: socketPath, lock: lock}, nil
}

func bindSocket(path string) (*net.UnixListener, fileIdentity, error) {
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, fileIdentity{}, fmt.Errorf("bind daemon socket: %w", err)
	}
	listener.SetUnlinkOnClose(false)
	identity, err := inspectOwnedSocket(path, false)
	if err != nil {
		_ = listener.Close()
		return nil, fileIdentity{}, err
	}
	if err := os.Chmod(path, privateMode); err != nil {
		_ = listener.Close()
		_ = removeExactSocket(path, identity)
		return nil, fileIdentity{}, fmt.Errorf("restrict daemon socket: %w", err)
	}
	verified, err := inspectOwnedSocket(path, true)
	if err != nil {
		_ = listener.Close()
		_ = removeExactSocket(path, identity)
		return nil, fileIdentity{}, err
	}
	if verified != identity {
		_ = listener.Close()
		return nil, fileIdentity{}, errors.New("daemon socket identity changed during bind")
	}
	return listener, verified, nil
}

func verifyOwnedDirectory(path string, exactMode bool) error {
	var status unix.Stat_t
	if err := unix.Lstat(path, &status); err != nil {
		return err
	}
	if status.Mode&unix.S_IFMT != unix.S_IFDIR {
		return errors.New("path is not a real directory")
	}
	if status.Uid != uint32(os.Geteuid()) {
		return errors.New("directory is not owned by the daemon user")
	}
	permissions := os.FileMode(status.Mode).Perm()
	if exactMode && permissions != runtimeMode {
		return fmt.Errorf("directory mode is %04o, want %04o", permissions, runtimeMode)
	}
	if !exactMode && permissions&0o022 != 0 {
		return fmt.Errorf("directory mode is %04o and writable by group or other users", permissions)
	}
	return nil
}

func verifyPrivateFileFD(fd int) error {
	var status unix.Stat_t
	if err := unix.Fstat(fd, &status); err != nil {
		return err
	}
	if status.Mode&unix.S_IFMT != unix.S_IFREG {
		return errors.New("lock path is not a regular file")
	}
	if status.Uid != uint32(os.Geteuid()) {
		return errors.New("lock file is not owned by the daemon user")
	}
	return nil
}

func inspectOwnedSocket(path string, requirePrivateMode bool) (fileIdentity, error) {
	var status unix.Stat_t
	if err := unix.Lstat(path, &status); err != nil {
		return fileIdentity{}, fmt.Errorf("inspect daemon socket: %w", err)
	}
	if status.Mode&unix.S_IFMT != unix.S_IFSOCK {
		return fileIdentity{}, errors.New("daemon socket path is not a socket")
	}
	if status.Uid != uint32(os.Geteuid()) {
		return fileIdentity{}, errors.New("daemon socket is not owned by the daemon user")
	}
	if permissions := os.FileMode(status.Mode).Perm(); requirePrivateMode && permissions != privateMode {
		return fileIdentity{}, fmt.Errorf("daemon socket mode is %04o, want %04o", permissions, privateMode)
	}
	return fileIdentity{device: uint64(status.Dev), inode: status.Ino}, nil
}

func removeExactSocket(path string, expected fileIdentity) error {
	current, err := inspectOwnedSocket(path, false)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cleanup daemon socket: %w", err)
	}
	if current != expected {
		return errors.New("cleanup daemon socket: socket identity changed")
	}
	if err := unix.Unlink(path); err != nil && !errors.Is(err, unix.ENOENT) {
		return fmt.Errorf("cleanup daemon socket: %w", err)
	}
	return nil
}

type peerListener struct {
	*net.UnixListener
	uid uint32
}

func newPeerListener(listener *net.UnixListener, uid uint32) net.Listener {
	return &peerListener{UnixListener: listener, uid: uid}
}

func (listener *peerListener) Accept() (net.Conn, error) {
	for {
		connection, err := listener.AcceptUnix()
		if err != nil {
			return nil, err
		}
		uid, err := peerUID(connection)
		if err == nil && uid == listener.uid {
			return connection, nil
		}
		_ = connection.Close()
	}
}
