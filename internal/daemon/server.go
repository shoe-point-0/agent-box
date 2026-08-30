// Package daemon owns Agent Box's local HTTP control-plane transport.
package daemon

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/journal"
)

const (
	APIMinVersion = 1
	APIMaxVersion = 1

	HeaderAPIMin         = "Abox-API-Min"
	HeaderAPIMax         = "Abox-API-Max"
	HeaderIdempotencyKey = "Idempotency-Key"
	HeaderOperationID    = "Abox-Operation-ID"

	maxRequestBytes = 1 << 20
	maxKeyBytes     = 256
	maxOpaqueBytes  = 512
	streamPoll      = 20 * time.Millisecond
	persistTimeout  = 5 * time.Second
)

// MutationRequest is the transport envelope for a daemon-owned mutation. The
// command-specific input remains opaque to this package; later API slices own
// and validate it before causing external side effects.
type MutationRequest struct {
	WorkspaceID    string          `json:"workspaceId"`
	BoxID          string          `json:"boxId"`
	TaskKey        string          `json:"taskKey"`
	ManifestPath   string          `json:"manifestPath"`
	ManifestDigest string          `json:"manifestDigest"`
	Input          json.RawMessage `json:"input"`
}

// Emit persists one redacted operation event before it can be streamed.
type Emit func(kind string, details any) error

// Runner performs one newly persisted operation. Its context belongs to the
// daemon, not the requesting client. Returning a Failure records a typed failed
// terminal state; returning nil records success.
type Runner func(context.Context, journal.Operation, json.RawMessage, Emit) *contract.Failure

// ListBoxes returns a read-only inventory. Implementations may read persisted
// observations but receive no mutation or reconciliation capability.
type ListBoxes func(context.Context) (contract.ListOutput, error)

// PlanRequest identifies one read-only desired-state compilation.
type PlanRequest struct {
	WorkspaceID  string `json:"workspaceId"`
	BoxID        string `json:"boxId"`
	TaskKey      string `json:"taskKey"`
	ManifestPath string `json:"manifestPath"`
}

// PlanBox compiles and compares desired state without mutation capability.
type PlanBox func(context.Context, PlanRequest) (contract.PlanOutput, error)

// Config contains explicit daemon-owned paths and operation dependencies.
type Config struct {
	RuntimeDir string
	Journal    *journal.Journal
	Writers    *WriterGate
	Run        Runner
	List       ListBoxes
	Plan       PlanBox
}

// Server owns one listener, HTTP server, daemon operation context, lock, and
// exact socket identity.
type Server struct {
	journal    *journal.Journal
	writers    *WriterGate
	run        Runner
	list       ListBoxes
	plan       PlanBox
	httpServer *http.Server
	listener   net.Listener
	lock       *os.File
	socketPath string
	socket     fileIdentity

	operationContext context.Context
	cancelOperations context.CancelFunc
	operations       sync.WaitGroup
	ownedMu          sync.RWMutex
	ownedOperations  map[string]struct{}
	serveDone        chan struct{}
	serveMu          sync.Mutex
	serveErr         error
	shutdownMu       sync.Mutex
	shutdownComplete bool
	shutdownErr      error
}

