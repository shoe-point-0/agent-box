package spike

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const (
	RoutingProbe               = "localhost-routing"
	routingReportSchema        = "abox.spike.localhost-routing/v1"
	routingProfileName         = "routing-browser-profile"
	routingProfileMarker       = ".abox-spike-browser-owner"
	routingControlHeaderPrefix = "X-Abox-"
)

const (
	alphaRoutingHost = "alpha.box-a.abox.localhost"
	betaRoutingHost  = "beta.box-b.abox.localhost"
)

type RoutingProbeReport struct {
	SchemaVersion       string                `json:"schemaVersion"`
	Listener            string                `json:"listener"`
	ListenerFamily      string                `json:"listenerFamily"`
	Routes              []RoutingRouteResult  `json:"routes"`
	CurlVersion         string                `json:"curlVersion"`
	Browser             RoutingBrowserResult  `json:"browser"`
	Protocols           RoutingProtocolResult `json:"protocols"`
	Resolver            RoutingResolverResult `json:"resolver"`
	NetworkEnvironment  RoutingNetworkResult  `json:"networkEnvironment"`
	Proxy               RoutingProxyResult    `json:"proxy"`
	CompatibilityLimits []string              `json:"compatibilityLimits"`
}

type RoutingRouteResult struct {
	Host                      string `json:"host"`
	ExpectedBackend           string `json:"expectedBackend"`
	ObservedBackend           string `json:"observedBackend"`
	UntrustedControlsStripped bool   `json:"untrustedControlsStripped"`
	TrustedControlsCorrect    bool   `json:"trustedControlsCorrect"`
	ForwardingMetadataCorrect bool   `json:"forwardingMetadataCorrect"`
}

type RoutingBrowserResult struct {
	Command string `json:"command"`
	Version string `json:"version"`
	Host    string `json:"host"`
	Passed  bool   `json:"passed"`
}

type RoutingProtocolResult struct {
	WebSocket         bool `json:"webSocket"`
	ServerSentEvents  bool `json:"serverSentEvents"`
	StreamingRequest  bool `json:"streamingRequest"`
	StreamingResponse bool `json:"streamingResponse"`
}

type RoutingResolverResult struct {
	Host          string   `json:"host"`
	Addresses     []string `json:"addresses"`
	IPv4Resolved  bool     `json:"ipv4Resolved"`
	IPv6Resolved  bool     `json:"ipv6Resolved"`
	IPv4Reachable bool     `json:"ipv4Reachable"`
	IPv6Reachable bool     `json:"ipv6Reachable"`
}

type RoutingNetworkResult struct {
	ProxyEnvironmentVariables []string `json:"proxyEnvironmentVariables"`
	VPNLikeInterfaces         []string `json:"vpnLikeInterfaces"`
}

type RoutingProxyResult struct {
	ExplicitProxyIntercepted bool `json:"explicitProxyIntercepted"`
	LocalhostBypassSucceeded bool `json:"localhostBypassSucceeded"`
}

type routingTarget struct {
	BoxID   string
	Route   string
	Backend *url.URL
}

type routingObservation struct {
	Backend string              `json:"backend"`
	Host    string              `json:"host"`
	Headers map[string][]string `json:"headers"`
}

type routingBackend struct {
	name            string
	server          *http.Server
	listener        net.Listener
	uploadFirstRead chan struct{}
	streamRelease   chan struct{}
}

func RegisterRoutingProbe(registry *Registry) error {
	return registry.RegisterScenario(RoutingProbe, Scenario{
		Run:     runRoutingProbe,
		Cleanup: cleanupRoutingProbe,
	})
}

