package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/daemon"
)

func TestRunReportsVersion(t *testing.T) {
	t.Parallel()

	for _, argument := range []string{"--version", "version"} {
		argument := argument
		t.Run(argument, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			if code := run(context.Background(), []string{argument}, &stdout, &stderr); code != 0 {
				t.Fatalf("run(%q) code = %d, want 0; stderr = %q", argument, code, stderr.String())
			}
			if got := stdout.String(); !strings.HasPrefix(got, "aboxd devel (commit unknown, built unknown)\n") {
				t.Fatalf("run(%q) stdout = %q, want development build identity", argument, got)
			}
			if stderr.Len() != 0 {
				t.Fatalf("run(%q) stderr = %q, want empty", argument, stderr.String())
			}
		})
	}
}

func TestRunServesHandshakeAndShutsDown(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	runtimeDir := filepath.Join(root, "runtime")
	stateDatabase := filepath.Join(root, "state", "state.db")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stdout, stderr bytes.Buffer
	exit := make(chan int, 1)
	go func() {
		exit <- run(ctx, []string{"serve", "--runtime-dir", runtimeDir, "--state-db", stateDatabase}, &stdout, &stderr)
	}()

	socketPath := filepath.Join(runtimeDir, "aboxd.sock")
	waitForSocket(t, socketPath)
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://unix/v1/handshake", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set(daemon.HeaderAPIMin, "1")
	request.Header.Set(daemon.HeaderAPIMax, "1")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var handshake struct {
		APIVersion string `json:"apiVersion"`
	}
	if err := json.NewDecoder(response.Body).Decode(&handshake); err != nil {
		_ = response.Body.Close()
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || handshake.APIVersion != contract.DaemonAPIVersion {
		t.Fatalf("handshake status/version = %d/%q", response.StatusCode, handshake.APIVersion)
	}

	cancel()
	select {
	case code := <-exit:
		if code != contract.ExitOK {
			t.Fatalf("run() code = %d, want %d; stderr=%q", code, contract.ExitOK, stderr.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("daemon did not shut down")
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("daemon output stdout/stderr = %q/%q, want empty", stdout.String(), stderr.String())
	}
}

func waitForSocket(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if info, err := os.Lstat(path); err == nil && info.Mode().Type() == os.ModeSocket {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("socket %q did not appear", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
