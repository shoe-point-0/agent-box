// Package toolchaintest provides the provider contract shared by offline
// fixtures, preinstalled artifacts, and future acquisition providers.
package toolchaintest

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/toolchain"
)

type Factory func(*testing.T, []toolchain.Entry) toolchain.Provider

// Run executes the provider-independent verification contract.
func Run(t *testing.T, factory Factory) {
	t.Helper()

	t.Run("complete immutable set", func(t *testing.T) {
		specification, entries := Fixture(t)
		verified, err := toolchain.Verify(t.Context(), specification, factory(t, entries))
		if err != nil {
			t.Fatalf("Verify() error = %v", err)
		}
		if verified.Lima.Identity() != specification.Lima || verified.GuestImage.Identity() != specification.GuestImage ||
			verified.Docker.Identity() != specification.Docker || verified.Compose.Identity() != specification.Compose {
			t.Fatal("Verify() returned identities different from the specification")
		}
		if got := verified.Utilities["git"].Identity(); got != specification.Utilities[0] {
			t.Fatalf("Git utility identity = %#v, want %#v", got, specification.Utilities[0])
		}
		file, err := verified.GuestImage.Open(t.Context())
		if err != nil {
			t.Fatalf("GuestImage.Open() error = %v", err)
		}
		_ = file.Close()
	})

	t.Run("missing candidate", func(t *testing.T) {
		specification, entries := Fixture(t)
		_, err := toolchain.Verify(t.Context(), specification, factory(t, entries[:len(entries)-1]))
		assertReason(t, err, toolchain.MissingArtifact)
	})

	t.Run("provider identity mismatch", func(t *testing.T) {
		specification, entries := Fixture(t)
		entries[0].Candidate.Identity.Version += "-wrong"
		_, err := toolchain.Verify(t.Context(), specification, factory(t, entries))
		assertReason(t, err, toolchain.IdentityMismatch)
	})

	t.Run("content mismatch", func(t *testing.T) {
		specification, entries := Fixture(t)
		if err := os.WriteFile(entries[0].Candidate.Path, []byte("changed\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		_, err := toolchain.Verify(t.Context(), specification, factory(t, entries))
		assertReason(t, err, toolchain.DigestMismatch)
	})

	t.Run("cancellation", func(t *testing.T) {
		specification, entries := Fixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := toolchain.Verify(ctx, specification, factory(t, entries)); err != context.Canceled {
			t.Fatalf("Verify() error = %v, want context.Canceled", err)
		}
	})
}

func Fixture(t *testing.T) (toolchain.Specification, []toolchain.Entry) {
	t.Helper()
	root := t.TempDir()
	identities := []toolchain.Identity{
		{Component: toolchain.Lima, Version: "1.2.3"},
		{Component: toolchain.GuestImage, Version: "2026.08"},
		{Component: toolchain.Docker, Version: "29.6.1"},
		{Component: toolchain.Compose, Version: "5.3.0"},
		{Component: toolchain.Utility, Name: "git", Version: "2.43.0"},
	}
	entries := make([]toolchain.Entry, 0, len(identities))
	for index, identity := range identities {
		content := []byte(fmt.Sprintf("artifact-%d\n", index))
		digest := sha256.Sum256(content)
		identity.Digest = contract.Digest(fmt.Sprintf("sha256:%x", digest))
		identities[index] = identity
		path := filepath.Join(root, fmt.Sprintf("artifact-%d", index))
		mode := os.FileMode(0o700)
		if identity.Component == toolchain.GuestImage {
			mode = 0o600
		}
		if err := os.WriteFile(path, content, mode); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, toolchain.Entry{
			Component: identity.Component, Name: identity.Name,
			Candidate: toolchain.Candidate{Identity: identity, Path: path},
		})
	}
	return toolchain.Specification{
		Lima: identities[0], GuestImage: identities[1], Docker: identities[2], Compose: identities[3],
		Utilities: []toolchain.Identity{identities[4]},
	}, entries
}

func assertReason(t *testing.T, err error, want toolchain.Reason) {
	t.Helper()
	prerequisite, ok := err.(*toolchain.PrerequisiteError)
	if !ok || prerequisite.Reason != want || prerequisite.ErrorCode() != contract.CodePrerequisiteFailed {
		t.Fatalf("error = %#v, want typed prerequisite reason %q", err, want)
	}
}