func runRoutingProbe(ctx context.Context, env *Environment) (runErr error) {
	alpha, err := startRoutingBackend("alpha")
	if err != nil {
		return err
	}
	defer alpha.Close()
	beta, err := startRoutingBackend("beta")
	if err != nil {
		return err
	}
	defer beta.Close()

	routerListener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen on IPv4 loopback: %w", err)
	}
	routes := map[string]routingTarget{
		alphaRoutingHost: {BoxID: "box-a", Route: "alpha", Backend: alpha.URL()},
		betaRoutingHost:  {BoxID: "box-b", Route: "beta", Backend: beta.URL()},
	}
	routerServer := newRoutingServer(routes)
	serveDone := make(chan error, 1)
	go func() { serveDone <- routerServer.Serve(routerListener) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		shutdownErr := routerServer.Shutdown(shutdownCtx)
		serveErr := <-serveDone
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) && runErr == nil {
			runErr = fmt.Errorf("serve routing probe: %w", serveErr)
		}
		if shutdownErr != nil && runErr == nil {
			runErr = fmt.Errorf("shutdown routing probe: %w", shutdownErr)
		}
	}()

	port := routerListener.Addr().(*net.TCPAddr).Port
	baseURL := func(host, path string) string {
		return fmt.Sprintf("http://%s:%d%s", host, port, path)
	}
	curlVersion, err := runText(ctx, env, "routing-curl-version", []string{"curl", "--version"})
	if err != nil {
		return err
	}
	curlVersion = firstNonemptyLine(curlVersion)

	routeResults := make([]RoutingRouteResult, 0, len(routes))
	for _, test := range []struct {
		host    string
		backend string
	}{
		{host: alphaRoutingHost, backend: "alpha"},
		{host: betaRoutingHost, backend: "beta"},
	} {
		output, err := runText(ctx, env, "routing-curl-"+test.backend, []string{
			"curl", "--silent", "--show-error", "--fail", "--noproxy", "*",
			"--header", "X-Abox-Box-ID: attacker-box",
			"--header", "X-Abox-Route: attacker-route",
			"--header", "X-Abox-Injected: attacker-control",
			"--header", "X-Forwarded-For: 203.0.113.7",
			"--header", "X-Forwarded-Host: attacker.invalid",
			"--header", "X-Forwarded-Proto: https",
			"--header", "Forwarded: for=203.0.113.7;host=attacker.invalid;proto=https",
			baseURL(test.host, "/identity"),
		})
		if err != nil {
			return err
		}
		var observed routingObservation
		if err := json.Unmarshal([]byte(output), &observed); err != nil {
			return fmt.Errorf("decode %s routing observation: %w", test.backend, err)
		}
		result := validateRoutingObservation(test.host, test.backend, routes[test.host], observed)
		if result.ObservedBackend != test.backend || !result.UntrustedControlsStripped || !result.TrustedControlsCorrect || !result.ForwardingMetadataCorrect {
			return fmt.Errorf("routing invariant failed for %s: %+v", test.host, result)
		}
		routeResults = append(routeResults, result)
	}

	unknownStatus, err := fetchStatus(ctx, baseURL("missing.abox.localhost", "/identity"))
	if err != nil {
		return err
	}
	if unknownStatus != http.StatusNotFound {
		return fmt.Errorf("unknown host status = %d, want %d", unknownStatus, http.StatusNotFound)
	}

	browser, err := runRoutingBrowser(ctx, env, baseURL(alphaRoutingHost, "/browser"))
	if err != nil {
		return err
	}
	protocols, err := exerciseRoutingProtocols(ctx, port, alpha)
	if err != nil {
		return err
	}
	resolver, limits, err := inspectRoutingResolver(ctx, alphaRoutingHost, port)
	if err != nil {
		return err
	}
	networkEnvironment, err := inspectRoutingNetworkEnvironment()
	if err != nil {
		return err
	}
	proxy, err := exerciseRoutingProxy(ctx, env, baseURL(betaRoutingHost, "/identity"))
	if err != nil {
		return err
	}
	if proxy.ExplicitProxyIntercepted {
		limits = append(limits, "explicit HTTP proxies intercept .localhost unless the client configures a localhost bypass")
	}
	if len(networkEnvironment.ProxyEnvironmentVariables) == 0 {
		limits = append(limits, "no Machine proxy environment was configured; only the controlled explicit-proxy case was exercised")
	}
	if len(networkEnvironment.VPNLikeInterfaces) == 0 {
		limits = append(limits, "no active VPN-like interface was available during this run")
	}
	limits = append(limits, "Lima forwarding and Darwin browser/resolver behavior remain deferred to the Darwin/arm64 hardware gate")
	sort.Strings(limits)

	report := RoutingProbeReport{
		SchemaVersion:       routingReportSchema,
		Listener:            routerListener.Addr().String(),
		ListenerFamily:      "tcp4-loopback",
		Routes:              routeResults,
		CurlVersion:         curlVersion,
		Browser:             browser,
		Protocols:           protocols,
		Resolver:            resolver,
		NetworkEnvironment:  networkEnvironment,
		Proxy:               proxy,
		CompatibilityLimits: limits,
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode routing report: %w", err)
	}
	data = append(data, '\n')
	if _, err := env.workspace.WriteDiagnostic("routing-report.json", data); err != nil {
		return err
	}
	return nil
}

