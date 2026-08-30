package up

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"time"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/lima"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/state"
)

// ComposeConfig binds deterministic Compose compilation to the running
// runtime host produced by the foundation prefix. Runtime apply is deliberately
// separate because its resource identities require durable partial authority.
type ComposeConfig struct {
	Runner      compose.Runner
	DockerPath  string
	Environment map[string]string
	Secrets     []compose.Secret
}

// ComposeAuthorityStore retains apply intent and the exact resource identities
// produced by it. Loaded records are recovery authority, not live observation.
type ComposeAuthorityStore interface {
	ReconciliationComposeAuthorityFor(context.Context, string, int64) (state.ReconciliationComposeAuthority, bool, error)
	SaveReconciliationComposeAuthority(context.Context, state.ReconciliationComposeAuthority, time.Time) error
}

var _ ComposeAuthorityStore = (*state.Store)(nil)

// ComposeRuntime is the concrete apply and observation boundary used by up
// reconciliation. Apply is idempotent for an unchanged RuntimeSpec.
type ComposeRuntime interface {
	Apply(context.Context, compose.RuntimeSpec) (compose.RuntimeObservation, error)
	Observe(context.Context, compose.RuntimeSpec) (compose.RuntimeObservation, error)
}

var _ ComposeRuntime = (*compose.Runtime)(nil)

// ComposeApplyConfig binds durable partial authority to the runtime mutation
// boundary. Now is injectable only for deterministic tests.
type ComposeApplyConfig struct {
	Authority ComposeAuthorityStore
	Runtime   ComposeRuntime
	Now       func() time.Time
}

// ComposeResult carries freshly observed runtime state and its exact retained
// identities into filesystem readiness and final READY publication.
type ComposeResult struct {
	Spec        compose.RuntimeSpec
	Records     compose.RuntimeRecords
	Observation compose.RuntimeObservation
}

// ReconcileComposeInputs freshly normalizes repository-owned Compose intent,
// materializes the generated policy inputs, and records the exact Machine path
// set required by runtime apply and filesystem readiness. Exact warm output
// keeps its existing inode while every normalization boundary is rerun.
func ReconcileComposeInputs(ctx context.Context, config ComposeConfig, attempt PreparedAttempt, foundation FoundationResult) (compose.RuntimeSpec, error) {
	if err := validateComposeFoundation(config, attempt, foundation); err != nil {
		return compose.RuntimeSpec{}, err
	}
	compiled, err := compose.Compile(ctx, compose.Input{
		Plan: attempt.Plan, Directory: attempt.BoxRoot, Generation: attempt.Generation,
		ComposeCommand: []string{config.DockerPath, "compose"}, Environment: config.Environment,
		Secrets: config.Secrets, Runner: config.Runner,
	})
	if err != nil {
		return compose.RuntimeSpec{}, fmt.Errorf("reconcile Compose inputs: compile: %w", err)
	}
	spec := compose.RuntimeSpec{
		RuntimeHost: foundation.Lima.Spec, Plan: attempt.Plan, Generation: attempt.Generation,
		DockerPath: config.DockerPath, Workdir: attempt.BoxRoot, Compiled: compiled,
	}
	spec.PathEvidence, err = compose.RecordRuntimePaths(attempt.Plan, compiled)
	if err != nil {
		return compose.RuntimeSpec{}, fmt.Errorf("reconcile Compose inputs: record runtime paths: %w", err)
	}
	return spec, nil
}

