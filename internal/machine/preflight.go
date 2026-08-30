// Package machine validates read-only Machine prerequisites before callers may
// mutate Box resources.
package machine

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/toolchain"
)

// Reason is a stable machine-readable prerequisite classification.
type Reason string

const (
	InvalidSpecification Reason = "invalid-specification"
	ObservationFailed    Reason = "observation-failed"
	UnsupportedPlatform  Reason = "unsupported-platform"
	PathUnavailable      Reason = "path-unavailable"
	InsufficientSpace    Reason = "insufficient-space"
	RouterUnavailable    Reason = "router-unavailable"
	ArtifactInvalid      Reason = "artifact-invalid"
)

type Status string

const (
	StatusPass Status = "pass"
	StatusFail Status = "fail"
)

// Fact is a value-only diagnostic suitable for structured doctor output.
type Fact struct {
	Name        string `json:"name"`
	Status      Status `json:"status"`
	Reason      Reason `json:"reason,omitempty"`
	Observed    string `json:"observed"`
	Required    string `json:"required"`
	Remediation string `json:"remediation,omitempty"`
}

// PrerequisiteError identifies one failed fact without requiring callers to
// parse its human-readable text.
type PrerequisiteError struct {
	Fact Fact
	Code contract.ErrorCode
	Err  error
}

func (e *PrerequisiteError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("validate Machine %s: %s: %v", e.Fact.Name, e.Fact.Reason, e.Err)
	}
	return fmt.Sprintf("validate Machine %s: %s", e.Fact.Name, e.Fact.Reason)
}

func (e *PrerequisiteError) Unwrap() error                 { return e.Err }
func (e *PrerequisiteError) ErrorCode() contract.ErrorCode { return e.Code }

// Platform is one complete Machine execution tuple. Accelerator names are
// adapter contracts such as "qemu" and "vz", not inferred support claims.
type Platform struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	Accelerator  string `json:"accelerator"`
}

func (p Platform) String() string {
	return p.OS + "/" + p.Architecture + "/" + p.Accelerator
}

type PathRequirement struct {
	Name         string
	Path         string
	MinFreeBytes uint64
}

type Specification struct {
	Supported     []Platform
	Paths         []PathRequirement
	RouterAddress string
	Toolchain     toolchain.Specification
	Provider      toolchain.Provider
}

type PathObservation struct {
	Name           string
	Path           string
	Usable         bool
	AvailableBytes uint64
	Detail         string
}

type Observation struct {
	Platform        Platform
	Paths           []PathObservation
	RouterAddress   string
	RouterAvailable bool
	RouterDetail    string
}

// Observer collects Machine facts without provisioning or retaining resources.
type Observer interface {
	Observe(context.Context) (Observation, error)
}

type Version struct {
	Component toolchain.Component `json:"component"`
	Name      string              `json:"name,omitempty"`
	Version   string              `json:"version"`
}

type Result struct {
	Platform  Platform            `json:"platform"`
	Facts     []Fact              `json:"facts"`
	Versions  []Version           `json:"versions"`
	Artifacts toolchain.Toolchain `json:"-"`
}

// Check performs only observation, local artifact resolution, and hashing. It
// cannot create Git, database, VM, generated-file, or persistent port state.
func Check(ctx context.Context, specification Specification, observer Observer) (Result, error) {
	if err := validateSpecification(specification, observer); err != nil {
		return Result{}, failure("specification", InvalidSpecification, "valid preflight configuration", "invalid", "fix the Agent Box installation or configuration", contract.CodePrerequisiteFailed, err)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	verified, err := toolchain.Verify(ctx, specification.Toolchain, specification.Provider)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Result{}, ctxErr
		}
		return Result{}, failure("artifacts", ArtifactInvalid, "complete pinned toolchain with matching SHA-256 digests", artifactObservation(err), "run abox doctor tools and install the pinned artifacts", contract.CodePrerequisiteFailed, err)
	}

	observation, err := observer.Observe(ctx)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Result{}, ctxErr
		}
		return Result{}, failure("observation", ObservationFailed, "readable Machine facts", "unavailable", "run abox doctor machine and correct the reported observation failure", contract.CodePrerequisiteFailed, err)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	facts := []Fact{{Name: "artifacts", Status: StatusPass, Observed: "verified", Required: "pinned identities and SHA-256 digests"}}
	platformRequired := platformSet(specification.Supported)
	platformFact := Fact{Name: "platform", Status: StatusPass, Observed: observation.Platform.String(), Required: platformRequired}
	if !slices.Contains(specification.Supported, observation.Platform) {
		platformFact.Status, platformFact.Reason = StatusFail, UnsupportedPlatform
		platformFact.Remediation = "use a supported OS, architecture, and accelerator tuple"
		return Result{}, &PrerequisiteError{Fact: platformFact, Code: contract.CodeMachineUnsupported}
	}
	facts = append(facts, platformFact)

	paths := make(map[string]PathObservation, len(observation.Paths))
	for _, observed := range observation.Paths {
		if observed.Name == "" {
			return Result{}, malformedObservation("path observation has an empty name")
		}
		if _, exists := paths[observed.Name]; exists {
			return Result{}, malformedObservation("duplicate path observation " + observed.Name)
		}
		paths[observed.Name] = observed
	}
	if len(paths) != len(specification.Paths) {
		return Result{}, malformedObservation("path observation set differs from specification")
	}
	for _, required := range specification.Paths {
		observed, ok := paths[required.Name]
		if !ok || observed.Path != required.Path {
			return Result{}, malformedObservation("path identity differs for " + required.Name)
		}
		fact := Fact{
			Name: required.Name, Status: StatusPass,
			Observed: fmt.Sprintf("path=%s freeBytes=%d", observed.Path, observed.AvailableBytes),
			Required: fmt.Sprintf("path=%s usable=true freeBytes>=%d", required.Path, required.MinFreeBytes),
		}
		if !observed.Usable {
			fact.Status, fact.Reason, fact.Remediation = StatusFail, PathUnavailable, "make the path and its existing parent accessible to the current user"
			if observed.Detail != "" {
				fact.Observed += " detail=" + observed.Detail
			}
			return Result{}, &PrerequisiteError{Fact: fact, Code: contract.CodePrerequisiteFailed}
		}
		if observed.AvailableBytes < required.MinFreeBytes {
			fact.Status, fact.Reason, fact.Remediation = StatusFail, InsufficientSpace, "free space on the containing filesystem or choose another Agent Box root"
			return Result{}, &PrerequisiteError{Fact: fact, Code: contract.CodePrerequisiteFailed}
		}
		facts = append(facts, fact)
	}

	routerFact := Fact{Name: "router", Status: StatusPass, Observed: observation.RouterAddress, Required: specification.RouterAddress + " available"}
	if observation.RouterAddress != specification.RouterAddress {
		return Result{}, malformedObservation("router address differs from specification")
	}
	if !observation.RouterAvailable {
		routerFact.Status, routerFact.Reason = StatusFail, RouterUnavailable
		routerFact.Remediation = "stop the process using the configured loopback address or configure another unprivileged port"
		if observation.RouterDetail != "" {
			routerFact.Observed += " detail=" + observation.RouterDetail
		}
		return Result{}, &PrerequisiteError{Fact: routerFact, Code: contract.CodePrerequisiteFailed}
	}
	facts = append(facts, routerFact)

	return Result{Platform: observation.Platform, Facts: facts, Versions: versions(specification.Toolchain), Artifacts: verified}, nil
}

