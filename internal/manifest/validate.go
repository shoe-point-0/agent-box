package manifest

import (
	"errors"
	"fmt"
	"math/big"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	namePattern        = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)
	environmentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func (l *Loader) validate(workspace *Workspace, manifestDir string) error {
	if workspace.APIVersion != "abox.dev/v1alpha1" {
		return field("apiVersion", "must be abox.dev/v1alpha1")
	}
	if workspace.Kind != "Workspace" {
		return field("kind", "must be Workspace")
	}
	if err := validateName("metadata.name", workspace.Metadata.Name); err != nil {
		return err
	}
	if len(workspace.Spec.Repositories) == 0 {
		return field("spec.repositories", "must contain at least one repository")
	}

	repositories := make(map[string]struct{}, len(workspace.Spec.Repositories))
	workspacePaths := make(map[string]struct{}, len(workspace.Spec.Repositories))
	for index := range workspace.Spec.Repositories {
		repository := &workspace.Spec.Repositories[index]
		path := indexed("spec.repositories", index)
		if err := addUnique(repositories, path+".name", repository.Name); err != nil {
			return err
		}
		if err := validateName(path+".name", repository.Name); err != nil {
			return err
		}
		if (repository.Source.Local == "") == (repository.Source.Remote == "") {
			return field(path+".source", "must set exactly one of local or remote")
		}
		if repository.Source.Local != "" {
			canonical, err := l.canonicalLocal(repository.Source.Local, manifestDir, path+".source.local")
			if err != nil {
				return err
			}
			repository.CanonicalLocal = canonical
		}
		if repository.Checkout.Ref == "" {
			return field(path+".checkout.ref", "is required")
		}
		if err := l.validateTemplate(path+".checkout.branch", repository.Checkout.Branch); err != nil {
			return err
		}
		if err := l.validateRelative(path+".workspacePath", repository.WorkspacePath); err != nil {
			return err
		}
		if _, exists := workspacePaths[repository.WorkspacePath]; exists {
			return field(path+".workspacePath", "must be unique")
		}
		workspacePaths[repository.WorkspacePath] = struct{}{}
	}

	var err error
	workspace.Spec.Host.CPUMillicores, err = parseCPU(string(workspace.Spec.Host.CPUs))
	if err != nil {
		return field("spec.host.cpus", "must be a positive CPU quantity")
	}
	workspace.Spec.Host.MemoryBytes, err = parseBytes(string(workspace.Spec.Host.Memory))
	if err != nil {
		return field("spec.host.memory", "must be a positive byte quantity")
	}
	workspace.Spec.Host.DiskBytes, err = parseBytes(string(workspace.Spec.Host.Disk))
	if err != nil {
		return field("spec.host.disk", "must be a positive byte quantity")
	}

	for index, include := range workspace.Spec.Compose.Includes {
		path := indexed("spec.compose.includes", index)
		if err := requireReference(path+".repository", include.Repository, repositories); err != nil {
			return err
		}
		if err := l.validateRelative(path+".path", include.Path); err != nil {
			return err
		}
	}
	if workspace.Spec.WorkspaceService.Name == "" {
		return field("spec.workspaceService.name", "is required")
	}
	if err := requireReference("spec.workspaceService.repository", workspace.Spec.WorkspaceService.Repository, repositories); err != nil {
		return err
	}
	if err := l.validateRelative("spec.workspaceService.workingDirectory", workspace.Spec.WorkspaceService.WorkingDirectory); err != nil {
		return err
	}
	for index, mount := range workspace.Spec.Mounts {
		path := indexed("spec.mounts", index)
		if err := requireReference(path+".repository", mount.Repository, repositories); err != nil {
			return err
		}
		if len(mount.Services) == 0 {
			return field(path+".services", "must contain at least one service")
		}
		if mount.Mode != "read-only" && mount.Mode != "read-write" {
			return field(path+".mode", "must be read-only or read-write")
		}
	}
	if err := validateCaches(workspace.Spec.Caches); err != nil {
		return err
	}
	if err := l.validateSetup(workspace.Spec.Setup); err != nil {
		return err
	}
	if err := validateRoutes(workspace.Spec.Routes); err != nil {
		return err
	}
	if err := validateTCPPorts(workspace.Spec.TCPPorts); err != nil {
		return err
	}
	if err := validateSecrets(workspace.Spec.Secrets); err != nil {
		return err
	}
	return validatePolicy(workspace.Spec.Policy)
}

