// Package installation loads an explicit, offline runtime artifact installation.
package installation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/toolchain"
)

const (
	SchemaVersion = "abox.installation/v1alpha1"
	maxBytes      = 1 << 20
)

var requiredUtilities = []string{"cat", "sha256sum", "stat", "systemctl", "test"}

// Error reports invalid installation metadata without exposing artifact paths.
type Error struct {
	err error
}

func (e *Error) Error() string {
	if e.err == nil {
		return "load Agent Box installation"
	}
	return "load Agent Box installation: " + e.err.Error()
}

func (e *Error) Unwrap() error                 { return e.err }
func (e *Error) ErrorCode() contract.ErrorCode { return contract.CodePrerequisiteFailed }

// Runtime is a completely verified local artifact set plus the immutable guest
// locations where the packaged image exposes those exact bytes.
type Runtime struct {
	Toolchain      toolchain.Toolchain
	Specification  toolchain.Specification
	Provider       toolchain.Provider
	GuestImagePath string
	DockerPath     string
	ComposePath    string
	UtilityPaths   map[string]string
	Workdir        string
	DockerSocket   string
	DockerService  string
}

type document struct {
	SchemaVersion string            `json:"schemaVersion"`
	Artifacts     artifactSet       `json:"artifacts"`
	Guest         guestInstallation `json:"guest"`
}

type artifactSet struct {
	Lima       artifact            `json:"lima"`
	GuestImage artifact            `json:"guestImage"`
	Docker     artifact            `json:"docker"`
	Compose    artifact            `json:"compose"`
	Utilities  map[string]artifact `json:"utilities"`
}

type artifact struct {
	Version string          `json:"version"`
	Digest  contract.Digest `json:"digest"`
	Path    string          `json:"path"`
}

type guestInstallation struct {
	DockerPath    string            `json:"dockerPath"`
	ComposePath   string            `json:"composePath"`
	UtilityPaths  map[string]string `json:"utilityPaths"`
	Workdir       string            `json:"workdir"`
	DockerSocket  string            `json:"dockerSocket"`
	DockerService string            `json:"dockerService"`
}

// Load reads bounded strict JSON and verifies every configured artifact before
// returning it. It performs no download, installation, discovery, or mutation.
func Load(ctx context.Context, path string) (Runtime, error) {
	if err := ctx.Err(); err != nil {
		return Runtime{}, err
	}
	data, err := readMetadata(path)
	if err != nil {
		return Runtime{}, invalid(err)
	}
	var configured document
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&configured); err != nil {
		return Runtime{}, invalid(fmt.Errorf("decode metadata: %w", err))
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("trailing JSON value")
		}
		return Runtime{}, invalid(fmt.Errorf("decode metadata: %w", err))
	}
	if configured.SchemaVersion != SchemaVersion {
		return Runtime{}, invalid(errors.New("schema version is unsupported"))
	}

	specification, entries, err := compileArtifacts(configured.Artifacts)
	if err != nil {
		return Runtime{}, invalid(err)
	}
	if err := validateGuest(configured.Guest, configured.Artifacts.Utilities); err != nil {
		return Runtime{}, invalid(err)
	}
	provider, err := toolchain.NewPreinstalledProvider(entries)
	if err != nil {
		return Runtime{}, invalid(err)
	}
	verified, err := toolchain.Verify(ctx, specification, provider)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Runtime{}, ctxErr
		}
		return Runtime{}, fmt.Errorf("load Agent Box installation: verify artifacts: %w", err)
	}
	guestImagePath, err := filepath.EvalSymlinks(configured.Artifacts.GuestImage.Path)
	if err != nil {
		return Runtime{}, invalid(fmt.Errorf("canonicalize guest image: %w", err))
	}
	guestImagePath, err = filepath.Abs(guestImagePath)
	if err != nil {
		return Runtime{}, invalid(fmt.Errorf("canonicalize guest image: %w", err))
	}
	return Runtime{
		Toolchain: verified, Specification: specification, Provider: provider,
		GuestImagePath: filepath.Clean(guestImagePath),
		DockerPath:     configured.Guest.DockerPath, ComposePath: configured.Guest.ComposePath,
		UtilityPaths: cloneMap(configured.Guest.UtilityPaths), Workdir: configured.Guest.Workdir,
		DockerSocket: configured.Guest.DockerSocket, DockerService: configured.Guest.DockerService,
	}, nil
}

