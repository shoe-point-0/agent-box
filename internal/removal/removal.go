// Package removal coordinates refusal-first, dependency-ordered Box cleanup.
package removal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	maxKeyBytes       = 512
	defaultEventLimit = 5 * time.Second
)

var (
	ErrSafetyRefusal  = errors.New("box removal refused")
	ErrInventoryDrift = errors.New("box removal inventory changed")
)

// Stage fixes the destruction order. A later stage may depend on every earlier
// stage having converged to absence.
type Stage uint8

const (
	StageCompose Stage = iota + 1
	StageRuntimeHost
	StageWorktree
	StagePrivateStore
	StageGeneratedState
)

func (s Stage) valid() bool { return s >= StageCompose && s <= StageGeneratedState }

// LossKind identifies an independently authorized class of user data loss.
type LossKind string

const (
	LossWorktreeChange    LossKind = "worktree-change"
	LossUnpublishedCommit LossKind = "unpublished-commit"
	LossVolume            LossKind = "volume"
)

func (k LossKind) valid() bool {
	return k == LossWorktreeChange || k == LossUnpublishedCommit || k == LossVolume
}

// Loss is one exact path, commit ID, or volume identity exposed by Preview.
type Loss struct {
	Kind  LossKind `json:"kind"`
	Value string   `json:"value"`
}

// Fact is fresh, read-only evidence for one destructive step. Evidence must
// include every ownership and policy fact whose drift must invalidate consent.
type Fact struct {
	Present  bool              `json:"present"`
	Evidence map[string]string `json:"evidence,omitempty"`
	Losses   []Loss            `json:"losses"`
}

// Step owns one independently observable and idempotently removable resource.
// Remove must consume the exact freshly observed fact and perform its own final
// typed ownership proof before touching the external system.
type Step interface {
	Key() string
	Stage() Stage
	Observe(context.Context) (Fact, error)
	Remove(context.Context, Fact) error
}

// Lease is the opaque exclusive mutation right supplied by the daemon journal.
type Lease struct {
	BoxID string
	Token string
}

// LeaseGuard acquires and releases the Box's fenced exclusive lease.
type LeaseGuard interface {
	Acquire(context.Context, string) (Lease, error)
	Release(context.Context, Lease) error
}

// Quiescer stops new Box writers and drains existing writers while the lease is
// held. Resume must be safe after a partially failed removal.
type Quiescer interface {
	Quiesce(context.Context, Lease) error
	Resume(context.Context, Lease) error
}

// Transition brackets each external side effect in durable state.
type Transition struct {
	Kind     string `json:"kind"`
	Resource string `json:"resource"`
}

const (
	TransitionRemoveStarted   = "remove-started"
	TransitionRemoveSucceeded = "remove-succeeded"
	TransitionRemoveFailed    = "remove-failed"
)

// Recorder persists a transition before returning.
type Recorder interface {
	Record(context.Context, Transition) error
}

// Resource is the canonical preview evidence for one step.
type Resource struct {
	Key      string            `json:"key"`
	Stage    Stage             `json:"stage"`
	Present  bool              `json:"present"`
	Evidence map[string]string `json:"evidence,omitempty"`
	Losses   []Loss            `json:"losses"`
	Digest   string            `json:"digest"`
}

// Preview is an immutable-by-convention loss inventory bound to one digest.
type Preview struct {
	BoxID     string     `json:"boxId"`
	Resources []Resource `json:"resources"`
	Losses    []Loss     `json:"losses"`
	Digest    string     `json:"digest"`
}

// Authorization models non-interactive CLI consent. Each loss class has a
// separate grant; Yes confirms the exact PreviewDigest.
type Authorization struct {
	PreviewDigest             string
	Yes                       bool
	DiscardWorktreeChanges    bool
	DiscardUnpublishedCommits bool
	DeleteVolumes             bool
}

// RefusalError names every missing explicit grant without mutating anything.
type RefusalError struct {
	Missing []string
}

func (e *RefusalError) Error() string {
	return fmt.Sprintf("%v: missing %s", ErrSafetyRefusal, strings.Join(e.Missing, ", "))
}

func (e *RefusalError) Unwrap() error { return ErrSafetyRefusal }

// Result retains enough progress to diagnose and resume a partial cleanup.
type Result struct {
	Completed []string
	Remaining []string
}