func validateCaches(caches []Cache) error {
	seen := make(map[string]struct{}, len(caches))
	for index, cache := range caches {
		path := indexed("spec.caches", index)
		if err := addUnique(seen, path+".name", cache.Name); err != nil {
			return err
		}
		if err := validateName(path+".name", cache.Name); err != nil {
			return err
		}
		if cache.Service == "" {
			return field(path+".service", "is required")
		}
		if !filepath.IsAbs(cache.Target) || filepath.Clean(cache.Target) != cache.Target {
			return field(path+".target", "must be a clean absolute path")
		}
	}
	return nil
}

func (l *Loader) validateSetup(jobs []SetupJob) error {
	seen := make(map[string]struct{}, len(jobs))
	for index, job := range jobs {
		path := indexed("spec.setup", index)
		if err := addUnique(seen, path+".name", job.Name); err != nil {
			return err
		}
		if err := validateName(path+".name", job.Name); err != nil {
			return err
		}
		if job.Service == "" {
			return field(path+".service", "is required")
		}
		if len(job.Command) == 0 {
			return field(path+".command", "must contain argv entries")
		}
		for argumentIndex, argument := range job.Command {
			if argument == "" || strings.IndexByte(argument, 0) >= 0 {
				return field(path+".command["+strconv.Itoa(argumentIndex)+"]", "must be a non-empty NUL-free argument")
			}
		}
		for changedIndex, changed := range job.RerunWhenChanged {
			if err := l.validateRelative(path+".rerunWhenChanged["+strconv.Itoa(changedIndex)+"]", changed); err != nil {
				return err
			}
		}
		if job.Rerun != "" && job.Rerun != "everyStart" {
			return field(path+".rerun", "must be everyStart when set")
		}
	}
	return nil
}

func validateRoutes(routes []Route) error {
	seen := make(map[string]struct{}, len(routes))
	for index := range routes {
		route := &routes[index]
		path := indexed("spec.routes", index)
		if err := addUnique(seen, path+".name", route.Name); err != nil {
			return err
		}
		if err := validateName(path+".name", route.Name); err != nil {
			return err
		}
		if route.Service == "" {
			return field(path+".service", "is required")
		}
		if route.TargetPort < 1 || route.TargetPort > 65535 {
			return field(path+".targetPort", "must be between 1 and 65535")
		}
		if route.Readiness.HTTP == nil {
			return field(path+".readiness.http", "is required")
		}
		if !strings.HasPrefix(route.Readiness.HTTP.Path, "/") {
			return field(path+".readiness.http.path", "must begin with /")
		}
		if route.Readiness.HTTP.ExpectedStatus < 100 || route.Readiness.HTTP.ExpectedStatus > 599 {
			return field(path+".readiness.http.expectedStatus", "must be between 100 and 599")
		}
		duration, err := time.ParseDuration(string(route.Readiness.Timeout))
		if err != nil || duration <= 0 {
			return field(path+".readiness.timeout", "must be a positive duration")
		}
		route.Readiness.TimeoutNanos = int64(duration)
	}
	return nil
}

func validateTCPPorts(ports []TCPPort) error {
	seen := make(map[string]struct{}, len(ports))
	for index, port := range ports {
		path := indexed("spec.tcpPorts", index)
		if err := addUnique(seen, path+".name", port.Name); err != nil {
			return err
		}
		if err := validateName(path+".name", port.Name); err != nil {
			return err
		}
		if port.Service == "" {
			return field(path+".service", "is required")
		}
		if port.TargetPort < 1 || port.TargetPort > 65535 {
			return field(path+".targetPort", "must be between 1 and 65535")
		}
		if port.Allocate != "dynamic" {
			return field(path+".allocate", "must be dynamic")
		}
	}
	return nil
}

