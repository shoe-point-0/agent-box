package boxplan_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/shoe-point-0/agent-box/internal/boxplan"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/machinegit"
	"github.com/shoe-point-0/agent-box/internal/paths"
	"github.com/shoe-point-0/agent-box/internal/plan"
	"github.com/shoe-point-0/agent-box/internal/process"
	"github.com/shoe-point-0/agent-box/internal/state"
)

const manifestFixture = `apiVersion: abox.dev/v1alpha1
kind: Workspace
metadata:
  name: workspace
spec:
  repositories:
    - name: repository
      source:
        local: ..
      checkout:
        ref: HEAD
        branch: "abox/{{ box.task }}/repository"
      workspacePath: workspace
  host:
    cpus: 2
    memory: 4GiB
    disk: 20GiB
  compose:
    includes:
      - repository: repository
        path: compose.yaml
    profiles: [development]
  workspaceService:
    name: app
    repository: repository
    workingDirectory: workspace
  mounts:
    - repository: repository
      services: [app]
      mode: read-write
  policy:
    gitMetadata: read-write
    dockerSocket: deny
    machineMounts: declared-only
    networkEgress: allow
`

type absentState struct{}

func (absentState) CurrentPlan(context.Context, string) (state.CurrentPlanRecord, bool, error) {
	return state.CurrentPlanRecord{}, false, nil
}

type retainedState struct{ path string }

func (s retainedState) CurrentPlan(context.Context, string) (state.CurrentPlanRecord, bool, error) {
	return state.CurrentPlanRecord{Path: s.path}, true, nil
}

