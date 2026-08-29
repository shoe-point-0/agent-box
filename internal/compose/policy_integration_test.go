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
	"slices"
	"strings"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/contract"
	aboxprocess "github.com/shoe-point-0/agent-box/internal/process"
)

func TestCompileRejectsForbiddenPolicyWithRealComposeWithoutResources(t *testing.T) {
	docker, version := realCompose(t)
	compiled, outputDirectory, _ := fixturePlan(t, version)
	project := compiled.Compose.ProjectName
	assertNoComposeProjectResources(t, docker, project)

	malicious := `services:
  workspace:
    image: alpine:3.22
    privileged: true
    network_mode: host
    pid: host
    ipc: host
    uts: host
    userns_mode: host
    cgroup: host
    cap_add: [SYS_ADMIN]
    security_opt: [seccomp=unconfined]
    devices: [/dev/null:/dev/xnull:rwm]
    device_cgroup_rules: ["c 1:3 rwm"]
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
  worker:
    image: alpine:3.22
`
	if err := os.WriteFile(compiled.Compose.Includes[0].Path, []byte(malicious), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := processAdapter{runner: aboxprocess.Runner{CaptureLimit: composeModelLimitForTest}}
	_, err := compose.Compile(context.Background(), compose.Input{
		Plan: compiled, Directory: outputDirectory, Generation: generation,
		ComposeCommand: []string{docker, "compose"}, Environment: map[string]string{"PATH": os.Getenv("PATH")}, Runner: runner,
	})
	var violation *compose.PolicyViolation
	if !errors.As(err, &violation) || violation.Field != "services.workspace.privileged" {
		t.Fatalf("Compile(real forbidden fixture) error = %v, want privileged PolicyViolation", err)
	}
	assertNoComposeProjectResources(t, docker, project)
	for _, artifact := range []string{compose.RootFileName, compose.OverrideFileName, compose.ResolvedModelFileName} {
		if _, statErr := os.Stat(filepath.Join(outputDirectory, artifact)); !os.IsNotExist(statErr) {
			t.Fatalf("artifact %s exists after real policy refusal: %v", artifact, statErr)
		}
	}
}

func TestAllowedPolicyMatchesLiveContainerInspect(t *testing.T) {
	docker, version := realCompose(t)
	if err := exec.Command(docker, "image", "inspect", "alpine:3.20").Run(); err != nil {
		t.Skipf("cached live-inspect fixture image alpine:3.20 is unavailable: %v", err)
	}
	compiled, outputDirectory, _ := fixturePlan(t, version)
	project := compiled.Compose.ProjectName
	assertNoComposeProjectResources(t, docker, project)

	include := compiled.Compose.Includes[0].Path
	composeSource, err := os.ReadFile(include)
	if err != nil {
		t.Fatal(err)
	}
	composeSource = bytes.ReplaceAll(composeSource, []byte("alpine:3.22"), []byte("alpine:3.20"))
	composeSource = bytes.Replace(composeSource, []byte("    command: [\"sleep\", \"infinity\"]\n"), []byte("    command: [\"sleep\", \"infinity\"]\n    security_opt: [no-new-privileges=true]\n"), 1)
	if err := os.WriteFile(include, composeSource, 0o600); err != nil {
		t.Fatal(err)
	}
	runner := processAdapter{runner: aboxprocess.Runner{CaptureLimit: composeModelLimitForTest}}
	result, err := compose.Compile(context.Background(), compose.Input{
		Plan: compiled, Directory: outputDirectory, Generation: generation,
		ComposeCommand: []string{docker, "compose"}, Environment: map[string]string{"PATH": os.Getenv("PATH")}, Runner: runner,
	})
	if err != nil {
		t.Fatalf("Compile(real allowed fixture) error = %v", err)
	}

	composeArgs := []string{"compose", "--project-name", project, "-f", result.RootPath, "-f", result.OverridePath}
	cleaned := false
	t.Cleanup(func() {
		if !cleaned {
			_ = exec.Command(docker, append(composeArgs, "down", "-v", "--remove-orphans")...).Run()
		}
	})
	if output, err := exec.Command(docker, append(composeArgs, "up", "-d", "--no-build", "--pull", "never", "workspace")...).CombinedOutput(); err != nil {
		t.Fatalf("apply validated fixture: %v: %s", err, output)
	}
	containerIDOutput, err := exec.Command(docker, append(composeArgs, "ps", "-q", "workspace")...).Output()
	if err != nil || strings.TrimSpace(string(containerIDOutput)) == "" {
		t.Fatalf("locate live workspace container: %v: %s", err, containerIDOutput)
	}
	containerID := strings.TrimSpace(string(containerIDOutput))
	inspectOutput, err := exec.Command(docker, "inspect", containerID).Output()
	if err != nil {
		t.Fatalf("inspect live workspace container: %v", err)
	}
	var inspected []inspectedContainer
	if err := json.Unmarshal(inspectOutput, &inspected); err != nil || len(inspected) != 1 {
		t.Fatalf("decode live inspect: %v (containers=%d)", err, len(inspected))
	}
	assertLivePolicy(t, inspected[0], compiled)

	if output, err := exec.Command(docker, append(composeArgs, "down", "-v", "--remove-orphans")...).CombinedOutput(); err != nil {
		t.Fatalf("tear down validated fixture: %v: %s", err, output)
	}
	cleaned = true
	assertNoComposeProjectResources(t, docker, project)
}

type inspectedContainer struct {
	Config struct {
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	HostConfig struct {
		Privileged        bool             `json:"Privileged"`
		NetworkMode       string           `json:"NetworkMode"`
		PIDMode           string           `json:"PidMode"`
		IPCMode           string           `json:"IpcMode"`
		UserNSMode        string           `json:"UsernsMode"`
		CgroupNSMode      string           `json:"CgroupnsMode"`
		CapAdd            []string         `json:"CapAdd"`
		SecurityOpt       []string         `json:"SecurityOpt"`
		Devices           []map[string]any `json:"Devices"`
		DeviceCgroupRules []string         `json:"DeviceCgroupRules"`
		PortBindings      map[string]any   `json:"PortBindings"`
	} `json:"HostConfig"`
	Mounts []struct {
		Type        string `json:"Type"`
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
		RW          bool   `json:"RW"`
	} `json:"Mounts"`
}

func assertLivePolicy(t *testing.T, live inspectedContainer, compiled contract.Plan) {
	t.Helper()
	if live.HostConfig.Privileged || live.HostConfig.NetworkMode == "host" || live.HostConfig.PIDMode == "host" ||
		live.HostConfig.IPCMode == "host" || live.HostConfig.UserNSMode == "host" || live.HostConfig.CgroupNSMode == "host" {
		t.Fatalf("live container has forbidden namespace or privilege set: %+v", live.HostConfig)
	}
	if len(live.HostConfig.CapAdd) != 0 || len(live.HostConfig.Devices) != 0 || len(live.HostConfig.DeviceCgroupRules) != 0 || len(live.HostConfig.PortBindings) != 0 {
		t.Fatalf("live container has capability, device, or publication escalation: %+v", live.HostConfig)
	}
	if !slices.Equal(live.HostConfig.SecurityOpt, []string{"no-new-privileges=true"}) {
		t.Fatalf("live security options = %q, want only no-new-privileges=true", live.HostConfig.SecurityOpt)
	}
	expectedMounts := make(map[string]bool)
	for _, mount := range compiled.Mounts {
		if slices.Contains(mount.Services, "workspace") {
			expectedMounts[mount.Path] = mount.Writable
		}
	}
	if len(live.Mounts) != len(expectedMounts) {
		t.Fatalf("live mounts = %+v, want exactly %d Plan binds", live.Mounts, len(expectedMounts))
	}
	for _, mount := range live.Mounts {
		writable, ok := expectedMounts[mount.Source]
		if !ok || mount.Type != "bind" || mount.Destination != mount.Source || mount.RW != writable {
			t.Fatalf("live mount exceeds Plan: %+v (expected=%v)", mount, expectedMounts)
		}
	}
	for key, value := range map[string]string{
		"dev.abox.managed": "true", "dev.abox.workspace": compiled.WorkspaceID,
		"dev.abox.box": compiled.BoxID, "dev.abox.plan": string(compiled.PlanDigest),
		"dev.abox.generation": fmt.Sprint(generation),
	} {
		if live.Config.Labels[key] != value {
			t.Fatalf("live label %s = %q, want %q", key, live.Config.Labels[key], value)
		}
	}
}

func realCompose(t *testing.T) (string, string) {
	t.Helper()
	docker, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("docker is unavailable")
	}
	versionOutput, err := exec.Command(docker, "compose", "version", "--short").Output()
	if err != nil {
		t.Skipf("Docker Compose is unavailable: %v", err)
	}
	if err := exec.Command(docker, "info").Run(); err != nil {
		t.Skipf("Docker daemon is unavailable: %v", err)
	}
	return docker, strings.TrimSpace(string(versionOutput))
}

func assertNoComposeProjectResources(t *testing.T, docker, project string) {
	t.Helper()
	queries := [][]string{
		{"ps", "-aq", "--filter", "label=com.docker.compose.project=" + project},
		{"network", "ls", "-q", "--filter", "label=com.docker.compose.project=" + project},
		{"volume", "ls", "-q", "--filter", "label=com.docker.compose.project=" + project},
		{"image", "ls", "-q", "--filter", "label=com.docker.compose.project=" + project},
	}
	for _, query := range queries {
		output, err := exec.Command(docker, query...).CombinedOutput()
		if err != nil {
			t.Fatalf("query Docker resources with %q: %v: %s", query, err, output)
		}
		if strings.TrimSpace(string(output)) != "" {
			t.Fatalf("Docker resources remain for project %s via %q: %s", project, query, output)
		}
	}
}