// Start binds and verifies the Unix socket before accepting requests.
func Start(config Config) (_ *Server, returnErr error) {
	if config.Journal == nil {
		return nil, errors.New("start daemon: operation journal is nil")
	}
	if config.Writers == nil {
		return nil, errors.New("start daemon: Box writer gate is nil")
	}
	if config.Run == nil {
		return nil, errors.New("start daemon: operation runner is nil")
	}
	if config.List == nil {
		return nil, errors.New("start daemon: Box list service is nil")
	}
	if config.Plan == nil {
		return nil, errors.New("start daemon: Box plan service is nil")
	}
	prepared, err := prepareRuntime(config.RuntimeDir)
	if err != nil {
		return nil, fmt.Errorf("start daemon: %w", err)
	}
	defer func() {
		if returnErr != nil {
			_ = prepared.lock.Close()
		}
	}()

	unixListener, socket, err := bindSocket(prepared.socketPath)
	if err != nil {
		return nil, fmt.Errorf("start daemon: %w", err)
	}
	defer func() {
		if returnErr != nil {
			_ = unixListener.Close()
			_ = removeExactSocket(prepared.socketPath, socket)
		}
	}()

	operationContext, cancelOperations := context.WithCancel(context.Background())
	server := &Server{
		journal: config.Journal, writers: config.Writers, run: config.Run, list: config.List, plan: config.Plan,
		listener: newPeerListener(unixListener, uint32(os.Geteuid())), lock: prepared.lock,
		socketPath: prepared.socketPath, socket: socket,
		operationContext: operationContext, cancelOperations: cancelOperations,
		ownedOperations: make(map[string]struct{}),
		serveDone:       make(chan struct{}),
	}
	server.httpServer = &http.Server{
		Handler:           server.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	go func() {
		err := server.httpServer.Serve(server.listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		server.serveMu.Lock()
		server.serveErr = err
		server.serveMu.Unlock()
		close(server.serveDone)
	}()
	return server, nil
}

func (s *Server) SocketPath() string { return s.socketPath }

// Wait reports an unexpected serving failure or waits until ctx is canceled.
// The owner must call Shutdown after either result to release runtime resources.
func (s *Server) Wait(ctx context.Context) error {
	select {
	case <-s.serveDone:
		s.serveMu.Lock()
		defer s.serveMu.Unlock()
		return s.serveErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Shutdown stops new work, cancels daemon-owned operations, drains active HTTP
// requests within ctx, and removes only the socket inode created by Start.
func (s *Server) Shutdown(ctx context.Context) error {
	s.shutdownMu.Lock()
	defer s.shutdownMu.Unlock()
	if s.shutdownComplete {
		return s.shutdownErr
	}

	s.cancelOperations()
	serverErr := s.httpServer.Shutdown(ctx)
	_ = s.listener.Close()
	if serverErr != nil {
		return serverErr
	}

	operationsDone := make(chan struct{})
	go func() {
		s.operations.Wait()
		close(operationsDone)
	}()
	select {
	case <-operationsDone:
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case <-s.serveDone:
		s.serveMu.Lock()
		serverErr = errors.Join(serverErr, s.serveErr)
		s.serveMu.Unlock()
	case <-ctx.Done():
		return ctx.Err()
	}
	serverErr = errors.Join(serverErr, removeExactSocket(s.socketPath, s.socket), s.lock.Close())
	s.shutdownComplete = true
	s.shutdownErr = serverErr
	return serverErr
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/handshake", s.handleHandshake)
	mux.HandleFunc("GET /v1/boxes", s.handleList)
	mux.HandleFunc("POST /v1/boxes:plan", s.handlePlan)
	mux.HandleFunc("POST /v1/boxes:up", s.handleUp)
	mux.HandleFunc("GET /v1/operations/{id}/events", s.handleEvents)
	return s.requireCompatibleAPI(mux)
}

func (s *Server) requireCompatibleAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		minimum, minErr := strconv.Atoi(request.Header.Get(HeaderAPIMin))
		maximum, maxErr := strconv.Atoi(request.Header.Get(HeaderAPIMax))
		if minErr != nil || maxErr != nil || minimum < 1 || maximum < minimum ||
			maximum < APIMinVersion || minimum > APIMaxVersion {
			writeFailure(writer, http.StatusUpgradeRequired, contract.Failure{
				SchemaVersion: contract.ErrorSchemaVersion,
				Code:          contract.CodeAPIIncompatible,
				Message:       "client and daemon API ranges are incompatible",
				Retryable:     false,
			})
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func (s *Server) handleHandshake(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, struct {
		APIVersion string `json:"apiVersion"`
		Minimum    int    `json:"minimum"`
		Maximum    int    `json:"maximum"`
	}{APIVersion: contract.DaemonAPIVersion, Minimum: APIMinVersion, Maximum: APIMaxVersion})
}

func (s *Server) handleList(writer http.ResponseWriter, request *http.Request) {
	output, err := s.list(request.Context())
	if err != nil {
		writeFailure(writer, http.StatusInternalServerError, internalFailure())
		return
	}
	if output.SchemaVersion != contract.ListSchemaVersion || output.Boxes == nil {
		writeFailure(writer, http.StatusInternalServerError, internalFailure())
		return
	}
	writeJSON(writer, http.StatusOK, output)
}

func (s *Server) handlePlan(writer http.ResponseWriter, request *http.Request) {
	var input PlanRequest
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, maxRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeFailure(writer, http.StatusBadRequest, requestFailure("invalid plan request"))
		return
	}
	if err := ensureJSONEOF(decoder); err != nil || validatePlanRequest(input) != nil {
		writeFailure(writer, http.StatusBadRequest, requestFailure("invalid plan request"))
		return
	}
	output, err := s.plan(request.Context(), input)
	if err != nil {
		code := contract.CodeManifestInvalid
		var coded interface{ ErrorCode() contract.ErrorCode }
		if errors.As(err, &coded) && coded.ErrorCode().Valid() {
			code = coded.ErrorCode()
		}
		writeFailure(writer, http.StatusUnprocessableEntity, contract.Failure{
			SchemaVersion: contract.ErrorSchemaVersion, Code: code,
			Message: "cannot compile read-only plan", Retryable: false,
		})
		return
	}
	if output.SchemaVersion != contract.PlanOutputSchemaVersion || output.Operations == nil || output.Plan.SchemaVersion != contract.PlanSchemaVersion {
		writeFailure(writer, http.StatusInternalServerError, internalFailure())
		return
	}
	writeJSON(writer, http.StatusOK, output)
}

func (s *Server) handleUp(writer http.ResponseWriter, request *http.Request) {
	key := request.Header.Get(HeaderIdempotencyKey)
	if err := validateIdempotencyKey(key); err != nil {
		writeFailure(writer, http.StatusBadRequest, requestFailure(err.Error()))
		return
	}

	var input MutationRequest
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, maxRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeFailure(writer, http.StatusBadRequest, requestFailure("invalid mutation request"))
		return
	}
	if err := ensureJSONEOF(decoder); err != nil {
		writeFailure(writer, http.StatusBadRequest, requestFailure("invalid mutation request"))
		return
	}
	if len(input.Input) == 0 {
		input.Input = json.RawMessage(`{}`)
	}
	if err := validateMutationRequest(input); err != nil {
		writeFailure(writer, http.StatusBadRequest, requestFailure("invalid mutation request"))
		return
	}
	digest, err := mutationDigest(input)
	if err != nil {
		writeFailure(writer, http.StatusBadRequest, requestFailure("invalid mutation input"))
		return
	}
	operationID, err := newOperationID()
	if err != nil {
		writeFailure(writer, http.StatusInternalServerError, internalFailure())
		return
	}
	operation, created, err := s.journal.Begin(request.Context(), journal.BeginRequest{
		WorkspaceID: identity.WorkspaceID(input.WorkspaceID), BoxID: identity.BoxID(input.BoxID),
		TaskKey: input.TaskKey, ManifestPath: input.ManifestPath, ManifestDigest: input.ManifestDigest,
		OperationID: operationID, IdempotencyKey: key, Kind: "up", RequestDigest: digest,
		StartedAt: time.Now().UTC(),
	})
	if err != nil {
		if errors.Is(err, journal.ErrIdempotencyConflict) || errors.Is(err, journal.ErrOperationActive) {
			writeFailure(writer, http.StatusConflict, operationConflict())
			return
		}
		writeFailure(writer, http.StatusInternalServerError, internalFailure())
		return
	}
	if created {
		s.ownedMu.Lock()
		s.ownedOperations[operation.ID] = struct{}{}
		s.ownedMu.Unlock()
		s.operations.Add(1)
		go s.execute(operation, input.Input)
	} else if operation.State == journal.StateRunning && !s.ownsOperation(operation.ID) {
		writeFailure(writer, http.StatusConflict, contract.Failure{
			SchemaVersion: contract.ErrorSchemaVersion, Code: contract.CodeOperationConflict,
			Message: "operation requires daemon recovery before attachment", Retryable: true,
		})
		return
	}
	writer.Header().Set(HeaderOperationID, operation.ID)
	s.streamEvents(writer, request, operation.ID)
}

func (s *Server) handleEvents(writer http.ResponseWriter, request *http.Request) {
	after, err := parseAfter(request.URL.Query().Get("after"))
	if err != nil {
		writeFailure(writer, http.StatusBadRequest, requestFailure("invalid event sequence"))
		return
	}
	operationID := request.PathValue("id")
	operation, err := s.journal.Operation(request.Context(), operationID)
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			writeFailure(writer, http.StatusNotFound, contract.Failure{
				SchemaVersion: contract.ErrorSchemaVersion, Code: contract.CodeNotFound,
				Message: "operation not found", Retryable: false,
			})
			return
		}
		writeFailure(writer, http.StatusBadRequest, requestFailure("invalid operation ID"))
		return
	}
	if operation.State == journal.StateRunning && !s.ownsOperation(operationID) {
		writeFailure(writer, http.StatusConflict, contract.Failure{
			SchemaVersion: contract.ErrorSchemaVersion, Code: contract.CodeOperationConflict,
			Message: "operation requires daemon recovery before attachment", Retryable: true,
		})
		return
	}
	writer.Header().Set(HeaderOperationID, operationID)
	s.streamEventsAfter(writer, request, operationID, after)
}

