// Package contextdoc generates the versioned context for one READY Box generation.
package contextdoc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/shoe-point-0/agent-box/internal/atomicfile"
	"github.com/shoe-point-0/agent-box/internal/contract"
)

// DurabilityStatement is the v0.1 accepted-loss contract presented to agents.
const DurabilityStatement = "filesystem synchronization provides operational redundancy; Git provides history and recovery; uncommitted work and unpushed commits are not backed up"

// Observations is one complete, live view captured after readiness checks for a
// specific Plan generation. An empty route or port collection means none are
// available for that generation, not that observation was skipped.
type Observations struct {
	BoxID        string
	Generation   int64
	PlanDigest   contract.Digest
	State        string
	Repositories []contract.ContextRepository
	Services     []contract.ServiceContext
	Routes       []contract.RouteContext
	Ports        []contract.PortContext
}

// Input contains immutable intent, live observations, and non-persisted
// redaction canaries. SensitiveValues may include secrets, credential paths,
// socket paths, or any other value that must not survive encoding.
type Input struct {
	Plan            contract.Plan
	Generation      int64
	WorkspaceRoot   string
	Observations    Observations
	Commands        []contract.CommandContext
	SensitiveValues []string
}

// Write validates and atomically replaces path with a mode-0600 context file.
// No file is changed when generation or redaction validation fails.
func Write(path string, input Input) (contract.Context, error) {
	document, encoded, err := Generate(input)
	if err != nil {
		return contract.Context{}, err
	}
	if err := atomicfile.WriteFile(path, encoded, 0o600); err != nil {
		return contract.Context{}, fmt.Errorf("write context document: %w", err)
	}
	return document, nil
}

// Generate validates a complete READY snapshot and returns its stable JSON
// representation without mutating the filesystem.
func Generate(input Input) (contract.Context, []byte, error) {
	if err := validateInput(input); err != nil {
		return contract.Context{}, nil, err
	}

	repositories := slices.Clone(input.Observations.Repositories)
	services := slices.Clone(input.Observations.Services)
	routes := slices.Clone(input.Observations.Routes)
	ports := slices.Clone(input.Observations.Ports)
	commands := cloneCommands(input.Commands)
	sortByName(repositories, func(item contract.ContextRepository) string { return item.Name })
	sortByName(services, func(item contract.ServiceContext) string { return item.Name })
	sortByName(routes, func(item contract.RouteContext) string { return item.Name })
	sortByName(ports, func(item contract.PortContext) string { return item.Name })
	sortByName(commands, func(item contract.CommandContext) string { return item.Name })

	document := contract.Context{
		SchemaVersion: contract.ContextSchemaVersion,
		Box: contract.BoxContext{
			ID: input.Plan.BoxID, TaskKey: input.Plan.TaskKey, State: input.Observations.State,
			Generation: input.Generation, PlanDigest: input.Plan.PlanDigest,
		},
		Workspace: contract.WorkspaceContext{
			ID: input.Plan.WorkspaceID, ManifestPath: input.Plan.Manifest.Path, Root: input.WorkspaceRoot,
		},
		Repositories: repositories,
		WorkspaceService: contract.WorkspaceService{
			Name: input.Plan.WorkspaceService.Name, WorkingDirectory: input.Plan.WorkspaceService.WorkingDirectory,
		},
		Services: services, Routes: routes, Ports: ports, Commands: commands,
		Policy: contract.PolicyContext{
			GitMetadata: input.Plan.Policy.GitMetadata, DockerSocket: input.Plan.Policy.DockerSocket,
			MachineMounts: input.Plan.Policy.MachineMounts, NetworkEgress: input.Plan.Policy.NetworkEgress,
		},
		Durability: DurabilityStatement,
	}
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return contract.Context{}, nil, fmt.Errorf("encode context document: %w", err)
	}
	encoded = append(encoded, '\n')
	for index, sensitive := range input.SensitiveValues {
		if sensitive == "" {
			return contract.Context{}, nil, fmt.Errorf("generate context document: sensitive value %d is empty", index)
		}
		if bytes.Contains(encoded, []byte(sensitive)) {
			return contract.Context{}, nil, fmt.Errorf("generate context document: sensitive value %d survived projection", index)
		}
	}
	return document, encoded, nil
}

