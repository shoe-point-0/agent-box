package identity

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

func TestIdentityGolden(t *testing.T) {
	t.Parallel()

	workspaceID, err := Workspace("/Users/shoe/src/billing/.abox/box.yaml")
	if err != nil {
		t.Fatalf("Workspace() error = %v", err)
	}
	if got, want := workspaceID.String(), "ws_7veize66"; got != want {
		t.Fatalf("Workspace() = %q, want %q", got, want)
	}

	boxID, err := Box(workspaceID, "AUTH-142")
	if err != nil {
		t.Fatalf("Box() error = %v", err)
	}
	if got, want := boxID.String(), "auth-142-itjwb74g"; got != want {
		t.Fatalf("Box() = %q, want %q", got, want)
	}
}

func TestIdentityRepeatedRandomized(t *testing.T) {
	t.Parallel()

	random := rand.New(rand.NewSource(42)) //nolint:gosec // deterministic property-test input
	for iteration := range 1_000 {
		manifestPath := fmt.Sprintf("/tmp/workspaces/%016x/.abox/box.yaml", random.Uint64())
		taskKey := fmt.Sprintf("TASK-%016X", random.Uint64())

		firstWorkspace, err := Workspace(manifestPath)
		if err != nil {
			t.Fatalf("iteration %d: Workspace() error = %v", iteration, err)
		}
		firstBox, err := Box(firstWorkspace, taskKey)
		if err != nil {
			t.Fatalf("iteration %d: Box() error = %v", iteration, err)
		}

		for repeat := range 10 {
			workspaceID, repeatErr := Workspace(manifestPath)
			if repeatErr != nil {
				t.Fatalf("iteration %d repeat %d: Workspace() error = %v", iteration, repeat, repeatErr)
			}
			boxID, repeatErr := Box(workspaceID, taskKey)
			if repeatErr != nil {
				t.Fatalf("iteration %d repeat %d: Box() error = %v", iteration, repeat, repeatErr)
			}
			if workspaceID != firstWorkspace || boxID != firstBox {
				t.Fatalf("iteration %d repeat %d: identity changed from %q/%q to %q/%q", iteration, repeat, firstWorkspace, firstBox, workspaceID, boxID)
			}
		}
	}
}

func TestSameTaskAcrossWorkspacesDoesNotCollide(t *testing.T) {
	t.Parallel()

	seen := make(map[BoxID]string, 1_000)
	for index := range 1_000 {
		manifestPath := fmt.Sprintf("/tmp/unrelated/%04d/.abox/box.yaml", index)
		workspaceID, err := Workspace(manifestPath)
		if err != nil {
			t.Fatalf("Workspace(%q) error = %v", manifestPath, err)
		}
		boxID, err := Box(workspaceID, "SAME-TASK")
		if err != nil {
			t.Fatalf("Box(%q) error = %v", manifestPath, err)
		}
		if previous, exists := seen[boxID]; exists {
			t.Fatalf("Box ID %q collides for %q and %q", boxID, previous, manifestPath)
		}
		seen[boxID] = manifestPath
	}
}

func TestIdentityRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	for _, manifestPath := range []string{"", "relative/.abox/box.yaml", "/tmp/../tmp/.abox/box.yaml", "/tmp/bad\x00path"} {
		if _, err := Workspace(manifestPath); err == nil {
			t.Errorf("Workspace(%q) error = nil, want rejection", manifestPath)
		}
	}

	validWorkspace, err := Workspace("/tmp/workspace/.abox/box.yaml")
	if err != nil {
		t.Fatalf("Workspace(valid) error = %v", err)
	}
	invalidTaskKeys := []string{"", "-TASK", "TASK-", "../TASK", "TASK/ONE", "task key", "tásk", strings.Repeat("a", maxTaskKeyBytes+1)}
	for _, taskKey := range invalidTaskKeys {
		if _, boxErr := Box(validWorkspace, taskKey); boxErr == nil {
			t.Errorf("Box(%q) error = nil, want rejection", taskKey)
		}
	}

	if _, err := Box(WorkspaceID("ws_not-valid"), "TASK-1"); err == nil {
		t.Error("Box(invalid workspace ID) error = nil, want rejection")
	}
}

func TestBoxSlugIsBoundedAndHashUsesOriginalTaskKey(t *testing.T) {
	t.Parallel()

	workspaceID, err := Workspace("/tmp/workspace/.abox/box.yaml")
	if err != nil {
		t.Fatalf("Workspace() error = %v", err)
	}
	upper, err := Box(workspaceID, "FEATURE.ONE_TWO")
	if err != nil {
		t.Fatalf("Box(upper) error = %v", err)
	}
	lower, err := Box(workspaceID, "feature.one_two")
	if err != nil {
		t.Fatalf("Box(lower) error = %v", err)
	}
	if upper == lower {
		t.Fatalf("case-distinct task keys produced identical Box ID %q", upper)
	}
	if !strings.HasPrefix(upper.String(), "feature-one-two-") {
		t.Fatalf("Box() = %q, want normalized display slug", upper)
	}

	longTask := "A" + strings.Repeat("b", maxTaskKeyBytes-1)
	boxID, err := Box(workspaceID, longTask)
	if err != nil {
		t.Fatalf("Box(long task) error = %v", err)
	}
	if err := ValidateBoxID(boxID); err != nil {
		t.Fatalf("ValidateBoxID(%q) error = %v", boxID, err)
	}
	if got, want := len(strings.Split(boxID.String(), "-")[0]), maxSlugBytes; got != want {
		t.Fatalf("slug length = %d, want %d", got, want)
	}
}