func (s *Server) execute(operation journal.Operation, input json.RawMessage) {
	defer func() {
		s.ownedMu.Lock()
		delete(s.ownedOperations, operation.ID)
		s.ownedMu.Unlock()
		s.operations.Done()
	}()
	emit := func(kind string, details any) error {
		encoded, err := json.Marshal(details)
		if err != nil {
			return fmt.Errorf("encode operation event: %w", err)
		}
		_, err = s.journal.AppendEvent(s.operationContext, operation.ID, kind, encoded, time.Now().UTC())
		return err
	}
	var failure *contract.Failure
	if operation.Kind == "remove" {
		// Removal owns the same gate through its fenced Quiescer and therefore
		// must not admit itself as a writer before it closes the barrier.
		failure = s.run(s.operationContext, operation, input, emit)
	} else {
		release, err := s.writers.Enter(s.operationContext, operation.BoxID)
		if err != nil {
			failure = &contract.Failure{Code: contract.CodeInfrastructureApplyFailed}
		} else {
			failure = s.run(s.operationContext, operation, input, emit)
			release()
		}
	}
	state := journal.StateSucceeded
	errorCode := ""
	terminalDetails := operationResult{APIVersion: contract.DaemonAPIVersion, State: state}
	if failure != nil {
		state = journal.StateFailed
		failureCode := failure.Code
		if !failureCode.Valid() {
			failureCode = contract.CodeInternalInvariant
		}
		errorCode = string(failureCode)
		terminalDetails.State = state
		terminalDetails.ErrorCode = failureCode
	}

	persistContext, cancel := context.WithTimeout(context.Background(), persistTimeout)
	defer cancel()
	if _, err := s.journal.AppendEvent(persistContext, operation.ID, "operation.finished", mustJSON(terminalDetails), time.Now().UTC()); err != nil {
		state = journal.StateFailed
		errorCode = string(contract.CodeInternalInvariant)
	}
	_ = s.journal.Finish(persistContext, operation.ID, state, errorCode, time.Now().UTC())
}

