package toolchain_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/process"
	"github.com/shoe-point-0/agent-box/internal/toolchain"
	"github.com/shoe-point-0/agent-box/internal/toolchain/toolchaintest"
)

func TestPreinstalledProviderContract(t *testing.T) {
	toolchaintest.Run(t, func(t *testing.T, entries []toolchain.Entry) toolchain.Provider {
		t.Helper()
		provider, err := toolchain.NewPreinstalledProvider(entries)
		if err != nil {
			t.Fatalf("NewPreinstalledProvider() error = %v", err)
		}
		return provider
	})
}

func TestInjectedFixtureProviderContract(t *testing.T) {
	toolchaintest.Run(t, func(_ *testing.T, entries []toolchain.Entry) toolchain.Provider {
		candidates := make(map[string]toolchain.Candidate, len(entries))
		for _, entry := range entries {
			candidates[fixtureKey(entry.Component, entry.Name)] = entry.Candidate
		}
		return fixtureProvider{candidates: candidates}
	})
}

type fixtureProvider struct {
	candidates map[string]toolchain.Candidate
}

func (p fixtureProvider) Resolve(ctx context.Context, expected toolchain.Identity) (toolchain.Candidate, error) {
	if err := ctx.Err(); err != nil {
		return toolchain.Candidate{}, err
	}
	candidate, ok := p.candidates[fixtureKey(expected.Component, expected.Name)]
	if !ok {
		return toolchain.Candidate{}, os.ErrNotExist
	}
	return candidate, nil
}

func fixtureKey(component toolchain.Component, name string) string {
	return string(component) + "\x00" + name
}

func TestArtifactRunRechecksDigestBeforeExecution(t *testing.T) {
	specification, entries := toolchaintest.Fixture(t)
	provider, err := toolchain.NewPreinstalledProvider(entries)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := toolchain.Verify(t.Context(), specification, provider)
	if err != nil {
		t.Fatal(err)
	}

	runner := &recordingRunner{}
	if _, err := verified.Lima.Run(t.Context(), runner, toolchain.Invocation{
		Args: []string{"--version"}, Env: map[string]string{"LANG": "C"},
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if runner.calls != 1 || len(runner.command.Argv) != 2 || runner.command.Argv[1] != "--version" {
		t.Fatalf("runner calls/argv = %d/%q", runner.calls, runner.command.Argv)
	}

	if err := os.WriteFile(entries[0].Candidate.Path, []byte("replacement\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err = verified.Lima.Run(t.Context(), runner, toolchain.Invocation{})
	assertPrerequisite(t, err, toolchain.DigestMismatch)
	if runner.calls != 1 {
		t.Fatalf("runner calls after digest mismatch = %d, want 1", runner.calls)
	}
}

func TestVerifyArtifactUsesTheSameFailClosedContract(t *testing.T) {
	specification, entries := toolchaintest.Fixture(t)
	provider, err := toolchain.NewPreinstalledProvider(entries)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := toolchain.VerifyArtifact(t.Context(), specification.Lima, provider)
	if err != nil || artifact.Identity() != specification.Lima {
		t.Fatalf("VerifyArtifact() = %#v, %v", artifact.Identity(), err)
	}

	changed := specification.Lima
	changed.Digest = contract.Digest("sha256:" + strings.Repeat("c", 64))
	if _, err := toolchain.VerifyArtifact(t.Context(), changed, provider); err == nil {
		t.Fatal("VerifyArtifact() accepted identity drift")
	}
	counting := &countingProvider{}
	invalid := specification.Lima
	invalid.Digest = "invalid"
	_, err = toolchain.VerifyArtifact(t.Context(), invalid, counting)
	assertPrerequisite(t, err, toolchain.InvalidSpecification)
	if counting.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", counting.calls)
	}
}

func TestGuestImageCannotExecute(t *testing.T) {
	specification, entries := toolchaintest.Fixture(t)
	provider, err := toolchain.NewPreinstalledProvider(entries)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := toolchain.Verify(t.Context(), specification, provider)
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{}
	_, err = verified.GuestImage.Run(t.Context(), runner, toolchain.Invocation{})
	assertPrerequisite(t, err, toolchain.UnsafeArtifact)
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want 0", runner.calls)
	}
}

func TestUnverifiedArtifactCannotExecute(t *testing.T) {
	runner := &recordingRunner{}
	var artifact toolchain.Artifact
	_, err := artifact.Run(t.Context(), runner, toolchain.Invocation{})
	assertPrerequisite(t, err, toolchain.UnsafeArtifact)
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want 0", runner.calls)
	}
}

func TestInvalidSpecificationFailsBeforeProvider(t *testing.T) {
	specification, _ := toolchaintest.Fixture(t)
	specification.Lima.Digest = "sha256:ABC"
	provider := &countingProvider{}
	_, err := toolchain.Verify(t.Context(), specification, provider)
	assertPrerequisite(t, err, toolchain.InvalidSpecification)
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", provider.calls)
	}
}

func TestUnsafeCandidatesFailClosed(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, *toolchain.Entry)
	}{
		{name: "relative path", mutate: func(_ *testing.T, entry *toolchain.Entry) { entry.Candidate.Path = "relative" }},
		{name: "non executable", mutate: func(t *testing.T, entry *toolchain.Entry) {
			if err := os.Chmod(entry.Candidate.Path, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			specification, entries := toolchaintest.Fixture(t)
			test.mutate(t, &entries[0])
			provider, err := toolchain.NewPreinstalledProvider(entries)
			if err != nil {
				t.Fatal(err)
			}
			_, err = toolchain.Verify(t.Context(), specification, provider)
			assertPrerequisite(t, err, toolchain.UnsafeArtifact)
		})
	}
}

func TestProviderConfigurationRejectsDuplicateSelectors(t *testing.T) {
	_, entries := toolchaintest.Fixture(t)
	_, err := toolchain.NewPreinstalledProvider(append(entries, entries[0]))
	if err == nil {
		t.Fatal("NewPreinstalledProvider() succeeded with duplicate selector")
	}
}

type recordingRunner struct {
	calls   int
	command process.Command
}

func (r *recordingRunner) Run(_ context.Context, command process.Command) (process.Result, error) {
	r.calls++
	r.command = command
	return process.Result{ExitCode: 0}, nil
}

type countingProvider struct{ calls int }

func (p *countingProvider) Resolve(context.Context, toolchain.Identity) (toolchain.Candidate, error) {
	p.calls++
	return toolchain.Candidate{}, errors.New("unexpected call")
}

func assertPrerequisite(t *testing.T, err error, reason toolchain.Reason) {
	t.Helper()
	var prerequisite *toolchain.PrerequisiteError
	if !errors.As(err, &prerequisite) || prerequisite.Reason != reason || prerequisite.ErrorCode() != contract.CodePrerequisiteFailed {
		t.Fatalf("error = %#v, want typed prerequisite reason %q", err, reason)
	}
}
