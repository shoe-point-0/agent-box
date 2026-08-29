package compose_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/manifest"
	"github.com/shoe-point-0/agent-box/internal/plan"
	aboxprocess "github.com/shoe-point-0/agent-box/internal/process"
	"github.com/shoe-point-0/agent-box/internal/repository"
)

const generation int64 = 7

func TestCompileGeneratedInputsAreDeterministic(t *testing.T) {
	compiled, outputDirectory, root := fixturePlan(t, "2.39.1")
	baseline := []byte(`{"services":{"workspace":{},"worker":{}}}`)
	adapted := adaptedModel(t, compiled, nil)
	var first map[string][]byte
	var firstDigest contract.Digest

	for iteration := range 20 {
		runner := &fakeRunner{outputs: [][]byte{[]byte("2.39.1\n"), baseline, adapted}}
		result, err := compose.Compile(context.Background(), compose.Input{
			Plan: compiled, Directory: outputDirectory, Generation: generation,
			ComposeCommand: []string{"docker", "compose"}, Environment: map[string]string{"PATH": "/usr/bin"}, Runner: runner,
		})
		if err != nil {
			t.Fatalf("iteration %d: Compile() error = %v", iteration, err)
		}
		assertCommandContract(t, runner.commands, compiled, outputDirectory)
		current := readGeneratedFiles(t, outputDirectory)
		if iteration == 0 {
			first = current
			firstDigest = result.ResolvedModelDigest
			assertGolden(t, "compose.root.golden.yaml", normalizeGenerated(current[compose.RootFileName], root, compiled))
			assertGolden(t, "compose.abox.golden.yaml", normalizeGenerated(current[compose.OverrideFileName], root, compiled))
			continue
		}
		if !reflect.DeepEqual(current, first) || result.ResolvedModelDigest != firstDigest {
			t.Fatalf("iteration %d changed generated artifacts or digest", iteration)
		}
	}
}

func TestCompileRejectsVersionDriftAndPlanMutationBeforeNormalization(t *testing.T) {
	compiled, outputDirectory, _ := fixturePlan(t, "2.39.1")
	t.Run("version drift", func(t *testing.T) {
		runner := &fakeRunner{outputs: [][]byte{[]byte("2.40.0\n")}}
		_, err := compose.Compile(context.Background(), compose.Input{
			Plan: compiled, Directory: outputDirectory, Generation: generation,
			ComposeCommand: []string{"docker", "compose"}, Runner: runner,
		})
		if err == nil || !strings.Contains(err.Error(), `got "2.40.0", want pinned "2.39.1"`) {
			t.Fatalf("Compile(version drift) error = %v", err)
		}
		if len(runner.commands) != 1 {
			t.Fatalf("commands = %d, want only version preflight", len(runner.commands))
		}
	})

	t.Run("mutated Plan", func(t *testing.T) {
		mutated := compiled
		mutated.TaskKey = "OTHER"
		runner := &fakeRunner{}
		_, err := compose.Compile(context.Background(), compose.Input{
			Plan: mutated, Directory: outputDirectory, Generation: generation,
			ComposeCommand: []string{"docker", "compose"}, Runner: runner,
		})
		if err == nil || !strings.Contains(err.Error(), "plan digest does not match content") {
			t.Fatalf("Compile(mutated Plan) error = %v", err)
		}
		if len(runner.commands) != 0 {
			t.Fatalf("mutated Plan executed %d commands", len(runner.commands))
		}
	})
}