func newRoutingServer(routes map[string]routingTarget) *http.Server {
	return &http.Server{
		Handler:           newRoutingHandler(routes),
		ReadHeaderTimeout: 2 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    32 * 1024,
	}
}

func newRoutingHandler(routes map[string]routingTarget) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		host, ok := normalizeRoutingHost(request.Host)
		if !ok {
			http.Error(writer, "invalid route authority", http.StatusBadRequest)
			return
		}
		target, ok := routes[host]
		if !ok {
			http.NotFound(writer, request)
			return
		}
		proxy := &httputil.ReverseProxy{
			Rewrite: func(proxyRequest *httputil.ProxyRequest) {
				stripRoutingControlHeaders(proxyRequest.Out.Header)
				proxyRequest.Out.Header.Del("Forwarded")
				proxyRequest.SetURL(target.Backend)
				proxyRequest.Out.Host = proxyRequest.In.Host
				proxyRequest.SetXForwarded()
				proxyRequest.Out.Header.Set("X-Abox-Box-ID", target.BoxID)
				proxyRequest.Out.Header.Set("X-Abox-Route", target.Route)
			},
			FlushInterval: -1,
			ErrorHandler: func(writer http.ResponseWriter, _ *http.Request, proxyErr error) {
				http.Error(writer, "route backend unavailable", http.StatusBadGateway)
			},
		}
		proxy.ServeHTTP(writer, request)
	})
}

func normalizeRoutingHost(authority string) (string, bool) {
	if authority == "" || strings.ContainsAny(authority, " /\\\t\r\n") {
		return "", false
	}
	host := authority
	if strings.Contains(authority, ":") {
		var port string
		var err error
		host, port, err = net.SplitHostPort(authority)
		if err != nil {
			return "", false
		}
		portNumber, err := strconv.Atoi(port)
		if err != nil || portNumber < 1 || portNumber > 65535 {
			return "", false
		}
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	return host, host != ""
}

func stripRoutingControlHeaders(headers http.Header) {
	for name := range headers {
		if strings.HasPrefix(strings.ToLower(name), strings.ToLower(routingControlHeaderPrefix)) {
			headers.Del(name)
		}
	}
}

func validateRoutingObservation(host, expectedBackend string, target routingTarget, observed routingObservation) RoutingRouteResult {
	headers := http.Header(observed.Headers)
	controlsStripped := headers.Get("X-Abox-Injected") == ""
	trustedCorrect := headers.Get("X-Abox-Box-ID") == target.BoxID && headers.Get("X-Abox-Route") == target.Route
	forwardedFor := headers.Get("X-Forwarded-For")
	observedHost, validHost := normalizeRoutingHost(observed.Host)
	forwardingCorrect := validHost && observedHost == host && headers.Get("Forwarded") == "" && headers.Get("X-Forwarded-Host") == observed.Host && headers.Get("X-Forwarded-Proto") == "http" && net.ParseIP(forwardedFor) != nil && forwardedFor != "203.0.113.7"
	return RoutingRouteResult{
		Host:                      host,
		ExpectedBackend:           expectedBackend,
		ObservedBackend:           observed.Backend,
		UntrustedControlsStripped: controlsStripped,
		TrustedControlsCorrect:    trustedCorrect,
		ForwardingMetadataCorrect: forwardingCorrect,
	}
}

func startRoutingBackend(name string) (*routingBackend, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for %s backend: %w", name, err)
	}
	backend := &routingBackend{
		name:            name,
		listener:        listener,
		uploadFirstRead: make(chan struct{}, 1),
		streamRelease:   make(chan struct{}),
	}
	backend.server = &http.Server{Handler: backend, ReadHeaderTimeout: 2 * time.Second}
	go func() { _ = backend.server.Serve(listener) }()
	return backend, nil
}

func (backend *routingBackend) URL() *url.URL {
	return &url.URL{Scheme: "http", Host: backend.listener.Addr().String()}
}

func (backend *routingBackend) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = backend.server.Shutdown(ctx)
}

