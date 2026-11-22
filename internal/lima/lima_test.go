package lima_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/lima"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/process"
	"github.com/shoe-point-0/agent-box/internal/testutil"
	"github.com/shoe-point-0/agent-box/internal/toolchain"
	"go.yaml.in/yaml/v3"
)

func TestGenerateConfigIsMinimalDeterministicAndExact(t *testing.T) {
	t.Parallel()
	spec := testSpec(t)
	first, err := lima.GenerateConfig(spec)
	if err != nil {
		t.Fatal(err)
	}
	second, err := lima.GenerateConfig(spec)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("GenerateConfig() is not deterministic")
	}

	var document map[string]any
	if err := yaml.Unmarshal(first, &document); err != nil {
		t.Fatal(err)
	}
	if document["mountInotify"] != true {
		t.Fatal("writable mounts require Lima event forwarding")
	}
	wantKeys := []string{"containerd", "cpus", "disk", "env", "images", "memory", "minimumLimaVersion", "mountInotify", "mountType", "mounts", "portForwards", "propagateProxyEnv", "ssh", "vmType"}
	gotKeys := make([]string, 0, len(document))
	for key := range document {
		gotKeys = append(gotKeys, key)
	}
	slices.Sort(gotKeys)
	if !slices.Equal(gotKeys, wantKeys) {
		t.Fatalf("config keys = %v, want %v", gotKeys, wantKeys)
	}
	if document["minimumLimaVersion"] != lima.SupportedVersion || document["vmType"] != "vz" || document["mountType"] != "virtiofs" {
		t.Fatalf("pinned config fields = %#v", document)
	}
	if document["cpus"] != 2 || document["memory"] != "4294967296B" || document["disk"] != "21474836480B" {
		t.Fatalf("resource fields = cpus=%#v memory=%#v disk=%#v", document["cpus"], document["memory"], document["disk"])
	}
	mounts := document["mounts"].([]any)
	if len(mounts) != 2 {
		t.Fatalf("mount count = %d, want 2", len(mounts))
	}
	for index, expected := range spec.Mounts {
		mount := mounts[index].(map[string]any)
		if mount["location"] != expected.Path || mount["mountPoint"] != expected.Path || mount["writable"] != expected.Writable {
			t.Fatalf("mount[%d] = %#v", index, mount)
		}
	}
	forwards := document["portForwards"].([]any)
	if len(forwards) != 1 {
		t.Fatal("port forwarding policy is not exact")
	}
	rule := forwards[0].(map[string]any)
	if rule["guestIP"] != "0.0.0.0" || rule["guestIPMustBeZero"] != false || rule["proto"] != "any" || rule["ignore"] != true || !reflect.DeepEqual(rule["guestPortRange"], []any{1, 65535}) {
		t.Fatalf("automatic forwards are not disabled: %#v", rule)
	}
	images := document["images"].([]any)
	image := images[0].(map[string]any)
	if image["location"] != "file:///opt/agent-box/images/guest%20image.img" || image["digest"] != string(spec.GuestImage.Digest) {
		t.Fatalf("image = %#v", image)
	}
	env := document["env"].(map[string]any)
	if env["ABOX_MANAGED"] != "true" || env["ABOX_BOX_ID"] != spec.BoxID || env["ABOX_PLAN_DIGEST"] != string(spec.PlanDigest) || env["ABOX_GENERATION"] != "7" {
		t.Fatalf("ownership env = %#v", env)
	}

	path := filepath.Join(testutil.TempDir(t), "lima.yaml")
	if err := lima.WriteConfig(path, spec); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %04o, want 0600", info.Mode().Perm())
	}
}

// Lima 2.0.0 warns on stderr when the inventory is empty. Cold creation must
// accept that ordinary state while keeping malformed or error output fatal.
func TestInspectEmptyInventorySuppressesInformationalLogging(t *testing.T) {
	t.Parallel()
	adapter, runner, spec := testAdapter(t)
	runner.warnWhenEmpty = true
	observation, err := adapter.Inspect(t.Context(), spec)
	if err != nil || observation.State != lima.Absent {
		t.Fatalf("Inspect() = %#v, %v; want absent", observation, err)
	}
}