func TestCompileRefusesPublicationOrUnredactedSecret(t *testing.T) {
	compiled, outputDirectory, _ := fixturePlan(t, "2.39.1")
	baseline := []byte(`{"services":{"workspace":{},"worker":{}}}`)

	t.Run("publication", func(t *testing.T) {
		adapted := adaptedModel(t, compiled, map[string]any{"workspace": map[string]any{
			"ports": []any{map[string]any{"target": 8080, "published": "30000"}},
		}})
		runner := &fakeRunner{outputs: [][]byte{[]byte("2.39.1\n"), baseline, adapted}}
		_, err := compose.Compile(context.Background(), compose.Input{
			Plan: compiled, Directory: outputDirectory, Generation: generation,
			ComposeCommand: []string{"docker", "compose"}, Runner: runner,
		})
		if err == nil || !strings.Contains(err.Error(), "retains Machine port publications") {
			t.Fatalf("Compile(publication) error = %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(outputDirectory, compose.ResolvedModelFileName)); !os.IsNotExist(statErr) {
			t.Fatalf("retained model exists after refusal: %v", statErr)
		}
	})

	t.Run("secret", func(t *testing.T) {
		const canary = "compose-secret-canary-do-not-retain"
		adapted := adaptedModel(t, compiled, map[string]any{"workspace": map[string]any{
			"environment": map[string]any{"CANARY": canary},
		}})
		runner := &fakeRunner{outputs: [][]byte{[]byte("2.39.1\n"), baseline, adapted}}
		_, err := compose.Compile(context.Background(), compose.Input{
			Plan: compiled, Directory: outputDirectory, Generation: generation,
			ComposeCommand: []string{"docker", "compose"}, Runner: runner,
			Secrets: []compose.Secret{{Value: canary}},
		})
		if err == nil || !strings.Contains(err.Error(), "registered secret survived redaction") {
			t.Fatalf("Compile(unredacted secret) error = %v", err)
		}
	})
}

func TestCompileRejectsForbiddenNormalizedPolicyBeforeApply(t *testing.T) {
	tests := []struct {
		name      string
		field     string
		configure func(contract.Plan, map[string]any)
	}{
		{"privileged", "services.workspace.privileged", setServiceField("privileged", true)},
		{"host network", "services.workspace.network_mode", setServiceField("network_mode", "host")},
		{"host PID", "services.workspace.pid", setServiceField("pid", "host")},
		{"host IPC", "services.workspace.ipc", setServiceField("ipc", "host")},
		{"host UTS", "services.workspace.uts", setServiceField("uts", "host")},
		{"host user namespace", "services.workspace.userns_mode", setServiceField("userns_mode", "host")},
		{"host cgroup namespace", "services.workspace.cgroup", setServiceField("cgroup", "host")},
		{"capability addition", "services.workspace.cap_add", setServiceField("cap_add", []any{"SYS_ADMIN"})},
		{"seccomp unconfined", "services.workspace.security_opt[0]", setServiceField("security_opt", []any{"seccomp=unconfined"})},
		{"AppArmor unconfined", "services.workspace.security_opt[0]", setServiceField("security_opt", []any{"apparmor=unconfined"})},
		{"SELinux disabled", "services.workspace.security_opt[0]", setServiceField("security_opt", []any{"label=disable"})},
		{"device", "services.workspace.devices[0]", setServiceField("devices", []any{map[string]any{"source": "/dev/null", "target": "/dev/xnull", "permissions": "rwm"}})},
		{"device cgroup rule", "services.workspace.device_cgroup_rules[0]", setServiceField("device_cgroup_rules", []any{"c 1:3 rwm"})},
		{"Docker socket source", "services.workspace.volumes[0]", replaceFirstVolume(map[string]any{"type": "bind", "source": "/var/run/docker.sock", "target": "/socket"})},
		{"Docker socket target", "services.workspace.volumes[0]", replaceFirstVolume(map[string]any{"type": "volume", "source": "docker-api", "target": "/run/docker.sock"})},
		{"proc mount", "services.workspace.volumes[0]", replaceFirstVolume(map[string]any{"type": "bind", "source": "/proc", "target": "/mnt/proc"})},
		{"sys mount", "services.workspace.volumes[0]", replaceFirstVolume(map[string]any{"type": "volume", "source": "sys", "target": "/sys/kernel"})},
		{"dev mount", "services.workspace.volumes[0]", replaceFirstVolume(map[string]any{"type": "tmpfs", "target": "/dev/shm/foreign"})},
		{"cgroup mount", "services.workspace.volumes[0]", replaceFirstVolume(map[string]any{"type": "volume", "source": "cgroup", "target": "/sys/fs/cgroup"})},
		{"undeclared bind", "services.workspace.volumes[0].source", replaceFirstVolume(map[string]any{"type": "bind", "source": "/opt/foreign", "target": "/opt/foreign"})},
		{"rewritten bind target", "services.workspace.volumes[0].target", mutateFirstVolume("target", "/workspace")},
		{"widened bind access", "services.workspace.volumes[0].read_only", mutateReadOnlyPlanBind},
		{"shared bind propagation", "services.workspace.volumes[0].bind.propagation", mutateFirstVolume("bind", map[string]any{"propagation": "rshared"})},
		{"missing Plan bind", "services.workspace.volumes", removeFirstVolume},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compiled, outputDirectory, _ := fixturePlan(t, "2.39.1")
			overlay := map[string]any{}
			test.configure(compiled, overlay)
			runner := &fakeRunner{outputs: [][]byte{
				[]byte("2.39.1\n"),
				[]byte(`{"services":{"workspace":{},"worker":{}}}`),
				adaptedModel(t, compiled, map[string]any{"workspace": overlay}),
			}}
			_, err := compose.Compile(context.Background(), compose.Input{
				Plan: compiled, Directory: outputDirectory, Generation: generation,
				ComposeCommand: []string{"docker", "compose"}, Runner: runner,
			})
			var violation *compose.PolicyViolation
			if !errors.As(err, &violation) {
				t.Fatalf("Compile() error = %v, want PolicyViolation", err)
			}
			if violation.Service != "workspace" || violation.Field != test.field {
				t.Fatalf("violation = %+v, want service workspace field %s", violation, test.field)
			}
			if len(runner.commands) != 3 {
				t.Fatalf("commands = %d, want version plus two config-only normalizations", len(runner.commands))
			}
			for _, command := range runner.commands {
				for _, forbidden := range []string{"pull", "build", "create", "up", "start"} {
					if slices.Contains(command.Argv, forbidden) {
						t.Fatalf("policy refusal executed forbidden command %q: %q", forbidden, command.Argv)
					}
				}
			}
			for _, artifact := range []string{compose.RootFileName, compose.OverrideFileName, compose.ResolvedModelFileName} {
				if _, statErr := os.Stat(filepath.Join(outputDirectory, artifact)); !os.IsNotExist(statErr) {
					t.Fatalf("artifact %s exists after policy refusal: %v", artifact, statErr)
				}
			}
		})
	}
}

