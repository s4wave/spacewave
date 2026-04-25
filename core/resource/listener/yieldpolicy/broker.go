//go:build !js

// Package yieldpolicy implements the yield-policy broker used by the
// desktop resource listener. The broker lets the local UI surface an
// interactive prompt when a peer (typically "spacewave serve") asks the
// listener to give up its socket via daemon-control.
//
// The broker exposes three surfaces:
//
//   - Policy. The listener installs broker.MakePolicy as its yield
//     policy. When daemon-control fires, the policy registers a
//     pending prompt on the broker and blocks until the UI resolves
//     the prompt or a timeout expires.
//   - Prompt watchers. The UI reads the broadcast snapshot of pending
//     prompts and emits them over a Watch RPC.
//   - Handoff watchers. After Allow, the broker tracks the "runtime
//     handed off" state so the UI can show a banner and surface a
//     Reclaim action. Reclaim re-binds the socket by closing the
//     handoff channel; the listener controller's restart loop picks
//     that up and invokes TakeoverSocket + Listen again.
package yieldpolicy

import (
	"context"
	"strconv"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
)

// DefaultPromptTimeout is the default auto-deny timeout for a prompt.
const DefaultPromptTimeout = 30 * time.Second

// Decision represents a resolved prompt outcome.
type Decision int

const (
	// DecisionPending means the prompt has not been resolved yet.
	DecisionPending Decision = 0
	// DecisionAllowed means the user allowed the takeover.
	DecisionAllowed Decision = 1
	// DecisionDenied means the user denied the takeover or the prompt
	// timed out with no response.
	DecisionDenied Decision = 2
)

// Prompt is a pending takeover prompt.
type Prompt struct {
	// ID is the unique prompt identifier.
	ID string
	// RequesterName is the human-readable name of the requesting
	// runtime (e.g. "spacewave serve").
	RequesterName string
	// SocketPath is the Unix socket path the requester wants to bind.
	SocketPath string
	// DeadlineUnixMs is the auto-deny deadline in unix milliseconds.
	DeadlineUnixMs int64
}

// HandoffState is the current runtime-handoff state emitted to the UI.
type HandoffState struct {
	// Active is true when the listener has yielded and is waiting for
	// the user to reclaim the runtime.
	Active bool
	// RequesterName is the name of the runtime that took over.
	RequesterName string
	// SocketPath is the socket path that was yielded.
	SocketPath string
	// SinceUnixMs is when the handoff started.
	SinceUnixMs int64
}

// pending is the internal record for a pending prompt.
type pending struct {
	prompt   Prompt
	decision Decision
	done     chan struct{}
}

// Broker coordinates takeover prompts and runtime-handoff state
// between the Go listener controller and the UI.
type Broker struct {
	timeout time.Duration
	nowFn   func() time.Time
	nextID  func() string

	bcast    broadcast.Broadcast
	pending  map[string]*pending
	handoff  HandoffState
	reclaim  chan struct{}
	promptID uint64
}

// NewBroker constructs a broker using the default prompt timeout.
func NewBroker() *Broker {
	return NewBrokerWithTimeout(DefaultPromptTimeout)
}

// NewBrokerWithTimeout constructs a broker with a specific timeout.
func NewBrokerWithTimeout(timeout time.Duration) *Broker {
	b := &Broker{
		timeout: timeout,
		nowFn:   time.Now,
		pending: make(map[string]*pending),
	}
	b.nextID = b.defaultNextID
	return b
}

// defaultNextID issues monotonic prompt ids guarded by the broadcast
// mutex. The caller must hold the broadcast lock.
func (b *Broker) defaultNextID() string {
	b.promptID++
	return strconv.FormatUint(b.promptID, 10)
}

// SetClock replaces the clock function. For tests only.
func (b *Broker) SetClock(nowFn func() time.Time) {
	b.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		b.nowFn = nowFn
	})
}

// SetIDFunc replaces the prompt id generator. For tests only.
func (b *Broker) SetIDFunc(next func() string) {
	b.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		b.nextID = next
	})
}

// MakePolicy returns a Policy suitable for registering with the
// daemon-control handler. requesterName and socketPath describe the
// caller for display to the user.
//
// The returned policy blocks until the user resolves the prompt (via
// ResolvePrompt) or the prompt times out. A denied or timed-out prompt
// returns a non-nil error so the daemon-control handler can propagate
// the failure to the caller.
func (b *Broker) MakePolicy(requesterName, socketPath string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		return b.requestTakeover(ctx, requesterName, socketPath)
	}
}

