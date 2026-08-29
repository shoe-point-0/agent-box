package contextdoc_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/shoe-point-0/agent-box/internal/atomicfile"
	"github.com/shoe-point-0/agent-box/internal/contextdoc"
	"github.com/shoe-point-0/agent-box/internal/contract"
)

const (
	planDigest     = contract.Digest("sha256:3333333333333333333333333333333333333333333333333333333333333333")
	secretCanary   = "secret-canary-do-not-persist"
	credentialPath = "/home/shoe/.ssh/id_ed25519"
	dockerSocket   = "/var/run/docker.sock"
)

func TestWriteProducesSchemaValidGoldenFromLiveObservations(t *testing.T) {
	directory := t.TempDir()
	destination := filepath.Join(directory, "context.json")
	input := baseInput()

	document, err := contextdoc.Write(destination, input)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if document.Box.Generation != input.Generation || document.Services[0].Name != "payments-api" {
		t.Fatalf("Write() document does not match live observations: %#v", document)
	}
	got := readFile(t, destination)
	want := readFile(t, filepath.Join("..", "..", "schemas", "v1alpha1", "examples", "context.json"))
	if !bytes.Equal(got, want) {
		t.Fatalf("context golden mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatalf("Stat(context) error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("context mode = %04o, want 0600", info.Mode().Perm())
	}
	validateSchema(t, got)

	for _, forbidden := range []string{
		secretCanary, credentialPath, dockerSocket,
		input.Plan.Repositories[0].SourcePath, input.Plan.Repositories[0].GitCommonDir,
	} {
		if bytes.Contains(got, []byte(forbidden)) {
			t.Errorf("context contains forbidden value at index %d", strings.Index(string(got), forbidden))
		}
	}
}

func TestGenerateSupportsAdditiveLiveRoutesAndPorts(t *testing.T) {
	input := baseInput()
	input.Observations.Routes = []contract.RouteContext{{
		Name: "api", URL: "http://api.auth-142-3m6y4k2p.abox.localhost:7777", Ready: true,
	}}
	input.Observations.Ports = []contract.PortContext{{Name: "postgres", Host: "127.0.0.1", Port: 23641, Ready: true}}

	document, encoded, err := contextdoc.Generate(input)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(document.Routes) != 1 || len(document.Ports) != 1 {
		t.Fatalf("generated resources = routes %d ports %d, want 1 each", len(document.Routes), len(document.Ports))
	}
	validateSchema(t, encoded)
}

func TestWriteRefusesIncompleteOrStaleGenerationWithoutReplacement(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*contextdoc.Input)
	}{
		{"Box identity", func(input *contextdoc.Input) { input.Observations.BoxID = "other-box" }},
		{"generation", func(input *contextdoc.Input) { input.Observations.Generation-- }},
		{"Plan digest", func(input *contextdoc.Input) {
			input.Observations.PlanDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		}},
		{"Box state", func(input *contextdoc.Input) { input.Observations.State = "STARTING" }},
		{"repository set", func(input *contextdoc.Input) { input.Observations.Repositories = nil }},
		{"repository fact", func(input *contextdoc.Input) { input.Observations.Repositories[0].Branch = "stale/branch" }},
		{"service set", func(input *contextdoc.Input) { input.Observations.Services = nil }},
		{"service readiness", func(input *contextdoc.Input) { input.Observations.Services[0].Ready = false }},
		{"route readiness", func(input *contextdoc.Input) {
			input.Observations.Routes = []contract.RouteContext{{Name: "api", URL: "http://api.example.invalid", Ready: false}}
		}},
		{"port readiness", func(input *contextdoc.Input) {
			input.Observations.Ports = []contract.PortContext{{Name: "db", Host: "127.0.0.1", Port: 20001, Ready: false}}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			destination := filepath.Join(directory, "context.json")
			old := []byte("complete-old-context\n")
			if err := os.WriteFile(destination, old, 0o600); err != nil {
				t.Fatal(err)
			}
			input := baseInput()
			test.mutate(&input)
			if _, err := contextdoc.Write(destination, input); err == nil {
				t.Fatal("Write() error = nil, want generation refusal")
			}
			if got := readFile(t, destination); !bytes.Equal(got, old) {
				t.Fatalf("context changed after refusal: %q", got)
			}
		})
	}
}

