package lima

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/shoe-point-0/agent-box/internal/atomicfile"
	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/ownership"
	"github.com/shoe-point-0/agent-box/internal/toolchain"
	"go.yaml.in/yaml/v3"
)

const (
	SupportedVersion = "2.0.0"

	managedEnvKey    = "ABOX_MANAGED"
	boxIDEnvKey      = "ABOX_BOX_ID"
	planDigestEnvKey = "ABOX_PLAN_DIGEST"
	generationEnvKey = "ABOX_GENERATION"

	planDigestFact = "dev.abox.plan-digest"
	generationFact = "dev.abox.generation"
)

// Spec is the immutable runtime-host projection consumed by the Lima adapter.
type Spec struct {
	RuntimeHost  contract.RuntimeHostPlan
	Mounts       []contract.MountPlan
	BoxID        string
	PlanDigest   contract.Digest
	Generation   int64
	GuestImage   toolchain.Identity
	ImagePath    string
	PathEvidence ownership.PathSetRecord
}

// RecordRuntimePaths captures the complete Machine path set consumed while a
// Lima instance is created, started, or reconciled.
func RecordRuntimePaths(spec Spec) (ownership.PathSetRecord, error) {
	if _, err := compileConfig(spec); err != nil {
		return ownership.PathSetRecord{}, err
	}
	return ownership.RecordPathSet(runtimePaths(spec))
}

type limaConfig struct {
	MinimumLimaVersion string            `yaml:"minimumLimaVersion" json:"minimumLimaVersion"`
	VMType             string            `yaml:"vmType" json:"vmType"`
	Images             []imageConfig     `yaml:"images" json:"images"`
	CPUs               int               `yaml:"cpus" json:"cpus"`
	Memory             string            `yaml:"memory" json:"memory"`
	Disk               string            `yaml:"disk" json:"disk"`
	Mounts             []mountConfig     `yaml:"mounts" json:"mounts"`
	MountType          string            `yaml:"mountType" json:"mountType"`
	MountInotify       bool              `yaml:"mountInotify" json:"mountInotify"`
	SSH                sshConfig         `yaml:"ssh" json:"ssh"`
	Containerd         containerdConfig  `yaml:"containerd" json:"containerd"`
	PropagateProxyEnv  bool              `yaml:"propagateProxyEnv" json:"propagateProxyEnv"`
	Env                map[string]string `yaml:"env" json:"env"`
}

type imageConfig struct {
	Location string          `yaml:"location" json:"location"`
	Digest   contract.Digest `yaml:"digest" json:"digest"`
}

type mountConfig struct {
	Location   string `yaml:"location" json:"location"`
	MountPoint string `yaml:"mountPoint" json:"mountPoint"`
	Writable   bool   `yaml:"writable" json:"writable"`
}

type sshConfig struct {
	LoadDotSSHPubKeys bool `yaml:"loadDotSSHPubKeys" json:"loadDotSSHPubKeys"`
	ForwardAgent      bool `yaml:"forwardAgent" json:"forwardAgent"`
	ForwardX11        bool `yaml:"forwardX11" json:"forwardX11"`
	ForwardX11Trusted bool `yaml:"forwardX11Trusted" json:"forwardX11Trusted"`
}

type containerdConfig struct {
	System bool `yaml:"system" json:"system"`
	User   bool `yaml:"user" json:"user"`
}

// GenerateConfig returns deterministic minimal Lima YAML. It does not create or
// mutate an instance.
func GenerateConfig(spec Spec) ([]byte, error) {
	config, err := compileConfig(spec)
	if err != nil {
		return nil, err
	}
	data, err := yaml.Marshal(config)
	if err != nil {
		return nil, &Error{Kind: InvalidSpecification, Operation: "generate-config", Err: err}
	}
	return data, nil
}