func (backend *routingBackend) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case "/identity":
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(routingObservation{Backend: backend.name, Host: request.Host, Headers: request.Header.Clone()})
	case "/browser":
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(writer, "<!doctype html><html><body>abox-browser-%s</body></html>", backend.name)
	case "/sse":
		writer.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(writer, "data: %s\n\n", backend.name)
		if flusher, ok := writer.(http.Flusher); ok {
			flusher.Flush()
		}
	case "/upload":
		first := make([]byte, 5)
		if _, err := io.ReadFull(request.Body, first); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		backend.uploadFirstRead <- struct{}{}
		rest, err := io.ReadAll(request.Body)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		_, _ = writer.Write(append(first, rest...))
	case "/stream":
		writer.Header().Set("Content-Type", "application/octet-stream")
		_, _ = io.WriteString(writer, "first\n")
		if flusher, ok := writer.(http.Flusher); ok {
			flusher.Flush()
		}
		select {
		case <-backend.streamRelease:
		case <-request.Context().Done():
			return
		}
		_, _ = io.WriteString(writer, "second\n")
	case "/ws":
		serveRoutingWebSocket(writer, request, backend.name)
	default:
		http.NotFound(writer, request)
	}
}

func runRoutingBrowser(ctx context.Context, env *Environment, targetURL string) (result RoutingBrowserResult, runErr error) {
	browser, err := findRoutingBrowser()
	if err != nil {
		return result, err
	}
	versionOutput, err := runText(ctx, env, "routing-browser-version", []string{browser, "--version"})
	if err != nil {
		return result, err
	}
	profile, resource, err := createRoutingBrowserProfile(env)
	if err != nil {
		return result, err
	}
	defer func() {
		if cleanupErr := removeRoutingBrowserProfile(env.workspace, resource); cleanupErr != nil && runErr == nil {
			runErr = cleanupErr
		}
	}()
	output, err := runText(ctx, env, "routing-browser-fetch", []string{
		browser, "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
		"--no-proxy-server", "--user-data-dir=" + profile, "--dump-dom", targetURL,
	})
	if err != nil {
		return result, err
	}
	result = RoutingBrowserResult{
		Command: filepath.Base(browser),
		Version: strings.TrimSpace(versionOutput),
		Host:    alphaRoutingHost,
		Passed:  strings.Contains(output, "abox-browser-alpha"),
	}
	if !result.Passed {
		return result, errors.New("headless browser did not render the selected backend identity")
	}
	return result, nil
}

func findRoutingBrowser() (string, error) {
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome"} {
		path, err := exec.LookPath(name)
		if err == nil {
			return path, nil
		}
	}
	return "", errors.New("routing probe requires Chromium or Google Chrome for the browser resolver path")
}

func createRoutingBrowserProfile(env *Environment) (string, Resource, error) {
	path, err := env.workspace.resolveRelative(routingProfileName)
	if err != nil {
		return "", Resource{}, err
	}
	resource := Resource{Kind: "routing-browser-profile", ID: path, OwnerID: env.workspace.metadata.ID, Attributes: map[string]string{"ownerMarker": routingProfileMarker}}
	if _, err := env.workspace.inventory.Append("planned", resource); err != nil {
		return "", Resource{}, err
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		return "", Resource{}, fmt.Errorf("create browser profile: %w", err)
	}
	if err := writeSyncedFile(filepath.Join(path, routingProfileMarker), []byte(env.workspace.metadata.ID+"\n"), 0o600); err != nil {
		return "", Resource{}, err
	}
	if _, err := env.workspace.inventory.Append("created", resource); err != nil {
		return "", Resource{}, err
	}
	return path, resource, nil
}

func removeRoutingBrowserProfile(workspace *Workspace, resource Resource) error {
	if resource.OwnerID != workspace.metadata.ID || filepath.Base(resource.ID) != routingProfileName || !isWithin(workspace.Root(), resource.ID) {
		return fmt.Errorf("%w: browser profile identity mismatch", ErrCleanupRefused)
	}
	info, err := os.Lstat(resource.ID)
	if errors.Is(err, os.ErrNotExist) {
		return workspace.MarkRemoved(resource)
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: browser profile is not a real directory", ErrCleanupRefused)
	}
	marker, err := os.ReadFile(filepath.Join(resource.ID, routingProfileMarker))
	if err != nil || strings.TrimSpace(string(marker)) != workspace.metadata.ID {
		return fmt.Errorf("%w: browser profile ownership marker mismatch", ErrCleanupRefused)
	}
	if err := os.RemoveAll(resource.ID); err != nil {
		return fmt.Errorf("remove owned browser profile: %w", err)
	}
	return workspace.MarkRemoved(resource)
}

