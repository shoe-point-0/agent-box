package manifest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocumentedWorkspaceExampleParses(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	manifestDir := filepath.Join(root, "workspace", ".abox")
	payments := filepath.Join(root, "workspace", "payment-service")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(payments, 0o755); err != nil {
		t.Fatal(err)
	}
	loader := mustLoader(t, []string{root}, DefaultLimits())
	workspace, err := loader.LoadBytes([]byte(documentedManifest), manifestDir)
	if err != nil {
		t.Fatalf("LoadBytes(documented example) error = %v", err)
	}
	if got, want := workspace.Metadata.Name, "billing-platform"; got != want {
		t.Errorf("metadata.name = %q, want %q", got, want)
	}
	if got, want := len(workspace.Spec.Repositories), 2; got != want {
		t.Fatalf("repository count = %d, want %d", got, want)
	}
	if got, want := workspace.Spec.Repositories[0].CanonicalLocal, payments; got != want {
		t.Errorf("canonical local = %q, want %q", got, want)
	}
	if got, want := workspace.Spec.Host.CPUMillicores, int64(6000); got != want {
		t.Errorf("CPU millicores = %d, want %d", got, want)
	}
	if got, want := workspace.Spec.Host.MemoryBytes, int64(12<<30); got != want {
		t.Errorf("memory bytes = %d, want %d", got, want)
	}
	if got, want := workspace.Spec.Routes[0].Readiness.TimeoutNanos, int64(90_000_000_000); got != want {
		t.Errorf("route timeout = %d, want %d", got, want)
	}
	var unsupported *FeatureNotSupportedError
	if err := CheckMilestone1Executable(workspace); !errors.As(err, &unsupported) || unsupported.Path != "spec.repositories[1].source.remote" {
		t.Errorf("CheckMilestone1Executable() error = %v, want remote feature error", err)
	}
}

func TestStrictStructureAndValidation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repository := filepath.Join(root, "repo")
	if err := os.Mkdir(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	loader := mustLoader(t, []string{root}, DefaultLimits())
	valid := minimalManifest(repository)

	tests := []struct {
		name    string
		input   string
		path    string
		problem string
	}{
		{"unknown field", strings.Replace(valid, "kind: Workspace", "kind: Workspace\nunknown: true", 1), "unknown", "unknown field"},
		{"duplicate field", strings.Replace(valid, "kind: Workspace", "kind: Workspace\nkind: Workspace", 1), "kind", "duplicate field"},
		{"wrong api version", strings.Replace(valid, "abox.dev/v1alpha1", "abox.dev/v2", 1), "apiVersion", "must be"},
		{"wrong kind", strings.Replace(valid, "kind: Workspace", "kind: Project", 1), "kind", "must be"},
		{"invalid workspace name", strings.Replace(valid, "name: demo", "name: Demo", 1), "metadata.name", "DNS-style"},
		{"repositories required", strings.Replace(valid, repositoryBlock(repository), "  repositories: []\n", 1), "spec.repositories", "at least one"},
		{"duplicate repository", strings.Replace(valid, repositoryBlock(repository), repositoryBlock(repository)+strings.TrimPrefix(repositoryBlock(repository), "  repositories:\n"), 1), "spec.repositories[1].name", "duplicates"},
		{"source exclusivity", strings.Replace(valid, "        local: "+repository, "        local: "+repository+"\n        remote: git@example/repo", 1), "spec.repositories[0].source", "exactly one"},
		{"checkout ref required", strings.Replace(valid, "ref: HEAD", "ref: ''", 1), "spec.repositories[0].checkout.ref", "required"},
		{"unsupported template", strings.Replace(valid, "abox/{{ box.task }}/repo", "abox/{{ shell }}/repo", 1), "spec.repositories[0].checkout.branch", "unsupported template"},
		{"invalid CPU", strings.Replace(valid, "cpus: 2", "cpus: nope", 1), "spec.host.cpus", "CPU quantity"},
		{"invalid memory", strings.Replace(valid, "memory: 4GiB", "memory: 4", 1), "spec.host.memory", "byte quantity"},
		{"invalid disk", strings.Replace(valid, "disk: 20GiB", "disk: -1GiB", 1), "spec.host.disk", "byte quantity"},
		{"unknown include repository", strings.Replace(valid, "repository: repo", "repository: missing", 1), "spec.compose.includes[0].repository", "unknown repository"},
		{"absolute include path", strings.Replace(valid, "path: compose.yaml", "path: /compose.yaml", 1), "spec.compose.includes[0].path", "relative path"},
		{"unknown workspace repository", strings.Replace(valid, "repository: repo\n    workingDirectory", "repository: missing\n    workingDirectory", 1), "spec.workspaceService.repository", "unknown repository"},
		{"unknown mount repository", strings.Replace(valid, "  mounts:\n    - repository: repo", "  mounts:\n    - repository: missing", 1), "spec.mounts[0].repository", "unknown repository"},
		{"mount services required", strings.Replace(valid, "services: [app]", "services: []", 1), "spec.mounts[0].services", "at least one"},
		{"mount mode", strings.Replace(valid, "mode: read-write", "mode: execute", 1), "spec.mounts[0].mode", "read-only"},
		{"typed integer path", valid + "  tcpPorts:\n    - {name: db, service: db, targetPort: nope, allocate: dynamic}\n", "spec.tcpPorts[0].targetPort", "integer"},
		{"argv only", valid + "  setup:\n    - name: install\n      service: app\n      command: npm install\n", "spec.setup[0].command", "sequence"},
		{"empty argv", valid + "  setup:\n    - name: install\n      service: app\n      command: []\n", "spec.setup[0].command", "argv"},
		{"secret literal rejected", valid + "  secrets:\n    - name: TOKEN\n      fromHostEnvironment: TOKEN\n      value: super-secret-value\n      exposeTo:\n        - service: app\n          as: environment\n", "spec.secrets[0].value", "unknown field"},
		{"policy value", strings.Replace(valid, "dockerSocket: deny", "dockerSocket: allow", 1), "spec.policy.dockerSocket", "unsupported"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := loader.LoadBytes([]byte(test.input), root)
			assertFieldError(t, err, test.path, test.problem)
			if strings.Contains(fmt.Sprint(err), "super-secret-value") {
				t.Fatalf("error exposed a secret value: %v", err)
			}
		})
	}
}