// ReconcileCompose applies newly claimed intent or freshly observes an already
// retained resource set. Intent is durable before Apply, and a warm recovery
// must prove the complete immutable ID set before later readiness may consume
// the observation.
func ReconcileCompose(ctx context.Context, config ComposeApplyConfig, attempt PreparedAttempt, spec compose.RuntimeSpec) (ComposeResult, error) {
	if config.Authority == nil || config.Runtime == nil {
		return ComposeResult{}, errors.New("reconcile Compose: authority store and runtime are required")
	}
	if err := validateComposeAttempt(attempt, spec); err != nil {
		return ComposeResult{}, err
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}

	authority, found, err := config.Authority.ReconciliationComposeAuthorityFor(ctx, attempt.Plan.BoxID, attempt.Generation)
	if err != nil {
		return ComposeResult{}, fmt.Errorf("reconcile Compose: load recovery authority: %w", err)
	}
	desired := state.ReconciliationComposeAuthority{
		BoxID: attempt.Plan.BoxID, Generation: attempt.Generation, PlanDigest: attempt.Plan.PlanDigest,
		OperationID: attempt.Operation.ID, Spec: spec,
	}
	if !found {
		authority = desired
		if attempt.Warm {
			if !reflect.DeepEqual(attempt.Current.ComposeSpec, spec) {
				return ComposeResult{}, errors.New("reconcile Compose: warm runtime authority differs from prepared attempt")
			}
			records := cloneRuntimeRecords(attempt.Current.Compose)
			authority.Records = &records
		}
	} else {
		observedBase := authority
		observedBase.OperationID, observedBase.Records = "", nil
		desired.OperationID = ""
		if !reflect.DeepEqual(observedBase, desired) {
			return ComposeResult{}, errors.New("reconcile Compose: recovery authority differs from prepared attempt")
		}
		authority.OperationID = attempt.Operation.ID
	}
	if err := config.Authority.SaveReconciliationComposeAuthority(ctx, authority, now().UTC()); err != nil {
		return ComposeResult{}, fmt.Errorf("reconcile Compose: persist intent: %w", err)
	}

	var observation compose.RuntimeObservation
	if authority.Records == nil {
		observation, err = config.Runtime.Apply(ctx, spec)
		if err != nil {
			return ComposeResult{}, fmt.Errorf("reconcile Compose: apply runtime: %w", err)
		}
		records := compose.RecordsFromObservation(observation)
		if err := verifyExactRuntimeRecords(records, observation); err != nil {
			return ComposeResult{}, fmt.Errorf("reconcile Compose: prove applied runtime: %w", err)
		}
		authority.Records = &records
		if err := config.Authority.SaveReconciliationComposeAuthority(ctx, authority, now().UTC()); err != nil {
			return ComposeResult{}, fmt.Errorf("reconcile Compose: retain runtime identities: %w", err)
		}
	} else {
		observation, err = config.Runtime.Observe(ctx, spec)
		if err != nil {
			return ComposeResult{}, fmt.Errorf("reconcile Compose: observe retained runtime: %w", err)
		}
	}
	if err := verifyExactRuntimeRecords(*authority.Records, observation); err != nil {
		return ComposeResult{}, fmt.Errorf("reconcile Compose: prove retained runtime: %w", err)
	}
	return ComposeResult{Spec: spec, Records: cloneRuntimeRecords(*authority.Records), Observation: observation}, nil
}

func verifyExactRuntimeRecords(records compose.RuntimeRecords, observation compose.RuntimeObservation) error {
	if len(records.Resources) == 0 {
		return errors.New("live runtime ownership is absent")
	}
	if err := compose.VerifyRemovalRecords(records, observation); err != nil {
		return fmt.Errorf("live runtime identities differ from recovery authority: %w", err)
	}
	if !reflect.DeepEqual(records, compose.RecordsFromObservation(observation)) {
		return errors.New("live runtime identities differ from recovery authority")
	}
	return nil
}

func validateComposeAttempt(attempt PreparedAttempt, spec compose.RuntimeSpec) error {
	if attempt.Operation.ID == "" || attempt.Operation.BoxID == "" || attempt.Plan.BoxID != attempt.Operation.BoxID ||
		attempt.Generation < 1 || spec.Plan.BoxID != attempt.Plan.BoxID || spec.Plan.PlanDigest != attempt.Plan.PlanDigest ||
		spec.Generation != attempt.Generation || spec.Workdir != attempt.BoxRoot || !reflect.DeepEqual(spec.Plan, attempt.Plan) {
		return errors.New("reconcile Compose: prepared attempt identity is incomplete")
	}
	return nil
}

