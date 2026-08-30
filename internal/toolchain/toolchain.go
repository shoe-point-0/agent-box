// Package toolchain verifies immutable runtime artifacts before consumers can
// read or execute them.
package toolchain

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/process"
)

type Component string

const (
	Lima       Component = "lima"
	GuestImage Component = "guest-image"
	Docker     Component = "docker"
	Compose    Component = "compose"
	Utility    Component = "utility"
)

type Reason string

const (
	InvalidSpecification Reason = "invalid-specification"
	MissingArtifact      Reason = "missing-artifact"
	IdentityMismatch     Reason = "identity-mismatch"
	UnsafeArtifact       Reason = "unsafe-artifact"
	DigestMismatch       Reason = "digest-mismatch"
)

// PrerequisiteError classifies failures that must stop before runtime mutation.
type PrerequisiteError struct {
	Reason    Reason
	Component Component
	Name      string
	Err       error
}

func (e *PrerequisiteError) Error() string {
	subject := string(e.Component)
	if e.Name != "" {
		subject += "/" + e.Name
	}
	if subject == "" {
		subject = "toolchain"
	}
	if e.Err != nil {
		return fmt.Sprintf("verify %s: %s: %v", subject, e.Reason, e.Err)
	}
	return fmt.Sprintf("verify %s: %s", subject, e.Reason)
}

func (e *PrerequisiteError) Unwrap() error { return e.Err }

func (e *PrerequisiteError) ErrorCode() contract.ErrorCode {
	return contract.CodePrerequisiteFailed
}

// Identity is the immutable identity expected for one runtime artifact.
type Identity struct {
	Component Component
	Name      string
	Version   string
	Digest    contract.Digest
}

func (i Identity) key() string { return string(i.Component) + "\x00" + i.Name }

func (i Identity) validate() error {
	switch i.Component {
	case Lima, GuestImage, Docker, Compose:
		if i.Name != "" {
			return errors.New("standard component name must be empty")
		}
	case Utility:
		if strings.TrimSpace(i.Name) == "" || strings.ContainsAny(i.Name, "\x00/\\") {
			return errors.New("utility name must be non-empty and path-free")
		}
	default:
		return errors.New("component is unsupported")
	}
	if strings.TrimSpace(i.Version) == "" || strings.ContainsAny(i.Version, "\x00\r\n") {
		return errors.New("version must be non-empty, single-line, and NUL-free")
	}
	digest := string(i.Digest)
	if len(digest) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(digest, "sha256:") || strings.Trim(digest[len("sha256:"):], "0123456789abcdef") != "" {
		return errors.New("digest must be canonical lowercase sha256")
	}
	return nil
}

func executable(component Component) bool {
	switch component {
	case Lima, Docker, Compose, Utility:
		return true
	default:
		return false
	}
}

// Specification names the complete runtime artifact set required by preflight
// and provisioning. It is validated before a provider is consulted.
type Specification struct {
	Lima       Identity
	GuestImage Identity
	Docker     Identity
	Compose    Identity
	Utilities  []Identity
}

func (s Specification) identities() ([]Identity, error) {
	standard := []struct {
		want Component
		got  Identity
	}{{Lima, s.Lima}, {GuestImage, s.GuestImage}, {Docker, s.Docker}, {Compose, s.Compose}}
	identities := make([]Identity, 0, len(standard)+len(s.Utilities))
	seen := make(map[string]struct{}, cap(identities))
	for _, item := range standard {
		if item.got.Component != item.want {
			return nil, fmt.Errorf("%s identity has component %q", item.want, item.got.Component)
		}
		identities = append(identities, item.got)
	}
	identities = append(identities, s.Utilities...)
	for _, identity := range identities {
		if err := identity.validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", identity.key(), err)
		}
		if _, exists := seen[identity.key()]; exists {
			return nil, fmt.Errorf("duplicate identity %s", identity.key())
		}
		seen[identity.key()] = struct{}{}
	}
	return identities, nil
}

// Candidate is a provider's local resolution of an expected identity.
type Candidate struct {
	Identity Identity
	Path     string
}

