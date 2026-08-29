// Package paths owns construction of Agent Box platform and state paths.
package paths

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/shoe-point-0/agent-box/internal/identity"
)

const maxComponentBytes = 255

// Provider constructs every Agent Box-owned filesystem path from validated roots.
type Provider struct {
	applicationRoot string
	cacheRoot       string
	logRoot         string
}

// New returns a path provider using explicit canonical platform roots.
func New(applicationRoot, cacheRoot, logRoot string) (*Provider, error) {
	roots := []struct {
		name string
		path string
	}{
		{name: "application", path: applicationRoot},
		{name: "cache", path: cacheRoot},
		{name: "log", path: logRoot},
	}
	for _, root := range roots {
		if err := validateRoot(root.path); err != nil {
			return nil, fmt.Errorf("%s root: %w", root.name, err)
		}
	}
	return &Provider{applicationRoot: applicationRoot, cacheRoot: cacheRoot, logRoot: logRoot}, nil
}

// Darwin returns the Section 15 macOS layout below a canonical home path.
// It only constructs paths; it does not claim that Darwin behavior was exercised.
func Darwin(canonicalHome string) (*Provider, error) {
	if err := validateRoot(canonicalHome); err != nil {
		return nil, fmt.Errorf("Darwin home: %w", err)
	}
	return New(
		filepath.Join(canonicalHome, "Library", "Application Support", "Agent Box"),
		filepath.Join(canonicalHome, "Library", "Caches", "Agent Box"),
		filepath.Join(canonicalHome, "Library", "Logs", "Agent Box"),
	)
}

func (p *Provider) ApplicationRoot() string   { return p.applicationRoot }
func (p *Provider) CacheRoot() string         { return p.cacheRoot }
func (p *Provider) LogRoot() string           { return p.logRoot }
func (p *Provider) RuntimeRoot() string       { return filepath.Join(p.applicationRoot, "runtime") }
func (p *Provider) StateDatabase() string     { return filepath.Join(p.applicationRoot, "state.db") }
func (p *Provider) BoxesRoot() string         { return filepath.Join(p.applicationRoot, "boxes") }
func (p *Provider) RepositoriesRoot() string  { return filepath.Join(p.applicationRoot, "repositories") }
func (p *Provider) ToolsRoot() string         { return filepath.Join(p.applicationRoot, "tools") }
func (p *Provider) DownloadsRoot() string     { return filepath.Join(p.cacheRoot, "downloads") }
func (p *Provider) ImageMetadataRoot() string { return filepath.Join(p.cacheRoot, "image-metadata") }
func (p *Provider) DaemonLog() string         { return filepath.Join(p.logRoot, "aboxd.log") }
func (p *Provider) DaemonSocket() string      { return filepath.Join(p.RuntimeRoot(), "aboxd.sock") }
func (p *Provider) DaemonPID() string         { return filepath.Join(p.RuntimeRoot(), "aboxd.pid") }

// BoxRoot returns the root owned by boxID.
func (p *Provider) BoxRoot(boxID identity.BoxID) (string, error) {
	if err := identity.ValidateBoxID(boxID); err != nil {
		return "", fmt.Errorf("box root: %w", err)
	}
	return filepath.Join(p.BoxesRoot(), boxID.String()), nil
}

func (p *Provider) boxPath(boxID identity.BoxID, elements ...string) (string, error) {
	root, err := p.BoxRoot(boxID)
	if err != nil {
		return "", err
	}
	return filepath.Join(append([]string{root}, elements...)...), nil
}

func (p *Provider) Plan(boxID identity.BoxID) (string, error) {
	return p.boxPath(boxID, "plan.json")
}
func (p *Provider) Context(boxID identity.BoxID) (string, error) {
	return p.boxPath(boxID, "context.json")
}
func (p *Provider) LimaConfig(boxID identity.BoxID) (string, error) {
	return p.boxPath(boxID, "lima.yaml")
}
func (p *Provider) ComposeRoot(boxID identity.BoxID) (string, error) {
	return p.boxPath(boxID, "compose.root.yaml")
}
func (p *Provider) ComposeOverride(boxID identity.BoxID) (string, error) {
	return p.boxPath(boxID, "compose.abox.yaml")
}
func (p *Provider) ComposeResolved(boxID identity.BoxID) (string, error) {
	return p.boxPath(boxID, "compose.resolved.redacted.json")
}
func (p *Provider) DiagnosticsRoot(boxID identity.BoxID) (string, error) {
	return p.boxPath(boxID, "diagnostics")
}
func (p *Provider) GitRoot(boxID identity.BoxID) (string, error) {
	return p.boxPath(boxID, "git")
}
func (p *Provider) WorkspaceRoot(boxID identity.BoxID) (string, error) {
	return p.boxPath(boxID, "workspace")
}