func TestNamedFeatureValidation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repository := filepath.Join(root, "repo")
	if err := os.Mkdir(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	loader := mustLoader(t, []string{root}, DefaultLimits())
	base := minimalManifest(repository)
	tests := []struct {
		name    string
		suffix  string
		path    string
		problem string
	}{
		{"cache duplicate", "  caches:\n    - {name: build, service: app, target: /cache/one}\n    - {name: build, service: app, target: /cache/two}\n", "spec.caches[1].name", "duplicates"},
		{"cache target", "  caches:\n    - {name: build, service: app, target: relative}\n", "spec.caches[0].target", "absolute"},
		{"setup duplicate", "  setup:\n    - {name: install, service: app, command: [npm]}\n    - {name: install, service: app, command: [npm]}\n", "spec.setup[1].name", "duplicates"},
		{"setup changed path", "  setup:\n    - name: install\n      service: app\n      command: [npm]\n      rerunWhenChanged: [../lock]\n", "spec.setup[0].rerunWhenChanged[0]", "relative path"},
		{"route duplicate", "  routes:\n    - &route {name: api, service: app, targetPort: 80, readiness: {http: {path: /, expectedStatus: 200}, timeout: 1s}}\n    - *route\n", "spec.routes[0]", "aliases"},
		{"route port", "  routes:\n    - {name: api, service: app, targetPort: 0, readiness: {http: {path: /, expectedStatus: 200}, timeout: 1s}}\n", "spec.routes[0].targetPort", "between"},
		{"route readiness", "  routes:\n    - {name: api, service: app, targetPort: 80, readiness: {timeout: 1s}}\n", "spec.routes[0].readiness.http", "required"},
		{"route status", "  routes:\n    - {name: api, service: app, targetPort: 80, readiness: {http: {path: /, expectedStatus: 999}, timeout: 1s}}\n", "spec.routes[0].readiness.http.expectedStatus", "between"},
		{"route timeout", "  routes:\n    - {name: api, service: app, targetPort: 80, readiness: {http: {path: /, expectedStatus: 200}, timeout: nope}}\n", "spec.routes[0].readiness.timeout", "duration"},
		{"TCP duplicate", "  tcpPorts:\n    - {name: db, service: db, targetPort: 5432, allocate: dynamic}\n    - {name: db, service: db, targetPort: 5433, allocate: dynamic}\n", "spec.tcpPorts[1].name", "duplicates"},
		{"TCP allocation", "  tcpPorts:\n    - {name: db, service: db, targetPort: 5432, allocate: fixed}\n", "spec.tcpPorts[0].allocate", "dynamic"},
		{"secret duplicate", "  secrets:\n    - {name: TOKEN, fromHostEnvironment: TOKEN, exposeTo: [{service: app, as: environment}]}\n    - {name: TOKEN, fromHostEnvironment: TOKEN, exposeTo: [{service: app, as: environment}]}\n", "spec.secrets[1].name", "duplicates"},
		{"secret reference", "  secrets:\n    - {name: TOKEN, fromHostEnvironment: bad-name, exposeTo: [{service: app, as: environment}]}\n", "spec.secrets[0].fromHostEnvironment", "host environment"},
		{"secret exposure", "  secrets:\n    - {name: TOKEN, fromHostEnvironment: TOKEN, exposeTo: [{service: app, as: file}]}\n", "spec.secrets[0].exposeTo[0].as", "environment"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := loader.LoadBytes([]byte(base+test.suffix), root)
			assertFieldError(t, err, test.path, test.problem)
		})
	}
}

