package spike

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	ComposeProbe        = "compose-include"
	composeTreeName     = "compose-probe-fixtures"
	composeTreeMarker   = ".abox-spike-compose-owner"
	composeOwnerLabel   = "dev.abox.spike.owner"
	composeBaseImage    = "alpine:3.20"
	composeReportSchema = "abox.spike.compose-include/v1"
)

type ComposeProbeReport struct {
	SchemaVersion            string                 `json:"schemaVersion"`
	ComposeVersion           string                 `json:"composeVersion"`
	BaseImage                string                 `json:"baseImage"`
	Project                  string                 `json:"project"`
	BaselinePortsPresent     bool                   `json:"baselinePortsPresent"`
	AdaptedPortsAbsent       bool                   `json:"adaptedPortsAbsent"`
	BuildContextsCorrect     bool                   `json:"buildContextsCorrect"`
	RepositoryFilesUnchanged bool                   `json:"repositoryFilesUnchanged"`
	Services                 []ComposeServiceResult `json:"services"`
}

type ComposeServiceResult struct {
	Name             string `json:"name"`
	ContainerID      string `json:"containerId"`
	Running          bool   `json:"running"`
	Identity         string `json:"identity"`
	ExpectedIdentity string `json:"expectedIdentity"`
	PortBindingsNone bool   `json:"portBindingsNone"`
	DockerPortEmpty  bool   `json:"dockerPortEmpty"`
	OwnerLabel       string `json:"ownerLabel"`
}

type composeModel struct {
	Services map[string]composeModelService `json:"services"`
}

type composeModelService struct {
	Build  composeModelBuild `json:"build"`
	Ports  json.RawMessage   `json:"ports"`
	Labels map[string]string `json:"labels"`
}

type composeModelBuild struct {
	Context    string `json:"context"`
	Dockerfile string `json:"dockerfile"`
}

type composeProjectResource struct {
	project      string
	rootFile     string
	overrideFile string
	images       []string
	ownerID      string
}

func RegisterComposeProbe(registry *Registry) error {
	return registry.RegisterScenario(ComposeProbe, Scenario{
		Run:     runComposeProbe,
		Cleanup: cleanupComposeProbe,
	})
}

