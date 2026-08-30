package filesystem

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/lima"
	"github.com/shoe-point-0/agent-box/internal/ownership"
)

func TestReadinessGatePublishesOnlyAfterLiveAndProbeVerification(t *testing.T) {
	spec, _ := readinessFixture(t)
	want := Observation{Docker: compose.RuntimeObservation{ProjectName: "box-one"}}
	calls := make([]string, 0, 3)
	gate, err := newReadinessGate(
		liveVerifierFunc(func(context.Context, LiveSpec) (Observation, error) {
			calls = append(calls, "live")
			return want, nil
		}),
		probeVerifierFunc(func(context.Context, contract.Plan, compose.RuntimeObservation) error {
			calls = append(calls, "probe")
			return nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	err = gate.VerifyAndPublish(t.Context(), spec, func(_ context.Context, observed Observation) error {
		calls = append(calls, "publish")
		if observed.Docker.ProjectName != want.Docker.ProjectName {
			t.Fatalf("published observation = %#v, want %#v", observed, want)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("VerifyAndPublish() error = %v", err)
	}
	if got := stringSequence(calls); got != "live,probe,publish" {
		t.Fatalf("call order = %q, want live,probe,publish", got)
	}
}

func TestReadinessGateNeverPublishesAfterVerifierFailure(t *testing.T) {
	tests := []struct {
		name       string
		liveError  error
		probeError error
		wantProbe  bool
	}{
		{name: "live mount or canary failure", liveError: failure(MountSetMismatch, "guest-mountinfo", "injected")},
		{name: "workspace visibility or ownership failure", probeError: failure(ProbeFailed, "workspace-read", "filesystem probe failed"), wantProbe: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec, _ := readinessFixture(t)
			probeCalled := false
			published := false
			gate, err := newReadinessGate(
				liveVerifierFunc(func(context.Context, LiveSpec) (Observation, error) {
					return Observation{Docker: compose.RuntimeObservation{ProjectName: "box-one"}}, test.liveError
				}),
				probeVerifierFunc(func(context.Context, contract.Plan, compose.RuntimeObservation) error {
					probeCalled = true
					return test.probeError
				}),
			)
			if err != nil {
				t.Fatal(err)
			}
			err = gate.VerifyAndPublish(t.Context(), spec, func(context.Context, Observation) error {
				published = true
				return nil
			})
			if err == nil {
				t.Fatal("VerifyAndPublish() succeeded after injected verifier failure")
			}
			if published {
				t.Fatal("READY publisher ran after filesystem verification failed")
			}
			if probeCalled != test.wantProbe {
				t.Fatalf("probe called = %t, want %t", probeCalled, test.wantProbe)
			}
		})
	}
}

func TestReadinessGateRefusesEveryMutablePathClassImmediatelyBeforeReady(t *testing.T) {
	for _, target := range []string{"worktree", "git-store", "compose-include", "generated-compose"} {
		t.Run(target, func(t *testing.T) {
			spec, paths := readinessFixture(t)
			published := false
			gate, err := newReadinessGate(
				liveVerifierFunc(func(context.Context, LiveSpec) (Observation, error) {
					return Observation{Docker: compose.RuntimeObservation{ProjectName: "box-one"}}, nil
				}),
				probeVerifierFunc(func(context.Context, contract.Plan, compose.RuntimeObservation) error {
					replacePath(t, paths[target])
					return nil
				}),
			)
			if err != nil {
				t.Fatal(err)
			}
			err = gate.VerifyAndPublish(t.Context(), spec, func(context.Context, Observation) error {
				published = true
				return nil
			})
			var verifierError *Error
			if !errors.As(err, &verifierError) || verifierError.Layer != "readiness-paths" || verifierError.Kind != InvalidObservation {
				t.Fatalf("VerifyAndPublish() error = %v, want readiness path refusal", err)
			}
			if published {
				t.Fatal("READY publisher ran after path substitution")
			}
		})
	}
}

func TestReadinessGateRefusesCancellationAndInvalidPublisher(t *testing.T) {
	spec, _ := readinessFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	gate, err := newReadinessGate(
		liveVerifierFunc(func(context.Context, LiveSpec) (Observation, error) {
			return Observation{}, nil
		}),
		probeVerifierFunc(func(context.Context, contract.Plan, compose.RuntimeObservation) error {
			cancel()
			return nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := gate.VerifyAndPublish(ctx, spec, func(context.Context, Observation) error {
		t.Fatal("READY publisher ran after cancellation")
		return nil
	}); err == nil {
		t.Fatal("VerifyAndPublish() succeeded after cancellation")
	}
	if err := gate.VerifyAndPublish(t.Context(), spec, nil); err == nil {
		t.Fatal("VerifyAndPublish(nil publisher) succeeded")
	}
}

type liveVerifierFunc func(context.Context, LiveSpec) (Observation, error)

func (verify liveVerifierFunc) Verify(ctx context.Context, spec LiveSpec) (Observation, error) {
	return verify(ctx, spec)
}

type probeVerifierFunc func(context.Context, contract.Plan, compose.RuntimeObservation) error

func (verify probeVerifierFunc) Verify(ctx context.Context, plan contract.Plan, observed compose.RuntimeObservation) error {
	return verify(ctx, plan, observed)
}

func readinessFixture(t *testing.T) (LiveSpec, map[string]string) {
	t.Helper()
	root := t.TempDir()
	paths := map[string]string{
		"image":             filepath.Join(root, "guest.img"),
		"worktree":          filepath.Join(root, "workspace", "repo"),
		"git-store":         filepath.Join(root, "git", "repo.git"),
		"compose-include":   filepath.Join(root, "source", "compose.yaml"),
		"generated-compose": filepath.Join(root, "generated", compose.RootFileName),
		"override":          filepath.Join(root, "generated", compose.OverrideFileName),
		"resolved":          filepath.Join(root, "generated", compose.ResolvedModelFileName),
	}
	for _, directory := range []string{paths["worktree"], paths["git-store"]} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{paths["image"], paths["compose-include"], paths["generated-compose"], paths["override"], paths["resolved"]} {
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("fixture\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	mounts := []contract.MountPlan{
		{Path: paths["worktree"], Services: []string{"workspace"}, Writable: true},
		{Path: paths["git-store"], Services: []string{"workspace"}, Writable: true},
	}
	plan := contract.Plan{
		Mounts:  mounts,
		Compose: contract.ComposePlan{Includes: []contract.Artifact{{Path: paths["compose-include"]}}},
	}
	host := lima.Spec{Mounts: mounts, ImagePath: paths["image"]}
	var err error
	host.PathEvidence, err = ownership.RecordPathSet([]string{paths["image"], paths["worktree"], paths["git-store"]})
	if err != nil {
		t.Fatal(err)
	}
	runtime := compose.RuntimeSpec{
		RuntimeHost: host,
		Plan:        plan,
		Compiled: compose.Result{
			RootPath: paths["generated-compose"], OverridePath: paths["override"], ResolvedModelPath: paths["resolved"],
		},
	}
	runtime.PathEvidence, err = ownership.RecordPathSet([]string{
		paths["worktree"], paths["git-store"], paths["compose-include"],
		paths["generated-compose"], paths["override"], paths["resolved"],
	})
	if err != nil {
		t.Fatal(err)
	}
	return LiveSpec{Runtime: runtime}, paths
}

func replacePath(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".original"); err != nil {
		t.Fatal(err)
	}
	if info.IsDir() {
		err = os.Mkdir(path, info.Mode().Perm())
	} else {
		err = os.WriteFile(path, []byte("replacement\n"), info.Mode().Perm())
	}
	if err != nil {
		t.Fatal(err)
	}
}

func stringSequence(values []string) string {
	result := ""
	for index, value := range values {
		if index > 0 {
			result += ","
		}
		result += value
	}
	return result
}
