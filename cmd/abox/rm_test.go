package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/shoe-point-0/agent-box/internal/contract"
	"github.com/shoe-point-0/agent-box/internal/daemonclient"
	"github.com/shoe-point-0/agent-box/internal/identity"
	"github.com/shoe-point-0/agent-box/internal/removal"
)

func TestRemoveDefaultsToPreviewOnlyAndReportsExactLosses(t *testing.T) {
	_, cwd, manifest := manifestFixture(t)
	preview := removePreviewFixture(t, manifest, "TASK-REMOVE")
	client := &fakeDaemon{
		removeEvents:   [][]daemonclient.Event{{removePreviewEvent(t, preview, 1)}},
		removeFailures: []*contract.Failure{testFailure(contract.CodeCleanupRefused)},
	}
	var stdout, stderr bytes.Buffer
	code := runWith(t.Context(), []string{"rm", "TASK-REMOVE"}, &stdout, &stderr, testDependencies(cwd, client))
	if code != contract.ExitSafety {
		t.Fatalf("rm preview code = %d, want %d", code, contract.ExitSafety)
	}
	wantOutput := "Box " + preview.BoxID + " removal preview " + preview.Digest + "\n" +
		"LOSS	worktree-change	\"workspace/dirty\\nname.txt\"\n" +
		"LOSS	unpublished-commit	\"0123456789abcdef0123456789abcdef01234567\"\n" +
		"LOSS	volume	\"volume-id\"\n"
	if stdout.String() != wantOutput {
		t.Fatalf("rm preview stdout = %q, want %q", stdout.String(), wantOutput)
	}
	if got, want := stderr.String(), "abox: ABOX_CLEANUP_REFUSED: test failure\n"; got != want {
		t.Fatalf("rm preview stderr = %q, want %q", got, want)
	}
	if len(client.removeRequests) != 1 || len(client.removeRequests[0].Input) != 0 {
		t.Fatalf("rm preview requests = %#v, want one authorization-free request", client.removeRequests)
	}
	if client.removeRequests[0].ManifestPath != manifest || client.removeRequests[0].BoxID != preview.BoxID || !client.closed {
		t.Fatalf("rm preview request/closed = %#v/%v", client.removeRequests[0], client.closed)
	}
}

func TestRemoveReturnsPreviewDigestWithEveryExplicitGrant(t *testing.T) {
	_, cwd, manifest := manifestFixture(t)
	preview := removePreviewFixture(t, manifest, "TASK-REMOVE")
	client := &fakeDaemon{
		removeEvents: [][]daemonclient.Event{
			{removePreviewEvent(t, preview, 1)},
			{removePreviewEvent(t, preview, 1), testEvent("removal.completed", 2)},
		},
		removeFailures: []*contract.Failure{testFailure(contract.CodeCleanupRefused), nil},
	}
	deps := testDependencies(cwd, client)
	deps.random = strings.NewReader("0123456789abcdeffedcba9876543210")
	var stdout, stderr bytes.Buffer
	code := runWith(t.Context(), []string{
		"rm", "--yes", "--discard-worktree-changes", "--discard-unpublished-commits", "--delete-volumes", "TASK-REMOVE",
	}, &stdout, &stderr, deps)
	if code != contract.ExitOK {
		t.Fatalf("authorized rm code = %d; stderr=%q", code, stderr.String())
	}
	if len(client.removeRequests) != 2 || len(client.removeKeys) != 2 || client.removeKeys[0] == client.removeKeys[1] {
		t.Fatalf("authorized rm requests/keys = %d/%v", len(client.removeRequests), client.removeKeys)
	}
	if len(client.removeRequests[0].Input) != 0 {
		t.Fatalf("preview input = %s, want empty", client.removeRequests[0].Input)
	}
	var input removal.OperationInput
	if err := json.Unmarshal(client.removeRequests[1].Input, &input); err != nil {
		t.Fatal(err)
	}
	if input.PreviewDigest != preview.Digest || !input.Yes || !input.DiscardWorktreeChanges || !input.DiscardUnpublishedCommits || !input.DeleteVolumes {
		t.Fatalf("authorized removal input = %#v", input)
	}
	if got, want := strings.Count(stdout.String(), "Box "+preview.BoxID+" removal preview "+preview.Digest), 2; got != want {
		t.Fatalf("preview render count = %d, want %d; output=%q", got, want, stdout.String())
	}
	if got, want := stderr.String(), "abox: removal.completed\nabox: removal complete\n"; got != want {
		t.Fatalf("authorized rm stderr = %q, want %q", got, want)
	}
}