func TestGenerateConfigRejectsUnsafeOrUnrepresentableInputs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*lima.Spec)
	}{
		{"wrong-driver", func(spec *lima.Spec) { spec.RuntimeHost.Driver = "lima-qemu" }},
		{"fractional-cpu", func(spec *lima.Spec) { spec.RuntimeHost.CPUMillicores = 1500 }},
		{"missing-box-identity", func(spec *lima.Spec) { spec.BoxID = "" }},
		{"relative-image", func(spec *lima.Spec) { spec.ImagePath = "guest.img" }},
		{"duplicate-mount", func(spec *lima.Spec) { spec.Mounts[1].Path = spec.Mounts[0].Path }},
		{"unverified-image", func(spec *lima.Spec) { spec.GuestImage.Component = toolchain.Utility }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := testSpec(t)
			test.mutate(&spec)
			_, err := lima.GenerateConfig(spec)
			assertKind(t, err, lima.InvalidSpecification)
		})
	}
}

func TestLifecycleIsObservedIdempotentAndUsesExactArgv(t *testing.T) {
	t.Parallel()
	adapter, runner, spec := testAdapter(t)
	configPath := filepath.Join(testutil.TempDir(t), "lima.yaml")
	if err := lima.WriteConfig(configPath, spec); err != nil {
		t.Fatal(err)
	}
	configEvidence := recordPathSet(t, []string{configPath})
	runner.config = configDocument(t, spec)

	observation, err := adapter.Create(t.Context(), spec, configPath, configEvidence)
	if err != nil || observation.State != lima.Stopped {
		t.Fatalf("Create() = %#v, %v", observation, err)
	}
	mounts := observation.Mounts()
	if len(mounts) != len(spec.Mounts) || mounts[0].Source != spec.Mounts[0].Path ||
		mounts[0].Destination != spec.Mounts[0].Path || mounts[0].Writable != spec.Mounts[0].Writable {
		t.Fatalf("Create().Mounts() = %#v, want live config mounts", mounts)
	}
	mounts[0].Source = "/mutated-copy"
	if observation.Mounts()[0].Source != spec.Mounts[0].Path {
		t.Fatal("Mounts() returned mutable observation storage")
	}
	if _, err := adapter.Create(t.Context(), spec, configPath, configEvidence); err != nil {
		t.Fatal(err)
	}
	if observation, err = adapter.Start(t.Context(), spec); err != nil || observation.State != lima.Running {
		t.Fatalf("Start() = %#v, %v", observation, err)
	}
	liveMounts, err := adapter.InspectMounts(t.Context(), spec)
	if err != nil || len(liveMounts) != len(spec.Mounts) || liveMounts[0].Source != spec.Mounts[0].Path {
		t.Fatalf("InspectMounts() = %#v, %v", liveMounts, err)
	}
	liveMounts[0].Source = "/mutated-live-copy"
	refreshedMounts, err := adapter.InspectMounts(t.Context(), spec)
	if err != nil || refreshedMounts[0].Source != spec.Mounts[0].Path {
		t.Fatalf("InspectMounts() retained caller mutation: %#v, %v", refreshedMounts, err)
	}
	if _, err := adapter.Start(t.Context(), spec); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	result, err := adapter.Invoke(t.Context(), spec, spec.Mounts[0].Path, []string{"printf", "%s", "safe value"}, &stdout, nil)
	if err != nil || result.ExitCode != 0 || stdout.String() != "guest-output" {
		t.Fatalf("Invoke() = %#v, %v, stdout=%q", result, err, stdout.String())
	}
	if observation, err = adapter.Stop(t.Context(), spec); err != nil || observation.State != lima.Stopped {
		t.Fatalf("Stop() = %#v, %v", observation, err)
	}
	if _, err := adapter.Stop(t.Context(), spec); err != nil {
		t.Fatal(err)
	}
	recorded := recordFor(spec)
	if observation, err = adapter.Delete(t.Context(), spec, recorded); err != nil || observation.State != lima.Absent {
		t.Fatalf("Delete() = %#v, %v", observation, err)
	}
	if _, err := adapter.Delete(t.Context(), spec, recorded); err != nil {
		t.Fatal(err)
	}

	if got := runner.count("create"); got != 1 {
		t.Fatalf("create calls = %d, want 1", got)
	}
	if got := runner.count("start"); got != 1 {
		t.Fatalf("start calls = %d, want 1", got)
	}
	if got := runner.count("stop"); got != 1 {
		t.Fatalf("stop calls = %d, want 1", got)
	}
	if got := runner.count("delete"); got != 1 {
		t.Fatalf("delete calls = %d, want 1", got)
	}
	if got := runner.call("create"); !slices.Equal(got, []string{"create", "--name=" + spec.RuntimeHost.InstanceName, "--tty=false", configPath}) {
		t.Fatalf("create argv = %q", got)
	}
	if got := runner.call("shell"); !slices.Equal(got, []string{"shell", "--tty=false", "--workdir=" + spec.Mounts[0].Path, spec.RuntimeHost.InstanceName, "--", "/usr/bin/sudo", "-n", "--", "printf", "%s", "safe value"}) {
		t.Fatalf("shell argv = %q", got)
	}
	for _, call := range runner.callsCopy() {
		if len(call) > 0 && call[0] == "list" && !slices.Equal(call, []string{"list", "--tty=false", "--format=json", "--all-fields", "--log-level=error"}) {
			t.Fatalf("list argv = %q", call)
		}
	}
}