func (s *Server) ownsOperation(operationID string) bool {
	s.ownedMu.RLock()
	defer s.ownedMu.RUnlock()
	_, owned := s.ownedOperations[operationID]
	return owned
}

func (s *Server) streamEvents(writer http.ResponseWriter, request *http.Request, operationID string) {
	after, err := parseAfter(request.URL.Query().Get("after"))
	if err != nil {
		writeFailure(writer, http.StatusBadRequest, requestFailure("invalid event sequence"))
		return
	}
	s.streamEventsAfter(writer, request, operationID, after)
}

func (s *Server) streamEventsAfter(writer http.ResponseWriter, request *http.Request, operationID string, after int64) {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		writeFailure(writer, http.StatusInternalServerError, internalFailure())
		return
	}
	writer.Header().Set("Content-Type", "application/x-ndjson")
	writer.WriteHeader(http.StatusOK)
	flusher.Flush()
	encoder := json.NewEncoder(writer)
	ticker := time.NewTicker(streamPoll)
	defer ticker.Stop()
	sequence := after
	for {
		events, err := s.journal.Events(request.Context(), operationID, sequence)
		if err != nil {
			return
		}
		for _, event := range events {
			if err := encoder.Encode(streamEventFrom(event)); err != nil {
				return
			}
			sequence = event.Sequence
			flusher.Flush()
		}
		operation, err := s.journal.Operation(request.Context(), operationID)
		if err != nil {
			return
		}
		if operation.State != journal.StateRunning {
			return
		}
		if !s.ownsOperation(operationID) {
			return
		}
		select {
		case <-request.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

type streamEvent struct {
	APIVersion  string          `json:"apiVersion"`
	OperationID string          `json:"operationId"`
	Sequence    int64           `json:"sequence"`
	Kind        string          `json:"kind"`
	Details     json.RawMessage `json:"details"`
	OccurredAt  time.Time       `json:"occurredAt"`
}

func streamEventFrom(event journal.Event) streamEvent {
	return streamEvent{
		APIVersion:  contract.DaemonAPIVersion,
		OperationID: event.OperationID, Sequence: event.Sequence, Kind: event.Kind,
		Details: event.Details, OccurredAt: event.OccurredAt,
	}
}

type operationResult struct {
	APIVersion string             `json:"apiVersion"`
	State      string             `json:"state"`
	ErrorCode  contract.ErrorCode `json:"errorCode,omitempty"`
}

func mutationDigest(request MutationRequest) (string, error) {
	encoded, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	var canonical any
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.UseNumber()
	if err := decoder.Decode(&canonical); err != nil {
		return "", err
	}
	encoded, err = json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func newOperationID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "op_" + strings.ToUpper(hex.EncodeToString(bytes)), nil
}

func validateIdempotencyKey(key string) error {
	if key == "" {
		return errors.New("idempotency key is required")
	}
	if len(key) > maxKeyBytes {
		return errors.New("idempotency key is too long")
	}
	if strings.TrimSpace(key) != key || strings.IndexFunc(key, func(character rune) bool {
		return character < ' ' || character == '\u007f'
	}) >= 0 {
		return errors.New("idempotency key is invalid")
	}
	return nil
}

func validateMutationRequest(request MutationRequest) error {
	workspaceID, err := identity.Workspace(request.ManifestPath)
	if err != nil || workspaceID.String() != request.WorkspaceID {
		return errors.New("workspace identity does not match manifest path")
	}
	boxID, err := identity.Box(workspaceID, request.TaskKey)
	if err != nil || boxID.String() != request.BoxID {
		return errors.New("box identity does not match workspace and task")
	}
	if request.ManifestDigest == "" || len(request.ManifestDigest) > maxOpaqueBytes || strings.IndexByte(request.ManifestDigest, 0) >= 0 {
		return errors.New("manifest digest is invalid")
	}
	return nil
}

func validatePlanRequest(request PlanRequest) error {
	workspaceID, err := identity.Workspace(request.ManifestPath)
	if err != nil || workspaceID.String() != request.WorkspaceID {
		return errors.New("workspace identity does not match manifest path")
	}
	boxID, err := identity.Box(workspaceID, request.TaskKey)
	if err != nil || boxID.String() != request.BoxID {
		return errors.New("box identity does not match workspace and task")
	}
	return nil
}

func parseAfter(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	after, err := strconv.ParseInt(value, 10, 64)
	if err != nil || after < 0 {
		return 0, errors.New("invalid sequence")
	}
	return after, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return errors.New("multiple JSON values")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func requestFailure(message string) contract.Failure {
	return contract.Failure{
		SchemaVersion: contract.ErrorSchemaVersion, Code: contract.CodeManifestInvalid,
		Message: message, Retryable: false,
	}
}

func operationConflict() contract.Failure {
	return contract.Failure{
		SchemaVersion: contract.ErrorSchemaVersion, Code: contract.CodeOperationConflict,
		Message: "operation idempotency key conflicts with recorded work", Retryable: false,
	}
}

func internalFailure() contract.Failure {
	return contract.Failure{
		SchemaVersion: contract.ErrorSchemaVersion, Code: contract.CodeInternalInvariant,
		Message: "daemon internal invariant failed", Retryable: false,
	}
}

func writeFailure(writer http.ResponseWriter, status int, failure contract.Failure) {
	writeJSON(writer, status, failure)
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func mustJSON(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}

func runtimeSocketPath(runtimeDir string) string {
	return filepath.Join(runtimeDir, "aboxd.sock")
}