// GitStore returns one Box-private bare Git store path.
func (p *Provider) GitStore(boxID identity.BoxID, repository string) (string, error) {
	if err := validateStateComponent(repository); err != nil {
		return "", fmt.Errorf("git store repository: %w", err)
	}
	return p.boxPath(boxID, "git", repository+".git")
}

// WorkspacePath resolves a validated relative path below one Box workspace.
func (p *Provider) WorkspacePath(boxID identity.BoxID, relative string) (string, error) {
	if err := validateWorkspaceRelative(relative); err != nil {
		return "", fmt.Errorf("workspace path: %w", err)
	}
	root, err := p.WorkspaceRoot(boxID)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(root, relative)
	if !isWithin(root, candidate) {
		return "", errors.New("workspace path escapes workspace root")
	}
	return candidate, nil
}

// RepositoryCache returns a Machine cache path for a validated opaque ID.
func (p *Provider) RepositoryCache(cacheID string) (string, error) {
	if err := validateStateComponent(cacheID); err != nil {
		return "", fmt.Errorf("repository cache ID: %w", err)
	}
	return filepath.Join(p.RepositoriesRoot(), cacheID+".git"), nil
}

// ToolRoot returns the root for one validated pinned tool-set version.
func (p *Provider) ToolRoot(version string) (string, error) {
	if err := validateStateComponent(version); err != nil {
		return "", fmt.Errorf("tool version: %w", err)
	}
	return filepath.Join(p.ToolsRoot(), version), nil
}

func validateRoot(root string) error {
	if root == "" {
		return errors.New("path is empty")
	}
	if strings.IndexByte(root, 0) >= 0 {
		return errors.New("path contains NUL")
	}
	if !filepath.IsAbs(root) {
		return errors.New("path is not absolute")
	}
	if filepath.Clean(root) != root {
		return errors.New("path is not clean")
	}
	return nil
}

func validateStateComponent(component string) error {
	if component == "" {
		return errors.New("component is empty")
	}
	if len(component) > maxComponentBytes-4 {
		return fmt.Errorf("component exceeds %d bytes", maxComponentBytes-4)
	}
	if !isASCIIAlphanumeric(component[0]) || !isASCIIAlphanumeric(component[len(component)-1]) {
		return errors.New("component must start and end with an ASCII letter or digit")
	}
	for index := range len(component) {
		character := component[index]
		if isASCIIAlphanumeric(character) || character == '-' || character == '_' || character == '.' {
			continue
		}
		return fmt.Errorf("component contains invalid byte at offset %d", index)
	}
	return nil
}

func validateWorkspaceRelative(relative string) error {
	if relative == "" || relative == "." {
		return errors.New("relative path is empty")
	}
	if strings.IndexByte(relative, 0) >= 0 {
		return errors.New("relative path contains NUL")
	}
	if strings.Contains(relative, "\\") {
		return errors.New("relative path contains a backslash")
	}
	if filepath.IsAbs(relative) {
		return errors.New("path is absolute")
	}
	if filepath.Clean(relative) != relative {
		return errors.New("path is not clean")
	}
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "" || component == "." || component == ".." {
			return errors.New("path contains an invalid component")
		}
		if len(component) > maxComponentBytes {
			return fmt.Errorf("component exceeds %d bytes", maxComponentBytes)
		}
		if !utf8.ValidString(component) {
			return errors.New("component is not valid UTF-8")
		}
		for _, character := range component {
			if character < ' ' || character == '\u007f' {
				return errors.New("component contains a control character")
			}
		}
	}
	return nil
}

func isWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func isASCIIAlphanumeric(character byte) bool {
	return character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9'
}