func validateSecrets(secrets []Secret) error {
	seen := make(map[string]struct{}, len(secrets))
	for index, secret := range secrets {
		path := indexed("spec.secrets", index)
		if err := addUnique(seen, path+".name", secret.Name); err != nil {
			return err
		}
		if !environmentPattern.MatchString(secret.Name) {
			return field(path+".name", "must be an environment-style name")
		}
		if !environmentPattern.MatchString(secret.FromHostEnvironment) {
			return field(path+".fromHostEnvironment", "must reference a host environment variable")
		}
		if len(secret.ExposeTo) == 0 {
			return field(path+".exposeTo", "must contain at least one target")
		}
		for targetIndex, target := range secret.ExposeTo {
			targetPath := path + ".exposeTo[" + strconv.Itoa(targetIndex) + "]"
			if target.Service == "" {
				return field(targetPath+".service", "is required")
			}
			if target.As != "environment" {
				return field(targetPath+".as", "must be environment")
			}
		}
	}
	return nil
}

func validatePolicy(policy Policy) error {
	checks := []struct {
		path  string
		value string
		valid map[string]struct{}
	}{
		{"spec.policy.gitMetadata", policy.GitMetadata, set("read-only", "read-write")},
		{"spec.policy.dockerSocket", policy.DockerSocket, set("deny")},
		{"spec.policy.machineMounts", policy.MachineMounts, set("declared-only")},
		{"spec.policy.networkEgress", policy.NetworkEgress, set("allow", "deny")},
	}
	for _, check := range checks {
		if _, ok := check.valid[check.value]; !ok {
			return field(check.path, "has an unsupported policy value")
		}
	}
	return nil
}

func (l *Loader) validateTemplate(path, value string) error {
	if value == "" {
		return field(path, "is required")
	}
	remaining := strings.ReplaceAll(value, "{{ box.task }}", "")
	if strings.Contains(remaining, "{{") || strings.Contains(remaining, "}}") {
		return field(path, "contains an unsupported template")
	}
	maximum := len(value) + strings.Count(value, "{{ box.task }}")*(128-len("{{ box.task }}"))
	if maximum > l.limits.MaxTemplateOutput {
		return field(path, "template output exceeds configured limit")
	}
	return nil
}

func (l *Loader) canonicalLocal(source, manifestDir, path string) (string, error) {
	if len(source) > l.limits.MaxPathBytes {
		return "", field(path, "path exceeds configured byte limit")
	}
	if !utf8.ValidString(source) || strings.IndexByte(source, 0) >= 0 {
		return "", field(path, "path is not valid UTF-8")
	}
	candidate := source
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(manifestDir, candidate)
	}
	candidate, err := filepath.Abs(candidate)
	if err != nil {
		return "", field(path, "cannot make path absolute")
	}
	canonical, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", field(path, "cannot canonicalize path")
	}
	canonical = filepath.Clean(canonical)
	for _, root := range l.roots {
		if isWithin(root, canonical) {
			return canonical, nil
		}
	}
	return "", field(path, "canonical path escapes allowed source roots")
}

