package machine_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/machine"
	"github.com/shoe-point-0/agent-box/internal/toolchain"
	"github.com/shoe-point-0/agent-box/internal/toolchain/toolchaintest"
)

var supported = machine.Platform{OS: "linux", Architecture: "amd64", Accelerator: "qemu"}

type observerFunc func(context.Context) (machine.Observation, error)

func (function observerFunc) Observe(ctx context.Context) (machine.Observation, error) {
	return function(ctx)
}

type countingObserver struct {
	observation machine.Observation
	err         error
	calls       int
}

func (observer *countingObserver) Observe(context.Context) (machine.Observation, error) {
	observer.calls++
	return observer.observation, observer.err
}

func TestCheckReturnsDeterministicDoctorFactsAndExactVersions(t *testing.T) {
	specification, observation := fixture(t)
	observer := observerFunc(func(context.Context) (machine.Observation, error) { return observation, nil })

	first, err := machine.Check(t.Context(), specification, observer)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	second, err := machine.Check(t.Context(), specification, observer)
	if err != nil {
		t.Fatalf("second Check() error = %v", err)
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("repeated results differ:\n%s\n%s", firstJSON, secondJSON)
	}
	if len(first.Facts) != 5 {
		t.Fatalf("facts = %#v, want artifacts, platform, two paths, and router", first.Facts)
	}
	for _, fact := range first.Facts {
		if fact.Status != machine.StatusPass || fact.Observed == "" || fact.Required == "" {
			t.Errorf("non-actionable passing fact = %#v", fact)
		}
	}
	wantVersions := map[string]string{
		"lima/": "1.2.3", "guest-image/": "2026.08", "docker/": "29.6.1", "compose/": "5.3.0", "utility/git": "2.43.0",
	}
	if len(first.Versions) != len(wantVersions) {
		t.Fatalf("versions = %#v", first.Versions)
	}
	for _, version := range first.Versions {
		key := string(version.Component) + "/" + version.Name
		if wantVersions[key] != version.Version {
			t.Errorf("version %s = %q, want %q", key, version.Version, wantVersions[key])
		}
	}
	if first.Artifacts.Lima.Identity().Version != "1.2.3" {
		t.Fatal("result did not retain the verified opaque artifacts")
	}
}

func TestEveryPrerequisiteFailureStopsBeforeMutation(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, *machine.Specification, *machine.Observation)
		reason machine.Reason
		code   contract.ErrorCode
	}{
		{
			name: "unsupported platform", reason: machine.UnsupportedPlatform, code: contract.CodeMachineUnsupported,
			mutate: func(_ *testing.T, _ *machine.Specification, observation *machine.Observation) {
				observation.Platform.Architecture = "riscv64"
			},
		},
		{
			name: "state path unavailable", reason: machine.PathUnavailable, code: contract.CodePrerequisiteFailed,
			mutate: func(_ *testing.T, _ *machine.Specification, observation *machine.Observation) {
				observation.Paths[0].Usable = false
				observation.Paths[0].Detail = "parent is not searchable"
			},
		},
		{
			name: "runtime space insufficient", reason: machine.InsufficientSpace, code: contract.CodePrerequisiteFailed,
			mutate: func(_ *testing.T, _ *machine.Specification, observation *machine.Observation) {
				observation.Paths[1].AvailableBytes = 1023
			},
		},
		{
			name: "router occupied", reason: machine.RouterUnavailable, code: contract.CodePrerequisiteFailed,
			mutate: func(_ *testing.T, _ *machine.Specification, observation *machine.Observation) {
				observation.RouterAvailable = false
				observation.RouterDetail = "address already in use"
			},
		},
		{
			name: "artifact digest mismatch", reason: machine.ArtifactInvalid, code: contract.CodePrerequisiteFailed,
			mutate: func(t *testing.T, specification *machine.Specification, _ *machine.Observation) {
				_, entries := toolchaintest.Fixture(t)
				entries[0].Candidate.Identity = specification.Toolchain.Lima
				entries[0].Candidate.Path = filepath.Join(t.TempDir(), "changed-lima")
				if err := os.WriteFile(entries[0].Candidate.Path, []byte("wrong\n"), 0o700); err != nil {
					t.Fatal(err)
				}
				for index := 1; index < len(entries); index++ {
					switch entries[index].Component {
					case toolchain.GuestImage:
						entries[index].Candidate.Identity = specification.Toolchain.GuestImage
					case toolchain.Docker:
						entries[index].Candidate.Identity = specification.Toolchain.Docker
					case toolchain.Compose:
						entries[index].Candidate.Identity = specification.Toolchain.Compose
					case toolchain.Utility:
						entries[index].Candidate.Identity = specification.Toolchain.Utilities[0]
					}
				}
				var err error
				specification.Provider, err = toolchain.NewPreinstalledProvider(entries)
				if err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			specification, observation := fixture(t)
			test.mutate(t, &specification, &observation)
			mutations := 0
			err := validateThenMutate(t.Context(), specification, observerFunc(func(context.Context) (machine.Observation, error) {
				return observation, nil
			}), func(machine.Result) { mutations++ })
			assertFailure(t, err, test.reason, test.code)
			if mutations != 0 {
				t.Fatalf("mutation calls = %d, want 0", mutations)
			}
			var failure *machine.PrerequisiteError
			_ = errors.As(err, &failure)
			if failure.Fact.Name == "" || failure.Fact.Observed == "" || failure.Fact.Required == "" || failure.Fact.Remediation == "" {
				t.Fatalf("failure fact is not doctor-compatible: %#v", failure.Fact)
			}
		})
	}
}

