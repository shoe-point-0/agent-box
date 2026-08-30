package daemon

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shoe-point-0/agent-box/internal/removal"
)

func TestWriterGateQuiesceDrainsAndBlocksBoxWriters(t *testing.T) {
	gate := NewWriterGate()
	releaseActive, err := gate.Enter(t.Context(), "box-one")
	if err != nil {
		t.Fatal(err)
	}
	lease := removal.Lease{BoxID: "box-one", Token: "operation-1:7"}
	quiesced := make(chan error, 1)
	go func() { quiesced <- gate.Quiesce(t.Context(), lease) }()

	waitForWriterBarrier(t, gate, "box-one")
	blockedEntered := make(chan struct{})
	blockedRelease := make(chan func(), 1)
	go func() {
		release, enterErr := gate.Enter(t.Context(), "box-one")
		if enterErr == nil {
			blockedRelease <- release
			close(blockedEntered)
		}
	}()
	select {
	case err := <-quiesced:
		t.Fatalf("Quiesce() completed before active writer drained: %v", err)
	case <-blockedEntered:
		t.Fatal("new writer entered while quiescence was pending")
	case <-time.After(25 * time.Millisecond):
	}

	releaseActive()
	select {
	case err := <-quiesced:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Quiesce() did not complete after active writer drained")
	}
	select {
	case <-blockedEntered:
		t.Fatal("new writer entered before Resume()")
	case <-time.After(25 * time.Millisecond):
	}
	if err := gate.Resume(t.Context(), removal.Lease{BoxID: "box-one", Token: "wrong"}); err == nil {
		t.Fatal("Resume() accepted another lease")
	}
	if err := gate.Resume(t.Context(), lease); err != nil {
		t.Fatal(err)
	}
	select {
	case <-blockedEntered:
		(<-blockedRelease)()
	case <-time.After(time.Second):
		t.Fatal("blocked writer did not enter after Resume()")
	}
}

func TestWriterGateCanceledQuiesceRestoresAdmission(t *testing.T) {
	gate := NewWriterGate()
	release, err := gate.Enter(t.Context(), "box-one")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		result <- gate.Quiesce(ctx, removal.Lease{BoxID: "box-one", Token: "operation-1:1"})
	}()
	waitForWriterBarrier(t, gate, "box-one")
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("Quiesce() error = %v, want context canceled", err)
	}

	entered := make(chan func(), 1)
	go func() {
		nextRelease, enterErr := gate.Enter(t.Context(), "box-one")
		if enterErr == nil {
			entered <- nextRelease
		}
	}()
	select {
	case nextRelease := <-entered:
		nextRelease()
	case <-time.After(time.Second):
		t.Fatal("canceled quiescence left writer admission closed")
	}
	release()
}

func TestWriterGateQuiescenceIsPerBox(t *testing.T) {
	gate := NewWriterGate()
	release, err := gate.Enter(t.Context(), "box-one")
	if err != nil {
		t.Fatal(err)
	}
	lease := removal.Lease{BoxID: "box-one", Token: "operation-1:2"}
	result := make(chan error, 1)
	go func() { result <- gate.Quiesce(t.Context(), lease) }()
	waitForWriterBarrier(t, gate, "box-one")

	otherRelease, err := gate.Enter(t.Context(), "box-two")
	if err != nil {
		t.Fatalf("unrelated Box writer was blocked: %v", err)
	}
	otherRelease()
	release()
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if err := gate.Resume(t.Context(), lease); err != nil {
		t.Fatal(err)
	}
}

func waitForWriterBarrier(t *testing.T, gate *WriterGate, boxID string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		gate.mu.Lock()
		state := gate.boxes[boxID]
		quiesced := state != nil && state.quiesced
		gate.mu.Unlock()
		if quiesced {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("writer barrier did not close")
		}
		time.Sleep(time.Millisecond)
	}
}