// WriteConfig atomically materializes owner-only generated Lima YAML.
func WriteConfig(path string, spec Spec) error {
	data, err := GenerateConfig(spec)
	if err != nil {
		return err
	}
	if err := atomicfile.WriteFile(path, data, 0o600); err != nil {
		return &Error{Kind: CommandFailed, Operation: "write-config", Err: err}
	}
	return nil
}

func verifyConfigFile(path string, spec Spec) error {
	want, err := GenerateConfig(spec)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return &Error{Kind: CommandFailed, Operation: "create", Resource: spec.RuntimeHost.InstanceName, Err: fmt.Errorf("inspect generated config: %w", err)}
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || info.Size() > 1024*1024 {
		return &Error{Kind: ConfigMismatch, Operation: "create", Resource: spec.RuntimeHost.InstanceName, Err: errors.New("generated config must be a bounded regular file with mode 0600")}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		return &Error{Kind: CommandFailed, Operation: "create", Resource: spec.RuntimeHost.InstanceName, Err: fmt.Errorf("read generated config: %w", err)}
	}
	if !bytes.Equal(got, want) {
		return &Error{Kind: ConfigMismatch, Operation: "create", Resource: spec.RuntimeHost.InstanceName, Err: errors.New("generated config content differs from immutable Spec")}
	}
	return nil
}

func compileConfig(spec Spec) (limaConfig, error) {
	if spec.RuntimeHost.Driver != "lima-vz" {
		return limaConfig{}, invalidSpec("runtime driver must be lima-vz")
	}
	if !validName(spec.RuntimeHost.InstanceName) || !validFact(spec.BoxID) {
		return limaConfig{}, invalidSpec("instance and Box identity must be non-empty and NUL-free")
	}
	if spec.RuntimeHost.CPUMillicores <= 0 || spec.RuntimeHost.CPUMillicores%1000 != 0 {
		return limaConfig{}, invalidSpec("CPU quantity must be a positive whole number of CPUs")
	}
	if spec.RuntimeHost.MemoryBytes <= 0 || spec.RuntimeHost.DiskBytes <= 0 {
		return limaConfig{}, invalidSpec("memory and disk quantities must be positive")
	}
	if spec.Generation <= 0 || !canonicalDigest(spec.PlanDigest) {
		return limaConfig{}, invalidSpec("generation and plan digest are invalid")
	}
	if spec.GuestImage.Component != toolchain.GuestImage || spec.GuestImage.Name != "" || spec.GuestImage.Version == "" || spec.GuestImage.Digest == "" {
		return limaConfig{}, invalidSpec("guest image identity is not verified-compatible")
	}
	imagePath, err := canonicalFileURL(spec.ImagePath)
	if err != nil {
		return limaConfig{}, invalidSpec(err.Error())
	}
	mounts := make([]mountConfig, len(spec.Mounts))
	seen := make(map[string]struct{}, len(spec.Mounts))
	for index, mount := range spec.Mounts {
		if !filepath.IsAbs(mount.Path) || filepath.Clean(mount.Path) != mount.Path || strings.IndexByte(mount.Path, 0) >= 0 {
			return limaConfig{}, invalidSpec("mount paths must be clean, absolute, and NUL-free")
		}
		if _, exists := seen[mount.Path]; exists {
			return limaConfig{}, invalidSpec("mount paths must be unique")
		}
		seen[mount.Path] = struct{}{}
		mounts[index] = mountConfig{Location: mount.Path, MountPoint: mount.Path, Writable: mount.Writable}
	}
	return limaConfig{
		MinimumLimaVersion: SupportedVersion,
		VMType:             "vz",
		Images:             []imageConfig{{Location: imagePath, Digest: spec.GuestImage.Digest}},
		CPUs:               int(spec.RuntimeHost.CPUMillicores / 1000),
		Memory:             strconv.FormatInt(spec.RuntimeHost.MemoryBytes, 10) + "B",
		Disk:               strconv.FormatInt(spec.RuntimeHost.DiskBytes, 10) + "B",
		Mounts:             mounts,
		MountType:          "virtiofs",
		MountInotify:       false,
		SSH:                sshConfig{},
		Containerd:         containerdConfig{},
		PropagateProxyEnv:  false,
		Env: map[string]string{
			managedEnvKey: "true", boxIDEnvKey: spec.BoxID,
			planDigestEnvKey: string(spec.PlanDigest), generationEnvKey: strconv.FormatInt(spec.Generation, 10),
		},
	}, nil
}

