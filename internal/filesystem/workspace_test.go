package filesystem_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/compose"
	"github.com/shoe-point-0/agent-box/internal/filesystem"
	"github.com/shoe-point-0/agent-box/internal/lima"
)

func TestWorkspaceAdapterUsesFixedArgvAndDecodesObservations(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".abox-probe", strings.Repeat("ab", 16), "source")
	host := &workspaceHost{captures: []lima.Capture{
		{Stdout: []byte("A\n")},
		{Stdout: []byte("N\n")},
		{Stdout: []byte("F 640\nprobe-bytes\nOK\n")},
		{}, {}, {}, {},
	}}
	adapter, err := filesystem.NewWorkspaceAdapter(host, workspaceRuntime(root))
	if err != nil {
		t.Fatal(err)
	}

	absent, err := adapter.Observe(t.Context(), "immutable-id", path)
	if err != nil || absent.Exists {
		t.Fatalf("Observe(absent) = %+v, %v", absent, err)
	}
	nonregular, err := adapter.Observe(t.Context(), "immutable-id", path)
	if err != nil || !nonregular.Exists || nonregular.Regular {
		t.Fatalf("Observe(nonregular) = %+v, %v", nonregular, err)
	}
	regular, err := adapter.Observe(t.Context(), "immutable-id", path)
	if err != nil || !regular.Regular || regular.Mode.Perm() != 0o640 || string(regular.Data) != "probe-bytes" {
		t.Fatalf("Observe(regular) = %+v, %v", regular, err)
	}
	if err := adapter.Write(t.Context(), "immutable-id", path, []byte("probe-bytes"), 0o640); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	target := filepath.Join(filepath.Dir(path), "target")
	if err := adapter.Rename(t.Context(), "immutable-id", path, target); err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if err := adapter.Remove(t.Context(), "immutable-id", target); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	watchTarget := filepath.Join(filepath.Dir(path), "watch-target")
	watchReady := filepath.Join(filepath.Dir(path), "watch-ready")
	if err := adapter.Watch(t.Context(), "immutable-id", watchTarget, watchReady); err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	if len(host.calls) != 7 {
		t.Fatalf("calls = %d, want 7", len(host.calls))
	}
	for _, call := range host.calls {
		if len(call) < 9 || !reflect.DeepEqual(call[:7], []string{
			"/usr/bin/docker", "exec", "--", "immutable-id", "/bin/sh", "-eu", "-c",
		}) || call[8] != "abox-probe" {
			t.Fatalf("unsafe workspace argv = %#v", call)
		}
		if strings.Contains(call[7], root) || strings.Contains(call[7], "probe-bytes") {
			t.Fatalf("runtime value interpolated into fixed script: %q", call[7])
		}
	}
}

func TestWorkspaceAdapterRejectsRequestsOutsideOwnedProbeNonce(t *testing.T) {
	root := t.TempDir()
	host := &workspaceHost{}
	adapter, err := filesystem.NewWorkspaceAdapter(host, workspaceRuntime(root))
	if err != nil {
		t.Fatal(err)
	}
	valid := filepath.Join(root, ".abox-probe", strings.Repeat("01", 16), "source")
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "invalid container", run: func() error { _, err := adapter.Observe(t.Context(), "bad\nid", valid); return err }},
		{name: "outside nonce", run: func() error {
			return adapter.Write(t.Context(), "id", filepath.Join(root, "source"), []byte("data"), 0o600)
		}},
		{name: "oversized data", run: func() error { return adapter.Write(t.Context(), "id", valid, make([]byte, 257), 0o600) }},
		{name: "unsupported mode", run: func() error { return adapter.Write(t.Context(), "id", valid, []byte("data"), 0o666) }},
		{name: "cross nonce rename", run: func() error {
			other := filepath.Join(root, ".abox-probe", strings.Repeat("02", 16), "target")
			return adapter.Rename(t.Context(), "id", valid, other)
		}},
		{name: "invalid watch names", run: func() error {
			return adapter.Watch(t.Context(), "id", valid, filepath.Join(filepath.Dir(valid), "watch-ready"))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); err == nil {
				t.Fatal("invalid request succeeded")
			}
		})
	}
	if len(host.calls) != 0 {
		t.Fatalf("invalid requests invoked workspace commands: %#v", host.calls)
	}
}

func TestWorkspaceAdapterBoundsAndRedactsCommandFailures(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".abox-probe", strings.Repeat("ab", 16), "source")
	secret := "secret-command-output"
	tests := []struct {
		name    string
		capture lima.Capture
		err     error
		cancel  bool
	}{
		{name: "command", err: errors.New(secret)},
		{name: "stderr", capture: lima.Capture{Stderr: []byte(secret)}},
		{name: "truncated", capture: lima.Capture{StdoutTruncated: true}},
		{name: "malformed", capture: lima.Capture{Stdout: []byte("F nope\n" + secret + "\nOK\n")}},
		{name: "missing-trailer", capture: lima.Capture{Stdout: []byte("F 600\n" + secret)}},
		{name: "canceled", err: context.Canceled, cancel: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			if test.cancel {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			host := &workspaceHost{captures: []lima.Capture{test.capture}, errs: []error{test.err}}
			adapter, err := filesystem.NewWorkspaceAdapter(host, workspaceRuntime(root))
			if err != nil {
				t.Fatal(err)
			}
			_, err = adapter.Observe(ctx, "immutable-id", path)
			if err == nil || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), root) {
				t.Fatalf("Observe() error = %v", err)
			}
			if test.cancel && !strings.Contains(err.Error(), "canceled") {
				t.Fatalf("cancellation error = %v", err)
			}
		})
	}
}

func TestNewWorkspaceAdapterRejectsInvalidBoundary(t *testing.T) {
	if _, err := filesystem.NewWorkspaceAdapter(nil, compose.RuntimeSpec{DockerPath: "/usr/bin/docker", Workdir: "/"}); err == nil {
		t.Fatal("NewWorkspaceAdapter(nil) succeeded")
	}
	if _, err := filesystem.NewWorkspaceAdapter(&workspaceHost{}, compose.RuntimeSpec{DockerPath: "docker", Workdir: "/"}); err == nil {
		t.Fatal("NewWorkspaceAdapter(relative Docker path) succeeded")
	}
}

func workspaceRuntime(workdir string) compose.RuntimeSpec {
	return compose.RuntimeSpec{DockerPath: "/usr/bin/docker", Workdir: workdir}
}

type workspaceHost struct {
	captures []lima.Capture
	errs     []error
	calls    [][]string
}

func (h *workspaceHost) Capture(_ context.Context, _ lima.Spec, _ string, argv []string) (lima.Capture, error) {
	h.calls = append(h.calls, append([]string(nil), argv...))
	index := len(h.calls) - 1
	var capture lima.Capture
	var err error
	if index < len(h.captures) {
		capture = h.captures[index]
	}
	if index < len(h.errs) {
		err = h.errs[index]
	}
	return capture, err
}
