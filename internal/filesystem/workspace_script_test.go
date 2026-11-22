package filesystem

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shoe-point-0/agent-box/internal/process"
)

func TestWorkspaceObservationHandlesReadRaceAndPreservesTrailingNewline(t *testing.T) {
	root := t.TempDir()
	runScript := func(t *testing.T, prefix, target string) (process.Result, error) {
		t.Helper()
		script := "stat() { printf '600\\n'; }\n" + prefix + observeProbeScript
		return (process.Runner{}).Run(t.Context(), process.Command{
			Argv: []string{"/bin/sh", "-eu", "-c", script, "probe", target},
			Dir:  root,
			Env:  map[string]string{"PATH": "/bin:/usr/bin"},
		})
	}

	t.Run("read race", func(t *testing.T) {
		path := filepath.Join(root, "race")
		if err := os.WriteFile(path, []byte("content\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		result, err := runScript(t, "head() { printf 'partial\\nOK\\n'; printf 'ENOENT\\n' >&2; return 1; }\n", path)
		if err != nil || len(result.Stderr) != 0 {
			t.Fatalf("observation = %q/%q, %v; want exit 0 with empty stderr", result.Stdout, result.Stderr, err)
		}
		observed, err := decodeProbeFile(result.Stdout)
		if err != nil {
			t.Fatalf("decodeProbeFile() error = %v", err)
		}
		if !observed.Exists || observed.Regular {
			t.Fatalf("observed = %+v, want invalid existing observation", observed)
		}
		if matchesProbe(observed, []byte("content\n"), 0o600) {
			t.Fatal("read race satisfied exact-file postcondition")
		}
	})

	t.Run("stat race", func(t *testing.T) {
		path := filepath.Join(root, "stat-race")
		if err := os.WriteFile(path, []byte("content\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		script := "stat() { printf 'stat: ENOENT\\n' >&2; return 1; }\n" + observeProbeScript
		result, err := (process.Runner{}).Run(t.Context(), process.Command{
			Argv: []string{"/bin/sh", "-eu", "-c", script, "probe", path},
			Dir:  root,
			Env:  map[string]string{"PATH": "/bin:/usr/bin"},
		})
		if err != nil || len(result.Stderr) != 0 {
			t.Fatalf("observation = %q/%q, %v; want exit 0 with empty stderr", result.Stdout, result.Stderr, err)
		}
		observed, err := decodeProbeFile(result.Stdout)
		if err != nil || !observed.Exists || observed.Regular {
			t.Fatalf("observed = %+v, %v; want invalid existing observation", observed, err)
		}
	})

	t.Run("missing utilities fail immediately", func(t *testing.T) {
		path := filepath.Join(root, "missing-util")
		if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			name   string
			prefix string
		}{
			{name: "missing head", prefix: "head() { return 127; }\n"},
			{name: "missing stat", prefix: "stat() { return 127; }\n"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				script := tc.prefix + observeProbeScript
				if tc.name == "missing head" {
					script = "stat() { printf '600\\n'; }\n" + script
				}
				if _, err := (process.Runner{}).Run(t.Context(), process.Command{
					Argv: []string{"/bin/sh", "-eu", "-c", script, "probe", path},
					Dir:  root,
					Env:  map[string]string{"PATH": "/bin:/usr/bin"},
				}); err == nil {
					t.Fatal("missing utility succeeded; want immediate command failure")
				}
			})
		}
	})

	t.Run("exact bytes", func(t *testing.T) {
		cases := []struct {
			name string
			data []byte
		}{
			{name: "trailing newlines", data: []byte("content\n\n")},
			{name: "embedded NUL corruption", data: []byte("abox\x00probe\n")},
			{name: "literal ERR trailer", data: []byte("\nERR\n")},
			{name: "literal OK trailer", data: []byte("\nOK\n")},
			{name: "max bounded size", data: bytes.Repeat([]byte("a"), maxProbeDataBytes)},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				path := filepath.Join(root, tc.name)
				if err := os.WriteFile(path, tc.data, 0o600); err != nil {
					t.Fatal(err)
				}
				result, err := runScript(t, "", path)
				if err != nil || len(result.Stderr) != 0 {
					t.Fatalf("observation = %q/%q, %v", result.Stdout, result.Stderr, err)
				}
				if len(result.Stdout) > maxProbeDataBytes+32 {
					t.Fatalf("stdout length = %d, exceeds capture bound %d", len(result.Stdout), maxProbeDataBytes+32)
				}
				observed, err := decodeProbeFile(result.Stdout)
				if err != nil {
					t.Fatalf("decodeProbeFile() error = %v", err)
				}
				if !observed.Exists || !observed.Regular || observed.Mode.Perm() != fs.FileMode(0o600) || !bytes.Equal(observed.Data, tc.data) {
					t.Fatalf("observed = %+v, want regular 0600 with %q", observed, tc.data)
				}
			})
		}
	})

	t.Run("oversized file stays bounded and fails malformed", func(t *testing.T) {
		path := filepath.Join(root, "oversized")
		if err := os.WriteFile(path, bytes.Repeat([]byte("z"), 4096), 0o600); err != nil {
			t.Fatal(err)
		}
		result, err := runScript(t, "", path)
		if err != nil || len(result.Stderr) != 0 {
			t.Fatalf("observation = %q/%q, %v", result.Stdout, result.Stderr, err)
		}
		if len(result.Stdout) > maxProbeDataBytes+32 {
			t.Fatalf("oversized stdout = %d bytes, exceeded bound %d", len(result.Stdout), maxProbeDataBytes+32)
		}
		if _, err := decodeProbeFile(result.Stdout); err == nil {
			t.Fatal("decodeProbeFile() accepted oversized file")
		}
	})

	t.Run("unreadable regular file", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root bypasses chmod 0000 read checks")
		}
		path := filepath.Join(root, "unreadable")
		if err := os.WriteFile(path, []byte("secret"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

		result, err := runScript(t, "", path)
		if err != nil || len(result.Stderr) != 0 {
			t.Fatalf("observation = %q/%q, %v; want exit 0 with empty stderr", result.Stdout, result.Stderr, err)
		}
		observed, err := decodeProbeFile(result.Stdout)
		if err != nil {
			t.Fatalf("decodeProbeFile() error = %v", err)
		}
		if !observed.Exists || observed.Regular {
			t.Fatalf("observed = %+v, want invalid existing observation (never absent or valid)", observed)
		}
	})

	t.Run("absent and symlink", func(t *testing.T) {
		absentResult, err := runScript(t, "", filepath.Join(root, "absent"))
		if err != nil || len(absentResult.Stderr) != 0 {
			t.Fatalf("absent observation = %q/%q, %v", absentResult.Stdout, absentResult.Stderr, err)
		}
		absent, err := decodeProbeFile(absentResult.Stdout)
		if err != nil || absent.Exists {
			t.Fatalf("absent = %+v, %v", absent, err)
		}

		target := filepath.Join(root, "sym-target")
		link := filepath.Join(root, "sym-link")
		if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		symResult, err := runScript(t, "", link)
		if err != nil || len(symResult.Stderr) != 0 {
			t.Fatalf("symlink observation = %q/%q, %v", symResult.Stdout, symResult.Stderr, err)
		}
		sym, err := decodeProbeFile(symResult.Stdout)
		if err != nil || !sym.Exists || sym.Regular {
			t.Fatalf("symlink = %+v, %v", sym, err)
		}
	})
}

