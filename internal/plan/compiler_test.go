package plan_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/manifest"
	"github.com/shoe-point-0/agent-box/internal/paths"
	"github.com/shoe-point-0/agent-box/internal/plan"
	"github.com/shoe-point-0/agent-box/internal/repository"
)

func TestCompileCanonicalGoldenIsStable(t *testing.T) {
	input, root := fixtureInput(t)
	var first []byte
	for iteration := range 100 {
		random := rand.New(rand.NewPCG(uint64(iteration+1), uint64(iteration+101)))
		random.Shuffle(len(input.Workspace.Spec.Compose.Profiles), func(left, right int) {
			input.Workspace.Spec.Compose.Profiles[left], input.Workspace.Spec.Compose.Profiles[right] =
				input.Workspace.Spec.Compose.Profiles[right], input.Workspace.Spec.Compose.Profiles[left]
		})
		services := input.Workspace.Spec.Mounts[0].Services
		random.Shuffle(len(services), func(left, right int) {
			services[left], services[right] = services[right], services[left]
		})

		compiled, encoded, err := plan.Compile(input)
		if err != nil {
			t.Fatalf("iteration %d: Compile() error = %v", iteration, err)
		}
		if iteration == 0 {
			first = append([]byte(nil), encoded...)
			assertGolden(t, normalizePlan(t, encoded, root, compiled))
			assertPlanSchema(t, encoded)
			assertOwnershipProjection(t, compiled)
			continue
		}
		if !bytes.Equal(encoded, first) {
			t.Fatalf("iteration %d changed canonical Plan\nfirst: %s\n got: %s", iteration, first, encoded)
		}
	}
}

func TestCompileHasNoSecretValueInputChannel(t *testing.T) {
	const canary = "plan-secret-canary-do-not-persist"
	t.Setenv("OPENAI_API_KEY", canary)
	input, _ := fixtureInput(t)
	_, encoded, err := plan.Compile(input)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(canary)) || bytes.Contains(encoded, []byte("OPENAI_API_KEY")) {
		t.Fatalf("canonical Plan contains secret material: %s", encoded)
	}

	input.Workspace.Spec.Secrets = []manifest.Secret{{
		Name: "OPENAI_API_KEY", FromHostEnvironment: "OPENAI_API_KEY",
		ExposeTo: []manifest.SecretTarget{{Service: "workspace", As: "environment"}},
	}}
	input.ManifestPath = filepath.Join(t.TempDir(), "missing.yaml")
	_, _, err = plan.Compile(input)
	var unsupported *manifest.FeatureNotSupportedError
	if !errors.As(err, &unsupported) || unsupported.Path != "spec.secrets" {
		t.Fatalf("Compile(deferred secret) error = %v, want field-aware feature refusal before file access", err)
	}
}

func TestCanonicalJSONRejectsMutatedPlan(t *testing.T) {
	input, _ := fixtureInput(t)
	compiled, _, err := plan.Compile(input)
	if err != nil {
		t.Fatal(err)
	}
	compiled.RuntimeHost.MemoryBytes++
	if _, err := plan.CanonicalJSON(compiled); err == nil {
		t.Fatal("CanonicalJSON(mutated Plan) succeeded")
	}
}

func TestCompileRejectsSymlinkedComposeInput(t *testing.T) {
	input, _ := fixtureInput(t)
	source := t.TempDir()
	target := filepath.Join(source, "target.yaml")
	if err := os.WriteFile(target, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(source, "compose.dev.yaml")); err != nil {
		t.Fatal(err)
	}
	input.Workspace.Spec.Repositories[0].CanonicalLocal = source
	input.Repositories[0].SourceRoot = source
	_, _, err := plan.Compile(input)
	if err == nil || !strings.Contains(err.Error(), "must not traverse symlinks") {
		t.Fatalf("Compile(symlinked include) error = %v", err)
	}
}

func TestCompileRejectsUnsupportedCardinalityBeforeArtifacts(t *testing.T) {
	input, _ := fixtureInput(t)
	input.Workspace.Spec.Repositories = append(input.Workspace.Spec.Repositories, input.Workspace.Spec.Repositories[0])
	input.ManifestPath = filepath.Join(t.TempDir(), "missing.yaml")
	_, _, err := plan.Compile(input)
	var unsupported *manifest.FeatureNotSupportedError
	if !errors.As(err, &unsupported) || unsupported.Path != "spec.repositories" {
		t.Fatalf("Compile(multiple repositories) error = %v, want feature refusal", err)
	}
}

