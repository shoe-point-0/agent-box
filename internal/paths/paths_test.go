package paths

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/identity"
)

func TestDarwinLayout(t *testing.T) {
	t.Parallel()

	provider, err := Darwin("/Users/shoe")
	if err != nil {
		t.Fatalf("Darwin() error = %v", err)
	}
	boxID := identity.BoxID("auth-142-itjwb74g")

	checks := map[string]struct {
		got  string
		want string
	}{
		"application":    {provider.ApplicationRoot(), "/Users/shoe/Library/Application Support/Agent Box"},
		"cache":          {provider.CacheRoot(), "/Users/shoe/Library/Caches/Agent Box"},
		"log":            {provider.LogRoot(), "/Users/shoe/Library/Logs/Agent Box"},
		"runtime":        {provider.RuntimeRoot(), "/Users/shoe/Library/Application Support/Agent Box/runtime"},
		"state database": {provider.StateDatabase(), "/Users/shoe/Library/Application Support/Agent Box/state.db"},
		"boxes":          {provider.BoxesRoot(), "/Users/shoe/Library/Application Support/Agent Box/boxes"},
		"repositories":   {provider.RepositoriesRoot(), "/Users/shoe/Library/Application Support/Agent Box/repositories"},
		"tools":          {provider.ToolsRoot(), "/Users/shoe/Library/Application Support/Agent Box/tools"},
		"downloads":      {provider.DownloadsRoot(), "/Users/shoe/Library/Caches/Agent Box/downloads"},
		"image metadata": {provider.ImageMetadataRoot(), "/Users/shoe/Library/Caches/Agent Box/image-metadata"},
		"daemon log":     {provider.DaemonLog(), "/Users/shoe/Library/Logs/Agent Box/aboxd.log"},
		"daemon socket":  {provider.DaemonSocket(), "/Users/shoe/Library/Application Support/Agent Box/runtime/aboxd.sock"},
		"daemon PID":     {provider.DaemonPID(), "/Users/shoe/Library/Application Support/Agent Box/runtime/aboxd.pid"},
	}

	boxChecks := map[string]struct {
		get  func(identity.BoxID) (string, error)
		want string
	}{
		"box":              {provider.BoxRoot, ""},
		"plan":             {provider.Plan, "plan.json"},
		"context":          {provider.Context, "context.json"},
		"Lima config":      {provider.LimaConfig, "lima.yaml"},
		"Compose root":     {provider.ComposeRoot, "compose.root.yaml"},
		"Compose override": {provider.ComposeOverride, "compose.abox.yaml"},
		"Compose resolved": {provider.ComposeResolved, "compose.resolved.redacted.json"},
		"diagnostics":      {provider.DiagnosticsRoot, "diagnostics"},
		"Git":              {provider.GitRoot, "git"},
		"workspace":        {provider.WorkspaceRoot, "workspace"},
	}
	boxRoot := "/Users/shoe/Library/Application Support/Agent Box/boxes/auth-142-itjwb74g"
	for name, check := range boxChecks {
		got, getErr := check.get(boxID)
		if getErr != nil {
			t.Fatalf("%s path error = %v", name, getErr)
		}
		want := boxRoot
		if check.want != "" {
			want = filepath.Join(boxRoot, check.want)
		}
		checks[name] = struct{ got, want string }{got, want}
	}

	gitStore, err := provider.GitStore(boxID, "payments")
	if err != nil {
		t.Fatalf("GitStore() error = %v", err)
	}
	checks["Git store"] = struct{ got, want string }{gitStore, filepath.Join(boxRoot, "git", "payments.git")}
	workspacePath, err := provider.WorkspacePath(boxID, "services/payments")
	if err != nil {
		t.Fatalf("WorkspacePath() error = %v", err)
	}
	checks["workspace path"] = struct{ got, want string }{workspacePath, filepath.Join(boxRoot, "workspace", "services", "payments")}

	for name, check := range checks {
		if check.got != check.want {
			t.Errorf("%s path = %q, want %q", name, check.got, check.want)
		}
	}
}

func TestWorkspacePathAllowsRequiredNamesAndRejectsEscape(t *testing.T) {
	t.Parallel()

	provider, err := New("/tmp/application", "/tmp/cache", "/tmp/log")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	boxID := identity.BoxID("task-1-abcdefgh")

	for _, relative := range []string{"services/payment API", "应用/portal", strings.Repeat("a", maxComponentBytes)} {
		path, pathErr := provider.WorkspacePath(boxID, relative)
		if pathErr != nil {
			t.Errorf("WorkspacePath(%q) error = %v", relative, pathErr)
			continue
		}
		root, rootErr := provider.WorkspaceRoot(boxID)
		if rootErr != nil {
			t.Fatalf("WorkspaceRoot() error = %v", rootErr)
		}
		if !isWithin(root, path) {
			t.Errorf("WorkspacePath(%q) = %q outside %q", relative, path, root)
		}
	}

	invalid := []string{"", ".", "..", "../sibling", "services/../sibling", "/absolute", "services//payments", "services\\payments", "bad\x00name", "bad\nname", strings.Repeat("a", maxComponentBytes+1)}
	for _, relative := range invalid {
		if path, pathErr := provider.WorkspacePath(boxID, relative); pathErr == nil {
			t.Errorf("WorkspacePath(%q) = %q, want rejection", relative, path)
		}
	}
}

func TestProviderRejectsInvalidRootsAndStateComponents(t *testing.T) {
	t.Parallel()

	for _, root := range []string{"", "relative", "/tmp/../tmp/root", "/tmp/bad\x00root"} {
		if _, err := New(root, "/tmp/cache", "/tmp/log"); err == nil {
			t.Errorf("New(%q) error = nil, want rejection", root)
		}
	}
	if _, err := Darwin("relative-home"); err == nil {
		t.Error("Darwin(relative home) error = nil, want rejection")
	}

	provider, err := New("/tmp/application", "/tmp/cache", "/tmp/log")
	if err != nil {
		t.Fatalf("New(valid) error = %v", err)
	}
	boxID := identity.BoxID("task-1-abcdefgh")
	for _, component := range []string{"", ".", "..", "../repo", "repo/name", "repo name", "répo", "repo-", strings.Repeat("a", maxComponentBytes)} {
		if path, pathErr := provider.GitStore(boxID, component); pathErr == nil {
			t.Errorf("GitStore(%q) = %q, want rejection", component, path)
		}
		if path, pathErr := provider.RepositoryCache(component); pathErr == nil {
			t.Errorf("RepositoryCache(%q) = %q, want rejection", component, path)
		}
		if path, pathErr := provider.ToolRoot(component); pathErr == nil {
			t.Errorf("ToolRoot(%q) = %q, want rejection", component, path)
		}
	}
	if _, err := provider.BoxRoot(identity.BoxID("../../forged")); err == nil {
		t.Error("BoxRoot(forged ID) error = nil, want rejection")
	}
}