// ExecutionError identifies the first non-converged resource and preserves
// progress from all earlier dependency stages.
type ExecutionError struct {
	Resource  string
	Completed []string
	Remaining []string
	Err       error
}

func (e *ExecutionError) Error() string {
	return fmt.Sprintf("remove Box resource %q: %v", e.Resource, e.Err)
}

func (e *ExecutionError) Unwrap() error { return e.Err }

// Config supplies only explicit lifecycle owners; there are no ambient locks
// or background contexts.
type Config struct {
	Steps             []Step
	Lease             LeaseGuard
	Quiescer          Quiescer
	Recorder          Recorder
	AfterEventTimeout time.Duration
}

// Coordinator runs one Box removal policy and lifecycle boundary.
type Coordinator struct {
	steps      []Step
	lease      LeaseGuard
	quiescer   Quiescer
	recorder   Recorder
	eventLimit time.Duration
}

// New validates the complete dependency order before any observation.
func New(config Config) (*Coordinator, error) {
	if len(config.Steps) == 0 || config.Lease == nil || config.Quiescer == nil || config.Recorder == nil {
		return nil, errors.New("configure Box removal: steps, lease, quiescer, and recorder are required")
	}
	steps := slices.Clone(config.Steps)
	seen := make(map[string]struct{}, len(steps))
	var previous Stage
	for index, step := range steps {
		if step == nil {
			return nil, fmt.Errorf("configure Box removal: step %d is nil", index)
		}
		key, stage := step.Key(), step.Stage()
		if !validOpaque(key) || len(key) > maxKeyBytes || !stage.valid() {
			return nil, fmt.Errorf("configure Box removal: step %d has invalid identity", index)
		}
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("configure Box removal: duplicate step %q", key)
		}
		if index > 0 && stage < previous {
			return nil, errors.New("configure Box removal: steps are not in dependency order")
		}
		seen[key], previous = struct{}{}, stage
	}
	limit := config.AfterEventTimeout
	if limit == 0 {
		limit = defaultEventLimit
	}
	if limit < 0 {
		return nil, errors.New("configure Box removal: after-event timeout must be positive")
	}
	return &Coordinator{steps: steps, lease: config.Lease, quiescer: config.Quiescer, recorder: config.Recorder, eventLimit: limit}, nil
}

// Preview performs the complete read-only preflight and returns exact losses.
func (c *Coordinator) Preview(ctx context.Context, boxID string) (Preview, error) {
	if !validOpaque(boxID) {
		return Preview{}, errors.New("preview Box removal: invalid Box ID")
	}
	resources, err := c.observe(ctx)
	if err != nil {
		return Preview{}, err
	}
	return buildPreview(boxID, resources)
}

// Execute requires a digest-bound preview, then leases, quiesces, and rechecks
// every fact before each dependency-ordered destructive step.
func (c *Coordinator) Execute(ctx context.Context, preview Preview, authorization Authorization) (result Result, returnErr error) {
	canonical, err := canonicalPreview(preview)
	if err != nil || canonical.Digest != preview.Digest {
		return Result{}, fmt.Errorf("execute Box removal: invalid preview: %w", errors.Join(err, ErrInventoryDrift))
	}
	if len(canonical.Resources) != len(c.steps) {
		return Result{}, fmt.Errorf("execute Box removal: configured resources differ from preview: %w", ErrInventoryDrift)
	}
	for index, resource := range canonical.Resources {
		if resource.Key != c.steps[index].Key() || resource.Stage != c.steps[index].Stage() {
			return Result{}, fmt.Errorf("execute Box removal: configured resources differ from preview: %w", ErrInventoryDrift)
		}
	}
	if err := authorize(canonical, authorization); err != nil {
		return Result{}, err
	}

	lease, err := c.lease.Acquire(ctx, canonical.BoxID)
	if err != nil {
		return Result{}, fmt.Errorf("execute Box removal: acquire exclusive lease: %w", err)
	}
	if lease.BoxID != canonical.BoxID || !validOpaque(lease.Token) {
		_ = c.releaseAfter(ctx, lease)
		return Result{}, errors.New("execute Box removal: lease identity is invalid")
	}
	defer func() {
		returnErr = errors.Join(returnErr, c.releaseAfter(ctx, lease))
	}()
	if err := c.quiescer.Quiesce(ctx, lease); err != nil {
		return Result{}, fmt.Errorf("execute Box removal: quiesce writers: %w", err)
	}
	defer func() {
		returnErr = errors.Join(returnErr, c.resumeAfter(ctx, lease))
	}()

	freshResources, err := c.observe(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("execute Box removal: reobserve after quiesce: %w", err)
	}
	fresh, err := buildPreview(canonical.BoxID, freshResources)
	if err != nil {
		return Result{}, err
	}
	if fresh.Digest != canonical.Digest {
		return Result{}, ErrInventoryDrift
	}

	for index, expected := range canonical.Resources {
		if !expected.Present {
			continue
		}
		fact, err := c.steps[index].Observe(ctx)
		if err != nil {
			return c.failed(index, result.Completed, fmt.Errorf("reobserve immediately before removal: %w", err))
		}
		resource, err := normalizeResource(c.steps[index], fact)
		if err != nil || resource.Digest != expected.Digest {
			return c.failed(index, result.Completed, errors.Join(err, ErrInventoryDrift))
		}
		if err := c.recorder.Record(ctx, Transition{Kind: TransitionRemoveStarted, Resource: expected.Key}); err != nil {
			return c.failed(index, result.Completed, fmt.Errorf("persist remove start: %w", err))
		}
		removeErr := c.steps[index].Remove(ctx, fact)
		kind := TransitionRemoveSucceeded
		if removeErr != nil {
			kind = TransitionRemoveFailed
		}
		recordErr := c.recordAfter(ctx, Transition{Kind: kind, Resource: expected.Key})
		if removeErr != nil || recordErr != nil {
			return c.failed(index, result.Completed, errors.Join(removeErr, wrap(recordErr, "persist remove result")))
		}
		result.Completed = append(result.Completed, expected.Key)
	}
	return result, nil
}