func TestWatchProbeScriptAcceptsBridgedAttribAndCleansUp(t *testing.T) {
	for _, event := range []string{"n", "c", "w", "e"} {
		t.Run("event-"+event, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, watchTargetName)
			ready := filepath.Join(root, watchReadyName)
			fakeInotifyd := `inotifyd() {
	dir=${2%%:*}
	while [ ! -e "$dir/watch-handshake" ]; do sleep 0.01; done
	printf 'n\t%s\twatch-handshake\n' "$dir"
	while [ ! -e "$dir/watch-ready" ]; do sleep 0.01; done
	printf 'e\t%s\tunrelated-file\n' "$dir"
	printf '` + event + `\t%s\twatch-target\n' "$dir"
	sleep 10
}
`
			result, err := (process.Runner{}).Run(t.Context(), process.Command{
				Argv: []string{"/bin/sh", "-eu", "-c", fakeInotifyd + watchProbeScript, "abox-probe", target, ready},
				Dir:  root,
				Env:  map[string]string{"PATH": "/bin:/usr/bin"},
			})
			if err != nil || len(result.Stdout) != 0 || len(result.Stderr) != 0 {
				t.Fatalf("watch event %q = %q/%q, %v", event, result.Stdout, result.Stderr, err)
			}
			for _, name := range []string{watchReadyName, "watch-handshake"} {
				if _, statErr := os.Lstat(filepath.Join(root, name)); !os.IsNotExist(statErr) {
					t.Fatalf("watch artifact %q survived cleanup: %v", name, statErr)
				}
			}
		})
	}

	t.Run("watchdog bounds lifetime and cleans up", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, watchTargetName)
		ready := filepath.Join(root, watchReadyName)
		fakeInotifyd := `inotifyd() {
	dir=${2%%:*}
	while [ ! -e "$dir/watch-handshake" ]; do sleep 0.01; done
	printf 'n\t%s\twatch-handshake\n' "$dir"
	sleep 10
}
`
		// Shorten the delivery watchdog in the script copy for a fast deterministic test.
		fastScript := strings.Replace(watchProbeScript, "start_watchdog 1.5", "start_watchdog 0.2", 1)
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()
		started := time.Now()
		_, err := (process.Runner{}).Run(ctx, process.Command{
			Argv: []string{"/bin/sh", "-eu", "-c", fakeInotifyd + fastScript, "abox-probe", target, ready},
			Dir:  root,
			Env:  map[string]string{"PATH": "/bin:/usr/bin"},
		})
		if err == nil {
			t.Fatal("undelivered watch succeeded")
		}
		if time.Since(started) > 2*time.Second {
			t.Fatalf("watchdog took too long: %v", time.Since(started))
		}
		for _, name := range []string{watchReadyName, "watch-handshake"} {
			if _, statErr := os.Lstat(filepath.Join(root, name)); !os.IsNotExist(statErr) {
				t.Fatalf("watch artifact %q survived watchdog cleanup: %v", name, statErr)
			}
		}
	})
}