func readMetadata(path string) ([]byte, error) {
	if !cleanAbsolute(path) {
		return nil, errors.New("metadata path must be clean and absolute")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect metadata: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > maxBytes {
		return nil, errors.New("metadata must be a bounded regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read metadata: %w", err)
	}
	return data, nil
}

func compileArtifacts(configured artifactSet) (toolchain.Specification, []toolchain.Entry, error) {
	standard := []struct {
		component toolchain.Component
		artifact  artifact
	}{
		{toolchain.Lima, configured.Lima},
		{toolchain.GuestImage, configured.GuestImage},
		{toolchain.Docker, configured.Docker},
		{toolchain.Compose, configured.Compose},
	}
	identities := make(map[toolchain.Component]toolchain.Identity, len(standard))
	entries := make([]toolchain.Entry, 0, len(standard)+len(configured.Utilities))
	for _, current := range standard {
		identity, entry, err := compileArtifact(current.component, "", current.artifact)
		if err != nil {
			return toolchain.Specification{}, nil, err
		}
		identities[current.component] = identity
		entries = append(entries, entry)
	}
	names := make([]string, 0, len(configured.Utilities))
	for name := range configured.Utilities {
		names = append(names, name)
	}
	slices.Sort(names)
	utilities := make([]toolchain.Identity, 0, len(names))
	for _, name := range names {
		identity, entry, err := compileArtifact(toolchain.Utility, name, configured.Utilities[name])
		if err != nil {
			return toolchain.Specification{}, nil, err
		}
		utilities = append(utilities, identity)
		entries = append(entries, entry)
	}
	return toolchain.Specification{
		Lima: identities[toolchain.Lima], GuestImage: identities[toolchain.GuestImage],
		Docker: identities[toolchain.Docker], Compose: identities[toolchain.Compose], Utilities: utilities,
	}, entries, nil
}

func compileArtifact(component toolchain.Component, name string, configured artifact) (toolchain.Identity, toolchain.Entry, error) {
	if !cleanAbsolute(configured.Path) {
		return toolchain.Identity{}, toolchain.Entry{}, errors.New("artifact path must be clean and absolute")
	}
	identity := toolchain.Identity{Component: component, Name: name, Version: configured.Version, Digest: configured.Digest}
	return identity, toolchain.Entry{
		Component: component, Name: name, Candidate: toolchain.Candidate{Identity: identity, Path: configured.Path},
	}, nil
}

func validateGuest(guest guestInstallation, utilities map[string]artifact) error {
	if !cleanAbsolute(guest.DockerPath) || !cleanAbsolute(guest.ComposePath) || !cleanAbsolute(guest.Workdir) ||
		!cleanAbsolute(guest.DockerSocket) || guest.DockerService != "docker.service" {
		return errors.New("guest paths or Docker service are invalid")
	}
	if len(utilities) != len(guest.UtilityPaths) {
		return errors.New("utility artifact and guest path sets differ")
	}
	for name := range utilities {
		if name == "" || strings.ContainsAny(name, "\x00/\\\r\n") || !cleanAbsolute(guest.UtilityPaths[name]) {
			return errors.New("utility artifact or guest path is invalid")
		}
	}
	for _, name := range requiredUtilities {
		if _, ok := utilities[name]; !ok {
			return fmt.Errorf("required utility is absent: %s", name)
		}
	}
	return nil
}

func cleanAbsolute(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path && !strings.ContainsAny(path, "\x00\r\n")
}

func cloneMap(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func invalid(err error) *Error { return &Error{err: err} }