func (c *Coordinator) observe(ctx context.Context) ([]Resource, error) {
	resources := make([]Resource, len(c.steps))
	for index, step := range c.steps {
		fact, err := step.Observe(ctx)
		if err != nil {
			return nil, fmt.Errorf("observe Box removal resource %q: %w", step.Key(), err)
		}
		resource, err := normalizeResource(step, fact)
		if err != nil {
			return nil, fmt.Errorf("observe Box removal resource %q: %w", step.Key(), err)
		}
		resources[index] = resource
	}
	return resources, nil
}

func normalizeResource(step Step, fact Fact) (Resource, error) {
	resource := Resource{Key: step.Key(), Stage: step.Stage(), Present: fact.Present}
	if !fact.Present {
		if len(fact.Evidence) != 0 || len(fact.Losses) != 0 {
			return Resource{}, errors.New("absent resource contains evidence or losses")
		}
		resource.Losses = []Loss{}
	} else {
		if len(fact.Evidence) == 0 {
			return Resource{}, errors.New("present resource lacks identity evidence")
		}
		resource.Evidence = make(map[string]string, len(fact.Evidence))
		for key, value := range fact.Evidence {
			if !validOpaque(key) || !validOpaque(value) {
				return Resource{}, errors.New("resource identity evidence is invalid")
			}
			resource.Evidence[key] = value
		}
		resource.Losses = slices.Clone(fact.Losses)
		slices.SortFunc(resource.Losses, compareLoss)
		for index, loss := range resource.Losses {
			if !loss.Kind.valid() || !validOpaque(loss.Value) {
				return Resource{}, errors.New("resource loss inventory is invalid")
			}
			if index > 0 && compareLoss(resource.Losses[index-1], loss) == 0 {
				return Resource{}, errors.New("resource loss inventory contains a duplicate")
			}
		}
	}
	digest, err := digestJSON(struct {
		Key      string            `json:"key"`
		Stage    Stage             `json:"stage"`
		Present  bool              `json:"present"`
		Evidence map[string]string `json:"evidence,omitempty"`
		Losses   []Loss            `json:"losses"`
	}{resource.Key, resource.Stage, resource.Present, resource.Evidence, resource.Losses})
	if err != nil {
		return Resource{}, err
	}
	resource.Digest = digest
	return resource, nil
}

func buildPreview(boxID string, resources []Resource) (Preview, error) {
	preview := Preview{BoxID: boxID, Resources: cloneResources(resources), Losses: []Loss{}}
	for _, resource := range resources {
		preview.Losses = append(preview.Losses, resource.Losses...)
	}
	slices.SortFunc(preview.Losses, compareLoss)
	digest, err := digestJSON(struct {
		BoxID     string     `json:"boxId"`
		Resources []Resource `json:"resources"`
	}{preview.BoxID, preview.Resources})
	if err != nil {
		return Preview{}, err
	}
	preview.Digest = digest
	return preview, nil
}

