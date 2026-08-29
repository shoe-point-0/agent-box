package compose

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/shoe-point-0/agent-box/internal/contract"
)

// PolicyViolation is a stable, field-aware refusal of a normalized Compose
// primitive. Field is a JSON path in the retained Compose model.
type PolicyViolation struct {
	Service string
	Field   string
	Reason  string
}

func (e *PolicyViolation) Error() string {
	return fmt.Sprintf("Compose policy: service %q field %q: %s", e.Service, e.Field, e.Reason)
}

type policyDocument struct {
	Services map[string]policyService `json:"services"`
}

type policyService struct {
	Privileged        bool           `json:"privileged"`
	NetworkMode       string         `json:"network_mode"`
	PID               string         `json:"pid"`
	IPC               string         `json:"ipc"`
	UTS               string         `json:"uts"`
	UserNSMode        string         `json:"userns_mode"`
	Cgroup            string         `json:"cgroup"`
	CapAdd            []string       `json:"cap_add"`
	SecurityOpt       []string       `json:"security_opt"`
	Devices           []policyDevice `json:"devices"`
	DeviceCgroupRules []string       `json:"device_cgroup_rules"`
	Volumes           []policyVolume `json:"volumes"`
}

type policyDevice struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type policyVolume struct {
	Type     string           `json:"type"`
	Source   string           `json:"source"`
	Target   string           `json:"target"`
	ReadOnly bool             `json:"read_only"`
	Bind     policyBindOption `json:"bind"`
}

type policyBindOption struct {
	Propagation string `json:"propagation"`
}

type expectedBind struct {
	path     string
	writable bool
}

func validatePolicy(model []byte, compiled contract.Plan, requirePlanBinds bool) error {
	var document policyDocument
	if err := decodeJSON(model, &document); err != nil {
		return fmt.Errorf("validate normalized Compose policy: %w", err)
	}

	serviceNames := make([]string, 0, len(document.Services))
	for name := range document.Services {
		serviceNames = append(serviceNames, name)
	}
	slices.Sort(serviceNames)
	for _, name := range serviceNames {
		if err := validateServicePolicy(name, document.Services[name], expectedBinds(compiled, name), requirePlanBinds); err != nil {
			return err
		}
	}
	return nil
}

func validateServicePolicy(name string, service policyService, expected map[string]expectedBind, requirePlanBinds bool) error {
	field := func(fieldName string) string { return "services." + name + "." + fieldName }
	violation := func(fieldName, reason string) error {
		return &PolicyViolation{Service: name, Field: field(fieldName), Reason: reason}
	}

	if service.Privileged {
		return violation("privileged", "privileged mode is forbidden")
	}
	for _, namespace := range []struct {
		field string
		value string
	}{
		{"network_mode", service.NetworkMode},
		{"pid", service.PID},
		{"ipc", service.IPC},
		{"uts", service.UTS},
		{"userns_mode", service.UserNSMode},
		{"cgroup", service.Cgroup},
	} {
		if strings.EqualFold(strings.TrimSpace(namespace.value), "host") {
			return violation(namespace.field, "host namespace mode is forbidden")
		}
	}
	if len(service.CapAdd) > 0 {
		return violation("cap_add", fmt.Sprintf("capability additions are forbidden: %q", service.CapAdd[0]))
	}
	for index, option := range service.SecurityOpt {
		if !allowedSecurityOption(option) {
			return violation(fmt.Sprintf("security_opt[%d]", index), fmt.Sprintf("security option %q is forbidden", option))
		}
	}
	if len(service.Devices) > 0 {
		device := service.Devices[0]
		return violation("devices[0]", fmt.Sprintf("device mapping %q to %q is forbidden", device.Source, device.Target))
	}
	if len(service.DeviceCgroupRules) > 0 {
		return violation("device_cgroup_rules[0]", fmt.Sprintf("device cgroup rule %q is forbidden", service.DeviceCgroupRules[0]))
	}

	seen := make(map[string]struct{}, len(expected))
	for index, volume := range service.Volumes {
		baseField := fmt.Sprintf("volumes[%d]", index)
		if dockerSocketPath(volume.Source) || dockerSocketPath(volume.Target) {
			return violation(baseField, "Docker API socket mounts are forbidden")
		}
		if sensitiveMountPath(volume.Source) || sensitiveMountPath(volume.Target) {
			return violation(baseField, "mounts of proc, sys, dev, or cgroup paths are forbidden")
		}
		if volume.Type != "bind" {
			continue
		}
		planned, ok := expected[volume.Source]
		if !ok {
			return violation(baseField+".source", fmt.Sprintf("bind source %q is outside the exact Plan allowlist", volume.Source))
		}
		if volume.Target != planned.path {
			return violation(baseField+".target", fmt.Sprintf("bind target %q does not preserve Plan path %q", volume.Target, planned.path))
		}
		if volume.ReadOnly == planned.writable {
			return violation(baseField+".read_only", fmt.Sprintf("bind access does not match Plan writable=%t", planned.writable))
		}
		if propagation := strings.TrimSpace(volume.Bind.Propagation); propagation != "" && propagation != "rprivate" && propagation != "private" {
			return violation(baseField+".bind.propagation", fmt.Sprintf("bind propagation %q is forbidden", propagation))
		}
		if _, duplicate := seen[planned.path]; duplicate {
			return violation(baseField, fmt.Sprintf("duplicate bind for Plan path %q", planned.path))
		}
		seen[planned.path] = struct{}{}
	}
	if requirePlanBinds {
		paths := make([]string, 0, len(expected))
		for path := range expected {
			paths = append(paths, path)
		}
		slices.Sort(paths)
		for _, path := range paths {
			if _, ok := seen[path]; !ok {
				return violation("volumes", fmt.Sprintf("required Plan bind %q is absent", path))
			}
		}
	}
	return nil
}

func expectedBinds(compiled contract.Plan, service string) map[string]expectedBind {
	result := make(map[string]expectedBind)
	for _, mount := range compiled.Mounts {
		if slices.Contains(mount.Services, service) {
			result[mount.Path] = expectedBind{path: mount.Path, writable: mount.Writable}
		}
	}
	return result
}

func allowedSecurityOption(option string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(option), ":", "="))
	return normalized == "no-new-privileges=true"
}

func dockerSocketPath(path string) bool {
	cleaned := filepath.Clean(path)
	return filepath.Base(cleaned) == "docker.sock" || cleaned == "/run/docker" || cleaned == "/var/run/docker"
}

func sensitiveMountPath(path string) bool {
	cleaned := filepath.Clean(path)
	for _, root := range []string{"/proc", "/sys", "/dev", "/sys/fs/cgroup"} {
		if cleaned == root || strings.HasPrefix(cleaned, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