func cloneRuntimeRecords(records compose.RuntimeRecords) compose.RuntimeRecords {
	cloned := compose.RuntimeRecords{
		Resources: append([]ownership.ComposeResourceRecord(nil), records.Resources...),
		Volumes:   append([]ownership.VolumeRecord(nil), records.Volumes...),
	}
	for index := range cloned.Resources {
		cloned.Resources[index].Labels = cloneStringMap(cloned.Resources[index].Labels)
	}
	for index := range cloned.Volumes {
		cloned.Volumes[index].Labels = cloneStringMap(cloned.Volumes[index].Labels)
	}
	return cloned
}

func validateComposeFoundation(config ComposeConfig, attempt PreparedAttempt, foundation FoundationResult) error {
	if config.Runner == nil || !cleanAbsoluteGeneratedPath(config.DockerPath) ||
		attempt.Operation.ID == "" || attempt.Plan.BoxID != attempt.Operation.BoxID || attempt.Generation < 1 ||
		filepath.Dir(attempt.PlanPath) != attempt.BoxRoot {
		return errors.New("reconcile Compose inputs: configuration or prepared attempt is incomplete")
	}
	if foundation.Generated.BoxRoot.BoxID != attempt.Plan.BoxID ||
		foundation.Generated.BoxRoot.Root.CanonicalPath != attempt.BoxRoot {
		return errors.New("reconcile Compose inputs: generated-state authority differs from prepared attempt")
	}
	if _, err := ownership.ProvePath(filepath.Dir(attempt.BoxRoot), foundation.Generated.BoxRoot.Root); err != nil {
		return fmt.Errorf("reconcile Compose inputs: prove generated Box root: %w", err)
	}
	if foundation.Git.Store.GitCommonDir != attempt.Repository.GitStorePath ||
		foundation.Git.Worktree.CanonicalPath != attempt.Repository.WorktreePath ||
		foundation.Git.Worktree.CanonicalCommon != attempt.Repository.GitStorePath {
		return errors.New("reconcile Compose inputs: Git observation differs from prepared attempt")
	}
	storeIdentity, storeErr := ownership.InspectPath(attempt.Repository.GitStorePath)
	worktreeIdentity, worktreeErr := ownership.InspectPath(attempt.Repository.WorktreePath)
	if storeErr != nil || worktreeErr != nil || storeIdentity != foundation.Git.Store.Identity ||
		worktreeIdentity != foundation.Git.Worktree.WorktreeIdentity {
		return errors.New("reconcile Compose inputs: Git path identity changed after observation")
	}
	if !reflect.DeepEqual(foundation.Lima.Spec.RuntimeHost, attempt.Plan.RuntimeHost) ||
		!reflect.DeepEqual(foundation.Lima.Spec.Mounts, attempt.Plan.Mounts) ||
		foundation.Lima.Spec.BoxID != attempt.Plan.BoxID || foundation.Lima.Spec.PlanDigest != attempt.Plan.PlanDigest ||
		foundation.Lima.Spec.Generation != attempt.Generation || foundation.Lima.Observation.State != lima.Running ||
		foundation.Lima.Observation.Name != attempt.Plan.RuntimeHost.InstanceName || foundation.Lima.Instance.Name != attempt.Plan.RuntimeHost.InstanceName {
		return errors.New("reconcile Compose inputs: running runtime-host observation differs from prepared attempt")
	}
	runtimePaths := make([]string, 0, len(foundation.Lima.Spec.Mounts)+1)
	runtimePaths = append(runtimePaths, foundation.Lima.Spec.ImagePath)
	for _, mount := range foundation.Lima.Spec.Mounts {
		runtimePaths = append(runtimePaths, mount.Path)
	}
	if err := ownership.VerifyPathSet(runtimePaths, foundation.Lima.Spec.PathEvidence); err != nil {
		return errors.New("reconcile Compose inputs: runtime-host path identity changed after observation")
	}
	return nil
}
