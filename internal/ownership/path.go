package ownership

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// FileIdentity is the stable filesystem identity recorded when a path is
// created. Device and inode are compared with fresh live observation.
type FileIdentity struct {
	Device uint64
	Inode  uint64
}

// PathRecord is the persisted canonical path and filesystem identity of an
// Agent Box-owned object.
type PathRecord struct {
	CanonicalPath string
	Identity      FileIdentity
}

// PathProof authorizes mutation below one verified canonical ownership root.
type PathProof struct {
	path     string
	identity FileIdentity
	mode     os.FileMode
}

func (p PathProof) Path() string           { return p.path }
func (p PathProof) Identity() FileIdentity { return p.identity }

// InspectPath returns the device/inode identity currently present at path. It
// uses lstat so a symlink's identity can never be confused with its target.
func InspectPath(path string) (FileIdentity, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return FileIdentity{}, fmt.Errorf("inspect path identity: %w", err)
	}
	return fileIdentity(info)
}

// ProvePath requires an absolute canonical target strictly below an absolute
// canonical root and a matching recorded/live device and inode. Symlinks in
// either path are rejected rather than followed for deletion.
func ProvePath(canonicalRoot string, recorded PathRecord) (PathProof, error) {
	if err := verifyCanonicalDirectory(canonicalRoot); err != nil {
		return PathProof{}, refuse("path", "ownership root is not canonical")
	}
	if err := verifyCanonicalPath(recorded.CanonicalPath); err != nil {
		return PathProof{}, refuse("path", "target is not canonical")
	}
	if !strictlyWithin(canonicalRoot, recorded.CanonicalPath) {
		return PathProof{}, refuse("path", "target is outside ownership root")
	}
	if recorded.Identity == (FileIdentity{}) {
		return PathProof{}, refuse("path", "recorded filesystem identity is empty")
	}

	info, err := os.Lstat(recorded.CanonicalPath)
	if err != nil {
		return PathProof{}, refuse("path", "target cannot be observed")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return PathProof{}, refuse("path", "target is a symbolic link")
	}
	liveIdentity, err := fileIdentity(info)
	if err != nil || liveIdentity != recorded.Identity {
		return PathProof{}, refuse("path", "recorded and live filesystem identities differ")
	}
	return PathProof{path: recorded.CanonicalPath, identity: liveIdentity, mode: info.Mode()}, nil
}

// GitWorktreeRecord is the persisted Git identity expected for a worktree.
type GitWorktreeRecord struct {
	CanonicalPath      string
	CanonicalCommonDir string
	LockReason         string
}

// GitWorktreeObservation is parsed from fresh Git porcelain and plumbing
// output immediately before authorization.
type GitWorktreeObservation struct {
	CanonicalPath      string
	CanonicalCommonDir string
	LockReason         string
}

// GitWorktreeProof authorizes a destructive Git worktree operation.
type GitWorktreeProof struct{ path string }

func (p GitWorktreeProof) Path() string { return p.path }

// ProveGitWorktree binds a fresh path proof to independently observed Git
// registration, common-directory identity, and Box lock reason.
func ProveGitWorktree(path PathProof, recorded GitWorktreeRecord, live GitWorktreeObservation) (GitWorktreeProof, error) {
	if path.path == "" || recorded.CanonicalPath != path.path {
		return GitWorktreeProof{}, refuse("git worktree", "path proof targets a different path")
	}
	if err := validateCanonicalString(recorded.CanonicalCommonDir); err != nil {
		return GitWorktreeProof{}, refuse("git worktree", "common directory is not canonical")
	}
	if !validOpaque(recorded.LockReason) {
		return GitWorktreeProof{}, refuse("git worktree", "lock reason is empty")
	}
	if live.CanonicalPath != recorded.CanonicalPath {
		return GitWorktreeProof{}, refuse("git worktree", "git registration path differs")
	}
	if live.CanonicalCommonDir != recorded.CanonicalCommonDir {
		return GitWorktreeProof{}, refuse("git worktree", "git common directory differs")
	}
	if live.LockReason != recorded.LockReason {
		return GitWorktreeProof{}, refuse("git worktree", "git lock reason differs")
	}
	return GitWorktreeProof{path: path.path}, nil
}

// SocketProof authorizes unlinking one verified Unix-domain socket path.
type SocketProof struct{ path string }

func (p SocketProof) Path() string { return p.path }

// ProveSocket specializes a fresh path proof and refuses regular files,
// directories, and symbolic links at the expected socket path.
func ProveSocket(path PathProof) (SocketProof, error) {
	if path.path == "" || path.mode.Type() != os.ModeSocket {
		return SocketProof{}, refuse("socket", "verified path is not a Unix socket")
	}
	return SocketProof{path: path.path}, nil
}

func verifyCanonicalDirectory(path string) error {
	if err := verifyCanonicalPath(path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("path is not a real directory")
	}
	return nil
}

func verifyCanonicalPath(path string) error {
	if err := validateCanonicalString(path); err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolve canonical path: %w", err)
	}
	if resolved != path {
		return fmt.Errorf("path contains a symbolic link")
	}
	return nil
}

func validateCanonicalString(path string) error {
	if path == "" || strings.IndexByte(path, 0) >= 0 || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("path is not an absolute clean path")
	}
	return nil
}

func strictlyWithin(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != "." && relative != ".." && !filepath.IsAbs(relative) &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func fileIdentity(info os.FileInfo) (FileIdentity, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return FileIdentity{}, fmt.Errorf("filesystem identity is unavailable")
	}
	return FileIdentity{Device: uint64(stat.Dev), Inode: uint64(stat.Ino)}, nil
}
