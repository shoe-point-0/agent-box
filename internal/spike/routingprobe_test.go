package spike

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRoutingHandlerSelectsExactHostAndRebuildsMetadata(t *testing.T) {
	t.Parallel()

	backend := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(writer).Encode(routingObservation{Backend: "alpha", Host: request.Host, Headers: request.Header.Clone()})
	}))
	defer backend.Close()
	backendURL, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatal(err)
	}
	handler := newRoutingHandler(map[string]routingTarget{
		alphaRoutingHost: {BoxID: "box-a", Route: "alpha", Backend: backendURL},
	})

	request := httptest.NewRequest(http.MethodGet, "http://"+alphaRoutingHost+"/identity", nil)
	request.RemoteAddr = "127.0.0.1:43210"
	request.Header.Set("X-Abox-Box-ID", "attacker")
	request.Header.Set("X-Abox-Injected", "attacker")
	request.Header.Set("X-Forwarded-For", "203.0.113.7")
	request.Header.Set("X-Forwarded-Host", "attacker.invalid")
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("Forwarded", "for=203.0.113.7;host=attacker.invalid;proto=https")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("routing status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
	var observed routingObservation
	if err := json.Unmarshal(recorder.Body.Bytes(), &observed); err != nil {
		t.Fatal(err)
	}
	result := validateRoutingObservation(alphaRoutingHost, "alpha", routingTarget{BoxID: "box-a", Route: "alpha"}, observed)
	if !result.UntrustedControlsStripped || !result.TrustedControlsCorrect || !result.ForwardingMetadataCorrect || result.ObservedBackend != "alpha" {
		t.Fatalf("routing result = %+v", result)
	}

	unknown := httptest.NewRecorder()
	unknownRequest := httptest.NewRequest(http.MethodGet, "http://missing.abox.localhost/identity", nil)
	handler.ServeHTTP(unknown, unknownRequest)
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown host status = %d, want %d", unknown.Code, http.StatusNotFound)
	}

	malformed := httptest.NewRecorder()
	malformedRequest := httptest.NewRequest(http.MethodGet, "http://example.invalid/identity", nil)
	malformedRequest.Host = "alpha.box-a.abox.localhost:bad"
	handler.ServeHTTP(malformed, malformedRequest)
	if malformed.Code != http.StatusBadRequest {
		t.Fatalf("malformed host status = %d, want %d", malformed.Code, http.StatusBadRequest)
	}
}

func TestRoutingProtocolsTraverseOneListener(t *testing.T) {
	backend, err := startRoutingBackend("alpha")
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := newRoutingServer(map[string]routingTarget{
		alphaRoutingHost: {BoxID: "box-a", Route: "alpha", Backend: backend.URL()},
	})
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		if err := <-serveDone; err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("Serve() error = %v", err)
		}
	}()

	protocols, err := exerciseRoutingProtocols(context.Background(), listener.Addr().(*net.TCPAddr).Port, backend)
	if err != nil {
		t.Fatalf("exerciseRoutingProtocols() error = %v", err)
	}
	if !protocols.WebSocket || !protocols.ServerSentEvents || !protocols.StreamingRequest || !protocols.StreamingResponse {
		t.Fatalf("protocol results = %+v", protocols)
	}
}

func TestNormalizeRoutingHost(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		authority string
		want      string
		valid     bool
	}{
		{authority: "Alpha.Box-A.Abox.Localhost:7777", want: alphaRoutingHost, valid: true},
		{authority: "alpha.box-a.abox.localhost.", want: alphaRoutingHost, valid: true},
		{authority: "alpha.box-a.abox.localhost:bad", valid: false},
		{authority: "alpha.box-a.abox.localhost/path", valid: false},
		{authority: "", valid: false},
	} {
		got, valid := normalizeRoutingHost(test.authority)
		if got != test.want || valid != test.valid {
			t.Errorf("normalizeRoutingHost(%q) = %q, %t; want %q, %t", test.authority, got, valid, test.want, test.valid)
		}
	}
}

func TestRoutingBrowserProfileCleanupRequiresExactMarker(t *testing.T) {
	t.Parallel()

	workspace, err := NewWorkspace(t.TempDir(), RoutingProbe)
	if err != nil {
		t.Fatal(err)
	}
	environment := &Environment{root: workspace.Root(), workspace: workspace}
	profile, resource, err := createRoutingBrowserProfile(environment)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, routingProfileMarker), []byte("wrong\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeRoutingBrowserProfile(workspace, resource); !errors.Is(err, ErrCleanupRefused) {
		t.Fatalf("removeRoutingBrowserProfile(wrong marker) error = %v, want ErrCleanupRefused", err)
	}
	if _, err := os.Stat(profile); err != nil {
		t.Fatalf("browser profile removed after refusal: %v", err)
	}
}
