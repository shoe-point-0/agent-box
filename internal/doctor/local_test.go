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

func TestLocalObserverReportsInvalidEnvironmentWithoutExecutingTools(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	for _, name := range []string{"git", "limactl", "docker"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("#!/bin/sh\n: > unexpected-probe\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		name   string
		path   string
		home   string
		result string
	}{
		{"empty PATH", "", root, "environment.path"},
		{"leading empty", ":" + root, root, "environment.path"},
		{"middle empty", root + "::" + root, root, "environment.path"},
		{"trailing empty", root + ":", root, "environment.path"},
		{"current directory", ".:" + root, root, "environment.path"},
		{"relative entry", "private-relative-bin:" + root, root, "environment.path"},
		{"trailing slash", root + "/", root, "environment.path"},
		{"unclean entry", root + "/../bin", root, "environment.path"},
		{"control character", root + "\nprivate-path-value", root, "environment.path"},
		{"empty HOME", root, "", "environment.home"},
		{"relative HOME", root, "private-home-value", "environment.home"},
		{"unclean HOME", root, root + "/", "environment.home"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("PATH", test.path)
			t.Setenv("HOME", test.home)
			t.Setenv("HERMES_CRUSHER_REAL_GIT", filepath.Join(root, "git"))
			observer, err := NewLocalObserver(root)
			if err != nil {
				t.Fatalf("configuration prevented a report: %v", err)
			}
			if observer.Tools != nil || observer.Git != nil {
				t.Fatal("invalid environment enabled tool execution")
			}
			report, err := Inspect(t.Context(), Request{Mode: ModeMachine}, observer)
			if err != nil {
				t.Fatal(err)
			}
			checks := make(map[string]Check)
			for _, check := range report.Checks {
				checks[check.Result] = check
				if strings.Contains(check.Observed+check.Required+check.Remediation, "private-") {
					t.Fatal("report disclosed an environment value")
				}
			}
			check := checks[test.result]
			if check.Status != StatusFail || check.Evidence != EvidenceTested || !strings.Contains(check.Remediation, "rerun abox doctor") || !report.Failed() {
				t.Fatalf("configuration check = %#v", check)
			}
			for _, result := range []string{"tool.git", "tool.lima", "tool.docker", "tool.compose", "runtime.docker"} {
				if !strings.Contains(checks[result].Observed, "skipped") || checks[result].Remediation != check.Remediation {
					t.Errorf("blocked probe %s = %#v", result, checks[result])
				}
			}
			if checks["machine.platform"].Result == "" || checks["disk.state"].Result == "" {
				t.Fatal("configuration failure suppressed independent checks")
			}
			if _, err := os.Stat(filepath.Join(root, "unexpected-probe")); !os.IsNotExist(err) {
				t.Fatalf("tool executed with invalid environment: %v", err)
			}
		})
	}
}
