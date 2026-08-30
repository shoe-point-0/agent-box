package up

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/shoe-point-0/agent-box/internal/lima"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/state"
	"github.com/shoe-point-0/agent-box/internal/toolchain"
)

// LimaAuthorityStore retains runtime-host intent and each recovered proof before
// a later operation can depend on it.
type LimaAuthorityStore interface {
	ReconciliationLimaAuthorityFor(context.Context, string, int64) (state.ReconciliationLimaAuthority, bool, error)
	SaveReconciliationLimaAuthority(context.Context, state.ReconciliationLimaAuthority, time.Time) error
}

var _ LimaAuthorityStore = (*state.Store)(nil)

// LimaRuntime is the observed create/start boundary used by up reconciliation.
// The concrete adapter revalidates its verified executable before every call.
type LimaRuntime interface {
	Inspect(context.Context, lima.Spec) (lima.Observation, error)
	Create(context.Context, lima.Spec, string, ownership.PathSetRecord) (lima.Observation, error)
	Start(context.Context, lima.Spec) (lima.Observation, error)
}

var _ LimaRuntime = (*lima.Adapter)(nil)

// LimaConfig binds immutable runtime intent to the concrete Lima lifecycle.
// WriteConfig is injectable only for interruption testing.
type LimaConfig struct {
	Authority   LimaAuthorityStore
	Runtime     LimaRuntime
	Artifact    toolchain.Identity
	Spec        lima.Spec
	WriteConfig func(string, lima.Spec) error
	Now         func() time.Time
}

// LimaResult contains freshly observed runtime-host authority for readiness and
// final READY publication.
type LimaResult struct {
	Artifact       toolchain.Identity
	Spec           lima.Spec
	ConfigEvidence ownership.PathSetRecord
	Instance       ownership.LimaInstanceRecord
	Observation    lima.Observation
}