func TestObserveMachineBoundsSizeAndPreservesExactBytes(t *testing.T) {
	root := t.TempDir()

	maxPath := filepath.Join(root, "max")
	maxData := bytes.Repeat([]byte("m"), maxProbeDataBytes)
	if err := os.WriteFile(maxPath, maxData, 0o600); err != nil {
		t.Fatal(err)
	}
	observed, err := observeMachine(maxPath)
	if err != nil || !observed.Exists || !observed.Regular || observed.Mode.Perm() != 0o600 || !bytes.Equal(observed.Data, maxData) {
		t.Fatalf("observeMachine(max) = %+v, %v", observed, err)
	}

	oversizedPath := filepath.Join(root, "oversized")
	if err := os.WriteFile(oversizedPath, bytes.Repeat([]byte("x"), maxProbeDataBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := observeMachine(oversizedPath); err == nil {
		t.Fatal("observeMachine(oversized) succeeded")
	}

	linkPath := filepath.Join(root, "link")
	if err := os.Symlink(maxPath, linkPath); err != nil {
		t.Fatal(err)
	}
	linkObserved, err := observeMachine(linkPath)
	if err != nil || !linkObserved.Exists || linkObserved.Regular {
		t.Fatalf("observeMachine(symlink) = %+v, %v", linkObserved, err)
	}
}
