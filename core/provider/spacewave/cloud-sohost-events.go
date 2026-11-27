package provider_spacewave

import (
	"context"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// connected resyncs the host after a new connection subscribed, since events
// published while no socket was subscribed are lost. It runs on the Session
// websocket's read goroutine and returns in constant time.
func (h *cloudSOHost) connected() {
	h.stateBacklog.markResync(SeedReasonReconnect)
	h.stateRoutine.Trigger()
}

// soNotify records an so_notify payload for the state routine. It runs on the
// Session websocket's read goroutine and returns in constant time. An inline
// snapshot or delta goes to the backlog with its block store nonce. A
// configuration notification carries no state, so soNotify pulls the state
// that holds the new configuration. Applying the state wakes the chain
// verifier.
func (h *cloudSOHost) soNotify(payload *api.SONotifyEventPayload) {
	// Hand inline state to the state routine.
	if payload == nil {
		return
	}
	msg := payload.GetStateMessage()
	switch {
	case msg.GetSnapshot() != nil || msg.GetDelta() != nil:
		h.stateBacklog.push(msg, payload.GetBlockStoreNonce())
		h.stateRoutine.Trigger()
		return
	case msg.GetConfigChanged() != nil:
		h.triggerPull()
		return
	case msg.GetError() != nil:
		h.le.WithField("code", msg.GetError().GetCode()).
			WithField("message", msg.GetError().GetMessage()).
			Warn("received error from SO DO via inline state")
		return
	}

	// Route a notification that carries no state by its change type.
	switch payload.GetChangeType() {
	case "configChanged":
		h.triggerPull()
	case "metadata", "ownershipChanged", "access_changed", "delete":
		// Account-level notification routing handles these.
	default:
		// Cloud always attaches an inline SOStateMessage for op and checkpoint
		// mutations, so a missing inline state payload indicates a publisher
		// bug rather than a missed update the host should pull behind.
		h.le.WithField("change-type", payload.GetChangeType()).
			WithField("seqno", payload.GetSeqno()).
			Warn("so_notify arrived without inline state payload; ignoring")
	}
}

// applyStateBacklog applies the inline state the backlog holds. It refreshes
// the block manifest first, so no accepted state references a block the
// manifest lacks, then pulls for a resync or applies the snapshot and the run.
func (h *cloudSOHost) applyStateBacklog(ctx context.Context) {
	// Refresh referenced blocks before accepting newer state. A failure keeps
	// the state out and resyncs on the next event.
	batch := h.stateBacklog.take()
	if err := h.refreshBlockManifestForNonce(ctx, batch.nonce); err != nil {
		if ctx.Err() != nil {
			return
		}
		h.le.WithError(err).
			WithField("blockstore-nonce", batch.nonce).
			Warn("failed to refresh block manifest before inline state apply")
		h.stateBacklog.markResync(SeedReasonGapRecovery)
		h.triggerPull()
		return
	}

	// A resync reads the cloud now, after the take, so it covers every message
	// the backlog dropped. Joining an earlier in-flight pull would not. A
	// failure resyncs on the next event.
	if batch.resync != "" {
		if err := h.pullState(ctx, batch.resync); err != nil && ctx.Err() == nil {
			h.le.WithError(err).
				WithField("reason", batch.resync).
				Warn("failed to resync shared object state")
			h.stateBacklog.markResync(batch.resync)
		}
		return
	}

	// Apply the newest snapshot, then the run that continues from it.
	for _, msg := range []*api.SOStateMessage{batch.snapshot, batch.run} {
		if msg != nil && !h.applyInlineState(ctx, msg) {
			return
		}
	}
}

// applyInlineState applies one held snapshot or delta, reporting whether it
// applied. A config chain mismatch waits for the chain verifier, which the
// failed verification already signaled. Any other failure pulls.
func (h *cloudSOHost) applyInlineState(ctx context.Context, msg *api.SOStateMessage) bool {
	// Apply the message under the host's acceptance lock.
	err := h.handleStateDelta(ctx, msg)
	if err == nil {
		return true
	}
	if ctx.Err() != nil {
		return false
	}

	// Firing /state on a config chain mismatch would read the same state and
	// fail verification the same way until the chain catches up. The next
	// event or gap recovery carries the state forward instead.
	if errors.Is(err, errSOConfigChainChanged) {
		h.le.WithError(err).
			WithField("seqno", msg.GetSeqno()).
			Debug("inline state deferred to config chain sync")
		return false
	}
	h.le.WithError(err).
		WithField("seqno", msg.GetSeqno()).
		Debug("inline state apply failed; falling back to pull")
	h.triggerPull()
	return false
}

// _ is a type assertion.
var _ soEvents = (*cloudSOHost)(nil)
