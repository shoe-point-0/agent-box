package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallationMetadataNeverClaimsRuntimeReadiness(t *testing.T) {
	path := filepath.Join(t.TempDir(), "installation.json")
	check := installationMetadata(path)
	if check.Status != StatusAdvisory || check.Evidence != EvidenceUnknown || !strings.Contains(check.Observed, "unavailable") || !strings.Contains(check.Observed, path) {
		t.Fatalf("missing installation: %#v", check)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("doctor created metadata: %v", err)
	}
	if err := os.WriteFile(path, []byte("not valid JSON"), 0o600); err != nil {
		t.Fatal(err)
	}
	check = installationMetadata(path)
	if check.Status != StatusAdvisory || check.Evidence != EvidenceUnknown || !strings.Contains(check.Observed, "not verified") {
		t.Fatalf("unverified installation: %#v", check)
	}
}