func TestInvokeCommandTransportsEnvironmentAndSecretsOutsideArgv(t *testing.T) {
	adapter, runner, spec := testAdapter(t)
	runner.config = configDocument(t, spec)
	runner.status = "Running"
	const secret = "guest-secret-canary"
	_, err := adapter.InvokeCommand(t.Context(), spec, lima.GuestCommand{
		Argv: []string{"/usr/bin/docker", "compose", "version"}, Workdir: spec.Mounts[0].Path,
		Environment: map[string]string{"HOME": "/runtime/home", "TOKEN": secret},
		Secrets:     []process.Secret{{Value: secret}},
	})
	if err != nil {
		t.Fatal(err)
	}
	call := runner.call("shell")
	if len(call) < 8 || !slices.Equal(call[5:8], []string{"/usr/bin/sudo", "-n", "--"}) {
		t.Fatalf("guest control must use noninteractive root execution: %q", call)
	}
	for _, argument := range call {
		if strings.Contains(argument, secret) || strings.Contains(argument, "/runtime/home") {
			t.Fatalf("shell argv contains transported environment value: %q", call)
		}
	}
	stdin, secrets := runner.shellTransport()
	if !bytes.Contains(stdin, []byte(secret)) || !bytes.Contains(stdin, []byte("/runtime/home")) ||
		!reflect.DeepEqual(secrets, []process.Secret{{Value: secret}}) {
		t.Fatalf("shell stdin/secrets = %q/%#v", stdin, secrets)
	}
}

func TestDeleteRequiresRecordedIdentityAndEmbeddedMetadata(t *testing.T) {
	t.Parallel()
	adapter, runner, spec := testAdapter(t)
	runner.config = configDocument(t, spec)
	runner.status = "Stopped"
	recorded := recordFor(spec)
	recorded.Metadata[ownership.BoxIDFact] = "forged-box"

	_, err := adapter.Delete(t.Context(), spec, recorded)
	if !errors.Is(err, ownership.ErrNotOwned) {
		t.Fatalf("Delete() error = %v, want ownership refusal", err)
	}
	if runner.count("delete") != 0 {
		t.Fatal("Delete() invoked limactl after ownership refusal")
	}
}

func TestObserveRemovalRequiresPlanConfigAndRecordedLiveOwnership(t *testing.T) {
	t.Parallel()
	adapter, runner, spec := testAdapter(t)
	runner.config = configDocument(t, spec)
	runner.status = "Running"
	recorded := recordFor(spec)

	observed, err := adapter.ObserveRemoval(t.Context(), spec, recorded)
	if err != nil || observed.State != lima.Running || observed.Metadata[ownership.BoxIDFact] != spec.BoxID {
		t.Fatalf("ObserveRemoval() = %#v, %v", observed, err)
	}
	if runner.count("delete") != 0 {
		t.Fatal("ObserveRemoval() mutated Lima")
	}

	runner.config["memory"] = "8589934592B"
	_, err = adapter.ObserveRemoval(t.Context(), spec, recorded)
	assertKind(t, err, lima.ConfigMismatch)
	runner.config = configDocument(t, spec)
	recorded.Metadata[ownership.BoxIDFact] = "forged-box"
	_, err = adapter.ObserveRemoval(t.Context(), spec, recorded)
	if !errors.Is(err, ownership.ErrNotOwned) {
		t.Fatalf("ObserveRemoval(forged record) error = %v, want ownership refusal", err)
	}
}