// Provider resolves already available artifacts. Resolve must not make network
// access implicit; a future download provider performs consent/acquisition
// before returning an installed local candidate.
type Provider interface {
	Resolve(context.Context, Identity) (Candidate, error)
}

// Entry binds an expected component selector to a preinstalled candidate.
type Entry struct {
	Component Component
	Name      string
	Candidate Candidate
}

type PreinstalledProvider struct {
	candidates map[string]Candidate
}

func NewPreinstalledProvider(entries []Entry) (*PreinstalledProvider, error) {
	provider := &PreinstalledProvider{candidates: make(map[string]Candidate, len(entries))}
	for _, entry := range entries {
		selector := Identity{Component: entry.Component, Name: entry.Name}.key()
		standard := entry.Component == Lima || entry.Component == GuestImage || entry.Component == Docker || entry.Component == Compose
		if (!standard && entry.Component != Utility) || (standard && entry.Name != "") || (entry.Component == Utility && entry.Name == "") {
			return nil, errors.New("configure preinstalled provider: invalid selector")
		}
		if _, exists := provider.candidates[selector]; exists {
			return nil, fmt.Errorf("configure preinstalled provider: duplicate selector %q", selector)
		}
		provider.candidates[selector] = entry.Candidate
	}
	return provider, nil
}

func (p *PreinstalledProvider) Resolve(ctx context.Context, expected Identity) (Candidate, error) {
	if err := ctx.Err(); err != nil {
		return Candidate{}, err
	}
	candidate, ok := p.candidates[expected.key()]
	if !ok {
		return Candidate{}, os.ErrNotExist
	}
	return candidate, nil
}

// Toolchain contains only artifacts whose provider identity and content have
// passed verification. Artifact fields cannot be constructed outside this
// package with a usable path.
type Toolchain struct {
	Lima       Artifact
	GuestImage Artifact
	Docker     Artifact
	Compose    Artifact
	Utilities  map[string]Artifact
}

type Artifact struct {
	identity Identity
	path     string
}

func (a Artifact) Identity() Identity { return a.identity }

// Open rechecks the artifact through the returned file descriptor, then seeks
// it back to the beginning. Consumers that stage files use this descriptor
// rather than reopening an unverified path.
func (a Artifact) Open(ctx context.Context) (*os.File, error) {
	if a.path == "" {
		return nil, prerequisite(a.identity, InvalidSpecification, errors.New("artifact is not verified"))
	}
	file, err := os.Open(a.path)
	if err != nil {
		return nil, prerequisite(a.identity, MissingArtifact, err)
	}
	if err := verifyOpenFile(ctx, file, a.identity); err != nil {
		_ = file.Close()
		return nil, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		return nil, prerequisite(a.identity, UnsafeArtifact, err)
	}
	return file, nil
}

type Runner interface {
	Run(context.Context, process.Command) (process.Result, error)
}

type Invocation struct {
	Args    []string
	Dir     string
	Env     map[string]string
	Secrets []process.Secret
	Stdout  io.Writer
	Stderr  io.Writer
}

// Run verifies content immediately before package-owned execution. The path is
// intentionally not exposed by Artifact.
func (a Artifact) Run(ctx context.Context, runner Runner, invocation Invocation) (process.Result, error) {
	if !executable(a.identity.Component) {
		return process.Result{}, prerequisite(a.identity, UnsafeArtifact, errors.New("artifact is not executable"))
	}
	if runner == nil {
		return process.Result{}, prerequisite(a.identity, InvalidSpecification, errors.New("runner is nil"))
	}
	file, err := a.Open(ctx)
	if err != nil {
		return process.Result{}, err
	}
	if err := file.Close(); err != nil {
		return process.Result{}, prerequisite(a.identity, UnsafeArtifact, err)
	}
	argv := make([]string, 1, len(invocation.Args)+1)
	argv[0] = a.path
	argv = append(argv, invocation.Args...)
	return runner.Run(ctx, process.Command{
		Argv: argv, Dir: invocation.Dir, Env: invocation.Env, Secrets: invocation.Secrets,
		Stdout: invocation.Stdout, Stderr: invocation.Stderr,
	})
}

