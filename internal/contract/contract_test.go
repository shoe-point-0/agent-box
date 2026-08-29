package contract_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/shoe-point-0/agent-box/internal/contract"
)

func TestGoldenExamplesValidateAgainstSchemasAndGoModels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		version    string
		newPayload func() any
	}{
		{"plan", contract.PlanSchemaVersion, func() any { return new(contract.Plan) }},
		{"context", contract.ContextSchemaVersion, func() any { return new(contract.Context) }},
		{"output", contract.OutputSchemaVersion, func() any { return new(contract.Output) }},
		{"event", contract.EventSchemaVersion, func() any { return new(contract.Event) }},
		{"error", contract.ErrorSchemaVersion, func() any { return new(contract.Failure) }},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			schema := compileSchema(t, test.name)
			document := readDocument(t, filepath.Join(schemaRoot(), "examples", test.name+".json"))
			if err := schema.Validate(document); err != nil {
				t.Fatalf("golden does not validate: %v", err)
			}

			data := readBytes(t, filepath.Join(schemaRoot(), "examples", test.name+".json"))
			decoder := json.NewDecoder(bytes.NewReader(data))
			decoder.DisallowUnknownFields()
			payload := test.newPayload()
			if err := decoder.Decode(payload); err != nil {
				t.Fatalf("decode golden into Go contract: %v", err)
			}
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal Go contract: %v", err)
			}
			var roundTrip map[string]any
			if err := json.Unmarshal(encoded, &roundTrip); err != nil {
				t.Fatalf("decode round trip: %v", err)
			}
			if got := roundTrip["schemaVersion"]; got != test.version {
				t.Fatalf("schemaVersion = %v, want %q", got, test.version)
			}
			if err := schema.Validate(roundTrip); err != nil {
				t.Fatalf("Go contract does not match schema: %v", err)
			}
		})
	}
}

func TestSchemasRejectUnknownAndSecretValueFields(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"plan", "context", "output", "error"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			schema := compileSchema(t, name)
			document := readDocument(t, filepath.Join(schemaRoot(), "examples", name+".json"))
			objects := collectObjects(document)
			if len(objects) == 0 {
				t.Fatal("golden has no objects")
			}
			for index, object := range objects {
				object["secretValue"] = "canary-do-not-persist"
				if err := schema.Validate(document); err == nil {
					t.Fatalf("object %d accepted secretValue field", index)
				}
				delete(object, "secretValue")
			}
		})
	}
}

func TestPlanRejectsDeferredExecutableFields(t *testing.T) {
	t.Parallel()

	schema := compileSchema(t, "plan")
	document := readDocument(t, filepath.Join(schemaRoot(), "examples", "plan.json")).(map[string]any)
	for _, field := range []string{"routes", "ports", "caches", "setup", "secretRefs"} {
		document[field] = []any{}
		if err := schema.Validate(document); err == nil {
			t.Errorf("Plan schema accepted deferred field %q", field)
		}
		delete(document, field)
	}
}

func TestErrorSchemaAndExitMappingAreExhaustive(t *testing.T) {
	t.Parallel()

	expected := map[contract.ErrorCode]int{
		contract.CodeCLIUsage: contract.ExitUsage, contract.CodeManifestInvalid: contract.ExitUsage,
		contract.CodeFeatureNotSupported: contract.ExitUsage,
		contract.CodeMachineUnsupported:  contract.ExitPrerequisite, contract.CodePrerequisiteFailed: contract.ExitPrerequisite,
		contract.CodeOwnershipConflict: contract.ExitConflict, contract.CodeIdentityConflict: contract.ExitConflict,
		contract.CodeBranchConflict: contract.ExitConflict, contract.CodeOperationConflict: contract.ExitConflict,
		contract.CodeDaemonUnavailable: contract.ExitInfrastructure, contract.CodeInfrastructureApplyFailed: contract.ExitInfrastructure,
		contract.CodeServiceNotReady: contract.ExitReadiness, contract.CodeRouteNotReady: contract.ExitReadiness,
		contract.CodeGitDirtySource: contract.ExitSafety, contract.CodeCleanupRefused: contract.ExitSafety,
		contract.CodeNotFound: contract.ExitNotFound, contract.CodeAPIIncompatible: contract.ExitCompatibility,
		contract.CodeInternalInvariant: contract.ExitInternal,
	}
	codes := contract.ErrorCodes()
	if len(codes) != len(expected) {
		t.Fatalf("ErrorCodes count = %d, want %d", len(codes), len(expected))
	}
	for _, code := range codes {
		if !code.Valid() || contract.ExitCodeFor(code) != expected[code] {
			t.Errorf("code %q maps to %d, want %d", code, contract.ExitCodeFor(code), expected[code])
		}
	}
	if got := contract.ExitCodeFor("ABOX_UNKNOWN"); got != contract.ExitInternal {
		t.Errorf("unknown code exit = %d, want %d", got, contract.ExitInternal)
	}

	var raw struct {
		Properties struct {
			Code struct {
				Enum []contract.ErrorCode `json:"enum"`
			} `json:"code"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(readBytes(t, filepath.Join(schemaRoot(), "error.schema.json")), &raw); err != nil {
		t.Fatalf("decode error schema: %v", err)
	}
	slices.Sort(raw.Properties.Code.Enum)
	if !reflect.DeepEqual(raw.Properties.Code.Enum, codes) {
		t.Fatalf("error schema codes = %q, Go codes = %q", raw.Properties.Code.Enum, codes)
	}
}

func compileSchema(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	schema, err := compiler.Compile(filepath.Join(schemaRoot(), name+".schema.json"))
	if err != nil {
		t.Fatalf("compile %s schema: %v", name, err)
	}
	return schema
}

func schemaRoot() string { return filepath.Join("..", "..", "schemas", "v1alpha1") }

func readDocument(t *testing.T, path string) any {
	t.Helper()
	var document any
	if err := json.Unmarshal(readBytes(t, path), &document); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return document
}

func readBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func collectObjects(value any) []map[string]any {
	var objects []map[string]any
	var visit func(any)
	visit = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			objects = append(objects, typed)
			for _, child := range typed {
				visit(child)
			}
		case []any:
			for _, child := range typed {
				visit(child)
			}
		}
	}
	visit(value)
	return objects
}