func TestDeleteStopsRunningInstanceBeforeDeleting(t *testing.T) {
	t.Parallel()
	adapter, runner, spec := testAdapter(t)
	runner.config = configDocument(t, spec)
	runner.status = "Running"

	observed, err := adapter.Delete(t.Context(), spec, recordFor(spec))
	if err != nil || observed.State != lima.Absent {
		t.Fatalf("Delete(running) = %#v, %v; want absent", observed, err)
	}
	if runner.count("stop") != 1 || runner.count("delete") != 1 {
		t.Fatalf("Delete(running) stop=%d delete=%d, want 1/1", runner.count("stop"), runner.count("delete"))
	}
}

func TestDeleteInterruptedAfterAbsenceConvergesWithoutReplay(t *testing.T) {
	t.Parallel()
	adapter, runner, spec := testAdapter(t)
	runner.config = configDocument(t, spec)
	runner.status = "Stopped"
	runner.fail["delete"] = &process.CancellationError{Cause: context.Canceled}

	observed, err := adapter.Delete(t.Context(), spec, recordFor(spec))
	assertKind(t, err, lima.Canceled)
	if observed.State != lima.Absent || runner.count("delete") != 1 {
		t.Fatalf("interrupted Delete() = %#v, %v, calls = %d", observed, err, runner.count("delete"))
	}
	delete(runner.fail, "delete")
	observed, err = adapter.Delete(t.Context(), spec, recordFor(spec))
	if err != nil || observed.State != lima.Absent || runner.count("delete") != 1 {
		t.Fatalf("retry Delete() = %#v, %v, calls = %d", observed, err, runner.count("delete"))
	}
}

func TestDeleteRejectsForgedRecordEvenWhenInstanceIsAbsent(t *testing.T) {
	t.Parallel()
	adapter, runner, spec := testAdapter(t)
	recorded := recordFor(spec)
	recorded.Metadata[ownership.BoxIDFact] = "forged-box"

	_, err := adapter.Delete(t.Context(), spec, recorded)
	if !errors.Is(err, ownership.ErrNotOwned) {
		t.Fatalf("Delete(absent, forged record) error = %v, want ownership refusal", err)
	}
	if runner.count("list") != 0 || runner.count("delete") != 0 {
		t.Fatalf("forged record reached Lima inventory/delete: calls = %v", runner.callsCopy())
	}
}

func TestCreateRejectsTamperedGeneratedConfigBeforeLimactl(t *testing.T) {
	t.Parallel()
	adapter, runner, spec := testAdapter(t)
	configPath := filepath.Join(testutil.TempDir(t), "lima.yaml")
	if err := lima.WriteConfig(configPath, spec); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("vmType: vz\nmounts:\n  - location: /home\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := adapter.Create(t.Context(), spec, configPath, recordPathSet(t, []string{configPath}))
	assertKind(t, err, lima.ConfigMismatch)
	if len(runner.callsCopy()) != 0 {
		t.Fatal("Create() invoked limactl with tampered generated config")
	}
}

func TestImmutableConfigMismatchRefusesEveryMutation(t *testing.T) {
	t.Parallel()
	adapter, runner, spec := testAdapter(t)
	runner.config = configDocument(t, spec)
	runner.config["cpus"] = float64(8)
	runner.status = "Stopped"

	_, err := adapter.Start(t.Context(), spec)
	assertKind(t, err, lima.ConfigMismatch)
	if runner.count("start") != 0 {
		t.Fatal("Start() mutated mismatched instance")
	}
	_, err = adapter.Delete(t.Context(), spec, recordFor(spec))
	assertKind(t, err, lima.ConfigMismatch)
	if runner.count("delete") != 0 {
		t.Fatal("Delete() mutated mismatched instance")
	}
}

func TestCreateClassifiesPartialCreationAndCancellationAfterReobservation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		runErr     error
		afterState string
		want       lima.ErrorKind
	}{
		{"partial", &process.ExitError{Code: 1}, "Broken", lima.PartialCreation},
		{"canceled-but-created", &process.CancellationError{Cause: context.Canceled}, "Stopped", lima.Canceled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter, runner, spec := testAdapter(t)
			runner.config = configDocument(t, spec)
			runner.fail["create"] = test.runErr
			runner.transition["create"] = test.afterState
			configPath := filepath.Join(testutil.TempDir(t), "lima.yaml")
			if err := lima.WriteConfig(configPath, spec); err != nil {
				t.Fatal(err)
			}
			observation, err := adapter.Create(t.Context(), spec, configPath, recordPathSet(t, []string{configPath}))
			assertKind(t, err, test.want)
			if observation.State == lima.Absent || runner.count("list") < 2 {
				t.Fatalf("Create() did not reobserve: state=%s list-calls=%d", observation.State, runner.count("list"))
			}
		})
	}
}

