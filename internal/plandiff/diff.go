// Package plandiff classifies immutable Plan changes without mutating resources.
package plandiff

import (
	"reflect"
	"slices"
	"strings"

	"github.com/shoe-point-0/agent-box/internal/contract"
)

// Class is the reconciliation behavior required by a Plan change.
type Class string

const (
	ClassNoOp     Class = "no-op"
	ClassInPlace  Class = "in-place"
	ClassGrowth   Class = "growth"
	ClassRemoval  Class = "removal"
	ClassRestart  Class = "restart"
	ClassConflict Class = "conflict"
)

// Reason is a stable machine-readable explanation for a classification.
type Reason string

const (
	ReasonPlanDigestsMatch          Reason = "plan-digests-match"
	ReasonObservedStateOnly         Reason = "observed-state-only"
	ReasonChangesRequired           Reason = "changes-required"
	ReasonInvalidPlan               Reason = "invalid-plan"
	ReasonPlanIdentityChanged       Reason = "plan-identity-changed"
	ReasonManifestInputChanged      Reason = "manifest-input-changed"
	ReasonRuntimeIdentityChanged    Reason = "runtime-identity-changed"
	ReasonRuntimeResourcesChanged   Reason = "runtime-resources-changed"
	ReasonRuntimeDiskIncrease       Reason = "runtime-disk-increase"
	ReasonRuntimeDiskShrink         Reason = "runtime-disk-shrink"
	ReasonRepositoryAdded           Reason = "repository-added"
	ReasonRepositoryCheckoutChanged Reason = "repository-checkout-changed"
	ReasonRepositoryIdentityChanged Reason = "repository-identity-changed"
	ReasonRepositoryRemoved         Reason = "repository-removed"
	ReasonDirtyCheckoutReplacement  Reason = "dirty-checkout-replacement"
	ReasonDirtyRepositoryRemoval    Reason = "dirty-repository-removal"
	ReasonComposeChanged            Reason = "compose-configuration-changed"
	ReasonTCPPortChanged            Reason = "tcp-port-configuration-changed"
	ReasonTCPPortRemoved            Reason = "tcp-port-removed"
	ReasonRouteChanged              Reason = "route-or-readiness-changed"
	ReasonSetupJobChanged           Reason = "setup-job-changed"
	ReasonSecretReferenceChanged    Reason = "secret-reference-changed"
	ReasonRuntimeToolsChanged       Reason = "runtime-tools-changed"
	ReasonControlToolsChanged       Reason = "control-tools-changed"
)

// Kind identifies one Section 9.4 rule independently of the milestone in which
// its Plan field becomes executable.
type Kind string

const (
	KindRouteOrReadiness   Kind = "route-or-readiness"
	KindTCPPort            Kind = "tcp-port"
	KindCompose            Kind = "compose"
	KindSetupJob           Kind = "setup-job"
	KindWorktreeEdit       Kind = "worktree-edit"
	KindRuntimeResources   Kind = "runtime-resources"
	KindRuntimeDiskGrow    Kind = "runtime-disk-grow"
	KindRuntimeDiskShrink  Kind = "runtime-disk-shrink"
	KindRepositoryCheckout Kind = "repository-checkout"
	KindRepositoryRemoval  Kind = "repository-removal"
	KindSecretReference    Kind = "secret-reference"
)

// Fact is a value-only classification input emitted by a Plan-field adapter.
// Dirty is meaningful only for checkout replacement and repository removal.
type Fact struct {
	Kind     Kind
	Resource string
	Dirty    bool
}

// Change is one deterministic machine- and human-readable reconciliation fact.
type Change struct {
	Class       Class  `json:"class"`
	Reason      Reason `json:"reason"`
	Resource    string `json:"resource"`
	Description string `json:"description"`
}

// Result is the aggregate classification and its ordered reasons.
type Result struct {
	Classification Class    `json:"classification"`
	Reason         Reason   `json:"reason"`
	Changes        []Change `json:"changes,omitempty"`
}

// Observation contains only mutable state needed for refusal decisions.
type Observation struct {
	Repositories []RepositoryObservation
}

// RepositoryObservation reports whether an existing worktree has edits.
type RepositoryObservation struct {
	Name  string
	Dirty bool
}