func cleanupRoutingProbe(_ context.Context, workspace *Workspace, resources []Resource) error {
	for _, resource := range resources {
		if resource.Kind == "routing-browser-profile" {
			return removeRoutingBrowserProfile(workspace, resource)
		}
	}
	return nil
}

func exerciseRoutingProtocols(ctx context.Context, port int, backend *routingBackend) (RoutingProtocolResult, error) {
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	client := &http.Client{Timeout: 5 * time.Second}

	sseRequest, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/sse", nil)
	sseRequest.Host = alphaRoutingHost
	sseResponse, err := client.Do(sseRequest)
	if err != nil {
		return RoutingProtocolResult{}, fmt.Errorf("request SSE route: %w", err)
	}
	sseData, readErr := io.ReadAll(sseResponse.Body)
	_ = sseResponse.Body.Close()
	if readErr != nil || string(sseData) != "data: alpha\n\n" {
		return RoutingProtocolResult{}, fmt.Errorf("SSE response = %q, error = %v", sseData, readErr)
	}

	reader, writer := io.Pipe()
	uploadRequest, _ := http.NewRequestWithContext(ctx, http.MethodPost, base+"/upload", reader)
	uploadRequest.Host = alphaRoutingHost
	uploadDone := make(chan struct {
		body string
		err  error
	}, 1)
	go func() {
		response, requestErr := client.Do(uploadRequest)
		if requestErr != nil {
			uploadDone <- struct {
				body string
				err  error
			}{err: requestErr}
			return
		}
		body, requestErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		uploadDone <- struct {
			body string
			err  error
		}{body: string(body), err: requestErr}
	}()
	if _, err := io.WriteString(writer, "first"); err != nil {
		return RoutingProtocolResult{}, err
	}
	select {
	case <-backend.uploadFirstRead:
	case <-time.After(2 * time.Second):
		return RoutingProtocolResult{}, errors.New("backend did not receive the first request chunk before upload completion")
	}
	if _, err := io.WriteString(writer, "-second"); err != nil {
		return RoutingProtocolResult{}, err
	}
	_ = writer.Close()
	uploadResult := <-uploadDone
	if uploadResult.err != nil || uploadResult.body != "first-second" {
		return RoutingProtocolResult{}, fmt.Errorf("streaming upload response = %q, error = %v", uploadResult.body, uploadResult.err)
	}

	streamRequest, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/stream", nil)
	streamRequest.Host = alphaRoutingHost
	streamResponse, err := client.Do(streamRequest)
	if err != nil {
		return RoutingProtocolResult{}, fmt.Errorf("request streaming route: %w", err)
	}
	streamReader := bufio.NewReader(streamResponse.Body)
	first, err := streamReader.ReadString('\n')
	if err != nil || first != "first\n" {
		_ = streamResponse.Body.Close()
		return RoutingProtocolResult{}, fmt.Errorf("first streaming chunk = %q, error = %v", first, err)
	}
	close(backend.streamRelease)
	second, err := streamReader.ReadString('\n')
	_ = streamResponse.Body.Close()
	if err != nil || second != "second\n" {
		return RoutingProtocolResult{}, fmt.Errorf("second streaming chunk = %q, error = %v", second, err)
	}

	if err := exerciseRoutingWebSocket(ctx, port, alphaRoutingHost, "probe-message"); err != nil {
		return RoutingProtocolResult{}, err
	}
	return RoutingProtocolResult{WebSocket: true, ServerSentEvents: true, StreamingRequest: true, StreamingResponse: true}, nil
}

