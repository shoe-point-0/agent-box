// Package compose compiles immutable Plans into generated Compose inputs.
package compose

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/shoe-point-0/agent-box/internal/atomicfile"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/plan"
)

const (
	RootFileName          = "compose.root.yaml"
	OverrideFileName      = "compose.abox.yaml"
	ResolvedModelFileName = "compose.resolved.redacted.json"
	maxNormalizedBytes    = 16 << 20
)

// Command is a complete normalization request. Env is the entire child
// environment, and Secrets must be redacted from all returned output.
type Command struct {
	Argv    []string
	Dir     string
	Env     map[string]string
	Secrets []Secret
}

type Secret struct {
	Value string
}

type CommandResult struct {
	Stdout          []byte
	Stderr          []byte
	StdoutTruncated bool
	StderrTruncated bool
}

// Runner is the cancellation-aware, bounded guest-command boundary used for
// Compose normalization. The runtime-host adapter owns process execution.
type Runner interface {
	Run(context.Context, Command) (CommandResult, error)
}

// Input contains one immutable Plan and apply-attempt facts that do not belong
// in the Plan. Environment is the complete Compose child environment.
type Input struct {
	Plan           contract.Plan
	Directory      string
	Generation     int64
	ComposeCommand []string
	Environment    map[string]string
	Secrets        []Secret
	Runner         Runner
}

// Result identifies the generated inputs and canonical retained model.
type Result struct {
	ProjectName         string
	RootPath            string
	OverridePath        string
	ResolvedModelPath   string
	ResolvedModel       []byte
	ResolvedModelDigest contract.Digest
}

// Compile writes deterministic generated inputs, normalizes them through the
// exact Plan-pinned Compose CLI, and retains only canonical redacted JSON.
// It never applies the Compose project.
func Compile(ctx context.Context, input Input) (Result, error) {
	if err := validateInput(input); err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("compile Compose: %w", err)
	}

	rootPath := filepath.Join(input.Directory, RootFileName)
	overridePath := filepath.Join(input.Directory, OverrideFileName)
	resolvedPath := filepath.Join(input.Directory, ResolvedModelFileName)
	version, err := run(ctx, input, appendCommand(input.ComposeCommand, "version", "--short"))
	if err != nil {
		return Result{}, fmt.Errorf("verify Compose version: %w", err)
	}
	if got := strings.TrimSpace(string(version)); got != input.Plan.Tools.Compose {
		return Result{}, fmt.Errorf("verify Compose version: got %q, want pinned %q", got, input.Plan.Tools.Compose)
	}
	stagingDirectory, err := os.MkdirTemp(input.Directory, ".abox-compose-compile-")
	if err != nil {
		return Result{}, fmt.Errorf("stage generated Compose inputs: %w", err)
	}
	defer func() { _ = os.RemoveAll(stagingDirectory) }()
	stagedRootPath := filepath.Join(stagingDirectory, RootFileName)
	stagedOverridePath := filepath.Join(stagingDirectory, OverrideFileName)
	root, err := renderRoot(input.Plan)
	if err != nil {
		return Result{}, err
	}
	if err := atomicfile.WriteFile(stagedRootPath, root, 0o600); err != nil {
		return Result{}, fmt.Errorf("stage Compose root: %w", err)
	}

	baseline, err := normalize(ctx, input, stagedRootPath)
	if err != nil {
		return Result{}, fmt.Errorf("normalize Compose root: %w", err)
	}
	if err := validatePolicy(baseline, input.Plan, false); err != nil {
		return Result{}, err
	}
	services, err := serviceNames(baseline)
	if err != nil {
		return Result{}, fmt.Errorf("inspect Compose root: %w", err)
	}
	volumes, err := resourceNames(baseline, "volumes")
	if err != nil {
		return Result{}, fmt.Errorf("inspect Compose root volumes: %w", err)
	}
	override, err := renderOverride(input.Plan, input.Generation, services, volumes)
	if err != nil {
		return Result{}, err
	}
	if err := atomicfile.WriteFile(stagedOverridePath, override, 0o600); err != nil {
		return Result{}, fmt.Errorf("stage Compose override: %w", err)
	}

	resolved, err := normalize(ctx, input, stagedRootPath, stagedOverridePath)
	if err != nil {
		return Result{}, fmt.Errorf("normalize adapted Compose model: %w", err)
	}
	if err := validatePolicy(resolved, input.Plan, true); err != nil {
		return Result{}, err
	}
	canonical, err := validateAndCanonicalize(resolved, input.Plan, input.Generation, services, input.Secrets)
	if err != nil {
		return Result{}, err
	}
	if err := writeGeneratedFile(rootPath, root); err != nil {
		return Result{}, fmt.Errorf("compile Compose root: %w", err)
	}
	if err := writeGeneratedFile(overridePath, override); err != nil {
		return Result{}, fmt.Errorf("compile Compose override: %w", err)
	}
	if err := writeGeneratedFile(resolvedPath, append(canonical, '\n')); err != nil {
		return Result{}, fmt.Errorf("retain normalized Compose model: %w", err)
	}
	digest := sha256.Sum256(canonical)
	return Result{
		ProjectName: input.Plan.Compose.ProjectName, RootPath: rootPath, OverridePath: overridePath,
		ResolvedModelPath: resolvedPath, ResolvedModel: canonical,
		ResolvedModelDigest: contract.Digest("sha256:" + hex.EncodeToString(digest[:])),
	}, nil
}