// Classify applies the complete Section 9.4 policy table to already-detected
// changes. Deferred Plan fields remain absent from the executable Plan schema;
// their future adapters can emit Facts without changing safety policy.
func Classify(facts []Fact) Result {
	changes := make([]Change, 0, len(facts))
	for _, fact := range facts {
		switch fact.Kind {
		case KindRouteOrReadiness:
			changes = append(changes, change(ClassInPlace, ReasonRouteChanged, fact.Resource, "route or readiness configuration may be updated in place and probed again"))
		case KindTCPPort:
			changes = append(changes, change(ClassInPlace, ReasonTCPPortChanged, fact.Resource, "TCP endpoint allocation may be reconciled in place and probed again"))
		case KindCompose:
			changes = append(changes, change(ClassInPlace, ReasonComposeChanged, fact.Resource, "Compose service, build, or image configuration may be reconciled in place"))
		case KindSetupJob:
			changes = append(changes, change(ClassInPlace, ReasonSetupJobChanged, fact.Resource, "setup-job stamp and dependent stamps must be invalidated"))
		case KindWorktreeEdit:
			// Worktree edits are ordinary observed state, not desired runtime work.
		case KindRuntimeResources:
			changes = append(changes, change(ClassRestart, ReasonRuntimeResourcesChanged, fact.Resource, "CPU or memory changes require a runtime-host restart"))
		case KindRuntimeDiskGrow:
			changes = append(changes, change(ClassGrowth, ReasonRuntimeDiskIncrease, fact.Resource, "disk growth requires the supported runtime-host growth operation"))
		case KindRuntimeDiskShrink:
			changes = append(changes, change(ClassConflict, ReasonRuntimeDiskShrink, fact.Resource, "runtime-host disk shrinking is not supported"))
		case KindRepositoryCheckout:
			if fact.Dirty {
				changes = append(changes, change(ClassConflict, ReasonDirtyCheckoutReplacement, fact.Resource, "dirty worktree base or branch cannot be replaced"))
			} else {
				changes = append(changes, change(ClassInPlace, ReasonRepositoryCheckoutChanged, fact.Resource, "clean worktree checkout may be replaced during reconciliation"))
			}
		case KindRepositoryRemoval:
			if fact.Dirty {
				changes = append(changes, change(ClassConflict, ReasonDirtyRepositoryRemoval, fact.Resource, "dirty repository worktree cannot be removed"))
			} else {
				changes = append(changes, change(ClassRemoval, ReasonRepositoryRemoved, fact.Resource, "clean repository worktree may be removed during reconciliation"))
			}
		case KindSecretReference:
			changes = append(changes, change(ClassRestart, ReasonSecretReferenceChanged, fact.Resource, "secret reference changes restart only declared consumers"))
		default:
			changes = append(changes, change(ClassConflict, ReasonInvalidPlan, fact.Resource, "unknown reconciliation change kind"))
		}
	}
	if len(changes) == 0 {
		return Result{Classification: ClassNoOp, Reason: ReasonObservedStateOnly}
	}
	sortChanges(changes)
	return Result{Classification: dominantClass(changes), Reason: ReasonChangesRequired, Changes: changes}
}