// requestTakeover is the synchronous policy entrypoint. It registers
// a pending prompt, broadcasts the change, and blocks until the
// prompt is resolved or the timeout elapses.
func (b *Broker) requestTakeover(ctx context.Context, requesterName, socketPath string) error {
	if requesterName == "" {
		requesterName = "spacewave serve"
	}

	now := b.nowFn()
	deadline := now.Add(b.timeout)

	p := &pending{
		decision: DecisionPending,
		done:     make(chan struct{}),
	}

	var id string
	b.bcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
		id = b.nextID()
		p.prompt = Prompt{
			ID:             id,
			RequesterName:  requesterName,
			SocketPath:     socketPath,
			DeadlineUnixMs: deadline.UnixMilli(),
		}
		b.pending[id] = p
		broadcastFn()
	})

	remaining := b.timeout
	if remaining <= 0 {
		remaining = DefaultPromptTimeout
	}
	timer := time.NewTimer(remaining)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		b.finalizePrompt(id, DecisionDenied)
		return ctx.Err()
	case <-timer.C:
		decision := b.finalizePrompt(id, DecisionDenied)
		if decision == DecisionAllowed {
			return nil
		}
		return errors.Errorf(
			"takeover prompt timed out after %s with no response from the Spacewave desktop app",
			b.timeout,
		)
	case <-p.done:
		var final Decision
		b.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			final = p.decision
			delete(b.pending, id)
		})
		if final == DecisionAllowed {
			return nil
		}
		return errors.New(
			"takeover denied by the Spacewave desktop app; quit the app or approve in-app before retrying",
		)
	}
}

// finalizePrompt finalizes a prompt with decision if still pending and
// broadcasts the change. Returns the effective decision.
func (b *Broker) finalizePrompt(id string, decision Decision) Decision {
	var result Decision
	b.bcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
		p, ok := b.pending[id]
		if !ok {
			result = DecisionDenied
			return
		}
		if p.decision == DecisionPending {
			p.decision = decision
			close(p.done)
		}
		result = p.decision
		delete(b.pending, id)
		broadcastFn()
	})
	return result
}

// ResolvePrompt resolves a pending prompt with the given allow flag.
// Returns an error if the prompt id is unknown or already resolved.
func (b *Broker) ResolvePrompt(id string, allow bool) error {
	var resolved bool
	var existed bool
	b.bcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
		p, ok := b.pending[id]
		if !ok {
			return
		}
		existed = true
		if p.decision != DecisionPending {
			return
		}
		if allow {
			p.decision = DecisionAllowed
		} else {
			p.decision = DecisionDenied
		}
		close(p.done)
		resolved = true
		broadcastFn()
	})
	if !existed {
		return errors.Errorf("prompt %q not found", id)
	}
	if !resolved {
		return errors.Errorf("prompt %q already resolved", id)
	}
	return nil
}

// SnapshotPrompts returns a copy of the current pending prompt list
// and a wait channel that closes on the next state change.
func (b *Broker) SnapshotPrompts() ([]Prompt, <-chan struct{}) {
	var out []Prompt
	var waitCh <-chan struct{}
	b.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		waitCh = getWaitCh()
		out = make([]Prompt, 0, len(b.pending))
		for _, p := range b.pending {
			if p.decision != DecisionPending {
				continue
			}
			out = append(out, p.prompt)
		}
	})
	return out, waitCh
}

// BeginHandoff records that the listener has yielded the socket to a
// remote runtime. It also prepares a reclaim channel that the
// controller blocks on until the user reclaims the runtime.
func (b *Broker) BeginHandoff(requesterName, socketPath string) <-chan struct{} {
	ch := make(chan struct{})
	b.bcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
		b.handoff = HandoffState{
			Active:        true,
			RequesterName: requesterName,
			SocketPath:    socketPath,
			SinceUnixMs:   b.nowFn().UnixMilli(),
		}
		b.reclaim = ch
		broadcastFn()
	})
	return ch
}

// ClearHandoff clears the handoff state without signalling reclaim.
// Used when the controller restarts a listener for reasons other than
// an explicit reclaim (e.g. startup takeover).
func (b *Broker) ClearHandoff() {
	b.bcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
		b.handoff = HandoffState{}
		b.reclaim = nil
		broadcastFn()
	})
}

// SnapshotHandoff returns the current handoff state and a wait
// channel that closes on the next change.
func (b *Broker) SnapshotHandoff() (HandoffState, <-chan struct{}) {
	var out HandoffState
	var waitCh <-chan struct{}
	b.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		waitCh = getWaitCh()
		out = b.handoff
	})
	return out, waitCh
}

// Reclaim signals the listener to reclaim the runtime. Returns true
// if a reclaim signal was actually fired, false when no handoff was
// in progress.
func (b *Broker) Reclaim() bool {
	var fired bool
	b.bcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
		if b.reclaim == nil {
			return
		}
		close(b.reclaim)
		b.reclaim = nil
		b.handoff = HandoffState{}
		fired = true
		broadcastFn()
	})
	return fired
}