func TestCanonicalPathBoundaries(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	allowed := filepath.Join(root, "allowed")
	outside := filepath.Join(root, "outside")
	for _, directory := range []string{allowed, filepath.Join(allowed, "repo"), outside} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(allowed, "escape")); err != nil {
		t.Fatal(err)
	}
	loader := mustLoader(t, []string{allowed}, DefaultLimits())
	tests := []struct {
		name   string
		source string
	}{
		{"parent traversal", "../outside"},
		{"symlink escape", filepath.Join(allowed, "escape")},
		{"absolute outside", outside},
		{"missing", filepath.Join(allowed, "missing")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := loader.LoadBytes([]byte(minimalManifest(test.source)), allowed)
			assertFieldError(t, err, "spec.repositories[0].source.local", "")
		})
	}
	workspace, err := loader.LoadBytes([]byte(minimalManifest(filepath.Join(allowed, "repo"))), allowed)
	if err != nil {
		t.Fatalf("valid source error = %v", err)
	}
	if workspace.Spec.Repositories[0].CanonicalLocal != filepath.Join(allowed, "repo") {
		t.Errorf("canonical source = %q", workspace.Spec.Repositories[0].CanonicalLocal)
	}
}

func TestParserLimits(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repository := filepath.Join(root, "repo")
	if err := os.Mkdir(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	valid := minimalManifest(repository)
	tests := []struct {
		name   string
		limits Limits
		input  string
		path   string
	}{
		{"input bytes", Limits{100, 32, 256, 4096, 4096}, valid, "$"},
		{"nesting", Limits{1 << 20, 2, 256, 4096, 4096}, valid, "metadata.name"},
		{"collection", Limits{1 << 20, 32, 1, 4096, 4096}, valid, "$"},
		{"path", Limits{1 << 20, 32, 256, 4, 4096}, valid, "spec.repositories[0].source.local"},
		{"template output", Limits{1 << 20, 32, 256, 4096, 10}, valid, "spec.repositories[0].checkout.branch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loader := mustLoader(t, []string{root}, test.limits)
			_, err := loader.LoadBytes([]byte(test.input), root)
			assertFieldError(t, err, test.path, "limit")
		})
	}
}

func TestMilestone1FeatureErrors(t *testing.T) {
	t.Parallel()
	features := []struct {
		name string
		set  func(*Workspace)
		path string
	}{
		{"remote", func(w *Workspace) { w.Spec.Repositories = []Repository{{Source: Source{Remote: "remote"}}} }, "spec.repositories[0].source.remote"},
		{"caches", func(w *Workspace) { w.Spec.Caches = []Cache{{}} }, "spec.caches"},
		{"setup", func(w *Workspace) { w.Spec.Setup = []SetupJob{{}} }, "spec.setup"},
		{"routes", func(w *Workspace) { w.Spec.Routes = []Route{{}} }, "spec.routes"},
		{"secrets", func(w *Workspace) { w.Spec.Secrets = []Secret{{}} }, "spec.secrets"},
	}
	for _, feature := range features {
		t.Run(feature.name, func(t *testing.T) {
			workspace := &Workspace{}
			feature.set(workspace)
			var target *FeatureNotSupportedError
			if err := CheckMilestone1Executable(workspace); !errors.As(err, &target) || target.Path != feature.path {
				t.Fatalf("error = %v, want feature error at %s", err, feature.path)
			}
		})
	}
}

func TestMilestone1AllowsTCPPortProjection(t *testing.T) {
	t.Parallel()
	workspace := &Workspace{Spec: WorkspaceSpec{TCPPorts: []TCPPort{{
		Name: "database", Service: "postgres", TargetPort: 5432, Allocate: "dynamic",
	}}}}
	if err := CheckMilestone1Executable(workspace); err != nil {
		t.Fatalf("CheckMilestone1Executable(TCP port) error = %v", err)
	}
}

func FuzzLoadBytes(f *testing.F) {
	root := f.TempDir()
	repository := filepath.Join(root, "repo")
	if err := os.Mkdir(repository, 0o755); err != nil {
		f.Fatal(err)
	}
	loader := mustLoader(f, []string{root}, DefaultLimits())
	f.Add([]byte(minimalManifest(repository)))
	f.Add([]byte("apiVersion: ["))
	f.Fuzz(func(t *testing.T, input []byte) {
		_, _ = loader.LoadBytes(input, root)
	})
}