func runComposeProbe(ctx context.Context, env *Environment) error {
	tree, err := createComposeTree(env)
	if err != nil {
		return err
	}
	composeVersion, err := runText(ctx, env, "compose-version", []string{"docker", "compose", "version", "--short"})
	if err != nil {
		return err
	}
	baseImage, err := runText(ctx, env, "compose-base-image", []string{"docker", "image", "inspect", "--format", "{{index .RepoDigests 0}}", composeBaseImage})
	if err != nil {
		return fmt.Errorf("inspect locally cached Compose base image %s: %w", composeBaseImage, err)
	}
	baseImage = strings.TrimSpace(baseImage)
	if !strings.Contains(baseImage, "@sha256:") {
		return fmt.Errorf("compose base image is not content-addressed: %q", baseImage)
	}

	rootFile, overrideFile, repositories, images, err := writeComposeFixtures(tree, env.workspace.metadata.ID, baseImage)
	if err != nil {
		return err
	}
	before, err := snapshotSelectedFiles(repositoryComposePaths(repositories))
	if err != nil {
		return err
	}
	baselineJSON, err := runText(ctx, env, "compose-baseline-config", []string{"docker", "compose", "-f", rootFile, "config", "--format", "json"})
	if err != nil {
		return err
	}
	adaptedJSON, err := runText(ctx, env, "compose-adapted-config", []string{"docker", "compose", "-f", rootFile, "-f", overrideFile, "config", "--format", "json"})
	if err != nil {
		return err
	}
	baseline, err := decodeComposeModel([]byte(baselineJSON))
	if err != nil {
		return fmt.Errorf("decode baseline Compose model: %w", err)
	}
	adapted, err := decodeComposeModel([]byte(adaptedJSON))
	if err != nil {
		return fmt.Errorf("decode adapted Compose model: %w", err)
	}
	baselinePorts, adaptedNoPorts, contextsCorrect, err := validateComposeModels(baseline, adapted, repositories)
	if err != nil {
		return err
	}
	if !baselinePorts || !adaptedNoPorts || !contextsCorrect {
		return fmt.Errorf("refuse Compose apply: baselinePorts=%t adaptedNoPorts=%t contextsCorrect=%t", baselinePorts, adaptedNoPorts, contextsCorrect)
	}

	project := "aboxspike" + env.workspace.metadata.ID[:12]
	projectResource := Resource{
		Kind:    "compose-project",
		ID:      project,
		OwnerID: env.workspace.metadata.ID,
		Attributes: map[string]string{
			"rootFile":     rootFile,
			"overrideFile": overrideFile,
			"images":       strings.Join(images, ","),
		},
	}
	if _, err := env.workspace.inventory.Append("planned", projectResource); err != nil {
		return err
	}
	composeArgs := composeCommand(project, rootFile, overrideFile)
	upArgs := append(append([]string{}, composeArgs...), "up", "--detach", "--build", "--pull", "never")
	if _, err := runText(ctx, env, "compose-up", upArgs); err != nil {
		return err
	}
	if _, err := env.workspace.inventory.Append("created", projectResource); err != nil {
		return err
	}

	containerOutput, err := runText(ctx, env, "compose-container-ids", append(append([]string{}, composeArgs...), "ps", "--all", "--quiet"))
	if err != nil {
		return err
	}
	containerIDs := nonemptyLines(containerOutput)
	if len(containerIDs) != 2 {
		return fmt.Errorf("expected 2 Compose containers, got %d: %v", len(containerIDs), containerIDs)
	}
	services := make([]ComposeServiceResult, 0, len(containerIDs))
	for index, containerID := range containerIDs {
		inspection, err := runText(ctx, env, fmt.Sprintf("compose-container-inspect-%d", index), []string{
			"docker", "inspect", "--format",
			"{{json .HostConfig.PortBindings}}|{{.State.Running}}|{{index .Config.Labels \"com.docker.compose.service\"}}|{{index .Config.Labels \"" + composeOwnerLabel + "\"}}",
			containerID,
		})
		if err != nil {
			return err
		}
		parts := strings.Split(strings.TrimSpace(inspection), "|")
		if len(parts) != 4 {
			return fmt.Errorf("unexpected container inspection for %s: %q", containerID, inspection)
		}
		service := parts[2]
		identity, err := runText(ctx, env, "compose-identity-"+service, append(append([]string{}, composeArgs...), "exec", "--no-TTY", service, "cat", "/identity.txt"))
		if err != nil {
			return err
		}
		portOutput, err := runText(ctx, env, "compose-docker-port-"+service, []string{"docker", "port", containerID})
		if err != nil {
			return err
		}
		expectedIdentity := service + "-repository"
		serviceResult := ComposeServiceResult{
			Name:             service,
			ContainerID:      containerID,
			Running:          parts[1] == "true",
			Identity:         strings.TrimSpace(identity),
			ExpectedIdentity: expectedIdentity,
			PortBindingsNone: parts[0] == "null" || parts[0] == "{}",
			DockerPortEmpty:  strings.TrimSpace(portOutput) == "",
			OwnerLabel:       parts[3],
		}
		if !serviceResult.Running || serviceResult.Identity != expectedIdentity || !serviceResult.PortBindingsNone || !serviceResult.DockerPortEmpty || serviceResult.OwnerLabel != env.workspace.metadata.ID {
			return fmt.Errorf("compose service invariant failed: %+v", serviceResult)
		}
		services = append(services, serviceResult)
	}
	sort.Slice(services, func(left, right int) bool { return services[left].Name < services[right].Name })
	after, err := snapshotSelectedFiles(repositoryComposePaths(repositories))
	if err != nil {
		return err
	}
	report := ComposeProbeReport{
		SchemaVersion:            composeReportSchema,
		ComposeVersion:           strings.TrimSpace(composeVersion),
		BaseImage:                baseImage,
		Project:                  project,
		BaselinePortsPresent:     baselinePorts,
		AdaptedPortsAbsent:       adaptedNoPorts,
		BuildContextsCorrect:     contextsCorrect,
		RepositoryFilesUnchanged: snapshotsEqual(before, after),
		Services:                 services,
	}
	if !report.RepositoryFilesUnchanged {
		return errors.New("repository Compose files changed during adaptation")
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Compose report: %w", err)
	}
	data = append(data, '\n')
	if _, err := env.workspace.WriteDiagnostic("compose-report.json", data); err != nil {
		return err
	}
	return nil
}