func validateInput(input Input) error {
	if input.Plan.SchemaVersion != contract.PlanSchemaVersion {
		return errors.New("generate context document: unsupported Plan schema version")
	}
	if input.Plan.BoxID == "" || input.Plan.WorkspaceID == "" || input.Plan.TaskKey == "" {
		return errors.New("generate context document: Plan identity is incomplete")
	}
	if !validDigest(input.Plan.PlanDigest) {
		return errors.New("generate context document: Plan digest is invalid")
	}
	if input.Generation < 1 {
		return errors.New("generate context document: generation must be positive")
	}
	observed := input.Observations
	if observed.BoxID != input.Plan.BoxID {
		return errors.New("generate context document: observed Box identity is stale")
	}
	if observed.Generation != input.Generation {
		return errors.New("generate context document: observed generation is stale")
	}
	if observed.PlanDigest != input.Plan.PlanDigest {
		return errors.New("generate context document: observed Plan digest is stale")
	}
	if observed.State != "READY" {
		return errors.New("generate context document: observed Box is not READY")
	}
	if err := validateAbsolutePath(input.Plan.Manifest.Path, "manifest"); err != nil {
		return err
	}
	if err := validateAbsolutePath(input.WorkspaceRoot, "workspace root"); err != nil {
		return err
	}
	if err := validateRepositories(input.Plan.Repositories, observed.Repositories, input.WorkspaceRoot); err != nil {
		return err
	}
	if err := validateServices(observed.Services, input.Plan.WorkspaceService.Name); err != nil {
		return err
	}
	if err := validateRoutes(observed.Routes); err != nil {
		return err
	}
	if err := validatePorts(input.Plan.TCPPorts, observed.Ports); err != nil {
		return err
	}
	if err := validateCommands(input.Commands); err != nil {
		return err
	}
	return validatePolicy(input.Plan.Policy)
}

func validateRepositories(planned []contract.PlanRepository, observed []contract.ContextRepository, workspaceRoot string) error {
	if len(planned) == 0 || len(observed) != len(planned) {
		return errors.New("generate context document: repository observations are incomplete")
	}
	byName := make(map[string]contract.ContextRepository, len(observed))
	for _, repository := range observed {
		if repository.Name == "" {
			return errors.New("generate context document: observed repository name is empty")
		}
		if _, exists := byName[repository.Name]; exists {
			return fmt.Errorf("generate context document: duplicate repository %q", repository.Name)
		}
		if err := validateAbsolutePath(repository.Path, "repository worktree"); err != nil {
			return err
		}
		if !within(workspaceRoot, repository.Path) {
			return fmt.Errorf("generate context document: repository %q is outside the Box workspace", repository.Name)
		}
		byName[repository.Name] = repository
	}
	for _, repository := range planned {
		live, exists := byName[repository.Name]
		if !exists || live.Branch != repository.Branch || live.BaseCommit != repository.BaseCommit || live.Path != repository.WorktreePath {
			return fmt.Errorf("generate context document: repository %q observation is stale", repository.Name)
		}
	}
	return nil
}

func validateServices(services []contract.ServiceContext, workspaceService string) error {
	if workspaceService == "" || len(services) == 0 {
		return errors.New("generate context document: service observations are incomplete")
	}
	seen := make(map[string]struct{}, len(services))
	workspaceReady := false
	for _, service := range services {
		if err := uniqueName(seen, service.Name, "service"); err != nil {
			return err
		}
		if !service.Ready {
			return fmt.Errorf("generate context document: service %q is not ready", service.Name)
		}
		if service.Name == workspaceService {
			workspaceReady = true
		}
	}
	if !workspaceReady {
		return errors.New("generate context document: workspace service observation is missing")
	}
	return nil
}