func Verify(ctx context.Context, specification Specification, provider Provider) (Toolchain, error) {
	if provider == nil {
		return Toolchain{}, prerequisite(Identity{}, InvalidSpecification, errors.New("provider is nil"))
	}
	identities, err := specification.identities()
	if err != nil {
		return Toolchain{}, prerequisite(Identity{}, InvalidSpecification, err)
	}
	resolved := make(map[string]Artifact, len(identities))
	for _, expected := range identities {
		if err := ctx.Err(); err != nil {
			return Toolchain{}, err
		}
		candidate, resolveErr := provider.Resolve(ctx, expected)
		if resolveErr != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return Toolchain{}, ctxErr
			}
			return Toolchain{}, prerequisite(expected, MissingArtifact, resolveErr)
		}
		if candidate.Identity != expected {
			return Toolchain{}, prerequisite(expected, IdentityMismatch, errors.New("provider returned a different immutable identity"))
		}
		artifact, verifyErr := verifyCandidate(ctx, candidate)
		if verifyErr != nil {
			return Toolchain{}, verifyErr
		}
		resolved[expected.key()] = artifact
	}
	utilities := make(map[string]Artifact, len(specification.Utilities))
	for _, identity := range specification.Utilities {
		utilities[identity.Name] = resolved[identity.key()]
	}
	return Toolchain{
		Lima: resolved[specification.Lima.key()], GuestImage: resolved[specification.GuestImage.key()],
		Docker: resolved[specification.Docker.key()], Compose: resolved[specification.Compose.key()], Utilities: utilities,
	}, nil
}

func verifyCandidate(ctx context.Context, candidate Candidate) (Artifact, error) {
	if !filepath.IsAbs(candidate.Path) || filepath.Clean(candidate.Path) != candidate.Path || strings.IndexByte(candidate.Path, 0) >= 0 {
		return Artifact{}, prerequisite(candidate.Identity, UnsafeArtifact, errors.New("path must be clean, absolute, and NUL-free"))
	}
	canonical, err := filepath.EvalSymlinks(candidate.Path)
	if err != nil {
		return Artifact{}, prerequisite(candidate.Identity, MissingArtifact, err)
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return Artifact{}, prerequisite(candidate.Identity, UnsafeArtifact, err)
	}
	artifact := Artifact{identity: candidate.Identity, path: filepath.Clean(canonical)}
	file, err := artifact.Open(ctx)
	if err != nil {
		return Artifact{}, err
	}
	if err := file.Close(); err != nil {
		return Artifact{}, prerequisite(candidate.Identity, UnsafeArtifact, err)
	}
	return artifact, nil
}

func verifyOpenFile(ctx context.Context, file *os.File, identity Identity) error {
	info, err := file.Stat()
	if err != nil {
		return prerequisite(identity, UnsafeArtifact, err)
	}
	if !info.Mode().IsRegular() {
		return prerequisite(identity, UnsafeArtifact, errors.New("artifact is not a regular file"))
	}
	if executable(identity.Component) && info.Mode().Perm()&0o111 == 0 {
		return prerequisite(identity, UnsafeArtifact, errors.New("artifact has no executable mode bit"))
	}
	hash := sha256.New()
	buffer := make([]byte, 128*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		count, readErr := file.Read(buffer)
		if count > 0 {
			_, _ = hash.Write(buffer[:count])
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return prerequisite(identity, UnsafeArtifact, readErr)
		}
	}
	observed := fmt.Sprintf("sha256:%x", hash.Sum(nil))
	if observed != string(identity.Digest) {
		return prerequisite(identity, DigestMismatch, errors.New("content does not match expected digest"))
	}
	return nil
}

func prerequisite(identity Identity, reason Reason, err error) *PrerequisiteError {
	return &PrerequisiteError{Reason: reason, Component: identity.Component, Name: identity.Name, Err: err}
}