func TestArtifactFailureOccursBeforeMachineObservation(t *testing.T) {
	specification, observation := fixture(t)
	specification.Toolchain.Lima.Digest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	observer := &countingObserver{observation: observation}
	_, err := machine.Check(t.Context(), specification, observer)
	assertFailure(t, err, machine.ArtifactInvalid, contract.CodePrerequisiteFailed)
	if observer.calls != 0 {
		t.Fatalf("observer calls = %d, want 0 before artifact verification passes", observer.calls)
	}
}

func TestMalformedOrFailedObservationFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name     string
		mutate   func(*machine.Observation)
		observer observerFunc
	}{
		{name: "missing path", mutate: func(observation *machine.Observation) { observation.Paths = observation.Paths[:1] }},
		{name: "wrong router", mutate: func(observation *machine.Observation) { observation.RouterAddress = "127.0.0.1:8888" }},
		{name: "collector error", observer: func(context.Context) (machine.Observation, error) {
			return machine.Observation{}, errors.New("statfs denied")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			specification, observation := fixture(t)
			if test.mutate != nil {
				test.mutate(&observation)
			}
			observer := test.observer
			if observer == nil {
				observer = func(context.Context) (machine.Observation, error) { return observation, nil }
			}
			_, err := machine.Check(t.Context(), specification, observer)
			assertFailure(t, err, machine.ObservationFailed, contract.CodePrerequisiteFailed)
		})
	}
}

func TestCancellationStopsPreflight(t *testing.T) {
	specification, observation := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	observer := &countingObserver{observation: observation}
	_, err := machine.Check(ctx, specification, observer)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Check() error = %v, want context.Canceled", err)
	}
	if observer.calls != 0 {
		t.Fatalf("observer calls = %d, want 0", observer.calls)
	}
}

func validateThenMutate(ctx context.Context, specification machine.Specification, observer machine.Observer, mutate func(machine.Result)) error {
	result, err := machine.Check(ctx, specification, observer)
	if err != nil {
		return err
	}
	mutate(result)
	return nil
}

func fixture(t *testing.T) (machine.Specification, machine.Observation) {
	t.Helper()
	toolchainSpecification, entries := toolchaintest.Fixture(t)
	provider, err := toolchain.NewPreinstalledProvider(entries)
	if err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(t.TempDir(), "state")
	runtimePath := filepath.Join(t.TempDir(), "runtime")
	specification := machine.Specification{
		Supported: []machine.Platform{supported},
		Paths: []machine.PathRequirement{
			{Name: "state", Path: statePath, MinFreeBytes: 4096},
			{Name: "runtime", Path: runtimePath, MinFreeBytes: 1024},
		},
		RouterAddress: "127.0.0.1:7777",
		Toolchain:     toolchainSpecification,
		Provider:      provider,
	}
	observation := machine.Observation{
		Platform: supported,
		Paths: []machine.PathObservation{
			{Name: "state", Path: statePath, Usable: true, AvailableBytes: 1 << 30},
			{Name: "runtime", Path: runtimePath, Usable: true, AvailableBytes: 1 << 20},
		},
		RouterAddress:   specification.RouterAddress,
		RouterAvailable: true,
	}
	return specification, observation
}

func assertFailure(t *testing.T, err error, reason machine.Reason, code contract.ErrorCode) {
	t.Helper()
	var failure *machine.PrerequisiteError
	if !errors.As(err, &failure) || failure.Fact.Reason != reason || failure.ErrorCode() != code {
		t.Fatalf("error = %#v, want Machine failure reason=%q code=%q", err, reason, code)
	}
}