func fixtureInput(t *testing.T) (plan.Input, string) {
	t.Helper()
	root, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, "workspace", ".abox", "box.yaml")
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
	boxID, err := identity.Box(workspaceID, "TASK-16")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := paths.New("/state", "/cache", "/logs")
	if err != nil {
		t.Fatal(err)
	}
	worktreePath, err := provider.WorkspacePath(boxID, workspace.Spec.Repositories[0].WorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	gitStorePath, err := provider.GitStore(boxID, "repo")
	if err != nil {
		t.Fatal(err)
	}
	facts := repository.Facts{
		Name: "repo", SourceRoot: workspace.Spec.Repositories[0].CanonicalLocal, Clean: true,
		BaseCommit: "0123456789abcdef0123456789abcdef01234567", Branch: "abox/TASK-16/repo",
		WorktreePath: worktreePath, GitStorePath: gitStorePath,
		PublishDestination: repository.Destination{Kind: repository.DestinationLocal, URL: workspace.Spec.Repositories[0].CanonicalLocal},
	}
	return plan.Input{
		Workspace: workspace, WorkspaceID: workspaceID, BoxID: boxID, TaskKey: "TASK-16",
		ManifestPath: manifestPath, Repositories: []repository.Facts{facts},
		Runtime: plan.RuntimeFacts{Driver: "lima-qemu"},
		Tools: contract.ToolVersions{
			AgentBox: "0.1.0-dev", Git: "2.51.0", Lima: "2.0.0", Docker: "28.3.3", Compose: "2.39.1",
		},
	}, root
}

func normalizePlan(t *testing.T, encoded []byte, root string, compiled contract.Plan) []byte {
	t.Helper()
	replacements := []struct{ old, new string }{
		{root, "$ROOT"},
		{compiled.BoxID, "$BOX_ID"},
		{compiled.WorkspaceID, "$WORKSPACE_ID"},
		{string(compiled.PlanDigest), "$PLAN_DIGEST"},
	}
	result := string(encoded)
	for _, replacement := range replacements {
		result = strings.ReplaceAll(result, replacement.old, replacement.new)
	}
	return []byte(result)
}

func assertGolden(t *testing.T, got []byte) {
	t.Helper()
	want, err := os.ReadFile(filepath.Join("testdata", "plan.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var gotDocument, wantDocument any
	if err := json.Unmarshal(got, &gotDocument); err != nil {
		t.Fatalf("decode compiled Plan: %v", err)
	}
	if err := json.Unmarshal(want, &wantDocument); err != nil {
		t.Fatalf("decode golden Plan: %v", err)
	}
	if !reflect.DeepEqual(gotDocument, wantDocument) {
		gotPretty, _ := json.MarshalIndent(gotDocument, "", "  ")
		t.Fatalf("compiled Plan differs from golden\nwant: %s\n got: %s", want, gotPretty)
	}
}

func assertPlanSchema(t *testing.T, encoded []byte) {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	schema, err := compiler.Compile(filepath.Join("..", "..", "schemas", "v1alpha1", "plan.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("compiled Plan does not satisfy schema: %v", err)
	}
}

func assertOwnershipProjection(t *testing.T, compiled contract.Plan) {
	t.Helper()
	if len(compiled.Repositories) != 1 || len(compiled.Compose.Includes) != 1 || len(compiled.Mounts) != 2 {
		t.Fatalf("incomplete ownership projection: %+v", compiled)
	}
	repositoryPlan := compiled.Repositories[0]
	if repositoryPlan.SourcePath == "" || repositoryPlan.BaseCommit == "" || repositoryPlan.Branch == "" ||
		repositoryPlan.WorktreePath == "" || repositoryPlan.GitCommonDir == "" {
		t.Fatalf("repository ownership keys are incomplete: %+v", repositoryPlan)
	}
	paths := []string{compiled.Manifest.Path, compiled.Compose.Includes[0].Path, compiled.Mounts[0].Path, compiled.Mounts[1].Path}
	if slices.Contains(paths, "") {
		t.Fatalf("artifact/mount observation key is empty: %q", paths)
	}
	if compiled.RuntimeHost.InstanceName != compiled.Compose.ProjectName {
		t.Fatalf("runtime/Compose ownership names diverged: %+v / %+v", compiled.RuntimeHost, compiled.Compose)
	}
}