func TestCompileAbsentBoxIsDeterministicAndCreatesNothing(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux implementation-lane evidence")
	}
	root := createRepository(t)
	manifestPath := filepath.Join(root, ".abox", "box.yaml")
	workspaceID, err := identity.Workspace(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	boxID, err := identity.Box(workspaceID, "TASK-31")
	if err != nil {
		t.Fatal(err)
	}
	stateParent := t.TempDir()
	applicationRoot := filepath.Join(stateParent, "application")
	provider, err := paths.New(applicationRoot, filepath.Join(stateParent, "cache"), filepath.Join(stateParent, "logs"))
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(t.TempDir(), "git-home")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	service := boxplan.Service{
		State: absentState{}, Paths: provider, Runtime: plan.RuntimeFacts{Driver: "lima-qemu"},
		Tools: contract.ToolVersions{AgentBox: "test", Git: "2.43.0", Lima: "test", Docker: "test", Compose: "test"},
		Git:   preparedMachineGit(t, home),
	}
	request := boxplan.Request{WorkspaceID: workspaceID, BoxID: boxID, TaskKey: "TASK-31", ManifestPath: manifestPath}
	before := snapshot(t, root)
	first, err := service.Compile(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Compile(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated plan changed:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if first.SchemaVersion != contract.PlanOutputSchemaVersion || first.Classification != "in-place" || first.Reason != "box-absent" {
		t.Fatalf("absent output = %#v", first)
	}
	if len(first.Operations) != 4 || first.Operations[0].Resource != "plan" || first.Operations[3].Resource != "compose" {
		t.Fatalf("absent operations = %#v", first.Operations)
	}
	if after := snapshot(t, root); !bytes.Equal(before, after) {
		t.Fatal("planning changed the source repository filesystem")
	}
	if _, err := os.Lstat(applicationRoot); !os.IsNotExist(err) {
		t.Fatalf("planning materialized application state: %v", err)
	}
	validateSchema(t, first)
}

func TestCompileExistingBoxExposesRestartAndDestructiveConflicts(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux implementation-lane evidence")
	}
	root := createRepository(t)
	manifestPath := filepath.Join(root, ".abox", "box.yaml")
	workspaceID, err := identity.Workspace(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	boxID, err := identity.Box(workspaceID, "TASK-31")
	if err != nil {
		t.Fatal(err)
	}
	stateParent := t.TempDir()
	provider, err := paths.New(filepath.Join(stateParent, "application"), filepath.Join(stateParent, "cache"), filepath.Join(stateParent, "logs"))
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(t.TempDir(), "git-home")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	service := boxplan.Service{
		State: absentState{}, Paths: provider, Runtime: plan.RuntimeFacts{Driver: "lima-qemu"},
		Tools: contract.ToolVersions{AgentBox: "test", Git: "2.43.0", Lima: "test", Docker: "test", Compose: "test"},
		Git:   preparedMachineGit(t, home),
	}
	request := boxplan.Request{WorkspaceID: workspaceID, BoxID: boxID, TaskKey: "TASK-31", ManifestPath: manifestPath}
	initial, err := service.Compile(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	worktree := initial.Plan.Repositories[0].WorktreePath
	if err := os.MkdirAll(filepath.Dir(worktree), 0o700); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "clone", "--no-local", "--", root, worktree)
	planPath, err := provider.Plan(boxID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(planPath), 0o700); err != nil {
		t.Fatal(err)
	}
	encoded, err := plan.CanonicalJSON(initial.Plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(planPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	service.State = retainedState{path: planPath}

	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	restarted := strings.Replace(string(manifestBytes), "cpus: 2", "cpus: 3", 1)
	if err := os.WriteFile(manifestPath, []byte(restarted), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", ".abox/box.yaml")
	runGit(t, root, "commit", "-m", "increase cpu")
	restartOutput, err := service.Compile(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if restartOutput.Classification != "restart" || !hasImplication(restartOutput.Operations, "runtime-host", true, false, false) {
		t.Fatalf("restart output = %#v", restartOutput)
	}

	shrunk := strings.Replace(restarted, "disk: 20GiB", "disk: 10GiB", 1)
	if err := os.WriteFile(manifestPath, []byte(shrunk), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", ".abox/box.yaml")
	runGit(t, root, "commit", "-m", "shrink disk")
	if err := os.WriteFile(filepath.Join(worktree, "dirty.txt"), []byte("preserve me\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	conflictOutput, err := service.Compile(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if conflictOutput.Classification != "conflict" ||
		!hasImplication(conflictOutput.Operations, "runtime-host", false, true, true) ||
		!hasImplication(conflictOutput.Operations, "repository/repository", false, true, true) {
		t.Fatalf("conflict output = %#v", conflictOutput)
	}
	validateSchema(t, conflictOutput)
}

func hasImplication(operations []contract.PlanOperation, resource string, restart, destructive, conflict bool) bool {
	for _, operation := range operations {
		if operation.Resource == resource && operation.Restart == restart && operation.Destructive == destructive && operation.Conflict == conflict {
			return true
		}
	}
	return false
}

func validateSchema(t *testing.T, output contract.PlanOutput) {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	schema, err := compiler.Compile(filepath.Join("..", "..", "schemas", "v1alpha1", "plan-output.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("plan output does not validate: %v", err)
	}
}

func createRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".abox"), 0o700); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		filepath.Join(root, ".abox", "box.yaml"): manifestFixture,
		filepath.Join(root, "compose.yaml"):      "services:\n  app:\n    image: alpine:3.20\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, root, "init", "--initial-branch=main")
	runGit(t, root, "config", "user.name", "Agent Box Test")
	runGit(t, root, "config", "user.email", "agent-box@example.invalid")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "fixture")
	return root
}

func runGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command(realGit(t), args...)
	command.Dir = directory
	command.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir(), "LANG=C", "LC_ALL=C"}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func preparedMachineGit(t *testing.T, home string) machinegit.Client {
	t.Helper()
	client := machinegit.Client{
		GitPath: realGit(t), Home: home, Path: "/usr/bin:/bin", ExcludesFile: filepath.Join(home, "git-excludes"),
		Executor: process.Runner{},
	}
	if err := client.Prepare(); err != nil {
		t.Fatal(err)
	}
	return client
}

func realGit(t *testing.T) string {
	t.Helper()
	for _, candidate := range []string{"/usr/bin/git", "/bin/git"} {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate
		}
	}
	t.Skip("real Git not installed")
	return ""
}

func snapshot(t *testing.T, root string) []byte {
	t.Helper()
	var output bytes.Buffer
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		_, _ = output.WriteString(relative + "\x00" + info.Mode().String() + "\x00")
		if info.Mode().IsRegular() {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			_, _ = output.Write(content)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
