package filesystem_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/filesystem"
	"github.com/shoe-point-0/agent-box/internal/lima"
	aboxprocess "github.com/shoe-point-0/agent-box/internal/process"
)

func TestProbeVerifierAgainstRealComposeWorkspaceContainer(t *testing.T) {
	docker := realProbeDocker(t)
	if err := exec.Command(docker, "image", "inspect", "alpine:3.20").Run(); err != nil {
		t.Skipf("cached filesystem fixture image alpine:3.20 is unavailable: %v", err)
	}

	root := t.TempDir()
	worktree := filepath.Join(root, "workspace")
	if err := os.Mkdir(worktree, 0o700); err != nil {
		t.Fatal(err)
	}
	project := "abox-fs-probe-" + randomProbeSuffix(t)
	composeFile := filepath.Join(root, "compose.yaml")
	fixture := fmt.Sprintf(`services:
  workspace:
    image: alpine:3.20
    user: %q
    command: ["sleep", "infinity"]
    volumes:
      - %q
`, fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), worktree+":"+worktree)
	if err := os.WriteFile(composeFile, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	composeArgs := []string{"compose", "--project-name", project, "-f", composeFile}
	cleaned := false
	t.Cleanup(func() {
		if !cleaned {
			_ = exec.Command(docker, append(composeArgs, "down", "--remove-orphans")...).Run()
		}
	})
	if output, err := exec.Command(docker, append(composeArgs, "up", "--detach", "--pull", "never")...).CombinedOutput(); err != nil {
		t.Fatalf("start Compose filesystem fixture: %v: %s", err, output)
	}
	idOutput, err := exec.Command(docker, append(composeArgs, "ps", "--quiet", "workspace")...).Output()
	containerID := strings.TrimSpace(string(idOutput))
	if err != nil || containerID == "" {
		t.Fatalf("locate immutable workspace container: %v: %s", err, idOutput)
	}

	host := localWorkspaceHost{runner: aboxprocess.Runner{CaptureLimit: 1024}}
	adapter, err := filesystem.NewWorkspaceAdapter(host, compose.RuntimeSpec{DockerPath: docker, Workdir: root})
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := filesystem.NewProbeVerifier(adapter)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifier.Verify(t.Context(), probePlan([]string{worktree}, true), workspaceObservation(containerID)); err != nil {
		t.Fatalf("Verify(real Compose workspace) error = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(worktree, ".abox-probe")); !os.IsNotExist(err) {
		t.Fatalf("probe artifacts remain after real container execution: %v", err)
	}

	if output, err := exec.Command(docker, append(composeArgs, "down", "--remove-orphans")...).CombinedOutput(); err != nil {
		t.Fatalf("tear down Compose filesystem fixture: %v: %s", err, output)
	}
	cleaned = true
	remaining, err := exec.Command(docker, "ps", "-aq", "--filter", "label=com.docker.compose.project="+project).Output()
	if err != nil || strings.TrimSpace(string(remaining)) != "" {
		t.Fatalf("Compose filesystem fixture leaked containers: %v: %s", err, remaining)
	}
}

type localWorkspaceHost struct {
	runner aboxprocess.Runner
}

func (h localWorkspaceHost) Capture(ctx context.Context, _ lima.Spec, workdir string, argv []string) (lima.Capture, error) {
	result, err := h.runner.Run(ctx, aboxprocess.Command{
		Argv: argv, Dir: workdir, Env: filesystemProcessEnvironment(),
	})
	return lima.Capture{
		Stdout: append([]byte(nil), result.Stdout...), Stderr: append([]byte(nil), result.Stderr...),
		StdoutTruncated: result.StdoutTruncated, StderrTruncated: result.StderrTruncated,
	}, err
}

func realProbeDocker(t *testing.T) string {
	t.Helper()
	docker, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("docker is unavailable")
	}
	if err := exec.Command(docker, "compose", "version", "--short").Run(); err != nil {
		t.Skipf("Docker Compose is unavailable: %v", err)
	}
	if err := exec.Command(docker, "info").Run(); err != nil {
		t.Skipf("Docker daemon is unavailable: %v", err)
	}
	return docker
}

func randomProbeSuffix(t *testing.T) string {
	t.Helper()
	data := make([]byte, 6)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(data)
}

func filesystemProcessEnvironment() map[string]string {
	environment := make(map[string]string)
	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if ok {
			environment[name] = value
		}
	}
	return environment
}