func TestContractRejectsUnsupportedVersionMalformedInventoryAndPostconditionFailure(t *testing.T) {
	t.Parallel()
	t.Run("unsupported-version", func(t *testing.T) {
		adapter, runner, spec := testAdapter(t)
		runner.version = "limactl version 2.1.0\n"
		_, err := adapter.Inspect(t.Context(), spec)
		assertKind(t, err, lima.UnsupportedVersion)
		if runner.count("list") != 0 {
			t.Fatal("Inspect() continued after unsupported version")
		}
	})
	t.Run("malformed-json-lines", func(t *testing.T) {
		adapter, runner, spec := testAdapter(t)
		runner.malformed = []byte("{not-json}\n")
		_, err := adapter.Inspect(t.Context(), spec)
		assertKind(t, err, lima.MalformedOutput)
	})
	t.Run("duplicate-instance", func(t *testing.T) {
		adapter, runner, spec := testAdapter(t)
		runner.config = configDocument(t, spec)
		runner.status = "Stopped"
		runner.duplicate = true
		_, err := adapter.Inspect(t.Context(), spec)
		assertKind(t, err, lima.MalformedOutput)
	})
	t.Run("create-postcondition", func(t *testing.T) {
		adapter, runner, spec := testAdapter(t)
		runner.config = configDocument(t, spec)
		runner.suppressTransition["create"] = true
		configPath := filepath.Join(testutil.TempDir(t), "lima.yaml")
		if err := lima.WriteConfig(configPath, spec); err != nil {
			t.Fatal(err)
		}
		_, err := adapter.Create(t.Context(), spec, configPath, recordPathSet(t, []string{configPath}))
		assertKind(t, err, lima.PostconditionFailed)
	})
}

func TestCreateAndStartRefusePathSubstitutionBeforeMutation(t *testing.T) {
	t.Parallel()

	t.Run("generated-config", func(t *testing.T) {
		adapter, runner, spec := testAdapter(t)
		configPath := filepath.Join(testutil.TempDir(t), "lima.yaml")
		if err := lima.WriteConfig(configPath, spec); err != nil {
			t.Fatal(err)
		}
		evidence := recordPathSet(t, []string{configPath})
		original, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(configPath, configPath+".recorded"); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(configPath, original, 0o600); err != nil {
			t.Fatal(err)
		}
		_, err = adapter.Create(t.Context(), spec, configPath, evidence)
		assertKind(t, err, lima.ConfigMismatch)
		if runner.count("create") != 0 {
			t.Fatal("Create() invoked limactl after generated config substitution")
		}
	})

	t.Run("runtime-ancestor", func(t *testing.T) {
		adapter, runner, spec := testAdapter(t)
		runner.config = configDocument(t, spec)
		runner.status = "Stopped"
		mountParent := filepath.Dir(spec.Mounts[0].Path)
		moved := mountParent + "-moved"
		if err := os.Rename(mountParent, moved); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(mountParent, 0o700); err != nil {
			t.Fatal(err)
		}
		_, err := adapter.Start(t.Context(), spec)
		assertKind(t, err, lima.ConfigMismatch)
		if runner.count("start") != 0 {
			t.Fatal("Start() invoked limactl after runtime ancestor substitution")
		}
	})
}

func TestRunningInstanceFromDifferentLimaVersionIsRejected(t *testing.T) {
	t.Parallel()
	adapter, runner, spec := testAdapter(t)
	runner.config = configDocument(t, spec)
	runner.status = "Running"
	runner.instanceVersion = "1.1.1"
	_, err := adapter.Stop(t.Context(), spec)
	assertKind(t, err, lima.UnsupportedVersion)
	if runner.count("stop") != 0 {
		t.Fatal("Stop() mutated instance created by unsupported Lima")
	}
}

func assertKind(t *testing.T, err error, want lima.ErrorKind) {
	t.Helper()
	var adapterErr *lima.Error
	if !errors.As(err, &adapterErr) || adapterErr.Kind != want {
		t.Fatalf("error = %v, want Lima kind %s", err, want)
	}
}

