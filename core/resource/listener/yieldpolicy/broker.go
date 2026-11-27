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
//     handoff channel. The listener controller's restart loop detects
//     the channel closure and invokes TakeoverSocket + Listen again.
package yieldpolicy

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
)

// defaultPromptTimeout is the default auto-deny timeout for a prompt.
const defaultPromptTimeout = 30 * time.Second

// promptDecision represents a resolved prompt outcome.
type promptDecision int

const (
	// decisionPending means the prompt has not been resolved yet.
	decisionPending promptDecision = 0
	// decisionAllowed means the user allowed the takeover.
	decisionAllowed promptDecision = 1
	// decisionDenied means the user denied the takeover or the prompt
	// timed out with no response.
	decisionDenied promptDecision = 2
)

// pending is the internal record for a pending prompt.
type pending struct {
	prompt   Prompt
	decision promptDecision
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
	return NewBrokerWithTimeout(defaultPromptTimeout)
}

// NewBrokerWithTimeout constructs a broker with a specific timeout.
// A nonpositive timeout uses defaultPromptTimeout.
func NewBrokerWithTimeout(timeout time.Duration) *Broker {
	// Resolve the prompt timeout shared by deadlines and expiry.
	if timeout <= 0 {
		timeout = defaultPromptTimeout
	}

	// Build the broker with an empty pending prompt set.
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
	// Identify the runtime requesting takeover for the desktop prompt.
	if requesterName == "" {
		requesterName = "spacewave serve"
	}

	// Determine the takeover prompt deadline from the broker clock.
	now := b.nowFn()
	deadline := now.Add(b.timeout)

	// Prepare the pending takeover decision and its completion notification.
	p := &pending{
		decision: decisionPending,
		done:     make(chan struct{}),
	}

	// Publish the takeover prompt and notify the desktop prompt watchers.
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

	// Bound the takeover wait by the prompt timeout.
	timer := time.NewTimer(b.timeout)
	defer timer.Stop()

	// Resolve the takeover result on cancellation, expiry, or desktop response.
	select {
	case <-ctx.Done():
		b.finalizePrompt(id, decisionDenied)
		return ctx.Err()
	case <-timer.C:
		decision := b.finalizePrompt(id, decisionDenied)
		if decision == decisionAllowed {
			return nil
		}
		return errors.Errorf(
			"takeover prompt timed out after %s with no response from the Spacewave desktop app",
			b.timeout,
		)
	case <-p.done:
		var final promptDecision
		b.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			final = p.decision
			delete(b.pending, id)
		})
		if final == decisionAllowed {
			return nil
		}
		return errors.New(
			"takeover denied by the Spacewave desktop app; quit the app or approve in-app before retrying",
		)
	}
}

// finalizePrompt finalizes a prompt with decision if still pending and
// broadcasts the change. Returns the effective decision.
func (b *Broker) finalizePrompt(id string, decision promptDecision) promptDecision {
	var result promptDecision
	b.bcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
		// Find the pending takeover prompt or report that it has already been removed.
		p, ok := b.pending[id]
		if !ok {
			result = decisionDenied
			return
		}

		// Finalize the takeover decision and remove the prompt from desktop snapshots.
		if p.decision == decisionPending {
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
	// Resolve the takeover prompt under the broker lock.
	var resolved bool
	var existed bool
	b.bcast.HoldLock(func(broadcastFn func(), _ func() <-chan struct{}) {
		// Find a takeover prompt whose decision is still pending.
		p, ok := b.pending[id]
		if !ok {
			return
		}
		existed = true
		if p.decision != decisionPending {
			return
		}

		// Publish the desktop decision and wake the takeover policy.
		if allow {
			p.decision = decisionAllowed
		} else {
			p.decision = decisionDenied
		}
		close(p.done)
		resolved = true
		broadcastFn()
	})

	// Report missing or already resolved takeover prompts to the caller.
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
	// Snapshot pending desktop prompts with their next change notification.
	var out []Prompt
	var waitCh <-chan struct{}
	b.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		waitCh = getWaitCh()
		out = make([]Prompt, 0, len(b.pending))
		for _, p := range b.pending {
			if p.decision != decisionPending {
				continue
			}
			out = append(out, p.prompt)
		}
	})

	// Sort outside the lock because an unchanged prompt set must
	// snapshot stably.
	slices.SortFunc(out, func(a, b Prompt) int { return strings.Compare(a.ID, b.ID) })
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
		// Require an active handoff before reclaiming the listener socket.
		if b.reclaim == nil {
			return
		}

		// Signal the listener to reclaim its socket and clear the desktop handoff state.
		close(b.reclaim)
		b.reclaim = nil
		b.handoff = HandoffState{}
		fired = true
		broadcastFn()
	})
	return fired
}