func TestWriteRejectsSensitiveCanaryBeforePersistence(t *testing.T) {
	directory := t.TempDir()
	destination := filepath.Join(directory, "context.json")
	old := []byte("complete-old-context\n")
	if err := os.WriteFile(destination, old, 0o600); err != nil {
		t.Fatal(err)
	}
	input := baseInput()
	input.Commands[0].Argv = append(input.Commands[0].Argv, secretCanary)

	_, err := contextdoc.Write(destination, input)
	if err == nil || strings.Contains(err.Error(), secretCanary) {
		t.Fatalf("Write() error = %q, want non-disclosing sensitive-value refusal", err)
	}
	if got := readFile(t, destination); !bytes.Equal(got, old) {
		t.Fatalf("context changed after redaction refusal: %q", got)
	}
}

func TestWritePropagatesAtomicDestinationRefusal(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target")
	destination := filepath.Join(directory, "context.json")
	if err := os.WriteFile(target, []byte("target-content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, destination); err != nil {
		t.Fatal(err)
	}

	_, err := contextdoc.Write(destination, baseInput())
	var writeErr *atomicfile.Error
	if !errors.As(err, &writeErr) || writeErr.Stage != atomicfile.StageValidate {
		t.Fatalf("Write() error = %T %v, want atomic validation error", err, err)
	}
	if got := readFile(t, target); string(got) != "target-content\n" {
		t.Fatalf("symlink target changed: %q", got)
	}
}

func baseInput() contextdoc.Input {
	worktree := "/state/boxes/auth-142-3m6y4k2p/workspace/services/payments"
	plan := contract.Plan{
		SchemaVersion: contract.PlanSchemaVersion,
		WorkspaceID:   "ws_h7n2x0m4",
		BoxID:         "auth-142-3m6y4k2p",
		TaskKey:       "AUTH-142",
		Manifest: contract.Artifact{
			Path: "/work/billing/.abox/box.yaml", Digest: "sha256:1111111111111111111111111111111111111111111111111111111111111111",
		},
		Repositories: []contract.PlanRepository{{
			Name: "payments", SourcePath: "/machine/source/billing", BaseCommit: "b6ea6ef0d5746f61f1a72df9c9e44abdb19d472a",
			Branch: "abox/auth-142/payments", WorktreePath: worktree, GitCommonDir: "/state/boxes/auth-142-3m6y4k2p/git/payments.git",
		}},
		WorkspaceService: contract.WorkspaceServicePlan{Name: "payments-api", Repository: "payments", WorkingDirectory: "services/payments"},
		Policy:           contract.PolicyPlan{GitMetadata: "read-write", DockerSocket: "deny", MachineMounts: "declared-only", NetworkEgress: "allow"},
		PlanDigest:       planDigest,
	}
	return contextdoc.Input{
		Plan:          plan,
		Generation:    4,
		WorkspaceRoot: "/state/boxes/auth-142-3m6y4k2p/workspace",
		Observations: contextdoc.Observations{
			BoxID: plan.BoxID, Generation: 4, PlanDigest: planDigest, State: "READY",
			Repositories: []contract.ContextRepository{{
				Name: "payments", Branch: "abox/auth-142/payments", BaseCommit: "b6ea6ef0d5746f61f1a72df9c9e44abdb19d472a", Path: worktree,
			}},
			Services: []contract.ServiceContext{{Name: "payments-api", Ready: true}},
			Routes:   []contract.RouteContext{},
			Ports:    []contract.PortContext{},
		},
		Commands: []contract.CommandContext{
			{Name: "test", Argv: []string{"go", "test", "./..."}},
			{Name: "status", Argv: []string{"abox", "status", "AUTH-142"}},
		},
		SensitiveValues: []string{secretCanary, credentialPath, dockerSocket},
	}
}

func validateSchema(t *testing.T, encoded []byte) {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	schema, err := compiler.Compile(filepath.Join("..", "..", "schemas", "v1alpha1", "context.schema.json"))
	if err != nil {
		t.Fatalf("compile context schema: %v", err)
	}
	var document any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("decode generated context: %v", err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatalf("generated context does not validate: %v", err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return data
}