func validateRoutes(routes []contract.RouteContext) error {
	seen := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		if err := uniqueName(seen, route.Name, "route"); err != nil {
			return err
		}
		parsed, err := url.Parse(route.URL)
		if err != nil || parsed.Scheme != "http" || parsed.Host == "" || parsed.User != nil {
			return fmt.Errorf("generate context document: route %q URL is invalid", route.Name)
		}
		if !route.Ready {
			return fmt.Errorf("generate context document: route %q is not ready", route.Name)
		}
	}
	return nil
}

func validatePorts(planned []contract.TCPPortPlan, ports []contract.PortContext) error {
	if len(planned) != len(ports) {
		return errors.New("generate context document: TCP port observations are incomplete")
	}
	plannedNames := make(map[string]struct{}, len(planned))
	for _, port := range planned {
		plannedNames[port.Name] = struct{}{}
	}
	seen := make(map[string]struct{}, len(ports))
	for _, port := range ports {
		if err := uniqueName(seen, port.Name, "port"); err != nil {
			return err
		}
		if port.Host != "127.0.0.1" || port.Port < 1 || port.Port > 65535 {
			return fmt.Errorf("generate context document: port %q is not a valid loopback endpoint", port.Name)
		}
		if !port.Ready {
			return fmt.Errorf("generate context document: port %q is not ready", port.Name)
		}
		if _, exists := plannedNames[port.Name]; !exists {
			return fmt.Errorf("generate context document: port %q is not planned", port.Name)
		}
	}
	return nil
}

func validateCommands(commands []contract.CommandContext) error {
	seen := make(map[string]struct{}, len(commands))
	for _, command := range commands {
		if err := uniqueName(seen, command.Name, "command"); err != nil {
			return err
		}
		if len(command.Argv) == 0 || command.Argv[0] == "" {
			return fmt.Errorf("generate context document: command %q argv is empty", command.Name)
		}
		for _, argument := range command.Argv {
			if strings.IndexByte(argument, 0) >= 0 {
				return fmt.Errorf("generate context document: command %q contains NUL", command.Name)
			}
		}
	}
	return nil
}

func validatePolicy(policy contract.PolicyPlan) error {
	if policy.GitMetadata != "read-only" && policy.GitMetadata != "read-write" {
		return errors.New("generate context document: Git metadata policy is invalid")
	}
	if policy.DockerSocket != "deny" {
		return errors.New("generate context document: Docker socket policy must deny access")
	}
	if policy.MachineMounts != "declared-only" {
		return errors.New("generate context document: Machine mount policy must be declared-only")
	}
	if policy.NetworkEgress != "allow" && policy.NetworkEgress != "deny" {
		return errors.New("generate context document: network egress policy is invalid")
	}
	return nil
}

func validateAbsolutePath(path, name string) error {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.IndexByte(path, 0) >= 0 {
		return fmt.Errorf("generate context document: %s is not a clean absolute path", name)
	}
	return nil
}

func within(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}

func uniqueName(seen map[string]struct{}, name, kind string) error {
	if name == "" || strings.IndexByte(name, 0) >= 0 {
		return fmt.Errorf("generate context document: %s name is invalid", kind)
	}
	if _, exists := seen[name]; exists {
		return fmt.Errorf("generate context document: duplicate %s %q", kind, name)
	}
	seen[name] = struct{}{}
	return nil
}

func validDigest(digest contract.Digest) bool {
	value := string(digest)
	return len(value) == len("sha256:")+64 && strings.HasPrefix(value, "sha256:") && strings.Trim(value[len("sha256:"):], "0123456789abcdef") == ""
}

func cloneCommands(commands []contract.CommandContext) []contract.CommandContext {
	cloned := slices.Clone(commands)
	for index := range cloned {
		cloned[index].Argv = slices.Clone(cloned[index].Argv)
	}
	return cloned
}

func sortByName[T any](items []T, name func(T) string) {
	slices.SortFunc(items, func(left, right T) int { return strings.Compare(name(left), name(right)) })
}