// writeGeneratedFile preserves the inode of an exact retained artifact. Warm
// reconciliation still normalizes through Compose, but unchanged evidence must
// not look like a filesystem mutation or invalidate a previously recorded path
// identity.
func writeGeneratedFile(path string, data []byte) error {
	info, err := os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 {
			return errors.New("retained generated file has unsafe type or mode")
		}
		current, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read retained generated file: %w", readErr)
		}
		if bytes.Equal(current, data) {
			return nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect retained generated file: %w", err)
	}
	return atomicfile.WriteFile(path, data, 0o600)
}

func validateInput(input Input) error {
	if _, err := plan.CanonicalJSON(input.Plan); err != nil {
		return fmt.Errorf("compile Compose: invalid Plan: %w", err)
	}
	if len(input.Plan.Repositories) != 1 || len(input.Plan.Compose.Includes) != 1 {
		return errors.New("compile Compose: Milestone 1 requires exactly one repository and one include")
	}
	if input.Plan.Compose.ProjectName == "" || input.Plan.Compose.ProjectName != input.Plan.RuntimeHost.InstanceName {
		return errors.New("compile Compose: project identity is missing or inconsistent")
	}
	if input.Generation < 1 {
		return errors.New("compile Compose: generation must be positive")
	}
	if input.Runner == nil {
		return errors.New("compile Compose: runner is required")
	}
	if len(input.ComposeCommand) == 0 {
		return errors.New("compile Compose: Compose command is required")
	}
	for _, argument := range input.ComposeCommand {
		if argument == "" || strings.IndexByte(argument, 0) >= 0 {
			return errors.New("compile Compose: Compose command contains an invalid argument")
		}
	}
	if input.Directory == "" || !filepath.IsAbs(input.Directory) || filepath.Clean(input.Directory) != input.Directory {
		return errors.New("compile Compose: output directory must be a clean absolute path")
	}
	info, err := os.Lstat(input.Directory)
	if err != nil {
		return fmt.Errorf("compile Compose: inspect output directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("compile Compose: output directory must be a real directory")
	}
	if input.Plan.Policy.NetworkEgress != "allow" && input.Plan.Policy.NetworkEgress != "deny" {
		return errors.New("compile Compose: Plan network policy is invalid")
	}
	return nil
}

func renderRoot(compiled contract.Plan) ([]byte, error) {
	repository := compiled.Repositories[0]
	include := compiled.Compose.Includes[0]
	if !contained(repository.WorktreePath, include.Path) {
		return nil, errors.New("compile Compose root: include escapes owning worktree")
	}
	var output strings.Builder
	output.WriteString("include:\n  - path: ")
	output.WriteString(yamlString(include.Path))
	output.WriteString("\n    project_directory: ")
	output.WriteString(yamlString(repository.WorktreePath))
	output.WriteByte('\n')
	return []byte(output.String()), nil
}

func renderOverride(compiled contract.Plan, generation int64, services, volumes []string) ([]byte, error) {
	known := make(map[string]struct{}, len(services))
	for _, service := range services {
		known[service] = struct{}{}
	}
	if _, ok := known[compiled.WorkspaceService.Name]; !ok {
		return nil, fmt.Errorf("compile Compose override: workspace service %q is absent", compiled.WorkspaceService.Name)
	}
	mounts := make(map[string][]contract.MountPlan, len(services))
	for _, mount := range compiled.Mounts {
		if mount.Path == "" || !filepath.IsAbs(mount.Path) || filepath.Clean(mount.Path) != mount.Path {
			return nil, errors.New("compile Compose override: mount path is not a clean absolute path")
		}
		for _, service := range mount.Services {
			if _, ok := known[service]; !ok {
				return nil, fmt.Errorf("compile Compose override: mount references absent service %q", service)
			}
			mounts[service] = append(mounts[service], mount)
		}
	}
	for service := range mounts {
		slices.SortFunc(mounts[service], func(left, right contract.MountPlan) int {
			if compared := strings.Compare(left.Path, right.Path); compared != 0 {
				return compared
			}
			return boolCompare(left.Writable, right.Writable)
		})
	}

	labels := ownershipLabels(compiled, generation)
	var output strings.Builder
	output.WriteString("services:\n")
	for _, service := range services {
		output.WriteString("  ")
		output.WriteString(yamlString(service))
		output.WriteString(":\n    ports: !reset []\n    labels:\n")
		writeLabels(&output, "      ", labels)
		output.WriteString("    networks:\n      default: {}\n")
		if len(mounts[service]) > 0 {
			output.WriteString("    volumes:\n")
			for _, mount := range mounts[service] {
				output.WriteString("      - type: bind\n        source: ")
				output.WriteString(yamlString(mount.Path))
				output.WriteString("\n        target: ")
				output.WriteString(yamlString(mount.Path))
				output.WriteString("\n        read_only: ")
				output.WriteString(strconv.FormatBool(!mount.Writable))
				output.WriteByte('\n')
			}
		}
	}
	output.WriteString("networks:\n  default:\n    internal: ")
	output.WriteString(strconv.FormatBool(compiled.Policy.NetworkEgress == "deny"))
	output.WriteString("\n    labels:\n")
	writeLabels(&output, "      ", labels)
	if len(volumes) > 0 {
		output.WriteString("volumes:\n")
		for _, volume := range volumes {
			output.WriteString("  ")
			output.WriteString(yamlString(volume))
			output.WriteString(":\n    labels:\n")
			writeLabels(&output, "      ", labels)
		}
	}
	return []byte(output.String()), nil
}

func normalize(ctx context.Context, input Input, files ...string) ([]byte, error) {
	argv := append([]string{}, input.ComposeCommand...)
	argv = append(argv, "--project-name", input.Plan.Compose.ProjectName, "--project-directory", input.Directory)
	for _, profile := range input.Plan.Compose.Profiles {
		argv = append(argv, "--profile", profile)
	}
	for _, file := range files {
		argv = append(argv, "-f", file)
	}
	argv = append(argv, "config", "--format", "json")
	output, err := run(ctx, input, argv)
	if err != nil {
		return nil, err
	}
	if len(output) == 0 || len(output) > maxNormalizedBytes {
		return nil, fmt.Errorf("normalized output size %d is outside bounds", len(output))
	}
	return output, nil
}

func run(ctx context.Context, input Input, argv []string) ([]byte, error) {
	result, err := input.Runner.Run(ctx, Command{
		Argv: argv, Dir: input.Directory, Env: cloneMap(input.Environment), Secrets: append([]Secret(nil), input.Secrets...),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(result.Stderr)))
	}
	if result.StdoutTruncated || result.StderrTruncated {
		return nil, errors.New("compose output exceeded capture limit")
	}
	return result.Stdout, nil
}