func serveRoutingWebSocket(writer http.ResponseWriter, request *http.Request, backend string) {
	if !strings.EqualFold(request.Header.Get("Upgrade"), "websocket") || !strings.Contains(strings.ToLower(request.Header.Get("Connection")), "upgrade") {
		http.Error(writer, "websocket upgrade required", http.StatusUpgradeRequired)
		return
	}
	key := request.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(writer, "websocket key required", http.StatusBadRequest)
		return
	}
	hijacker, ok := writer.(http.Hijacker)
	if !ok {
		http.Error(writer, "websocket unsupported", http.StatusInternalServerError)
		return
	}
	connection, buffer, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer connection.Close()
	acceptHash := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	fmt.Fprintf(buffer, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(acceptHash[:]))
	if err := buffer.Flush(); err != nil {
		return
	}
	payload, err := readRoutingWebSocketFrame(buffer.Reader)
	if err != nil {
		return
	}
	_ = writeRoutingWebSocketFrame(buffer.Writer, []byte(backend+":"+string(payload)))
	_ = buffer.Flush()
}

func exerciseRoutingWebSocket(ctx context.Context, port int, host, payload string) error {
	dialer := net.Dialer{Timeout: 2 * time.Second}
	connection, err := dialer.DialContext(ctx, "tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("dial WebSocket route: %w", err)
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(3 * time.Second))
	key := base64.StdEncoding.EncodeToString([]byte("abox-routing-key"))
	fmt.Fprintf(connection, "GET /ws HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: %s\r\n\r\n", host, key)
	reader := bufio.NewReader(connection)
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodGet})
	if err != nil {
		return fmt.Errorf("read WebSocket handshake: %w", err)
	}
	if response.StatusCode != http.StatusSwitchingProtocols {
		return fmt.Errorf("WebSocket handshake status = %d", response.StatusCode)
	}
	if err := writeMaskedRoutingWebSocketFrame(connection, []byte(payload)); err != nil {
		return err
	}
	echo, err := readRoutingWebSocketFrame(reader)
	if err != nil {
		return err
	}
	if string(echo) != "alpha:"+payload {
		return fmt.Errorf("WebSocket echo = %q", echo)
	}
	return nil
}

func readRoutingWebSocketFrame(reader io.Reader) ([]byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(reader, header); err != nil {
		return nil, fmt.Errorf("read WebSocket frame header: %w", err)
	}
	length := int(header[1] & 0x7f)
	if length >= 126 {
		return nil, errors.New("routing WebSocket fixture only supports short frames")
	}
	masked := header[1]&0x80 != 0
	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(reader, mask[:]); err != nil {
			return nil, err
		}
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	if masked {
		for index := range payload {
			payload[index] ^= mask[index%len(mask)]
		}
	}
	return payload, nil
}

func writeMaskedRoutingWebSocketFrame(writer io.Writer, payload []byte) error {
	if len(payload) >= 126 {
		return errors.New("routing WebSocket fixture only supports short frames")
	}
	mask := [4]byte{0x12, 0x34, 0x56, 0x78}
	frame := []byte{0x81, byte(len(payload)) | 0x80}
	frame = append(frame, mask[:]...)
	for index, value := range payload {
		frame = append(frame, value^mask[index%len(mask)])
	}
	_, err := writer.Write(frame)
	return err
}

func writeRoutingWebSocketFrame(writer io.Writer, payload []byte) error {
	if len(payload) >= 126 {
		return errors.New("routing WebSocket fixture only supports short frames")
	}
	frame := []byte{0x81, byte(len(payload))}
	frame = append(frame, payload...)
	_, err := writer.Write(frame)
	return err
}

func inspectRoutingResolver(ctx context.Context, host string, port int) (RoutingResolverResult, []string, error) {
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return RoutingResolverResult{}, nil, fmt.Errorf("resolve %s: %w", host, err)
	}
	result := RoutingResolverResult{Host: host}
	for _, address := range addresses {
		result.Addresses = append(result.Addresses, address.IP.String())
		result.IPv4Resolved = result.IPv4Resolved || address.IP.To4() != nil
		result.IPv6Resolved = result.IPv6Resolved || address.IP.To4() == nil
	}
	sort.Strings(result.Addresses)
	result.IPv4Reachable = dialRoutingFamily(ctx, "tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	result.IPv6Reachable = dialRoutingFamily(ctx, "tcp6", fmt.Sprintf("[::1]:%d", port))
	limits := make([]string, 0, 2)
	if !result.IPv6Resolved {
		limits = append(limits, "the executed Linux resolver returned only IPv4 for wildcard .localhost names")
	}
	if !result.IPv6Reachable {
		limits = append(limits, "the v0.1 probe listener is intentionally IPv4-only at 127.0.0.1")
	}
	return result, limits, nil
}