func canonicalFileURL(path string) (string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.IndexByte(path, 0) >= 0 {
		return "", errors.New("guest image path must be clean, absolute, and NUL-free")
	}
	return (&url.URL{Scheme: "file", Path: path}).String(), nil
}

func runtimePaths(spec Spec) []string {
	paths := make([]string, 0, len(spec.Mounts)+1)
	paths = append(paths, spec.ImagePath)
	for _, mount := range spec.Mounts {
		paths = append(paths, mount.Path)
	}
	return paths
}

func canonicalDigest(value contract.Digest) bool {
	text := string(value)
	return len(text) == len("sha256:")+sha256.Size*2 && strings.HasPrefix(text, "sha256:") && strings.Trim(text[len("sha256:"):], "0123456789abcdef") == ""
}

func validName(value string) bool {
	if !validFact(value) || len(value) > 80 || value == "." || value == ".." {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' {
			continue
		}
		return false
	}
	return true
}

func validFact(value string) bool { return value != "" && !strings.ContainsRune(value, '\x00') }

func ownershipMetadata(config *limaConfig) map[string]string {
	if config == nil || config.Env[managedEnvKey] != "true" {
		return nil
	}
	return map[string]string{
		ownership.BoxIDFact: config.Env[boxIDEnvKey],
		planDigestFact:      config.Env[planDigestEnvKey],
		generationFact:      config.Env[generationEnvKey],
	}
}

func recordedIdentity(spec Spec) ownership.LimaInstanceRecord {
	return ownership.LimaInstanceRecord{Name: spec.RuntimeHost.InstanceName, Metadata: map[string]string{
		ownership.BoxIDFact: spec.BoxID,
		planDigestFact:      string(spec.PlanDigest),
		generationFact:      strconv.FormatInt(spec.Generation, 10),
	}}
}

func matchesConfig(spec Spec, observed *limaConfig) error {
	expected, err := compileConfig(spec)
	if err != nil {
		return err
	}
	if observed == nil {
		return &Error{Kind: PartialCreation, Operation: "inspect", Resource: spec.RuntimeHost.InstanceName, Err: errors.New("instance config is absent")}
	}
	// Lima may append defaults to other fields. These are the complete immutable
	// fields Agent Box owns and therefore the only fields compared here.
	if observed.MinimumLimaVersion != expected.MinimumLimaVersion || observed.VMType != expected.VMType ||
		observed.CPUs != expected.CPUs || observed.Memory != expected.Memory || observed.Disk != expected.Disk ||
		observed.MountType != expected.MountType || observed.MountInotify != expected.MountInotify ||
		observed.SSH != expected.SSH || observed.Containerd != expected.Containerd ||
		observed.PropagateProxyEnv != expected.PropagateProxyEnv || !equalImages(observed.Images, expected.Images) ||
		!equalMounts(observed.Mounts, expected.Mounts) || !equalMetadata(observed.Env, expected.Env) {
		return &Error{Kind: ConfigMismatch, Operation: "inspect", Resource: spec.RuntimeHost.InstanceName, Err: errors.New("observed immutable Lima config differs from Plan")}
	}
	return nil
}

func equalImages(left, right []imageConfig) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalMounts(left, right []mountConfig) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalMetadata(left, right map[string]string) bool {
	for _, key := range []string{managedEnvKey, boxIDEnvKey, planDigestEnvKey, generationEnvKey} {
		if left[key] != right[key] {
			return false
		}
	}
	return true
}

func invalidSpec(message string) *Error {
	return &Error{Kind: InvalidSpecification, Operation: "validate", Err: fmt.Errorf("%s", message)}
}