func writeComposeFixtures(tree, ownerID, baseImage string) (string, string, map[string]string, []string, error) {
	repositories := map[string]string{
		"alpha": filepath.Join(tree, "repo-alpha"),
		"beta":  filepath.Join(tree, "repo-beta"),
	}
	images := []string{"abox-spike-" + ownerID[:12] + "-alpha", "abox-spike-" + ownerID[:12] + "-beta"}
	services := []string{"alpha", "beta"}
	for index, service := range services {
		repository := repositories[service]
		if err := os.Mkdir(repository, 0o700); err != nil {
			return "", "", nil, nil, fmt.Errorf("create %s repository fixture: %w", service, err)
		}
		dockerfile := fmt.Sprintf("FROM %s\nCOPY identity.txt /identity.txt\nCMD [\"sleep\", \"3600\"]\n", baseImage)
		compose := fmt.Sprintf("services:\n  %s:\n    image: %s\n    build:\n      context: .\n      dockerfile: Dockerfile\n    ports:\n      - target: 8080\n        published: \"0\"\n        host_ip: 127.0.0.1\n        protocol: tcp\n", service, images[index])
		files := map[string][]byte{
			filepath.Join(repository, "Dockerfile"):   []byte(dockerfile),
			filepath.Join(repository, "compose.yaml"): []byte(compose),
			filepath.Join(repository, "identity.txt"): []byte(service + "-repository\n"),
		}
		for path, data := range files {
			if err := writeSyncedFile(path, data, 0o600); err != nil {
				return "", "", nil, nil, fmt.Errorf("write Compose fixture %s: %w", path, err)
			}
		}
	}
	rootFile := filepath.Join(tree, "compose.root.yaml")
	overrideFile := filepath.Join(tree, "compose.abox.yaml")
	root := "include:\n  - path: repo-alpha/compose.yaml\n    project_directory: repo-alpha\n  - path: repo-beta/compose.yaml\n    project_directory: repo-beta\n"
	override := fmt.Sprintf("services:\n  alpha:\n    ports: !reset []\n    labels:\n      %s: %q\n  beta:\n    ports: !reset []\n    labels:\n      %s: %q\nnetworks:\n  default:\n    labels:\n      %s: %q\n", composeOwnerLabel, ownerID, composeOwnerLabel, ownerID, composeOwnerLabel, ownerID)
	if err := writeSyncedFile(rootFile, []byte(root), 0o600); err != nil {
		return "", "", nil, nil, err
	}
	if err := writeSyncedFile(overrideFile, []byte(override), 0o600); err != nil {
		return "", "", nil, nil, err
	}
	return rootFile, overrideFile, repositories, images, nil
}

func decodeComposeModel(data []byte) (composeModel, error) {
	var model composeModel
	if err := json.Unmarshal(data, &model); err != nil {
		return composeModel{}, err
	}
	return model, nil
}

func validateComposeModels(baseline, adapted composeModel, repositories map[string]string) (bool, bool, bool, error) {
	if len(baseline.Services) != 2 || len(adapted.Services) != 2 {
		return false, false, false, fmt.Errorf("expected exactly alpha and beta services")
	}
	baselinePorts := true
	adaptedNoPorts := true
	contextsCorrect := true
	for _, service := range []string{"alpha", "beta"} {
		baselineService, baselineExists := baseline.Services[service]
		adaptedService, adaptedExists := adapted.Services[service]
		if !baselineExists || !adaptedExists {
			return false, false, false, fmt.Errorf("missing service %s", service)
		}
		if rawJSONEmpty(baselineService.Ports) {
			baselinePorts = false
		}
		if !rawJSONEmpty(adaptedService.Ports) {
			adaptedNoPorts = false
		}
		expected, err := filepath.Abs(repositories[service])
		if err != nil {
			return false, false, false, err
		}
		if filepath.Clean(adaptedService.Build.Context) != filepath.Clean(expected) || filepath.Clean(baselineService.Build.Context) != filepath.Clean(expected) {
			contextsCorrect = false
		}
	}
	return baselinePorts, adaptedNoPorts, contextsCorrect, nil
}

