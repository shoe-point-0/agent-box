package spike

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateComposeModelsRequiresPortsResetAndOwningContexts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repositories := map[string]string{
		"alpha": filepath.Join(root, "repo-alpha"),
		"beta":  filepath.Join(root, "repo-beta"),
	}
	baseline := composeModel{Services: map[string]composeModelService{
		"alpha": {Build: composeModelBuild{Context: repositories["alpha"]}, Ports: json.RawMessage(`[{"target":8080}]`)},
		"beta":  {Build: composeModelBuild{Context: repositories["beta"]}, Ports: json.RawMessage(`[{"target":8080}]`)},
	}}
	adapted := composeModel{Services: map[string]composeModelService{
		"alpha": {Build: composeModelBuild{Context: repositories["alpha"]}},
		"beta":  {Build: composeModelBuild{Context: repositories["beta"]}, Ports: json.RawMessage(`[]`)},
	}}
	baselinePorts, adaptedNoPorts, contexts, err := validateComposeModels(baseline, adapted, repositories)
	if err != nil {
		t.Fatalf("validateComposeModels() error = %v", err)
	}
	if !baselinePorts || !adaptedNoPorts || !contexts {
		t.Fatalf("validateComposeModels() = %t, %t, %t; want all true", baselinePorts, adaptedNoPorts, contexts)
	}

	service := adapted.Services["beta"]
	service.Ports = json.RawMessage(`[{"target":8080}]`)
	adapted.Services["beta"] = service
	_, adaptedNoPorts, _, err = validateComposeModels(baseline, adapted, repositories)
	if err != nil {
		t.Fatalf("validateComposeModels(with port) error = %v", err)
	}
	if adaptedNoPorts {
		t.Fatal("adaptedNoPorts = true with a remaining publication")
	}
}

func TestWriteComposeFixturesUsesRelativeIncludesAndLeavesRepositoriesStable(t *testing.T) {
	t.Parallel()

	tree := t.TempDir()
	rootFile, overrideFile, repositories, images, err := writeComposeFixtures(tree, "1234567890abcdef", "alpine@sha256:fixture")
	if err != nil {
		t.Fatalf("writeComposeFixtures() error = %v", err)
	}
	if len(images) != 2 {
		t.Fatalf("images = %v, want two", images)
	}
	root, err := os.ReadFile(rootFile)
	if err != nil {
		t.Fatalf("ReadFile(root) error = %v", err)
	}
	if string(root) != "include:\n  - path: repo-alpha/compose.yaml\n    project_directory: repo-alpha\n  - path: repo-beta/compose.yaml\n    project_directory: repo-beta\n" {
		t.Fatalf("root Compose = %q", root)
	}
	override, err := os.ReadFile(overrideFile)
	if err != nil {
		t.Fatalf("ReadFile(override) error = %v", err)
	}
	if !strings.Contains(string(override), "ports: !reset []") || !strings.Contains(string(override), composeOwnerLabel) || !strings.Contains(string(override), "1234567890abcdef") {
		t.Fatalf("override Compose missing policy fields: %s", override)
	}
	before, err := snapshotSelectedFiles(repositoryComposePaths(repositories))
	if err != nil {
		t.Fatalf("snapshotSelectedFiles(before) error = %v", err)
	}
	after, err := snapshotSelectedFiles(repositoryComposePaths(repositories))
	if err != nil {
		t.Fatalf("snapshotSelectedFiles(after) error = %v", err)
	}
	if !snapshotsEqual(before, after) {
		t.Fatal("fixture generation changed repository files after creation")
	}
}

func TestComposeTreeCleanupRequiresMarker(t *testing.T) {
	t.Parallel()

	workspace, err := NewWorkspace(t.TempDir(), ComposeProbe)
	if err != nil {
		t.Fatalf("NewWorkspace() error = %v", err)
	}
	environment := &Environment{root: workspace.Root(), workspace: workspace}
	tree, err := createComposeTree(environment)
	if err != nil {
		t.Fatalf("createComposeTree() error = %v", err)
	}
	entries, err := workspace.Entries()
	if err != nil {
		t.Fatalf("Entries() error = %v", err)
	}
	resource := entries[len(entries)-1].Resource
	if err := os.WriteFile(filepath.Join(tree, composeTreeMarker), []byte("wrong\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(marker) error = %v", err)
	}
	if err := cleanupComposeTree(workspace, resource); !errors.Is(err, ErrCleanupRefused) {
		t.Fatalf("cleanupComposeTree(wrong marker) error = %v, want ErrCleanupRefused", err)
	}
	if _, err := os.Stat(tree); err != nil {
		t.Fatalf("tree removed after marker refusal: %v", err)
	}
}

func TestCleanupReceivesPlannedScenarioResources(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	var got []Resource
	if err := registry.RegisterScenario("planned", Scenario{
		Run: func(context.Context, *Environment) error { return nil },
		Cleanup: func(_ context.Context, workspace *Workspace, resources []Resource) error {
			got = append(got, resources...)
			return workspace.MarkRemoved(resources[0])
		},
	}); err != nil {
		t.Fatalf("RegisterScenario() error = %v", err)
	}
	workspace, err := NewWorkspace(t.TempDir(), "planned")
	if err != nil {
		t.Fatalf("NewWorkspace() error = %v", err)
	}
	resource := Resource{Kind: "external", ID: "planned-id", OwnerID: workspace.metadata.ID}
	if _, err := workspace.inventory.Append("planned", resource); err != nil {
		t.Fatalf("Append(planned) error = %v", err)
	}
	if err := Cleanup(context.Background(), registry, workspace.Root()); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "planned-id" {
		t.Fatalf("cleanup resources = %+v, want planned-id", got)
	}
}