func serviceNames(model []byte) ([]string, error) {
	var document struct {
		Services map[string]json.RawMessage `json:"services"`
	}
	if err := decodeJSON(model, &document); err != nil {
		return nil, err
	}
	if len(document.Services) == 0 {
		return nil, errors.New("normalized model has no services")
	}
	services := make([]string, 0, len(document.Services))
	for service := range document.Services {
		if service == "" || strings.ContainsAny(service, "\x00\r\n") {
			return nil, errors.New("normalized model contains an invalid service name")
		}
		services = append(services, service)
	}
	slices.Sort(services)
	return services, nil
}

func resourceNames(model []byte, field string) ([]string, error) {
	var document map[string]json.RawMessage
	if err := decodeJSON(model, &document); err != nil {
		return nil, err
	}
	value, ok := document[field]
	if !ok {
		return nil, nil
	}
	var resources map[string]json.RawMessage
	if err := decodeJSON(value, &resources); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(resources))
	for name := range resources {
		if name == "" || strings.ContainsAny(name, "\x00\r\n") {
			return nil, fmt.Errorf("normalized model contains an invalid %s name", field)
		}
		names = append(names, name)
	}
	slices.Sort(names)
	return names, nil
}

func validateAndCanonicalize(model []byte, compiled contract.Plan, generation int64, services []string, secrets []Secret) ([]byte, error) {
	for _, secret := range secrets {
		if secret.Value != "" && bytes.Contains(model, []byte(secret.Value)) {
			return nil, errors.New("retain normalized Compose model: registered secret survived redaction")
		}
	}
	var document map[string]any
	if err := decodeJSON(model, &document); err != nil {
		return nil, fmt.Errorf("retain normalized Compose model: %w", err)
	}
	serviceValue, ok := document["services"].(map[string]any)
	if !ok || len(serviceValue) != len(services) {
		return nil, errors.New("retain normalized Compose model: service set changed during adaptation")
	}
	expectedLabels := ownershipLabels(compiled, generation)
	for _, name := range services {
		value, ok := serviceValue[name].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("retain normalized Compose model: service %q is missing", name)
		}
		if ports, exists := value["ports"]; exists && !emptyJSONValue(ports) {
			return nil, fmt.Errorf("retain normalized Compose model: service %q retains Machine port publications", name)
		}
		if !labelsContain(value["labels"], expectedLabels) {
			return nil, fmt.Errorf("retain normalized Compose model: service %q lacks ownership labels", name)
		}
	}
	canonical, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("retain normalized Compose model: encode canonical JSON: %w", err)
	}
	return canonical, nil
}