func TestCompileAllowsNoNewPrivilegesSecurityOption(t *testing.T) {
	compiled, outputDirectory, _ := fixturePlan(t, "2.39.1")
	adapted := adaptedModel(t, compiled, map[string]any{"workspace": map[string]any{
		"security_opt": []any{"no-new-privileges=true"},
	}})
	runner := &fakeRunner{outputs: [][]byte{[]byte("2.39.1\n"), []byte(`{"services":{"workspace":{},"worker":{}}}`), adapted}}
	if _, err := compose.Compile(context.Background(), compose.Input{
		Plan: compiled, Directory: outputDirectory, Generation: generation,
		ComposeCommand: []string{"docker", "compose"}, Runner: runner,
	}); err != nil {
		t.Fatalf("Compile(no-new-privileges) error = %v", err)
	}
}

func TestCompileNormalizesWithRealPinnedCompose(t *testing.T) {
	docker, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("docker is unavailable")
	}
	versionCommand := exec.Command(docker, "compose", "version", "--short")
	versionOutput, err := versionCommand.Output()
	if err != nil {
		t.Skipf("Docker Compose is unavailable: %v", err)
	}
	version := strings.TrimSpace(string(versionOutput))
	compiled, outputDirectory, root := fixturePlan(t, version)
	const canary = "real-compose-secret-canary-do-not-retain"
	runner := processAdapter{runner: aboxprocess.Runner{CaptureLimit: composeModelLimitForTest}}
	result, err := compose.Compile(context.Background(), compose.Input{
		Plan: compiled, Directory: outputDirectory, Generation: generation,
		ComposeCommand: []string{docker, "compose"},
		Environment:    map[string]string{"PATH": os.Getenv("PATH"), "CANARY": canary},
		Secrets:        []compose.Secret{{Value: canary}}, Runner: runner,
	})
	if err != nil {
		t.Fatalf("Compile(real Compose %s) error = %v", version, err)
	}
	if bytes.Contains(result.ResolvedModel, []byte(canary)) {
		t.Fatal("retained real Compose model contains canary secret")
	}
	var model struct {
		Services map[string]struct {
			Build struct {
				Context string `json:"context"`
			} `json:"build"`
			Ports []any `json:"ports"`
		} `json:"services"`
		Networks map[string]struct {
			Internal bool `json:"internal"`
		} `json:"networks"`
	}
	if err := json.Unmarshal(result.ResolvedModel, &model); err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(root, "worktree")
	if got := model.Services["workspace"].Build.Context; filepath.Clean(got) != worktree {
		t.Fatalf("build context = %q, want owning project directory %q", got, worktree)
	}
	for name, service := range model.Services {
		if len(service.Ports) != 0 {
			t.Fatalf("service %s retained ports: %v", name, service.Ports)
		}
	}
	if network, ok := model.Networks[compiled.Compose.ProjectName+"_default"]; !ok || !network.Internal {
		// Compose versions differ on whether the normalized key is "default" or
		// the resource name. Accept either, but require an internal network.
		if network, ok = model.Networks["default"]; !ok || !network.Internal {
			t.Fatalf("normalized networks do not enforce denied egress: %+v", model.Networks)
		}
	}
}

const composeModelLimitForTest = 16 << 20