func testSpec(t *testing.T) lima.Spec {
	t.Helper()
	return lima.Spec{
		RuntimeHost: contract.RuntimeHostPlan{Driver: "lima-vz", InstanceName: "task-a1b2c3d4", CPUMillicores: 2000, MemoryBytes: 4 << 30, DiskBytes: 20 << 30},
		Mounts: []contract.MountPlan{
			{Path: "/home/user/box/git/repository.git", Writable: true},
			{Path: "/home/user/box/workspace/repository", Writable: true},
		},
		BoxID: "box-task-a1b2c3d4", PlanDigest: digest([]byte("plan")), Generation: 7,
		GuestImage: toolchain.Identity{Component: toolchain.GuestImage, Version: "ubuntu-24.04", Digest: digest([]byte("guest-image"))},
		ImagePath:  "/opt/agent-box/images/guest image.img",
	}
}

func recordFor(spec lima.Spec) ownership.LimaInstanceRecord {
	return ownership.LimaInstanceRecord{Name: spec.RuntimeHost.InstanceName, Metadata: map[string]string{
		ownership.BoxIDFact:    spec.BoxID,
		"dev.abox.plan-digest": string(spec.PlanDigest),
		"dev.abox.generation":  "7",
	}}
}

func configDocument(t *testing.T, spec lima.Spec) map[string]any {
	t.Helper()
	data, err := lima.GenerateConfig(spec)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	return config
}