func FuzzPathBoundaries(f *testing.F) {
	root := f.TempDir()
	loader := mustLoader(f, []string{root}, DefaultLimits())
	f.Add("../escape")
	f.Add("services/payments")
	f.Add("bad\\path")
	f.Fuzz(func(t *testing.T, input string) {
		_ = loader.validateRelative("path", input)
	})
}

func mustLoader(tb testing.TB, roots []string, limits Limits) *Loader {
	tb.Helper()
	loader, err := NewLoader(roots, limits)
	if err != nil {
		tb.Fatalf("NewLoader() error = %v", err)
	}
	return loader
}

func assertFieldError(t *testing.T, err error, path, problem string) {
	t.Helper()
	var target *FieldError
	if !errors.As(err, &target) {
		t.Fatalf("error = %v, want FieldError", err)
	}
	if target.Path != path {
		t.Errorf("error path = %q, want %q (error: %v)", target.Path, path, err)
	}
	if problem != "" && !strings.Contains(target.Problem, problem) {
		t.Errorf("error problem = %q, want substring %q", target.Problem, problem)
	}
}

func repositoryBlock(repository string) string {
	return "  repositories:\n" +
		"    - name: repo\n" +
		"      source:\n" +
		"        local: " + repository + "\n" +
		"      checkout:\n" +
		"        ref: HEAD\n" +
		"        branch: abox/{{ box.task }}/repo\n" +
		"      workspacePath: services/repo\n"
}

func minimalManifest(repository string) string {
	return "apiVersion: abox.dev/v1alpha1\n" +
		"kind: Workspace\n" +
		"metadata:\n  name: demo\n" +
		"spec:\n" +
		repositoryBlock(repository) +
		"  host:\n    cpus: 2\n    memory: 4GiB\n    disk: 20GiB\n" +
		"  compose:\n    includes:\n      - repository: repo\n        path: compose.yaml\n    profiles: [development]\n" +
		"  workspaceService:\n    name: app\n    repository: repo\n    workingDirectory: services/repo\n" +
		"  mounts:\n    - repository: repo\n      services: [app]\n      mode: read-write\n" +
		"  policy:\n    gitMetadata: read-write\n    dockerSocket: deny\n    machineMounts: declared-only\n    networkEgress: allow\n"
}

const documentedManifest = `apiVersion: abox.dev/v1alpha1
kind: Workspace
metadata:
  name: billing-platform
spec:
  repositories:
    - name: payments
      source:
        local: ../payment-service
      checkout:
        ref: HEAD
        branch: "abox/{{ box.task }}/payments"
      workspacePath: services/payments
    - name: portal
      source:
        remote: git@github.com:example/frontend-portal.git
      checkout:
        ref: origin/main
        branch: "abox/{{ box.task }}/portal"
      workspacePath: apps/portal
  host: {cpus: 6, memory: 12GiB, disk: 60GiB}
  compose:
    includes:
      - {repository: payments, path: compose.dev.yaml}
      - {repository: portal, path: compose.dev.yaml}
    profiles: [development]
  workspaceService: {name: payments-api, repository: payments, workingDirectory: services/payments}
  mounts:
    - {repository: payments, services: [payments-api], mode: read-write}
    - {repository: portal, services: [payments-api, portal-web], mode: read-write}
  caches:
    - {name: pnpm-store, service: payments-api, target: /home/dev/.local/share/pnpm/store}
    - {name: go-build, service: payments-api, target: /home/dev/.cache/go-build}
  setup:
    - name: install-payments
      service: payments-api
      command: [pnpm, install, --frozen-lockfile]
      rerunWhenChanged: [payments/pnpm-lock.yaml]
    - name: migrate-database
      service: payments-api
      command: [pnpm, "db:migrate"]
      after: [install-payments, postgres-ready]
      rerun: everyStart
  routes:
    - {name: api, service: payments-api, targetPort: 8080, readiness: {http: {path: /healthz, expectedStatus: 200}, timeout: 90s}}
    - {name: portal, service: portal-web, targetPort: 3000, readiness: {http: {path: /, expectedStatus: 200}, timeout: 120s}}
  tcpPorts:
    - {name: postgres, service: postgres, targetPort: 5432, allocate: dynamic}
  secrets:
    - name: OPENAI_API_KEY
      fromHostEnvironment: OPENAI_API_KEY
      exposeTo:
        - {service: payments-api, as: environment}
  policy: {gitMetadata: read-write, dockerSocket: deny, machineMounts: declared-only, networkEgress: allow}
`
