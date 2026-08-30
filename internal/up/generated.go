package up

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shoe-point-0/agent-box/internal/atomicfile"
	"github.com/shoe-point-0/agent-box/internal/generatedstate"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/state"
)

// GeneratedAuthorityStore retains the exact Box-root identity established
// before generated files or external runtime resources are created.
type GeneratedAuthorityStore interface {
	ReconciliationGeneratedAuthorityFor(context.Context, string, int64) (state.ReconciliationGeneratedAuthority, bool, error)
	SaveReconciliationGeneratedAuthority(context.Context, state.ReconciliationGeneratedAuthority, time.Time) error
}

var _ GeneratedAuthorityStore = (*state.Store)(nil)

// GeneratedPlanConfig binds generated-state recovery authority to filesystem
// mutation. Functions are injectable only to exercise interruption boundaries.
type GeneratedPlanConfig struct {
	Authority GeneratedAuthorityStore
	MakeRoot  func(string) error
	WritePlan func(string, []byte) error
	Now       func() time.Time
}

// GeneratedPlanResult is the freshly proven generated-state authority used by
// later runtime reconciliation and final READY publication.
type GeneratedPlanResult struct {
	BoxRoot generatedstate.Record
}

// ReconcileGeneratedPlan claims one generation before mutation, creates or
// freshly proves the Box root, and materializes the exact immutable plan.json.
// An unclaimed deterministic root is refused rather than adopted.
func ReconcileGeneratedPlan(ctx context.Context, config GeneratedPlanConfig, attempt PreparedAttempt) (GeneratedPlanResult, error) {
	if config.Authority == nil {
		return GeneratedPlanResult{}, errors.New("reconcile generated Plan: authority store is required")
	}
	if attempt.Operation.ID == "" || attempt.Operation.BoxID == "" || attempt.Plan.BoxID != attempt.Operation.BoxID ||
		attempt.Generation < 1 || !cleanAbsoluteGeneratedPath(attempt.BoxRoot) || !cleanAbsoluteGeneratedPath(attempt.PlanPath) ||
		filepath.Dir(attempt.PlanPath) != attempt.BoxRoot ||
		len(attempt.CanonicalPlan) == 0 {
		return GeneratedPlanResult{}, errors.New("reconcile generated Plan: prepared attempt identity is incomplete")
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	makeRoot := config.MakeRoot
	if makeRoot == nil {
		makeRoot = makeGeneratedRoot
	}
	writePlan := config.WritePlan
	if writePlan == nil {
		writePlan = func(path string, data []byte) error { return atomicfile.WriteFile(path, data, 0o400) }
	}

	authority, found, err := config.Authority.ReconciliationGeneratedAuthorityFor(ctx, attempt.Plan.BoxID, attempt.Generation)
	if err != nil {
		return GeneratedPlanResult{}, fmt.Errorf("reconcile generated Plan: load recovery authority: %w", err)
	}
	desired := state.ReconciliationGeneratedAuthority{
		BoxID: attempt.Plan.BoxID, Generation: attempt.Generation, PlanDigest: attempt.Plan.PlanDigest, OperationID: attempt.Operation.ID,
	}
	if !found {
		authority = desired
		if attempt.Warm {
			if attempt.Current.BoxRoot.BoxID != attempt.Plan.BoxID || attempt.Current.BoxRoot.Root.CanonicalPath != attempt.BoxRoot {
				return GeneratedPlanResult{}, errors.New("reconcile generated Plan: warm Box-root authority is incomplete")
			}
			root := attempt.Current.BoxRoot
			authority.BoxRoot = &root
		}
	} else {
		observedBase := authority
		observedBase.OperationID, observedBase.BoxRoot = "", nil
		desired.OperationID = ""
		if observedBase != desired {
			return GeneratedPlanResult{}, errors.New("reconcile generated Plan: recovery authority differs from prepared attempt")
		}
		authority.OperationID = attempt.Operation.ID
	}
	if err := config.Authority.SaveReconciliationGeneratedAuthority(ctx, authority, now().UTC()); err != nil {
		return GeneratedPlanResult{}, fmt.Errorf("reconcile generated Plan: persist intent: %w", err)
	}

	if authority.BoxRoot == nil {
		if _, err := os.Lstat(attempt.BoxRoot); err == nil {
			return GeneratedPlanResult{}, errors.New("reconcile generated Plan: unclaimed Box root already exists")
		} else if !errors.Is(err, os.ErrNotExist) {
			return GeneratedPlanResult{}, fmt.Errorf("reconcile generated Plan: inspect Box root: %w", err)
		}
		if err := makeRoot(attempt.BoxRoot); err != nil {
			return GeneratedPlanResult{}, fmt.Errorf("reconcile generated Plan: create Box root: %w", err)
		}
		identity, err := ownership.InspectPath(attempt.BoxRoot)
		if err != nil {
			return GeneratedPlanResult{}, fmt.Errorf("reconcile generated Plan: inspect created Box root: %w", err)
		}
		root := generatedstate.Record{BoxID: attempt.Plan.BoxID, Root: ownership.PathRecord{CanonicalPath: attempt.BoxRoot, Identity: identity}}
		authority.BoxRoot = &root
		if err := config.Authority.SaveReconciliationGeneratedAuthority(ctx, authority, now().UTC()); err != nil {
			return GeneratedPlanResult{}, fmt.Errorf("reconcile generated Plan: retain Box-root identity: %w", err)
		}
	}
	if _, err := ownership.ProvePath(filepath.Dir(attempt.BoxRoot), authority.BoxRoot.Root); err != nil {
		return GeneratedPlanResult{}, fmt.Errorf("reconcile generated Plan: prove Box-root identity: %w", err)
	}
	if err := reconcilePlanFile(attempt.PlanPath, attempt.CanonicalPlan, writePlan); err != nil {
		return GeneratedPlanResult{}, err
	}
	return GeneratedPlanResult{BoxRoot: *authority.BoxRoot}, nil
}

func makeGeneratedRoot(path string) error {
	boxesRoot := filepath.Dir(path)
	applicationRoot := filepath.Dir(boxesRoot)
	resolved, err := filepath.EvalSymlinks(applicationRoot)
	if err != nil || resolved != applicationRoot {
		return errors.New("application root is absent or not canonical")
	}
	info, err := os.Lstat(boxesRoot)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(boxesRoot, 0o700); err != nil {
			return err
		}
	} else if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("boxes root is not a real directory")
	}
	resolved, err = filepath.EvalSymlinks(boxesRoot)
	if err != nil || resolved != boxesRoot {
		return errors.New("boxes root is not canonical")
	}
	return os.Mkdir(path, 0o700)
}

func cleanAbsoluteGeneratedPath(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path && !strings.ContainsRune(path, '\x00')
}

func reconcilePlanFile(path string, expected []byte, write func(string, []byte) error) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := write(path, expected); err != nil {
			return fmt.Errorf("reconcile generated Plan: write immutable Plan: %w", err)
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return fmt.Errorf("reconcile generated Plan: inspect immutable Plan: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o400 {
		return errors.New("reconcile generated Plan: immutable Plan has unsafe type or mode")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reconcile generated Plan: read immutable Plan: %w", err)
	}
	if !bytes.Equal(contents, expected) {
		return errors.New("reconcile generated Plan: immutable Plan content differs")
	}
	return nil
}