// ReconcileLima materializes an exact generated config, creates or re-observes
// the matching instance, and starts it. Intent and each proof are retained in
// monotonic order so restart never adopts a name or deterministic path alone.
func ReconcileLima(ctx context.Context, config LimaConfig, attempt PreparedAttempt, generated GeneratedPlanResult) (LimaResult, error) {
	if config.Authority == nil || config.Runtime == nil {
		return LimaResult{}, errors.New("reconcile Lima: authority store and runtime are required")
	}
	if err := validateLimaAttempt(config, attempt, generated); err != nil {
		return LimaResult{}, err
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	writeConfig := config.WriteConfig
	if writeConfig == nil {
		writeConfig = lima.WriteConfig
	}

	authority, found, err := config.Authority.ReconciliationLimaAuthorityFor(ctx, attempt.Plan.BoxID, attempt.Generation)
	if err != nil {
		return LimaResult{}, fmt.Errorf("reconcile Lima: load recovery authority: %w", err)
	}
	desired := state.ReconciliationLimaAuthority{
		BoxID: attempt.Plan.BoxID, Generation: attempt.Generation, PlanDigest: attempt.Plan.PlanDigest,
		OperationID: attempt.Operation.ID, LimaArtifact: config.Artifact, Spec: config.Spec, ConfigPath: attempt.LimaConfigPath,
	}
	if !found {
		authority = desired
		if attempt.Warm && (attempt.Current.LimaArtifact != config.Artifact || !reflect.DeepEqual(attempt.Current.LimaSpec, config.Spec)) {
			return LimaResult{}, errors.New("reconcile Lima: warm runtime-host authority differs from prepared attempt")
		}
	} else {
		observedBase := authority
		observedBase.OperationID, observedBase.ConfigEvidence, observedBase.Instance = "", nil, nil
		desired.OperationID = ""
		if !reflect.DeepEqual(observedBase, desired) {
			return LimaResult{}, errors.New("reconcile Lima: recovery authority differs from prepared attempt")
		}
		authority.OperationID = attempt.Operation.ID
	}
	if err := config.Authority.SaveReconciliationLimaAuthority(ctx, authority, now().UTC()); err != nil {
		return LimaResult{}, fmt.Errorf("reconcile Lima: persist intent: %w", err)
	}

	if authority.ConfigEvidence == nil {
		evidence, err := reconcileLimaConfig(attempt.LimaConfigPath, config.Spec, writeConfig)
		if err != nil {
			return LimaResult{}, err
		}
		authority.ConfigEvidence = &evidence
		if err := config.Authority.SaveReconciliationLimaAuthority(ctx, authority, now().UTC()); err != nil {
			return LimaResult{}, fmt.Errorf("reconcile Lima: retain config identity: %w", err)
		}
	} else if err := verifyLimaConfig(attempt.LimaConfigPath, config.Spec, *authority.ConfigEvidence); err != nil {
		return LimaResult{}, err
	}

	var observation lima.Observation
	if authority.Instance == nil {
		observation, err = config.Runtime.Create(ctx, config.Spec, attempt.LimaConfigPath, *authority.ConfigEvidence)
		if err != nil {
			return LimaResult{}, fmt.Errorf("reconcile Lima: create runtime host: %w", err)
		}
		instance := instanceRecord(observation)
		if _, err := ownership.ProveLimaInstance(instance, instanceObservation(observation)); err != nil {
			return LimaResult{}, fmt.Errorf("reconcile Lima: prove created runtime host: %w", err)
		}
		authority.Instance = &instance
		if err := config.Authority.SaveReconciliationLimaAuthority(ctx, authority, now().UTC()); err != nil {
			return LimaResult{}, fmt.Errorf("reconcile Lima: retain runtime-host identity: %w", err)
		}
	} else {
		observation, err = config.Runtime.Inspect(ctx, config.Spec)
		if err != nil {
			return LimaResult{}, fmt.Errorf("reconcile Lima: inspect retained runtime host: %w", err)
		}
		if _, err := ownership.ProveLimaInstance(*authority.Instance, instanceObservation(observation)); err != nil {
			return LimaResult{}, fmt.Errorf("reconcile Lima: prove retained runtime host: %w", err)
		}
	}
	observation, err = config.Runtime.Start(ctx, config.Spec)
	if err != nil {
		return LimaResult{}, fmt.Errorf("reconcile Lima: start runtime host: %w", err)
	}
	if observation.State != lima.Running {
		return LimaResult{}, errors.New("reconcile Lima: runtime host is not running after start")
	}
	if _, err := ownership.ProveLimaInstance(*authority.Instance, instanceObservation(observation)); err != nil {
		return LimaResult{}, fmt.Errorf("reconcile Lima: prove running runtime host: %w", err)
	}
	return LimaResult{
		Artifact: config.Artifact, Spec: config.Spec, ConfigEvidence: *authority.ConfigEvidence,
		Instance: cloneLimaInstance(*authority.Instance), Observation: observation,
	}, nil
}

func validateLimaAttempt(config LimaConfig, attempt PreparedAttempt, generated GeneratedPlanResult) error {
	if attempt.Operation.ID == "" || attempt.Operation.BoxID == "" || attempt.Plan.BoxID != attempt.Operation.BoxID ||
		attempt.Generation < 1 || attempt.LimaConfigPath != filepath.Join(attempt.BoxRoot, "lima.yaml") ||
		config.Artifact.Component != toolchain.Lima || config.Artifact.Version != lima.SupportedVersion ||
		config.Spec.BoxID != attempt.Plan.BoxID || config.Spec.PlanDigest != attempt.Plan.PlanDigest ||
		config.Spec.Generation != attempt.Generation || !reflect.DeepEqual(config.Spec.RuntimeHost, attempt.Plan.RuntimeHost) ||
		!reflect.DeepEqual(config.Spec.Mounts, attempt.Plan.Mounts) || generated.BoxRoot.BoxID != attempt.Plan.BoxID ||
		generated.BoxRoot.Root.CanonicalPath != attempt.BoxRoot {
		return errors.New("reconcile Lima: prepared attempt identity is incomplete")
	}
	if _, err := ownership.ProvePath(filepath.Dir(attempt.BoxRoot), generated.BoxRoot.Root); err != nil {
		return fmt.Errorf("reconcile Lima: prove generated Box root: %w", err)
	}
	if _, err := lima.GenerateConfig(config.Spec); err != nil {
		return fmt.Errorf("reconcile Lima: validate runtime Spec: %w", err)
	}
	return nil
}

func reconcileLimaConfig(path string, spec lima.Spec, write func(string, lima.Spec) error) (ownership.PathSetRecord, error) {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		if err := write(path, spec); err != nil {
			return ownership.PathSetRecord{}, fmt.Errorf("reconcile Lima: write generated config: %w", err)
		}
	} else if err != nil {
		return ownership.PathSetRecord{}, fmt.Errorf("reconcile Lima: inspect generated config: %w", err)
	}
	evidence, err := ownership.RecordPathSet([]string{path})
	if err != nil {
		return ownership.PathSetRecord{}, fmt.Errorf("reconcile Lima: record generated config: %w", err)
	}
	if err := verifyLimaConfig(path, spec, evidence); err != nil {
		return ownership.PathSetRecord{}, err
	}
	return evidence, nil
}

func verifyLimaConfig(path string, spec lima.Spec, evidence ownership.PathSetRecord) error {
	if err := ownership.VerifyPathSet([]string{path}, evidence); err != nil {
		return fmt.Errorf("reconcile Lima: prove generated config: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 || info.Size() > 1024*1024 {
		return errors.New("reconcile Lima: generated config has unsafe type, mode, or size")
	}
	want, err := lima.GenerateConfig(spec)
	if err != nil {
		return fmt.Errorf("reconcile Lima: generate expected config: %w", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reconcile Lima: read generated config: %w", err)
	}
	if !bytes.Equal(got, want) {
		return errors.New("reconcile Lima: generated config content differs")
	}
	return nil
}

func instanceRecord(observation lima.Observation) ownership.LimaInstanceRecord {
	return ownership.LimaInstanceRecord{Name: observation.Name, Metadata: cloneStringMap(observation.Metadata)}
}

func instanceObservation(observation lima.Observation) ownership.LimaInstanceObservation {
	return ownership.LimaInstanceObservation{Name: observation.Name, Metadata: cloneStringMap(observation.Metadata)}
}

func cloneLimaInstance(record ownership.LimaInstanceRecord) ownership.LimaInstanceRecord {
	return ownership.LimaInstanceRecord{Name: record.Name, Metadata: cloneStringMap(record.Metadata)}
}

func cloneStringMap(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