func validateSpecification(specification Specification, observer Observer) error {
	if observer == nil {
		return errors.New("observer is nil")
	}
	if specification.Provider == nil {
		return errors.New("toolchain provider is nil")
	}
	if len(specification.Supported) == 0 {
		return errors.New("supported platform set is empty")
	}
	seenPlatforms := make(map[Platform]struct{}, len(specification.Supported))
	for _, platform := range specification.Supported {
		if !validToken(platform.OS) || !validToken(platform.Architecture) || !validToken(platform.Accelerator) {
			return errors.New("supported platform contains an invalid token")
		}
		if _, exists := seenPlatforms[platform]; exists {
			return errors.New("supported platform is duplicated")
		}
		seenPlatforms[platform] = struct{}{}
	}
	if len(specification.Paths) == 0 {
		return errors.New("path requirement set is empty")
	}
	seenPaths := make(map[string]struct{}, len(specification.Paths))
	for _, path := range specification.Paths {
		if !validToken(path.Name) || !filepath.IsAbs(path.Path) || filepath.Clean(path.Path) != path.Path || path.MinFreeBytes == 0 {
			return errors.New("path requirement is invalid")
		}
		if _, exists := seenPaths[path.Name]; exists {
			return errors.New("path requirement name is duplicated")
		}
		seenPaths[path.Name] = struct{}{}
	}
	host, port, err := net.SplitHostPort(specification.RouterAddress)
	portNumber, portErr := strconv.Atoi(port)
	ip := net.ParseIP(host)
	if err != nil || portErr != nil || portNumber < 1 || portNumber > 65535 || ip == nil || !ip.IsLoopback() {
		return errors.New("router address must be a fixed loopback TCP address")
	}
	return nil
}

func validToken(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\x00/\\\r\n\t")
}

func platformSet(platforms []Platform) string {
	values := make([]string, len(platforms))
	for index, platform := range platforms {
		values[index] = platform.String()
	}
	slices.Sort(values)
	return strings.Join(values, ",")
}

func versions(specification toolchain.Specification) []Version {
	identities := []toolchain.Identity{specification.Lima, specification.GuestImage, specification.Docker, specification.Compose}
	identities = append(identities, specification.Utilities...)
	result := make([]Version, len(identities))
	for index, identity := range identities {
		result[index] = Version{Component: identity.Component, Name: identity.Name, Version: identity.Version}
	}
	slices.SortFunc(result, func(left, right Version) int {
		return strings.Compare(string(left.Component)+"\x00"+left.Name, string(right.Component)+"\x00"+right.Name)
	})
	return result
}

func artifactObservation(err error) string {
	var prerequisite *toolchain.PrerequisiteError
	if errors.As(err, &prerequisite) {
		subject := string(prerequisite.Component)
		if prerequisite.Name != "" {
			subject += "/" + prerequisite.Name
		}
		return subject + " reason=" + string(prerequisite.Reason)
	}
	return "verification failed"
}

func malformedObservation(detail string) *PrerequisiteError {
	return failure("observation", ObservationFailed, "complete facts matching the specification", "malformed", "run abox doctor machine and report the collector failure", contract.CodePrerequisiteFailed, errors.New(detail))
}

func failure(name string, reason Reason, required, observed, remediation string, code contract.ErrorCode, err error) *PrerequisiteError {
	return &PrerequisiteError{Fact: Fact{Name: name, Status: StatusFail, Reason: reason, Observed: observed, Required: required, Remediation: remediation}, Code: code, Err: err}
}