func (l *Loader) validateRelative(path, value string) error {
	if value == "" {
		return field(path, "is required")
	}
	if len(value) > l.limits.MaxPathBytes {
		return field(path, "path exceeds configured byte limit")
	}
	if !utf8.ValidString(value) || strings.IndexByte(value, 0) >= 0 {
		return field(path, "path is not valid UTF-8")
	}
	if filepath.IsAbs(value) || filepath.Clean(value) != value || hasParentComponent(value) || strings.Contains(value, `\`) {
		return field(path, "must be a clean relative path without traversal")
	}
	return nil
}

func validateName(path, value string) error {
	if len(value) > 63 || !namePattern.MatchString(value) {
		return field(path, "must be a lowercase DNS-style name of at most 63 bytes")
	}
	return nil
}

func addUnique(seen map[string]struct{}, path, value string) error {
	if _, exists := seen[value]; exists {
		return field(path, "duplicates an earlier name")
	}
	seen[value] = struct{}{}
	return nil
}

func requireReference(path, value string, known map[string]struct{}) error {
	if _, ok := known[value]; !ok {
		return field(path, "references an unknown repository")
	}
	return nil
}

func parseCPU(value string) (int64, error) {
	if strings.HasSuffix(value, "m") {
		millicores, err := strconv.ParseInt(strings.TrimSuffix(value, "m"), 10, 64)
		if err != nil || millicores <= 0 {
			return 0, errors.New("invalid CPU quantity")
		}
		return millicores, nil
	}
	rational, ok := new(big.Rat).SetString(value)
	if !ok || rational.Sign() <= 0 {
		return 0, errors.New("invalid CPU quantity")
	}
	rational.Mul(rational, big.NewRat(1000, 1))
	if !rational.IsInt() || !rational.Num().IsInt64() {
		return 0, errors.New("CPU quantity is not an exact millicore value")
	}
	return rational.Num().Int64(), nil
}

func parseBytes(value string) (int64, error) {
	units := []struct {
		suffix string
		factor int64
	}{
		{"TiB", 1 << 40}, {"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10},
		{"TB", 1_000_000_000_000}, {"GB", 1_000_000_000}, {"MB", 1_000_000}, {"KB", 1_000}, {"B", 1},
	}
	for _, unit := range units {
		if !strings.HasSuffix(value, unit.suffix) {
			continue
		}
		number := strings.TrimSuffix(value, unit.suffix)
		quantity, err := strconv.ParseInt(number, 10, 64)
		if err != nil || quantity <= 0 || quantity > (1<<63-1)/unit.factor {
			return 0, errors.New("invalid byte quantity")
		}
		return quantity * unit.factor, nil
	}
	return 0, errors.New("byte quantity requires an explicit unit")
}

func hasParentComponent(path string) bool {
	for _, component := range strings.FieldsFunc(path, func(character rune) bool { return character == '/' || character == '\\' }) {
		if component == ".." {
			return true
		}
	}
	return false
}

func isWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func indexed(path string, index int) string { return path + "[" + strconv.Itoa(index) + "]" }
func field(path, problem string) error      { return &FieldError{Path: path, Problem: problem} }

func set(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

// FeatureNotSupportedError marks syntax accepted by v1alpha1 but not executable
// by the current Milestone 1 planner.
type FeatureNotSupportedError struct {
	Path    string
	Feature string
}

func (e *FeatureNotSupportedError) Error() string {
	return fmt.Sprintf("%s: feature %s is not supported in Milestone 1", e.Path, e.Feature)
}

// CheckMilestone1Executable rejects recognized syntax whose owner lands later.
func CheckMilestone1Executable(workspace *Workspace) error {
	for index, repository := range workspace.Spec.Repositories {
		if repository.Source.Remote != "" {
			return &FeatureNotSupportedError{Path: indexed("spec.repositories", index) + ".source.remote", Feature: "remote repositories"}
		}
	}
	checks := []struct {
		present bool
		path    string
		feature string
	}{
		{len(workspace.Spec.Caches) > 0, "spec.caches", "caches"},
		{len(workspace.Spec.Setup) > 0, "spec.setup", "setup jobs"},
		{len(workspace.Spec.Routes) > 0, "spec.routes", "routes"},
		{len(workspace.Spec.Secrets) > 0, "spec.secrets", "secrets"},
	}
	for _, check := range checks {
		if check.present {
			return &FeatureNotSupportedError{Path: check.path, Feature: check.feature}
		}
	}
	return nil
}