// Compare classifies current-to-desired executable Plan changes. It is pure:
// callers retain ownership of both Plans and every observed resource.
func Compare(current, desired *contract.Plan, observed Observation) Result {
	if current == nil || desired == nil || current.PlanDigest == "" || desired.PlanDigest == "" {
		return conflictResult(ReasonInvalidPlan, "plan", "current and desired Plans must have non-empty digests")
	}
	if current.PlanDigest == desired.PlanDigest {
		if plansEqual(current, desired) {
			return Result{Classification: ClassNoOp, Reason: ReasonPlanDigestsMatch}
		}
		return conflictResult(ReasonInvalidPlan, "plan", "matching Plan digests contain different content")
	}

	changes := make([]Change, 0, 8)
	structuralChange := false
	if current.SchemaVersion != desired.SchemaVersion || current.WorkspaceID != desired.WorkspaceID ||
		current.BoxID != desired.BoxID || current.TaskKey != desired.TaskKey {
		changes = append(changes, change(ClassConflict, ReasonPlanIdentityChanged, "plan", "Plan identity cannot change during reconciliation"))
		structuralChange = true
	}
	if current.Manifest.Path != desired.Manifest.Path {
		changes = append(changes, change(ClassConflict, ReasonPlanIdentityChanged, "manifest", "manifest ownership path cannot change during reconciliation"))
		structuralChange = true
	}
	if current.RuntimeHost.Driver != desired.RuntimeHost.Driver || current.RuntimeHost.InstanceName != desired.RuntimeHost.InstanceName {
		changes = append(changes, change(ClassConflict, ReasonRuntimeIdentityChanged, "runtime-host", "runtime-host identity cannot change during reconciliation"))
		structuralChange = true
	}
	if current.RuntimeHost.CPUMillicores != desired.RuntimeHost.CPUMillicores || current.RuntimeHost.MemoryBytes != desired.RuntimeHost.MemoryBytes {
		changes = append(changes, change(ClassRestart, ReasonRuntimeResourcesChanged, "runtime-host", "CPU or memory changes require a runtime-host restart"))
		structuralChange = true
	}
	if current.RuntimeHost.DiskBytes < desired.RuntimeHost.DiskBytes {
		changes = append(changes, change(ClassGrowth, ReasonRuntimeDiskIncrease, "runtime-host", "disk growth requires the supported runtime-host growth operation"))
		structuralChange = true
	} else if current.RuntimeHost.DiskBytes > desired.RuntimeHost.DiskBytes {
		changes = append(changes, change(ClassConflict, ReasonRuntimeDiskShrink, "runtime-host", "runtime-host disk shrinking is not supported"))
		structuralChange = true
	}

	before := len(changes)
	changes = compareRepositories(changes, current.Repositories, desired.Repositories, observed)
	structuralChange = structuralChange || len(changes) != before

	if !reflect.DeepEqual(current.Compose, desired.Compose) ||
		!reflect.DeepEqual(current.WorkspaceService, desired.WorkspaceService) ||
		!reflect.DeepEqual(current.Mounts, desired.Mounts) ||
		!reflect.DeepEqual(current.Policy, desired.Policy) {
		changes = append(changes, change(ClassInPlace, ReasonComposeChanged, "compose", "Compose service, build, image, mount, or policy configuration changed"))
		structuralChange = true
	}
	before = len(changes)
	changes = compareTCPPorts(changes, current.TCPPorts, desired.TCPPorts)
	structuralChange = structuralChange || len(changes) != before
	if current.Tools.Lima != desired.Tools.Lima || current.Tools.Docker != desired.Tools.Docker || current.Tools.Compose != desired.Tools.Compose {
		changes = append(changes, change(ClassRestart, ReasonRuntimeToolsChanged, "runtime-tools", "runtime tool contract changes require restart and verification"))
		structuralChange = true
	}
	if current.Tools.AgentBox != desired.Tools.AgentBox || current.Tools.Git != desired.Tools.Git {
		changes = append(changes, change(ClassInPlace, ReasonControlToolsChanged, "control-tools", "control-plane tool contract changed"))
		structuralChange = true
	}
	if !structuralChange && current.Manifest.Digest != desired.Manifest.Digest {
		changes = append(changes, change(ClassInPlace, ReasonManifestInputChanged, "manifest", "manifest input changed without a projected resource change"))
	}
	if len(changes) == 0 {
		return conflictResult(ReasonInvalidPlan, "plan", "Plan digests differ but no content change was found")
	}

	sortChanges(changes)
	return Result{Classification: dominantClass(changes), Reason: ReasonChangesRequired, Changes: changes}
}

