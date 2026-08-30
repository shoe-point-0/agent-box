// Package daemonclient implements the bounded versioned aboxd Unix-socket client.
package daemonclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/daemon"
)

const (
	maxResponseBytes  = 1 << 20
	maxResumeAttempts = 3
)

var (
	ErrUnavailable = errors.New("daemon unavailable")
	ErrProtocol    = errors.New("daemon protocol violation")
)

// Event is one durable operation event from the daemon API.
type Event struct {
	APIVersion  string          `json:"apiVersion"`
	OperationID string          `json:"operationId"`
	Sequence    int64           `json:"sequence"`
	Kind        string          `json:"kind"`
	Details     json.RawMessage `json:"details"`
	OccurredAt  time.Time       `json:"occurredAt"`
}

// Client speaks HTTP over one exact Unix socket.
type Client struct {
	http *http.Client
}

func New(socketPath string) (*Client, error) {
	if socketPath == "" {
		return nil, errors.New("daemon socket path is empty")
	}
	transport := &http.Transport{
		DisableCompression:    true,
		ResponseHeaderTimeout: 5 * time.Second,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			connection, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", socketPath)
			if err != nil {
				return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
			}
			return connection, nil
		},
	}
	return &Client{http: &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("daemon redirects are forbidden")
		},
	}}, nil
}

func (c *Client) Close() {
	if transport, ok := c.http.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
}

// Handshake verifies that both peers agree on the current API contract.
func (c *Client) Handshake(ctx context.Context) (*contract.Failure, error) {
	response, err := c.do(ctx, http.MethodGet, "/v1/handshake", nil, "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return decodeFailure(response)
	}
	var handshake struct {
		APIVersion string `json:"apiVersion"`
		Minimum    int    `json:"minimum"`
		Maximum    int    `json:"maximum"`
	}
	if err := decodeOne(response.Body, &handshake); err != nil {
		return nil, fmt.Errorf("%w: decode handshake: %v", ErrProtocol, err)
	}
	if handshake.APIVersion != contract.DaemonAPIVersion ||
		handshake.Maximum < daemon.APIMinVersion || handshake.Minimum > daemon.APIMaxVersion {
		return &contract.Failure{
			SchemaVersion: contract.ErrorSchemaVersion, Code: contract.CodeAPIIncompatible,
			Message: "client and daemon API ranges are incompatible", Retryable: false,
		}, nil
	}
	return nil, nil
}

// List returns the daemon's bounded, strictly decoded read-only Box inventory.
func (c *Client) List(ctx context.Context) (contract.ListOutput, *contract.Failure, error) {
	response, err := c.do(ctx, http.MethodGet, "/v1/boxes", nil, "")
	if err != nil {
		return contract.ListOutput{}, nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		failure, failureErr := decodeFailure(response)
		return contract.ListOutput{}, failure, failureErr
	}
	var output contract.ListOutput
	if err := decodeOne(response.Body, &output); err != nil {
		return contract.ListOutput{}, nil, fmt.Errorf("%w: decode Box list: %v", ErrProtocol, err)
	}
	if output.SchemaVersion != contract.ListSchemaVersion || output.Boxes == nil {
		return contract.ListOutput{}, nil, fmt.Errorf("%w: invalid Box list envelope", ErrProtocol)
	}
	return output, nil, nil
}

// Plan returns one bounded, strictly decoded read-only reconciliation plan.
func (c *Client) Plan(ctx context.Context, request daemon.PlanRequest) (contract.PlanOutput, *contract.Failure, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return contract.PlanOutput{}, nil, fmt.Errorf("encode plan request: %w", err)
	}
	response, err := c.do(ctx, http.MethodPost, "/v1/boxes:plan", body, "")
	if err != nil {
		return contract.PlanOutput{}, nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		failure, failureErr := decodeFailure(response)
		return contract.PlanOutput{}, failure, failureErr
	}
	var output contract.PlanOutput
	if err := decodeOne(response.Body, &output); err != nil {
		return contract.PlanOutput{}, nil, fmt.Errorf("%w: decode Box plan: %v", ErrProtocol, err)
	}
	if output.SchemaVersion != contract.PlanOutputSchemaVersion || output.Plan.SchemaVersion != contract.PlanSchemaVersion || output.Operations == nil {
		return contract.PlanOutput{}, nil, fmt.Errorf("%w: invalid Box plan envelope", ErrProtocol)
	}
	return output, nil, nil
}

