package removal

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/generatedstate"
	"github.com/shoe-point-0/agent-box/internal/ownership"
)

func TestGeneratedStateDependencyTransitionAllowsOnlyExactArtifactShrink(t *testing.T) {
	t.Parallel()
	first := generatedstate.Artifact{Path: "/boxes/box/context.json", Identity: ownership.FileIdentity{Device: 1, Inode: 2}, Kind: "context"}
	second := generatedstate.Artifact{Path: "/boxes/box/workspace", Identity: ownership.FileIdentity{Device: 1, Inode: 3}, Kind: "workspace-root"}
	expected := generatedTransitionFact(t, "1:1", []generatedstate.Artifact{first, second})
	step := &GeneratedStateStep{}

	if err := step.validateDependencyTransition(expected, generatedTransitionFact(t, "1:1", []generatedstate.Artifact{first})); err != nil {
		t.Fatalf("exact subset rejected: %v", err)
	}
	if err := step.validateDependencyTransition(expected, Fact{Present: false}); err != nil {
		t.Fatalf("terminal absence rejected: %v", err)
	}

	replacement := first
	replacement.Identity.Inode++
	for name, actual := range map[string]Fact{
		"replacement artifact": generatedTransitionFact(t, "1:1", []generatedstate.Artifact{replacement}),
		"changed root":         generatedTransitionFact(t, "1:9", []generatedstate.Artifact{first}),
		"new artifact": generatedTransitionFact(t, "1:1", []generatedstate.Artifact{first, {
			Path: "/boxes/box/foreign", Identity: ownership.FileIdentity{Device: 1, Inode: 4}, Kind: "foreign",
		}}),
	} {
		t.Run(name, func(t *testing.T) {
			if err := step.validateDependencyTransition(expected, actual); !errors.Is(err, ErrInventoryDrift) {
				t.Fatalf("transition error = %v, want inventory drift", err)
			}
		})
	}
}

func generatedTransitionFact(t *testing.T, rootIdentity string, artifacts []generatedstate.Artifact) Fact {
	t.Helper()
	encoded, err := json.Marshal(artifacts)
	if err != nil {
		t.Fatal(err)
	}
	return Fact{Present: true, Evidence: map[string]string{
		"boxId": "box", "root": "/boxes/box", "rootIdentity": rootIdentity,
		"planDigest": "sha256:plan", "artifacts": string(encoded), "retainedState": "plan.json,diagnostics",
	}, Losses: []Loss{}}
}