func dialRoutingFamily(ctx context.Context, network, address string) bool {
	dialer := net.Dialer{Timeout: 250 * time.Millisecond}
	connection, err := dialer.DialContext(ctx, network, address)
	if err != nil {
		return false
	}
	_ = connection.Close()
	return true
}

func inspectRoutingNetworkEnvironment() (RoutingNetworkResult, error) {
	result := RoutingNetworkResult{
		ProxyEnvironmentVariables: []string{},
		VPNLikeInterfaces:         []string{},
	}
	for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "all_proxy", "no_proxy"} {
		if _, present := os.LookupEnv(name); present {
			result.ProxyEnvironmentVariables = append(result.ProxyEnvironmentVariables, name)
		}
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return result, fmt.Errorf("inspect network interfaces: %w", err)
	}
	for _, networkInterface := range interfaces {
		lower := strings.ToLower(networkInterface.Name)
		vpnLike := strings.HasPrefix(lower, "tun") || strings.HasPrefix(lower, "tap") || strings.HasPrefix(lower, "tailscale") || strings.HasPrefix(lower, "wg") || strings.Contains(lower, "vpn")
		if vpnLike && networkInterface.Flags&net.FlagUp != 0 {
			result.VPNLikeInterfaces = append(result.VPNLikeInterfaces, networkInterface.Name)
		}
	}
	sort.Strings(result.ProxyEnvironmentVariables)
	sort.Strings(result.VPNLikeInterfaces)
	return result, nil
}

func exerciseRoutingProxy(ctx context.Context, env *Environment, targetURL string) (RoutingProxyResult, error) {
	var requests atomic.Int64
	proxyListener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return RoutingProxyResult{}, err
	}
	proxyServer := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(writer, "controlled proxy rejection", http.StatusBadGateway)
	})}
	serveDone := make(chan error, 1)
	go func() { serveDone <- proxyServer.Serve(proxyListener) }()
	defer func() {
		_ = proxyServer.Close()
		<-serveDone
	}()
	proxyURL := "http://" + proxyListener.Addr().String()

	interceptedResult, interceptedErr := env.RunCommand(ctx, "routing-explicit-proxy", Command{Argv: []string{
		"curl", "--silent", "--show-error", "--fail", "--proxy", proxyURL, targetURL,
	}, Env: proxyFreeEnvironment()})
	intercepted := interceptedErr != nil && interceptedResult.ExitCode != 0 && requests.Load() == 1
	if !intercepted {
		return RoutingProxyResult{}, fmt.Errorf("explicit proxy test did not intercept .localhost: exit=%d requests=%d", interceptedResult.ExitCode, requests.Load())
	}

	bypassOutput, err := runRoutingTextWithEnvironment(ctx, env, "routing-proxy-bypass", []string{
		"curl", "--silent", "--show-error", "--fail", "--proxy", proxyURL, "--noproxy", ".localhost", targetURL,
	}, proxyFreeEnvironment())
	if err != nil {
		return RoutingProxyResult{}, err
	}
	var observed routingObservation
	if err := json.Unmarshal([]byte(bypassOutput), &observed); err != nil {
		return RoutingProxyResult{}, err
	}
	bypass := observed.Backend == "beta" && requests.Load() == 1
	if !bypass {
		return RoutingProxyResult{}, errors.New(".localhost proxy bypass did not reach beta directly")
	}
	return RoutingProxyResult{ExplicitProxyIntercepted: true, LocalhostBypassSucceeded: true}, nil
}

func proxyFreeEnvironment() []string {
	blocked := map[string]bool{
		"http_proxy": true, "https_proxy": true, "all_proxy": true, "no_proxy": true,
	}
	environment := make([]string, 0, len(os.Environ()))
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		if !blocked[strings.ToLower(name)] {
			environment = append(environment, value)
		}
	}
	return environment
}

func fetchStatus(ctx context.Context, targetURL string) (int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return 0, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	return response.StatusCode, nil
}

func runRoutingTextWithEnvironment(ctx context.Context, env *Environment, label string, argv, environment []string) (string, error) {
	result, err := env.RunCommand(ctx, label, Command{Argv: argv, Env: environment})
	return string(result.Stdout), err
}

func firstNonemptyLine(value string) string {
	for _, line := range strings.Split(value, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}
