package daemon

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/shoe-point-0/agent-box/internal/removal"
)

// WriterGate owns daemon-local admission for Box-scoped writers. Journaled
// mutations and background reconcilers must enter this gate before touching a
// Box. Removal quiesces the same gate only after acquiring its fenced lease.
type WriterGate struct {
	mu    sync.Mutex
	boxes map[string]*boxWriters
}

var _ removal.Quiescer = (*WriterGate)(nil)

type boxWriters struct {
	active          int
	quiesced        bool
	token           string
	quiesceWaiters  int
	quiesceComplete bool
	changed         chan struct{}
}

// NewWriterGate constructs an empty per-Box writer admission gate.
func NewWriterGate() *WriterGate {
	return &WriterGate{boxes: make(map[string]*boxWriters)}
}

// Enter admits one Box writer or waits until removal resumes admission. The
// returned release function is idempotent and must be called by the writer.
func (g *WriterGate) Enter(ctx context.Context, boxID string) (func(), error) {
	if g == nil || !validWriterIdentity(boxID) {
		return nil, errors.New("enter Box writer gate: invalid Box identity")
	}
	for {
		g.mu.Lock()
		state := g.state(boxID)
		if !state.quiesced {
			state.active++
			g.mu.Unlock()
			var once sync.Once
			return func() {
				once.Do(func() {
					g.mu.Lock()
					defer g.mu.Unlock()
					current := g.boxes[boxID]
					if current == nil || current.active == 0 {
						return
					}
					current.active--
					g.signal(current)
				})
			}, nil
		}
		changed := state.changed
		g.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-changed:
		}
	}
}

// Quiesce closes writer admission for the leased Box and waits for every
// already-admitted writer to leave. Cancellation reopens admission unless a
// concurrent call using the same lease has already completed the barrier.
func (g *WriterGate) Quiesce(ctx context.Context, lease removal.Lease) error {
	if g == nil || !validWriterIdentity(lease.BoxID) || !validWriterIdentity(lease.Token) {
		return errors.New("quiesce Box writers: invalid lease identity")
	}
	g.mu.Lock()
	state := g.state(lease.BoxID)
	if state.quiesced && state.token != lease.Token {
		g.mu.Unlock()
		return errors.New("quiesce Box writers: Box is fenced by another lease")
	}
	if !state.quiesced {
		state.quiesced = true
		state.token = lease.Token
		g.signal(state)
	}
	if state.quiesceComplete {
		g.mu.Unlock()
		return nil
	}
	state.quiesceWaiters++
	for {
		if state.active == 0 {
			state.quiesceWaiters--
			state.quiesceComplete = true
			g.mu.Unlock()
			return nil
		}
		changed := state.changed
		g.mu.Unlock()
		select {
		case <-ctx.Done():
			g.mu.Lock()
			state = g.boxes[lease.BoxID]
			if state != nil && state.quiesced && state.token == lease.Token {
				state.quiesceWaiters--
				if state.quiesceWaiters == 0 && !state.quiesceComplete {
					state.quiesced = false
					state.token = ""
					g.signal(state)
				}
			}
			g.mu.Unlock()
			return ctx.Err()
		case <-changed:
			g.mu.Lock()
			state = g.boxes[lease.BoxID]
			if state == nil || !state.quiesced || state.token != lease.Token {
				g.mu.Unlock()
				return errors.New("quiesce Box writers: lease barrier changed unexpectedly")
			}
		}
	}
}

// Resume reopens writer admission only for the lease that completed quiescence.
func (g *WriterGate) Resume(_ context.Context, lease removal.Lease) error {
	if g == nil || !validWriterIdentity(lease.BoxID) || !validWriterIdentity(lease.Token) {
		return errors.New("resume Box writers: invalid lease identity")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	state := g.boxes[lease.BoxID]
	if state == nil || !state.quiesced || !state.quiesceComplete || state.token != lease.Token {
		return errors.New("resume Box writers: lease does not own the completed barrier")
	}
	state.quiesced = false
	state.token = ""
	state.quiesceComplete = false
	g.signal(state)
	return nil
}

func (g *WriterGate) state(boxID string) *boxWriters {
	state := g.boxes[boxID]
	if state == nil {
		state = &boxWriters{changed: make(chan struct{})}
		g.boxes[boxID] = state
	}
	return state
}

func (*WriterGate) signal(state *boxWriters) {
	close(state.changed)
	state.changed = make(chan struct{})
}

func validWriterIdentity(value string) bool {
	if value == "" || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character < ' ' || character == '\x7f' {
			return false
		}
	}
	return true
}
