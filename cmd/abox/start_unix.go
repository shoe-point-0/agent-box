//go:build linux || darwin

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/shoe-point-0/agent-box/internal/daemon"
	"golang.org/x/sys/unix"
)

func startDaemon(paths daemon.UserPaths) (<-chan error, error) {
	executable, err := daemonExecutable()
	if err != nil {
		return nil, err
	}
	log, err := openDaemonLog(paths.LogFile)
	if err != nil {
		return nil, err
	}
	command := exec.Command(executable, "serve", "--runtime-dir", paths.RuntimeDir, "--state-db", paths.StateDatabase)
	command.Stdin = nil
	command.Stdout = log
	command.Stderr = log
	command.Env = os.Environ()
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		_ = log.Close()
		return nil, fmt.Errorf("start aboxd: %w", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- errors.Join(command.Wait(), log.Close())
		close(done)
	}()
	return done, nil
}

func daemonExecutable() (string, error) {
	current, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(current), "aboxd")
		if info, statErr := os.Stat(candidate); statErr == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return candidate, nil
		}
	}
	path, err := exec.LookPath("aboxd")
	if err != nil {
		return "", errors.New("find aboxd executable: install abox and aboxd together or add aboxd to PATH")
	}
	return path, nil
}

func openDaemonLog(path string) (*os.File, error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create daemon log directory: %w", err)
	}
	var status unix.Stat_t
	if err := unix.Lstat(directory, &status); err != nil {
		return nil, fmt.Errorf("inspect daemon log directory: %w", err)
	}
	if status.Mode&unix.S_IFMT != unix.S_IFDIR || status.Uid != uint32(os.Geteuid()) {
		return nil, errors.New("daemon log directory is not an owned real directory")
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return nil, fmt.Errorf("restrict daemon log directory: %w", err)
	}
	fd, err := unix.Open(path, unix.O_WRONLY|unix.O_CREAT|unix.O_APPEND|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open daemon log: %w", err)
	}
	if err := unix.Fstat(fd, &status); err != nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("inspect daemon log: %w", err)
	}
	if status.Mode&unix.S_IFMT != unix.S_IFREG || status.Uid != uint32(os.Geteuid()) {
		_ = unix.Close(fd)
		return nil, errors.New("daemon log is not an owned regular file")
	}
	if err := unix.Fchmod(fd, 0o600); err != nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("restrict daemon log: %w", err)
	}
	return os.NewFile(uintptr(fd), path), nil
}
