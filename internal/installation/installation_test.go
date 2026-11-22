package installation_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/installation"
	"github.com/shoe-point-0/agent-box/internal/testutil"
	"github.com/shoe-point-0/agent-box/internal/toolchain"
)

type artifactDocument struct {
	Version string          `json:"version"`
	Digest  contract.Digest `json:"digest"`
	Path    string          `json:"path"`
}

type installationDocument struct {
	SchemaVersion string `json:"schemaVersion"`
	Artifacts     struct {
		Lima       artifactDocument            `json:"lima"`
		GuestImage artifactDocument            `json:"guestImage"`
		Docker     artifactDocument            `json:"docker"`
		Compose    artifactDocument            `json:"compose"`
		Utilities  map[string]artifactDocument `json:"utilities"`
	} `json:"artifacts"`
	Guest struct {
		DockerPath    string            `json:"dockerPath"`
		ComposePath   string            `json:"composePath"`
		UtilityPaths  map[string]string `json:"utilityPaths"`
		Workdir       string            `json:"workdir"`
		DockerSocket  string            `json:"dockerSocket"`
		DockerService string            `json:"dockerService"`
	} `json:"guest"`
}

func TestLoadVerifiesCompleteOfflineInstallation(t *testing.T) {
	path, document := writeInstallation(t)

	loaded, err := installation.Load(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Toolchain.Lima.Identity().Component != toolchain.Lima ||
		loaded.Toolchain.GuestImage.Identity().Component != toolchain.GuestImage ||
		loaded.Toolchain.Docker.Identity().Version != document.Artifacts.Docker.Version ||
		loaded.Toolchain.Compose.Identity().Version != document.Artifacts.Compose.Version {
		t.Fatalf("verified standard artifacts = %#v", loaded.Toolchain)
	}
	if len(loaded.Toolchain.Utilities) != len(document.Artifacts.Utilities) ||
		loaded.GuestImagePath != document.Artifacts.GuestImage.Path ||
		loaded.DockerPath != document.Guest.DockerPath || loaded.ComposePath != document.Guest.ComposePath ||
		loaded.Workdir != document.Guest.Workdir || loaded.DockerSocket != document.Guest.DockerSocket ||
		loaded.DockerService != "docker.service" {
		t.Fatalf("loaded runtime = %#v", loaded)
	}
	loaded.UtilityPaths["cat"] = "/changed"
	if document.Guest.UtilityPaths["cat"] == "/changed" {
		t.Fatal("Load returned aliased utility paths")
	}
}

func TestLoadRejectsMalformedMetadataBeforeArtifactUse(t *testing.T) {
	validPath, document := writeInstallation(t)
	valid, err := os.ReadFile(validPath)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(validPath)

	tests := []struct {
		name string
		data []byte
	}{
		{name: "unknown-field", data: []byte(strings.TrimSuffix(string(valid), "\n")[:len(strings.TrimSuffix(string(valid), "\n"))-1] + `,"unknown":true}`)},
		{name: "trailing-value", data: append(append([]byte(nil), valid...), []byte(` {}`)...)},
		{name: "unsupported-schema", data: mutateDocument(t, document, func(value *installationDocument) { value.SchemaVersion = "abox.installation/v2" })},
		{name: "relative-artifact", data: mutateDocument(t, document, func(value *installationDocument) { value.Artifacts.Lima.Path = "limactl" })},
		{name: "missing-utility", data: mutateDocument(t, document, func(value *installationDocument) {
			delete(value.Artifacts.Utilities, "test")
			delete(value.Guest.UtilityPaths, "test")
		})},
		{name: "utility-set-drift", data: mutateDocument(t, document, func(value *installationDocument) { delete(value.Guest.UtilityPaths, "cat") })},
		{name: "unsafe-guest-path", data: mutateDocument(t, document, func(value *installationDocument) { value.Guest.DockerPath = "docker" })},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(root, test.name+".json")
			if err := os.WriteFile(path, test.data, 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := installation.Load(t.Context(), path)
			var configured *installation.Error
			if !errors.As(err, &configured) || configured.ErrorCode() != contract.CodePrerequisiteFailed {
				t.Fatalf("Load() error = %v, want typed prerequisite error", err)
			}
		})
	}
}

func TestLoadRejectsArtifactDigestMismatchAndUnsafeMetadataPath(t *testing.T) {
	path, document := writeInstallation(t)
	document.Artifacts.Docker.Digest = contract.Digest("sha256:" + strings.Repeat("0", 64))
	if err := os.WriteFile(path, mustJSON(t, document), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := installation.Load(t.Context(), path)
	var prerequisite *toolchain.PrerequisiteError
	if !errors.As(err, &prerequisite) || prerequisite.Component != toolchain.Docker || prerequisite.Reason != toolchain.DigestMismatch {
		t.Fatalf("digest mismatch error = %v", err)
	}

	link := filepath.Join(filepath.Dir(path), "installation-link.json")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := installation.Load(t.Context(), link); err == nil {
		t.Fatal("Load() accepted symlinked metadata")
	}
	if _, err := installation.Load(t.Context(), filepath.Base(path)); err == nil {
		t.Fatal("Load() accepted relative metadata path")
	}
}

func TestLoadHonorsCancellationBeforeReadingMetadata(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := installation.Load(ctx, "/does/not/exist")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Load() error = %v, want cancellation", err)
	}
}

func writeInstallation(t *testing.T) (string, installationDocument) {
	t.Helper()
	root := testutil.TempDir(t)
	artifact := func(name, version string, executable bool) artifactDocument {
		mode := os.FileMode(0o600)
		if executable {
			mode = 0o700
		}
		data := []byte("fixture-" + name + "\n")
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, data, mode); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		return artifactDocument{Version: version, Digest: contract.Digest(fmt.Sprintf("sha256:%x", digest)), Path: path}
	}

	var document installationDocument
	document.SchemaVersion = installation.SchemaVersion
	document.Artifacts.Lima = artifact("limactl", "2.0.0", true)
	document.Artifacts.GuestImage = artifact("guest-image", "2026.08", false)
	document.Artifacts.Docker = artifact("docker", "29.6.1", true)
	document.Artifacts.Compose = artifact("docker-compose", "5.3.0", true)
	document.Artifacts.Utilities = make(map[string]artifactDocument)
	document.Guest.UtilityPaths = make(map[string]string)
	for _, name := range []string{"cat", "sha256sum", "stat", "systemctl", "test"} {
		document.Artifacts.Utilities[name] = artifact(name, "fixture-1", true)
		document.Guest.UtilityPaths[name] = "/usr/bin/" + name
	}
	document.Guest.DockerPath = "/usr/bin/docker"
	document.Guest.ComposePath = "/usr/libexec/docker/cli-plugins/docker-compose"
	document.Guest.Workdir = "/tmp"
	document.Guest.DockerSocket = "/var/run/docker.sock"
	document.Guest.DockerService = "docker.service"
	path := filepath.Join(root, "installation.json")
	if err := os.WriteFile(path, mustJSON(t, document), 0o600); err != nil {
		t.Fatal(err)
	}
	return path, document
}

func mutateDocument(t *testing.T, document installationDocument, mutate func(*installationDocument)) []byte {
	t.Helper()
	encoded := mustJSON(t, document)
	var cloned installationDocument
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		t.Fatal(err)
	}
	mutate(&cloned)
	return mustJSON(t, cloned)
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return append(data, '\n')
}