func ownershipLabels(compiled contract.Plan, generation int64) map[string]string {
	return map[string]string{
		"dev.abox.managed":    "true",
		"dev.abox.workspace":  compiled.WorkspaceID,
		"dev.abox.box":        compiled.BoxID,
		"dev.abox.plan":       string(compiled.PlanDigest),
		"dev.abox.generation": strconv.FormatInt(generation, 10),
	}
}

func writeLabels(output *strings.Builder, indent string, labels map[string]string) {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		output.WriteString(indent)
		output.WriteString(yamlString(key))
		output.WriteString(": ")
		output.WriteString(yamlString(labels[key]))
		output.WriteByte('\n')
	}
}

func labelsContain(value any, expected map[string]string) bool {
	labels, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for key, expectedValue := range expected {
		if labels[key] != expectedValue {
			return false
		}
	}
	return true
}

func decodeJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode normalized JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("decode normalized JSON: trailing value")
		}
		return fmt.Errorf("decode normalized JSON trailer: %w", err)
	}
	return nil
}

func emptyJSONValue(value any) bool {
	if value == nil {
		return true
	}
	list, ok := value.([]any)
	return ok && len(list) == 0
}

func yamlString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func contained(root, path string) bool {
	if root == "" || path == "" || !filepath.IsAbs(root) || !filepath.IsAbs(path) {
		return false
	}
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func appendCommand(base []string, values ...string) []string {
	result := append([]string{}, base...)
	return append(result, values...)
}

func cloneMap(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func boolCompare(left, right bool) int {
	if left == right {
		return 0
	}
	if !left {
		return -1
	}
	return 1
}