func TestRemoveRejectsMalformedDaemonPreviewBeforeAuthorization(t *testing.T) {
	_, cwd, _ := manifestFixture(t)
	client := &fakeDaemon{
		removeEvents: [][]daemonclient.Event{{{
			APIVersion: contract.DaemonAPIVersion, OperationID: "op_0123456789ABCDEF", Sequence: 1,
			Kind: "removal.preview", Details: json.RawMessage(`{"boxId":"wrong","resources":[],"losses":[],"digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`),
			OccurredAt: testEvent("test", 1).OccurredAt,
		}}},
		removeFailures: []*contract.Failure{testFailure(contract.CodeCleanupRefused)},
	}
	var stdout, stderr bytes.Buffer
	code := runWith(t.Context(), []string{"rm", "--yes", "TASK-REMOVE"}, &stdout, &stderr, testDependencies(cwd, client))
	if code != contract.ExitCompatibility || len(client.removeRequests) != 1 {
		t.Fatalf("malformed preview code/requests = %d/%d, want %d/1", code, len(client.removeRequests), contract.ExitCompatibility)
	}
	if got, want := stderr.String(), "abox: ABOX_API_INCOMPATIBLE: daemon returned an incompatible response\n"; got != want {
		t.Fatalf("malformed preview stderr = %q, want %q", got, want)
	}
}

func TestRemoveJSONStreamsPreviewThenStableRefusal(t *testing.T) {
	_, cwd, manifest := manifestFixture(t)
	preview := removePreviewFixture(t, manifest, "TASK-REMOVE")
	client := &fakeDaemon{
		removeEvents:   [][]daemonclient.Event{{removePreviewEvent(t, preview, 1)}},
		removeFailures: []*contract.Failure{testFailure(contract.CodeCleanupRefused)},
	}
	var stdout, stderr bytes.Buffer
	code := runWith(t.Context(), []string{"rm", "--json", "TASK-REMOVE"}, &stdout, &stderr, testDependencies(cwd, client))
	if code != contract.ExitSafety || stderr.Len() != 0 {
		t.Fatalf("JSON rm code/stderr = %d/%q", code, stderr.String())
	}
	lines := bytes.Split(bytes.TrimSpace(stdout.Bytes()), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("JSON rm line count = %d, want 2; output=%q", len(lines), stdout.String())
	}
	validateJSONLine(t, compileCLISchema(t, "event"), lines[0])
	validateJSONLine(t, compileCLISchema(t, "error"), lines[1])
	var event contract.Event
	decodeStrictTest(t, lines[0], &event)
	if event.Kind != "removal.preview" || !bytes.Equal(event.Details, client.removeEvents[0][0].Details) {
		t.Fatalf("JSON rm event = %#v", event)
	}
	var operationFailure contract.Failure
	decodeStrictTest(t, lines[1], &operationFailure)
	if operationFailure.Code != contract.CodeCleanupRefused {
		t.Fatalf("JSON rm failure = %#v", operationFailure)
	}
}

func TestRemoveArgumentsAndHelp(t *testing.T) {
	for _, args := range [][]string{
		{"rm"}, {"rm", "one", "two"}, {"rm", "--yes", "--yes", "TASK"},
		{"rm", "--delete-volumes", "--delete-volumes", "TASK"}, {"rm", "--help", "TASK"},
	} {
		var stdout, stderr bytes.Buffer
		if code := runWith(t.Context(), args, &stdout, &stderr, dependencies{}); code != contract.ExitUsage {
			t.Errorf("runWith(%v) code = %d, want %d", args, code, contract.ExitUsage)
		}
	}
	var stdout, stderr bytes.Buffer
	if code := runWith(t.Context(), []string{"rm", "--help"}, &stdout, &stderr, dependencies{}); code != contract.ExitOK {
		t.Fatalf("rm help code = %d", code)
	}
	for _, flag := range []string{"--yes", "--discard-worktree-changes", "--discard-unpublished-commits", "--delete-volumes"} {
		if !strings.Contains(stdout.String(), flag) {
			t.Errorf("rm help lacks %q: %q", flag, stdout.String())
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("rm help stderr = %q", stderr.String())
	}
}

func removePreviewFixture(t *testing.T, manifest, taskKey string) removal.Preview {
	t.Helper()
	workspaceID, err := identity.Workspace(manifest)
	if err != nil {
		t.Fatal(err)
	}
	boxID, err := identity.Box(workspaceID, taskKey)
	if err != nil {
		t.Fatal(err)
	}
	return removal.Preview{
		BoxID: boxID.String(), Resources: []removal.Resource{},
		Losses: []removal.Loss{
			{Kind: removal.LossWorktreeChange, Value: "workspace/dirty\nname.txt"},
			{Kind: removal.LossUnpublishedCommit, Value: "0123456789abcdef0123456789abcdef01234567"},
			{Kind: removal.LossVolume, Value: "volume-id"},
		},
		Digest: "sha256:" + strings.Repeat("a", 64),
	}
}

func removePreviewEvent(t *testing.T, preview removal.Preview, sequence int64) daemonclient.Event {
	t.Helper()
	details, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	event := testEvent("removal.preview", sequence)
	event.Details = details
	return event
}
