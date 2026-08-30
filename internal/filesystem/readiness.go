package filesystem

import (
	"context"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/ownership"
)

// ReadyPublisher atomically publishes one Box generation as READY. The
// callback must not perform unrelated work: the gate revalidates every Machine
// path identity immediately before invoking it.
type ReadyPublisher func(context.Context, Observation) error

type liveReadinessVerifier interface {
	Verify(context.Context, LiveSpec) (Observation, error)
}

type probeReadinessVerifier interface {
	Verify(context.Context, contract.Plan, compose.RuntimeObservation) error
}

// ReadinessGate is the only filesystem boundary that may precede a READY
// publication. It joins the independently observed mount-set/canary checks and
// the bidirectional workspace probe, then revalidates all Lima and Compose path
// identities immediately before publication.
type ReadinessGate struct {
	live  liveReadinessVerifier
	probe probeReadinessVerifier
}

// NewReadinessGate constructs the complete filesystem readiness boundary from
// the live runtime and immutable-container adapters.
func NewReadinessGate(host RuntimeHost, runtime ContainerRuntime, workspace WorkspaceProbe) (*ReadinessGate, error) {
	live, err := NewLiveVerifier(host, runtime)
	if err != nil {
		return nil, err
	}
	probe, err := NewProbeVerifier(workspace)
	if err != nil {
		return nil, err
	}
	return newReadinessGate(live, probe)
}

func newReadinessGate(live liveReadinessVerifier, probe probeReadinessVerifier) (*ReadinessGate, error) {
	if live == nil || probe == nil {
		return nil, failure(InvalidObservation, "readiness", "filesystem verifier is nil")
	}
	return &ReadinessGate{live: live, probe: probe}, nil
}

// VerifyAndPublish invokes publish only after every filesystem readiness check
// succeeds. No READY state may be committed by this boundary on any verifier,
// cancellation, or final path-identity failure.
func (g *ReadinessGate) VerifyAndPublish(ctx context.Context, spec LiveSpec, publish ReadyPublisher) error {
	if publish == nil {
		return failure(InvalidObservation, "readiness", "READY publisher is nil")
	}
	observed, err := g.live.Verify(ctx, spec)
	if err != nil {
		return err
	}
	if err := g.probe.Verify(ctx, spec.Runtime.Plan, observed.Docker); err != nil {
		return err
	}
	if err := verifyReadinessPathIdentity(spec); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return failure(ProbeFailed, "readiness", "filesystem readiness was canceled")
	}
	return publish(ctx, observed)
}

func verifyReadinessPathIdentity(spec LiveSpec) error {
	hostPaths := make([]string, 0, len(spec.Runtime.RuntimeHost.Mounts)+1)
	hostPaths = append(hostPaths, spec.Runtime.RuntimeHost.ImagePath)
	for _, mount := range spec.Runtime.RuntimeHost.Mounts {
		hostPaths = append(hostPaths, mount.Path)
	}
	if err := ownership.VerifyPathSet(hostPaths, spec.Runtime.RuntimeHost.PathEvidence); err != nil {
		return failure(InvalidObservation, "readiness-paths", "runtime-host path identity changed")
	}

	composePaths := make([]string, 0, len(spec.Runtime.Plan.Mounts)+len(spec.Runtime.Plan.Compose.Includes)+3)
	for _, mount := range spec.Runtime.Plan.Mounts {
		composePaths = append(composePaths, mount.Path)
	}
	for _, include := range spec.Runtime.Plan.Compose.Includes {
		composePaths = append(composePaths, include.Path)
	}
	composePaths = append(composePaths,
		spec.Runtime.Compiled.RootPath,
		spec.Runtime.Compiled.OverridePath,
		spec.Runtime.Compiled.ResolvedModelPath,
	)
	if err := ownership.VerifyPathSet(composePaths, spec.Runtime.PathEvidence); err != nil {
		return failure(InvalidObservation, "readiness-paths", "Compose path identity changed")
	}
	return nil
}