func rawJSONEmpty(value json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(value))
	return trimmed == "" || trimmed == "null" || trimmed == "[]"
}

func composeCommand(project, rootFile, overrideFile string) []string {
	return []string{"docker", "compose", "--project-name", project, "-f", rootFile, "-f", overrideFile}
}

func createComposeTree(env *Environment) (string, error) {
	path, err := env.workspace.resolveRelative(composeTreeName)
	if err != nil {
		return "", err
	}
	resource := Resource{
		Kind:    "compose-fixture-tree",
		ID:      path,
		OwnerID: env.workspace.metadata.ID,
		Attributes: map[string]string{
			"ownerMarker": composeTreeMarker,
		},
	}
	if _, err := env.workspace.inventory.Append("planned", resource); err != nil {
		return "", err
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		return "", fmt.Errorf("create Compose fixture tree: %w", err)
	}
	if err := writeSyncedFile(filepath.Join(path, composeTreeMarker), []byte(env.workspace.metadata.ID+"\n"), 0o600); err != nil {
		return "", err
	}
	if _, err := env.workspace.inventory.Append("created", resource); err != nil {
		return "", err
	}
	return path, nil
}

func cleanupComposeProbe(ctx context.Context, workspace *Workspace, resources []Resource) error {
	var projectResource *Resource
	var treeResource *Resource
	for index := range resources {
		resource := &resources[index]
		switch resource.Kind {
		case "compose-project":
			projectResource = resource
		case "compose-fixture-tree":
			treeResource = resource
		}
	}
	if projectResource != nil {
		if err := cleanupComposeProject(ctx, workspace, *projectResource); err != nil {
			return err
		}
	}
	if treeResource != nil {
		if err := cleanupComposeTree(workspace, *treeResource); err != nil {
			return err
		}
	}
	return nil
}

func cleanupComposeProject(ctx context.Context, workspace *Workspace, resource Resource) error {
	project, err := parseComposeProjectResource(resource, workspace.metadata.ID)
	if err != nil {
		return err
	}
	environment := &Environment{root: workspace.Root(), workspace: workspace, runner: Runner{MaxOutput: defaultOutputLimit}}
	containerIDs, err := cleanupText(ctx, environment, "cleanup-compose-containers", []string{"docker", "ps", "--all", "--filter", "label=com.docker.compose.project=" + project.project, "--format", "{{.ID}}"})
	if err != nil {
		return err
	}
	for index, containerID := range nonemptyLines(containerIDs) {
		labels, err := cleanupText(ctx, environment, fmt.Sprintf("cleanup-compose-container-%d", index), []string{"docker", "inspect", "--format", "{{index .Config.Labels \"com.docker.compose.project\"}}|{{index .Config.Labels \"" + composeOwnerLabel + "\"}}", containerID})
		if err != nil {
			return err
		}
		if strings.TrimSpace(labels) != project.project+"|"+project.ownerID {
			return fmt.Errorf("%w: container %s labels do not prove Compose ownership", ErrCleanupRefused, containerID)
		}
	}
	networkIDs, err := cleanupText(ctx, environment, "cleanup-compose-networks", []string{"docker", "network", "ls", "--filter", "label=com.docker.compose.project=" + project.project, "--format", "{{.ID}}"})
	if err != nil {
		return err
	}
	for index, networkID := range nonemptyLines(networkIDs) {
		labels, err := cleanupText(ctx, environment, fmt.Sprintf("cleanup-compose-network-%d", index), []string{"docker", "network", "inspect", "--format", "{{index .Labels \"com.docker.compose.project\"}}|{{index .Labels \"" + composeOwnerLabel + "\"}}", networkID})
		if err != nil {
			return err
		}
		if strings.TrimSpace(labels) != project.project+"|"+project.ownerID {
			return fmt.Errorf("%w: network %s labels do not prove Compose ownership", ErrCleanupRefused, networkID)
		}
	}
	down := append(composeCommand(project.project, project.rootFile, project.overrideFile), "down", "--volumes", "--remove-orphans", "--rmi", "local")
	if _, err := cleanupText(ctx, environment, "cleanup-compose-down", down); err != nil {
		return err
	}
	remaining, err := cleanupText(ctx, environment, "cleanup-compose-remaining", []string{"docker", "ps", "--all", "--filter", "label=com.docker.compose.project=" + project.project, "--format", "{{.ID}}"})
	if err != nil {
		return err
	}
	if strings.TrimSpace(remaining) != "" {
		return fmt.Errorf("compose project still has containers after down: %s", remaining)
	}
	for _, image := range project.images {
		result, inspectErr := (Runner{MaxOutput: defaultOutputLimit}).Run(ctx, Command{Argv: []string{"docker", "image", "inspect", image}, Env: os.Environ()})
		if inspectErr == nil {
			return fmt.Errorf("compose project image still exists after down: %s", image)
		}
		if result.ExitCode == -1 {
			return fmt.Errorf("inspect removed Compose image %s failed unexpectedly", image)
		}
	}
	return workspace.MarkRemoved(resource)
}