func canonicalPreview(preview Preview) (Preview, error) {
	if !validOpaque(preview.BoxID) || len(preview.Resources) == 0 {
		return Preview{}, errors.New("preview identity is incomplete")
	}
	resources := make([]Resource, len(preview.Resources))
	for index, supplied := range preview.Resources {
		if !validOpaque(supplied.Key) || !supplied.Stage.valid() {
			return Preview{}, errors.New("preview resource identity is invalid")
		}
		fact := Fact{Present: supplied.Present, Evidence: supplied.Evidence, Losses: supplied.Losses}
		step := previewStep{key: supplied.Key, stage: supplied.Stage}
		normalized, err := normalizeResource(step, fact)
		if err != nil || normalized.Digest != supplied.Digest {
			return Preview{}, errors.Join(err, ErrInventoryDrift)
		}
		if index > 0 && normalized.Stage < resources[index-1].Stage {
			return Preview{}, errors.New("preview resources are not dependency ordered")
		}
		resources[index] = normalized
	}
	return buildPreview(preview.BoxID, resources)
}

type previewStep struct {
	key   string
	stage Stage
}

func (s previewStep) Key() string                         { return s.key }
func (s previewStep) Stage() Stage                        { return s.stage }
func (previewStep) Observe(context.Context) (Fact, error) { panic("preview step cannot observe") }
func (previewStep) Remove(context.Context, Fact) error    { panic("preview step cannot remove") }

func authorize(preview Preview, authorization Authorization) error {
	if authorization.PreviewDigest != preview.Digest {
		return ErrInventoryDrift
	}
	present := false
	missing := []string{}
	needChanges, needCommits, needVolumes := false, false, false
	for _, resource := range preview.Resources {
		present = present || resource.Present
	}
	for _, loss := range preview.Losses {
		switch loss.Kind {
		case LossWorktreeChange:
			needChanges = true
		case LossUnpublishedCommit:
			needCommits = true
		case LossVolume:
			needVolumes = true
		}
	}
	if present && !authorization.Yes {
		missing = append(missing, "--yes")
	}
	if needChanges && !authorization.DiscardWorktreeChanges {
		missing = append(missing, "--discard-worktree-changes")
	}
	if needCommits && !authorization.DiscardUnpublishedCommits {
		missing = append(missing, "--discard-unpublished-commits")
	}
	if needVolumes && !authorization.DeleteVolumes {
		missing = append(missing, "--delete-volumes")
	}
	if len(missing) != 0 {
		return &RefusalError{Missing: missing}
	}
	return nil
}

func (c *Coordinator) failed(index int, completed []string, err error) (Result, error) {
	remaining := make([]string, 0, len(c.steps)-index)
	for _, step := range c.steps[index:] {
		remaining = append(remaining, step.Key())
	}
	result := Result{Completed: slices.Clone(completed), Remaining: remaining}
	return result, &ExecutionError{Resource: c.steps[index].Key(), Completed: slices.Clone(completed), Remaining: slices.Clone(remaining), Err: err}
}

func (c *Coordinator) recordAfter(parent context.Context, transition Transition) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), c.eventLimit)
	defer cancel()
	return c.recorder.Record(ctx, transition)
}

func (c *Coordinator) releaseAfter(parent context.Context, lease Lease) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), c.eventLimit)
	defer cancel()
	return c.lease.Release(ctx, lease)
}

func (c *Coordinator) resumeAfter(parent context.Context, lease Lease) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), c.eventLimit)
	defer cancel()
	return c.quiescer.Resume(ctx, lease)
}

func digestJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func cloneResources(resources []Resource) []Resource {
	clone := make([]Resource, len(resources))
	for index, resource := range resources {
		clone[index] = resource
		clone[index].Evidence = make(map[string]string, len(resource.Evidence))
		for key, value := range resource.Evidence {
			clone[index].Evidence[key] = value
		}
		clone[index].Losses = slices.Clone(resource.Losses)
	}
	return clone
}

func compareLoss(left, right Loss) int {
	if compared := strings.Compare(string(left.Kind), string(right.Kind)); compared != 0 {
		return compared
	}
	return strings.Compare(left.Value, right.Value)
}

func validOpaque(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && !strings.ContainsRune(value, '\x00')
}

func wrap(err error, message string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}