func testAdapter(t *testing.T) (*lima.Adapter, *fakeRunner, lima.Spec) {
	t.Helper()
	artifact := verifiedLima(t)
	runner := &fakeRunner{
		version: "limactl version " + lima.SupportedVersion + "\n", instanceVersion: lima.SupportedVersion,
		fail: make(map[string]error), transition: make(map[string]string), suppressTransition: make(map[string]bool),
	}
	adapter, err := lima.NewAdapter(artifact, runner, testutil.TempDir(t), map[string]string{"PATH": "/usr/bin:/bin", "HOME": testutil.TempDir(t), "LANG": "C", "LC_ALL": "C", "TZ": "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	spec := testSpec(t)
	runtimeRoot := testutil.TempDir(t)
	spec.Mounts[0].Path = filepath.Join(runtimeRoot, "git", "repository.git")
	spec.Mounts[1].Path = filepath.Join(runtimeRoot, "workspace", "repository")
	for _, mount := range spec.Mounts {
		if err := os.MkdirAll(mount.Path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	spec.ImagePath = filepath.Join(runtimeRoot, "images", "guest image.img")
	if err := os.MkdirAll(filepath.Dir(spec.ImagePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec.ImagePath, []byte("guest-image"), 0o600); err != nil {
		t.Fatal(err)
	}
	spec.PathEvidence, err = lima.RecordRuntimePaths(spec)
	if err != nil {
		t.Fatal(err)
	}
	return adapter, runner, spec
}

func recordPathSet(t *testing.T, paths []string) ownership.PathSetRecord {
	t.Helper()
	record, err := ownership.RecordPathSet(paths)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func verifiedLima(t *testing.T) toolchain.Artifact {
	t.Helper()
	directory := testutil.TempDir(t)
	type fixture struct {
		component toolchain.Component
		version   string
		mode      fs.FileMode
	}
	fixtures := []fixture{
		{toolchain.Lima, lima.SupportedVersion, 0o700},
		{toolchain.GuestImage, "ubuntu-24.04", 0o600},
		{toolchain.Docker, "28.3.3", 0o700},
		{toolchain.Compose, "2.39.1", 0o700},
	}
	identities := make(map[toolchain.Component]toolchain.Identity, len(fixtures))
	entries := make([]toolchain.Entry, 0, len(fixtures))
	for _, fixture := range fixtures {
		content := []byte("fixture-" + string(fixture.component))
		path := filepath.Join(directory, string(fixture.component))
		if err := os.WriteFile(path, content, fixture.mode); err != nil {
			t.Fatal(err)
		}
		identity := toolchain.Identity{Component: fixture.component, Version: fixture.version, Digest: digest(content)}
		identities[fixture.component] = identity
		entries = append(entries, toolchain.Entry{Component: fixture.component, Candidate: toolchain.Candidate{Identity: identity, Path: path}})
	}
	provider, err := toolchain.NewPreinstalledProvider(entries)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := toolchain.Verify(t.Context(), toolchain.Specification{
		Lima: identities[toolchain.Lima], GuestImage: identities[toolchain.GuestImage],
		Docker: identities[toolchain.Docker], Compose: identities[toolchain.Compose],
	}, provider)
	if err != nil {
		t.Fatal(err)
	}
	return verified.Lima
}

func digest(content []byte) contract.Digest {
	sum := sha256.Sum256(content)
	return contract.Digest(fmt.Sprintf("sha256:%x", sum[:]))
}

type fakeRunner struct {
	warnWhenEmpty      bool
	mu                 sync.Mutex
	calls              [][]string
	version            string
	instanceVersion    string
	status             string
	config             map[string]any
	malformed          []byte
	duplicate          bool
	fail               map[string]error
	transition         map[string]string
	suppressTransition map[string]bool
	shellStdin         []byte
	shellSecrets       []process.Secret
}

func (r *fakeRunner) Run(_ context.Context, command process.Command) (process.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	args := append([]string(nil), command.Argv[1:]...)
	r.calls = append(r.calls, args)
	result := process.Result{ExitCode: 0}
	if len(args) == 0 {
		return result, errors.New("empty fake argv")
	}
	operation := args[0]
	switch operation {
	case "--version":
		result.Stdout = []byte(r.version)
	case "list":
		if r.warnWhenEmpty && r.status == "" && !slices.Contains(args, "--log-level=error") {
			result.Stderr = []byte("level=warning msg=\"No instance found. Run `limactl create` to create an instance.\"\n")
		}
		if r.malformed != nil {
			result.Stdout = append([]byte(nil), r.malformed...)
			break
		}
		if r.status != "" {
			instance := map[string]any{
				"name": "task-a1b2c3d4", "status": r.status, "limaVersion": r.instanceVersion, "config": cloneDocument(r.config),
			}
			encoded, _ := json.Marshal(instance)
			result.Stdout = append(encoded, '\n')
			if r.duplicate {
				result.Stdout = append(result.Stdout, append(encoded, '\n')...)
			}
		}
	case "create", "start", "stop", "delete":
		if operation == "delete" && r.status == "Running" {
			result.ExitCode = 1
			return result, &process.ExitError{Code: 1}
		}
		if !r.suppressTransition[operation] {
			next := r.transition[operation]
			if next == "" {
				switch operation {
				case "create", "stop":
					next = "Stopped"
				case "start":
					next = "Running"
				case "delete":
					next = ""
				}
			}
			r.status = next
		}
	case "shell":
		r.shellStdin = append([]byte(nil), command.Stdin...)
		r.shellSecrets = slices.Clone(command.Secrets)
		result.Stdout = []byte("guest-output")
		if command.Stdout != nil {
			_, _ = command.Stdout.Write(result.Stdout)
		}
	default:
		return result, fmt.Errorf("unexpected fake operation %q", operation)
	}
	if err := r.fail[operation]; err != nil {
		result.ExitCode = 1
		return result, err
	}
	return result, nil
}

func (r *fakeRunner) count(operation string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, call := range r.calls {
		if len(call) > 0 && call[0] == operation {
			count++
		}
	}
	return count
}

func (r *fakeRunner) call(operation string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, call := range r.calls {
		if len(call) > 0 && call[0] == operation {
			return append([]string(nil), call...)
		}
	}
	return nil
}

func (r *fakeRunner) callsCopy() [][]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.calls)
}

func (r *fakeRunner) shellTransport() ([]byte, []process.Secret) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]byte(nil), r.shellStdin...), slices.Clone(r.shellSecrets)
}

func cloneDocument(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	encoded, _ := json.Marshal(input)
	var output map[string]any
	_ = json.Unmarshal(encoded, &output)
	return output
}

var _ toolchain.Runner = (*fakeRunner)(nil)

func TestStartRejectsChangedPortForwardPolicy(t *testing.T) {
	adapter, runner, spec := testAdapter(t)
	runner.config = configDocument(t, spec)
	runner.status = "Stopped"
	delete(runner.config, "portForwards")
	_, err := adapter.Start(t.Context(), spec)
	assertKind(t, err, lima.ConfigMismatch)
	if runner.count("start") != 0 {
		t.Fatal("started a VM with unowned forwarding policy")
	}
}