func cleanupComposeTree(workspace *Workspace, resource Resource) error {
	if resource.OwnerID != workspace.metadata.ID || !isWithin(workspace.Root(), resource.ID) || filepath.Base(resource.ID) != composeTreeName {
		return fmt.Errorf("%w: Compose fixture tree identity mismatch", ErrCleanupRefused)
	}
	info, err := os.Lstat(resource.ID)
	if errors.Is(err, os.ErrNotExist) {
		return workspace.MarkRemoved(resource)
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: Compose fixture tree is not a real directory", ErrCleanupRefused)
	}
	marker, err := os.ReadFile(filepath.Join(resource.ID, composeTreeMarker))
	if err != nil || strings.TrimSpace(string(marker)) != workspace.metadata.ID {
		return fmt.Errorf("%w: Compose fixture ownership marker mismatch", ErrCleanupRefused)
	}
	if err := preflightRemovableTree(resource.ID); err != nil {
		return err
	}
	if err := os.RemoveAll(resource.ID); err != nil {
		return fmt.Errorf("remove owned Compose fixture tree: %w", err)
	}
	return workspace.MarkRemoved(resource)
}

func parseComposeProjectResource(resource Resource, ownerID string) (composeProjectResource, error) {
	if resource.OwnerID != ownerID || resource.ID == "" {
		return composeProjectResource{}, fmt.Errorf("%w: Compose project owner mismatch", ErrCleanupRefused)
	}
	rootFile := resource.Attributes["rootFile"]
	overrideFile := resource.Attributes["overrideFile"]
	images := strings.Split(resource.Attributes["images"], ",")
	if rootFile == "" || overrideFile == "" || len(images) != 2 {
		return composeProjectResource{}, fmt.Errorf("%w: incomplete Compose project ownership record", ErrCleanupRefused)
	}
	return composeProjectResource{project: resource.ID, rootFile: rootFile, overrideFile: overrideFile, images: images, ownerID: ownerID}, nil
}

func cleanupText(ctx context.Context, env *Environment, label string, argv []string) (string, error) {
	result, err := env.RunCommand(ctx, label, Command{Argv: argv, Env: os.Environ()})
	return string(result.Stdout), err
}

func repositoryComposePaths(repositories map[string]string) []string {
	paths := make([]string, 0, len(repositories)*3)
	for _, repository := range repositories {
		paths = append(paths, filepath.Join(repository, "Dockerfile"), filepath.Join(repository, "compose.yaml"), filepath.Join(repository, "identity.txt"))
	}
	sort.Strings(paths)
	return paths
}

func snapshotSelectedFiles(paths []string) (map[string]fileSnapshot, error) {
	snapshot := make(map[string]fileSnapshot, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		digest := sha256.Sum256(data)
		snapshot[path] = fileSnapshot{Mode: info.Mode(), Hash: hex.EncodeToString(digest[:])}
	}
	return snapshot, nil
}

func nonemptyLines(value string) []string {
	lines := make([]string, 0)
	for _, line := range strings.Split(value, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