func plansEqual(left, right *contract.Plan) bool {
	if left.SchemaVersion != right.SchemaVersion || left.WorkspaceID != right.WorkspaceID || left.BoxID != right.BoxID ||
		left.TaskKey != right.TaskKey || left.Manifest != right.Manifest || left.RuntimeHost != right.RuntimeHost ||
		left.WorkspaceService != right.WorkspaceService || left.Policy != right.Policy || left.Tools != right.Tools ||
		left.PlanDigest != right.PlanDigest || left.Compose.ProjectName != right.Compose.ProjectName ||
		len(left.Repositories) != len(right.Repositories) || len(left.Compose.Includes) != len(right.Compose.Includes) ||
		!equalStrings(left.Compose.Profiles, right.Compose.Profiles) || len(left.Mounts) != len(right.Mounts) ||
		len(left.TCPPorts) != len(right.TCPPorts) {
		return false
	}
	for index := range left.Repositories {
		if left.Repositories[index] != right.Repositories[index] {
			return false
		}
	}
	for index := range left.Compose.Includes {
		if left.Compose.Includes[index] != right.Compose.Includes[index] {
			return false
		}
	}
	for index := range left.Mounts {
		if left.Mounts[index].Path != right.Mounts[index].Path || left.Mounts[index].Writable != right.Mounts[index].Writable ||
			!equalStrings(left.Mounts[index].Services, right.Mounts[index].Services) {
			return false
		}
	}
	for index := range left.TCPPorts {
		if left.TCPPorts[index] != right.TCPPorts[index] {
			return false
		}
	}
	return true
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func compareRepositories(changes []Change, current, desired []contract.PlanRepository, observed Observation) []Change {
	for _, existing := range current {
		wanted, found := repositoryByName(desired, existing.Name)
		if !found {
			if observed.dirty(existing.Name) {
				changes = append(changes, change(ClassConflict, ReasonDirtyRepositoryRemoval, "repository/"+existing.Name, "dirty repository worktree cannot be removed"))
			} else {
				changes = append(changes, change(ClassRemoval, ReasonRepositoryRemoved, "repository/"+existing.Name, "clean repository worktree may be removed during reconciliation"))
			}
			continue
		}
		if existing.SourcePath != wanted.SourcePath || existing.WorktreePath != wanted.WorktreePath || existing.GitCommonDir != wanted.GitCommonDir {
			changes = append(changes, change(ClassConflict, ReasonRepositoryIdentityChanged, "repository/"+existing.Name, "repository ownership paths cannot change during reconciliation"))
		}
		if existing.BaseCommit != wanted.BaseCommit || existing.Branch != wanted.Branch {
			if observed.dirty(existing.Name) {
				changes = append(changes, change(ClassConflict, ReasonDirtyCheckoutReplacement, "repository/"+existing.Name, "dirty worktree base or branch cannot be replaced"))
			} else {
				changes = append(changes, change(ClassInPlace, ReasonRepositoryCheckoutChanged, "repository/"+existing.Name, "clean worktree checkout may be replaced during reconciliation"))
			}
		}
	}
	for _, wanted := range desired {
		if _, found := repositoryByName(current, wanted.Name); !found {
			changes = append(changes, change(ClassInPlace, ReasonRepositoryAdded, "repository/"+wanted.Name, "repository may be created during reconciliation"))
		}
	}
	return changes
}

func repositoryByName(repositories []contract.PlanRepository, name string) (contract.PlanRepository, bool) {
	for _, repository := range repositories {
		if repository.Name == name {
			return repository, true
		}
	}
	return contract.PlanRepository{}, false
}

func compareTCPPorts(changes []Change, current, desired []contract.TCPPortPlan) []Change {
	for _, existing := range current {
		wanted, found := tcpPortByName(desired, existing.Name)
		if !found {
			changes = append(changes, change(ClassRemoval, ReasonTCPPortRemoved, "tcp-port/"+existing.Name, "removed TCP endpoint allocation must be released"))
			continue
		}
		if existing != wanted {
			changes = append(changes, change(ClassInPlace, ReasonTCPPortChanged, "tcp-port/"+existing.Name, "TCP endpoint target configuration changed"))
		}
	}
	for _, wanted := range desired {
		if _, found := tcpPortByName(current, wanted.Name); !found {
			changes = append(changes, change(ClassInPlace, ReasonTCPPortChanged, "tcp-port/"+wanted.Name, "TCP endpoint allocation must be created and probed"))
		}
	}
	return changes
}

func tcpPortByName(ports []contract.TCPPortPlan, name string) (contract.TCPPortPlan, bool) {
	for _, port := range ports {
		if port.Name == name {
			return port, true
		}
	}
	return contract.TCPPortPlan{}, false
}

func (observed Observation) dirty(name string) bool {
	for _, repository := range observed.Repositories {
		if repository.Name == name {
			return repository.Dirty
		}
	}
	return false
}

func change(class Class, reason Reason, resource, description string) Change {
	return Change{Class: class, Reason: reason, Resource: resource, Description: description}
}

func conflictResult(reason Reason, resource, description string) Result {
	return Result{
		Classification: ClassConflict,
		Reason:         ReasonChangesRequired,
		Changes:        []Change{change(ClassConflict, reason, resource, description)},
	}
}

func sortChanges(changes []Change) {
	slices.SortFunc(changes, func(left, right Change) int {
		if compared := strings.Compare(left.Resource, right.Resource); compared != 0 {
			return compared
		}
		if compared := strings.Compare(string(left.Reason), string(right.Reason)); compared != 0 {
			return compared
		}
		return strings.Compare(string(left.Class), string(right.Class))
	})
}

func dominantClass(changes []Change) Class {
	dominant := ClassInPlace
	for _, item := range changes {
		if classRank(item.Class) > classRank(dominant) {
			dominant = item.Class
		}
	}
	return dominant
}

func classRank(class Class) int {
	switch class {
	case ClassConflict:
		return 5
	case ClassRestart:
		return 4
	case ClassRemoval:
		return 3
	case ClassGrowth:
		return 2
	case ClassInPlace:
		return 1
	default:
		return 0
	}
}