// Up submits or reattaches to one idempotent mutation and resumes a broken event
// stream by durable sequence. Returning a Failure means the daemon completed or
// rejected the request with a stable symbolic code.
func (c *Client) Up(ctx context.Context, request daemon.MutationRequest, idempotencyKey string, consume func(Event) error) (*contract.Failure, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode up request: %w", err)
	}
	operationID := ""
	sequence := int64(0)
	for attempt := 0; attempt < maxResumeAttempts; attempt++ {
		method, path, requestBody := http.MethodPost, "/v1/boxes:up", body
		if operationID != "" {
			method = http.MethodGet
			path = "/v1/operations/" + url.PathEscape(operationID) + "/events?after=" + strconv.FormatInt(sequence, 10)
			requestBody = nil
		}
		response, requestErr := c.do(ctx, method, path, requestBody, idempotencyKey)
		if requestErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}
		if response.StatusCode != http.StatusOK {
			failure, failureErr := decodeFailure(response)
			_ = response.Body.Close()
			return failure, failureErr
		}
		responseOperation := response.Header.Get(daemon.HeaderOperationID)
		if !validOperationID(responseOperation) || operationID != "" && responseOperation != operationID {
			_ = response.Body.Close()
			return nil, fmt.Errorf("%w: invalid operation identity", ErrProtocol)
		}
		operationID = responseOperation
		complete, terminal, streamErr := decodeEvents(response.Body, operationID, &sequence, consume)
		_ = response.Body.Close()
		if streamErr != nil {
			return nil, streamErr
		}
		if complete {
			return terminal, nil
		}
	}
	return nil, fmt.Errorf("%w: event stream ended before a terminal result", ErrUnavailable)
}

func (c *Client) do(ctx context.Context, method, path string, body []byte, idempotencyKey string) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://unix"+path, reader)
	if err != nil {
		return nil, err
	}
	request.Header.Set(daemon.HeaderAPIMin, strconv.Itoa(daemon.APIMinVersion))
	request.Header.Set(daemon.HeaderAPIMax, strconv.Itoa(daemon.APIMaxVersion))
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" && method == http.MethodPost {
		request.Header.Set(daemon.HeaderIdempotencyKey, idempotencyKey)
	}
	response, err := c.http.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, ErrUnavailable) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return response, nil
}

func decodeEvents(reader io.Reader, operationID string, sequence *int64, consume func(Event) error) (bool, *contract.Failure, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), maxResponseBytes)
	for scanner.Scan() {
		var event Event
		if err := decodeStrict(scanner.Bytes(), &event); err != nil {
			return false, nil, fmt.Errorf("%w: decode event: %v", ErrProtocol, err)
		}
		if event.APIVersion != contract.DaemonAPIVersion || event.OperationID != operationID ||
			event.Sequence != *sequence+1 || event.Kind == "" || len(event.Details) == 0 || !json.Valid(event.Details) || event.OccurredAt.IsZero() {
			return false, nil, fmt.Errorf("%w: invalid event envelope", ErrProtocol)
		}
		*sequence = event.Sequence
		var terminal *contract.Failure
		if event.Kind == "operation.finished" {
			failure, err := terminalFailure(event.Details)
			if err != nil {
				return false, nil, err
			}
			terminal = failure
		}
		if err := consume(event); err != nil {
			return false, nil, err
		}
		if event.Kind == "operation.finished" {
			return true, terminal, nil
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return false, nil, fmt.Errorf("%w: event exceeds %d bytes", ErrProtocol, maxResponseBytes)
		}
		return false, nil, nil
	}
	return false, nil, nil
}

func terminalFailure(details json.RawMessage) (*contract.Failure, error) {
	var terminal struct {
		APIVersion string             `json:"apiVersion"`
		State      string             `json:"state"`
		ErrorCode  contract.ErrorCode `json:"errorCode,omitempty"`
	}
	if err := decodeStrict(details, &terminal); err != nil || terminal.APIVersion != contract.DaemonAPIVersion {
		return nil, fmt.Errorf("%w: invalid terminal event", ErrProtocol)
	}
	switch terminal.State {
	case "succeeded":
		if terminal.ErrorCode != "" {
			return nil, fmt.Errorf("%w: successful terminal event contains an error", ErrProtocol)
		}
		return nil, nil
	case "failed":
		if !terminal.ErrorCode.Valid() {
			return nil, fmt.Errorf("%w: terminal event contains an unknown error", ErrProtocol)
		}
		return &contract.Failure{
			SchemaVersion: contract.ErrorSchemaVersion, Code: terminal.ErrorCode,
			Message: "operation failed", Retryable: false,
		}, nil
	default:
		return nil, fmt.Errorf("%w: terminal event has invalid state", ErrProtocol)
	}
}

func decodeFailure(response *http.Response) (*contract.Failure, error) {
	var failure contract.Failure
	if err := decodeOne(response.Body, &failure); err != nil {
		return nil, fmt.Errorf("%w: decode failure: %v", ErrProtocol, err)
	}
	if failure.SchemaVersion != contract.ErrorSchemaVersion || !failure.Code.Valid() || failure.Message == "" {
		return nil, fmt.Errorf("%w: invalid failure envelope", ErrProtocol)
	}
	return &failure, nil
}

func decodeOne(reader io.Reader, destination any) error {
	limited := io.LimitReader(reader, maxResponseBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if len(data) > maxResponseBytes {
		return errors.New("response exceeds size limit")
	}
	return decodeStrict(data, destination)
}

func decodeStrict(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func validOperationID(value string) bool {
	if !strings.HasPrefix(value, "op_") || len(value) <= 3 || len(value) > 128 {
		return false
	}
	for _, character := range value[3:] {
		if character < '0' || character > '9' {
			if character < 'A' || character > 'Z' {
				return false
			}
		}
	}
	return true
}
