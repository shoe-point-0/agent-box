package up

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shoe-point-0/agent-box/internal/state"
)

func TestReconcileGeneratedPlanResumesOnlyRetainedRoot(t *testing.T) {
	fixture := newPreparationFixture(t)
	attempt, err := PrepareAttempt(t.Context(), fixture.config(), fixture.operation)
	if err != nil {
		t.Fatal(err)
	}
	prepareGeneratedApplicationRoot(t, attempt)
	authority := &memoryGeneratedAuthority{}
	writes := 0
	failWrite := true
	config := GeneratedPlanConfig{
		Authority: authority, Now: monotonicNow(),
		WritePlan: func(path string, data []byte) error {
			writes++
			if failWrite {
				return errors.New("injected interruption after root persistence")
			}
			return writeImmutableFixture(path, data)
		},
	}
	if _, err := ReconcileGeneratedPlan(t.Context(), config, attempt); err == nil || !strings.Contains(err.Error(), "injected interruption") {
		t.Fatalf("first reconciliation error = %v", err)
	}
	if authority.value.BoxRoot == nil {
		t.Fatal("Box-root identity was not retained before generated-file mutation")
	}
	rootIdentity := authority.value.BoxRoot.Root.Identity

	failWrite = false
	attempt.Operation.ID = "operation-two"
	second, err := ReconcileGeneratedPlan(t.Context(), config, attempt)
	if err != nil {
		t.Fatal(err)
	}
	if writes != 2 || second.BoxRoot.Root.Identity != rootIdentity {
		t.Fatalf("resumed result = %#v writes=%d", second, writes)
	}
	attempt.Operation.ID = "operation-three"
	third, err := ReconcileGeneratedPlan(t.Context(), config, attempt)
	if err != nil {
		t.Fatal(err)
	}
	if writes != 2 || !reflect.DeepEqual(third, second) {
		t.Fatalf("warm generated reconciliation mutated: result=%#v writes=%d", third, writes)
	}
}

func TestReconcileGeneratedPlanRefusesUnclaimedAndReplacementRoots(t *testing.T) {
	t.Run("symlinked application root", func(t *testing.T) {
		fixture := newPreparationFixture(t)
		attempt, err := PrepareAttempt(t.Context(), fixture.config(), fixture.operation)
		if err != nil {
			t.Fatal(err)
		}
		actual := filepath.Join(t.TempDir(), "actual")
		if err := os.Mkdir(actual, 0o700); err != nil {
			t.Fatal(err)
		}
		linked := filepath.Join(t.TempDir(), "linked")
		if err := os.Symlink(actual, linked); err != nil {
			t.Fatal(err)
		}
		attempt.BoxRoot = filepath.Join(linked, "boxes", attempt.Plan.BoxID)
		attempt.PlanPath = filepath.Join(attempt.BoxRoot, "plan.json")
		_, err = ReconcileGeneratedPlan(t.Context(), GeneratedPlanConfig{Authority: &memoryGeneratedAuthority{}}, attempt)
		if err == nil || !strings.Contains(err.Error(), "not canonical") {
			t.Fatalf("symlinked root error = %v", err)
		}
		if _, statErr := os.Lstat(filepath.Join(actual, "boxes")); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("symlinked root was mutated: %v", statErr)
		}
	})

	t.Run("unclaimed deterministic root", func(t *testing.T) {
		fixture := newPreparationFixture(t)
		attempt, err := PrepareAttempt(t.Context(), fixture.config(), fixture.operation)
		if err != nil {
			t.Fatal(err)
		}
		prepareGeneratedApplicationRoot(t, attempt)
		if err := os.MkdirAll(attempt.BoxRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		marker := filepath.Join(attempt.BoxRoot, "foreign")
		if err := os.WriteFile(marker, []byte("preserve\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err = ReconcileGeneratedPlan(t.Context(), GeneratedPlanConfig{Authority: &memoryGeneratedAuthority{}}, attempt)
		if err == nil || !strings.Contains(err.Error(), "unclaimed Box root") {
			t.Fatalf("unclaimed root error = %v", err)
		}
		if contents, readErr := os.ReadFile(marker); readErr != nil || string(contents) != "preserve\n" {
			t.Fatalf("foreign root changed: %q, %v", contents, readErr)
		}
	})

	t.Run("replacement after retained proof", func(t *testing.T) {
		fixture := newPreparationFixture(t)
		attempt, err := PrepareAttempt(t.Context(), fixture.config(), fixture.operation)
		if err != nil {
			t.Fatal(err)
		}
		prepareGeneratedApplicationRoot(t, attempt)
		authority := &memoryGeneratedAuthority{}
		if _, err := ReconcileGeneratedPlan(t.Context(), GeneratedPlanConfig{Authority: authority}, attempt); err != nil {
			t.Fatal(err)
		}
		moved := attempt.BoxRoot + "-owned"
		if err := os.Rename(attempt.BoxRoot, moved); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(attempt.BoxRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		marker := filepath.Join(attempt.BoxRoot, "foreign")
		if err := os.WriteFile(marker, []byte("preserve\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		attempt.Operation.ID = "operation-two"
		if _, err := ReconcileGeneratedPlan(t.Context(), GeneratedPlanConfig{Authority: authority}, attempt); err == nil {
			t.Fatal("replacement root was adopted")
		}
		if contents, readErr := os.ReadFile(marker); readErr != nil || string(contents) != "preserve\n" {
			t.Fatalf("replacement root changed: %q, %v", contents, readErr)
		}
	})
}

type memoryGeneratedAuthority struct {
	value state.ReconciliationGeneratedAuthority
	found bool
}

func (s *memoryGeneratedAuthority) ReconciliationGeneratedAuthorityFor(_ context.Context, boxID string, generation int64) (state.ReconciliationGeneratedAuthority, bool, error) {
	if !s.found {
		return state.ReconciliationGeneratedAuthority{}, false, nil
	}
	if s.value.BoxID != boxID || s.value.Generation != generation {
		return state.ReconciliationGeneratedAuthority{}, false, errors.New("unexpected authority lookup")
	}
	return s.value, true, nil
}

func (s *memoryGeneratedAuthority) SaveReconciliationGeneratedAuthority(_ context.Context, authority state.ReconciliationGeneratedAuthority, _ time.Time) error {
	if s.found {
		existing, next := s.value, authority
		existing.OperationID, next.OperationID = "", ""
		existing.BoxRoot, next.BoxRoot = nil, nil
		if existing != next {
			return errors.New("authority identity drift")
		}
	}
	s.value, s.found = authority, true
	return nil
}

func writeImmutableFixture(path string, data []byte) error {
	return os.WriteFile(path, data, 0o400)
}

func prepareGeneratedApplicationRoot(t *testing.T, attempt PreparedAttempt) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Dir(attempt.BoxRoot)), 0o700); err != nil {
		t.Fatal(err)
	}
}