type fakeRunner struct {
	outputs  [][]byte
	commands []compose.Command
}

func (r *fakeRunner) Run(_ context.Context, command compose.Command) (compose.CommandResult, error) {
	r.commands = append(r.commands, command)
	index := len(r.commands) - 1
	if index >= len(r.outputs) {
		return compose.CommandResult{}, fmt.Errorf("unexpected command %d", index)
	}
	return compose.CommandResult{Stdout: append([]byte(nil), r.outputs[index]...)}, nil
}

type processAdapter struct {
	runner aboxprocess.Runner
}

func (a processAdapter) Run(ctx context.Context, command compose.Command) (compose.CommandResult, error) {
	secrets := make([]aboxprocess.Secret, len(command.Secrets))
	for index, secret := range command.Secrets {
		secrets[index] = aboxprocess.Secret{Value: secret.Value}
	}
	result, err := a.runner.Run(ctx, aboxprocess.Command{
		Argv: command.Argv, Dir: command.Dir, Env: command.Env, Secrets: secrets,
	})
	return compose.CommandResult{
		Stdout: result.Stdout, Stderr: result.Stderr,
		StdoutTruncated: result.StdoutTruncated, StderrTruncated: result.StderrTruncated,
	}, err
}

func fixturePlan(t *testing.T, composeVersion string) (contract.Plan, string, string) {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "source")
	worktree := filepath.Join(root, "worktree")
	gitStore := filepath.Join(root, "git", "repo.git")
	outputDirectory := filepath.Join(root, "generated")
	manifestDirectory := filepath.Join(root, "workspace", ".abox")
	for _, directory := range []string{source, worktree, gitStore, outputDirectory, manifestDirectory} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	composeFile := `services:
  workspace:
    build:
      context: .
    image: alpine:3.22
    command: ["sleep", "infinity"]
    environment:
      CANARY: ${CANARY:-unset}
    ports:
      - target: 8080
        published: "0"
        host_ip: 127.0.0.1
  worker:
    image: alpine:3.22
`
	for _, directory := range []string{source, worktree} {
		if err := os.WriteFile(filepath.Join(directory, "compose.dev.yaml"), []byte(composeFile), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	manifestPath := filepath.Join(manifestDirectory, "box.yaml")
	manifestContent := fmt.Sprintf(`apiVersion: abox.dev/v1alpha1
kind: Workspace
metadata:
  name: compose-fixture
spec:
  repositories:
    - name: repo
      source:
        local: %s
      checkout:
        ref: HEAD
        branch: abox/{{ box.task }}/repo
      workspacePath: repo
  host:
    cpus: 2
    memory: 2GiB
    disk: 20GiB
  compose:
    includes:
      - repository: repo
        path: compose.dev.yaml
    profiles: [test, development]
  workspaceService:
    name: workspace
    repository: repo
    workingDirectory: repo
  mounts:
    - repository: repo
      services: [workspace, worker]
      mode: read-write
  policy:
    gitMetadata: read-only
    dockerSocket: deny
    machineMounts: declared-only
    networkEgress: deny
`, source)
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0o600); err != nil {
		t.Fatal(err)
	}
	loader, err := manifest.NewLoader([]string{root}, manifest.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := loader.Load(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	workspaceID, err := identity.Workspace(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	boxID, err := identity.Box(workspaceID, "COMPOSE-17")
	if err != nil {
		t.Fatal(err)
	}
	compiled, _, err := plan.Compile(plan.Input{
		Workspace: workspace, WorkspaceID: workspaceID, BoxID: boxID, TaskKey: "COMPOSE-17", ManifestPath: manifestPath,
		Repositories: []repository.Facts{{
			Name: "repo", SourceRoot: source, Clean: true,
			BaseCommit: "0123456789abcdef0123456789abcdef01234567", Branch: "abox/COMPOSE-17/repo",
			WorktreePath: worktree, GitStorePath: gitStore,
			PublishDestination: repository.Destination{Kind: repository.DestinationLocal, URL: source},
		}},
		Runtime: plan.RuntimeFacts{Driver: "lima-qemu"},
		Tools:   contract.ToolVersions{AgentBox: "0.1.0-dev", Git: "2.51.0", Lima: "2.0.0", Docker: "29.6.1", Compose: composeVersion},
	})
	if err != nil {
		t.Fatal(err)
	}
	return compiled, outputDirectory, root
}

func adaptedModel(t *testing.T, compiled contract.Plan, overlays map[string]any) []byte {
	t.Helper()
	labels := map[string]any{
		"dev.abox.managed": "true", "dev.abox.workspace": compiled.WorkspaceID,
		"dev.abox.box": compiled.BoxID, "dev.abox.plan": string(compiled.PlanDigest),
		"dev.abox.generation": fmt.Sprint(generation),
	}
	services := map[string]any{
		"worker":    map[string]any{"labels": labels},
		"workspace": map[string]any{"labels": labels, "build": map[string]any{"context": compiled.Repositories[0].WorktreePath}},
	}
	for _, mount := range compiled.Mounts {
		for _, serviceName := range mount.Services {
			service := services[serviceName].(map[string]any)
			service["volumes"] = append(serviceVolumes(service), map[string]any{
				"type": "bind", "source": mount.Path, "target": mount.Path, "read_only": !mount.Writable,
			})
		}
	}
	for name, overlay := range overlays {
		service := services[name].(map[string]any)
		for key, value := range overlay.(map[string]any) {
			service[key] = value
		}
	}
	encoded, err := json.Marshal(map[string]any{"name": compiled.Compose.ProjectName, "services": services})
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func serviceVolumes(service map[string]any) []any {
	volumes, _ := service["volumes"].([]any)
	return volumes
}

func setServiceField(field string, value any) func(contract.Plan, map[string]any) {
	return func(_ contract.Plan, service map[string]any) {
		service[field] = value
	}
}

func replaceFirstVolume(replacement map[string]any) func(contract.Plan, map[string]any) {
	return func(compiled contract.Plan, service map[string]any) {
		volumes := plannedServiceVolumes(compiled, "workspace")
		volumes[0] = replacement
		service["volumes"] = volumes
	}
}

func mutateFirstVolume(field string, value any) func(contract.Plan, map[string]any) {
	return func(compiled contract.Plan, service map[string]any) {
		volumes := plannedServiceVolumes(compiled, "workspace")
		volumes[0].(map[string]any)[field] = value
		service["volumes"] = volumes
	}
}

func mutateReadOnlyPlanBind(compiled contract.Plan, service map[string]any) {
	volumes := plannedServiceVolumes(compiled, "workspace")
	volume := volumes[0].(map[string]any)
	volume["read_only"] = !volume["read_only"].(bool)
	service["volumes"] = volumes
}

func removeFirstVolume(compiled contract.Plan, service map[string]any) {
	service["volumes"] = plannedServiceVolumes(compiled, "workspace")[1:]
}

func plannedServiceVolumes(compiled contract.Plan, service string) []any {
	var volumes []any
	for _, mount := range compiled.Mounts {
		if slices.Contains(mount.Services, service) {
			volumes = append(volumes, map[string]any{
				"type": "bind", "source": mount.Path, "target": mount.Path, "read_only": !mount.Writable,
			})
		}
	}
	return volumes
}

func assertCommandContract(t *testing.T, commands []compose.Command, compiled contract.Plan, directory string) {
	t.Helper()
	if len(commands) != 3 {
		t.Fatalf("commands = %d, want version plus two normalizations", len(commands))
	}
	if !slices.Equal(commands[0].Argv, []string{"docker", "compose", "version", "--short"}) {
		t.Fatalf("version argv = %q", commands[0].Argv)
	}
	for _, command := range commands[1:] {
		joined := strings.Join(command.Argv, "\x00")
		if !strings.Contains(joined, "--project-name\x00"+compiled.Compose.ProjectName) || command.Dir != directory {
			t.Fatalf("normalization command lacks explicit identity/directory: %+v", command)
		}
		if !reflect.DeepEqual(command.Env, map[string]string{"PATH": "/usr/bin"}) {
			t.Fatalf("command environment = %#v", command.Env)
		}
	}
}

func readGeneratedFiles(t *testing.T, directory string) map[string][]byte {
	t.Helper()
	result := make(map[string][]byte, 3)
	for _, name := range []string{compose.RootFileName, compose.OverrideFileName, compose.ResolvedModelFileName} {
		path := filepath.Join(directory, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %04o", name, info.Mode().Perm())
		}
		result[name] = data
	}
	return result
}

func normalizeGenerated(data []byte, root string, compiled contract.Plan) []byte {
	result := string(data)
	for _, replacement := range []struct{ old, new string }{
		{root, "$ROOT"}, {compiled.BoxID, "$BOX_ID"}, {compiled.WorkspaceID, "$WORKSPACE_ID"},
		{string(compiled.PlanDigest), "$PLAN_DIGEST"},
	} {
		result = strings.ReplaceAll(result, replacement.old, replacement.new)
	}
	return []byte(result)
}

func assertGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	want, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, got)
	}
}
