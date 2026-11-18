package space_world

import (
	"bytes"

	"github.com/s4wave/spacewave/core/sobject"
)

// coverOutcome is what an installed checkpoint decides about one of this
// device's operations.
type coverOutcome int

const (
	// coverNone means the checkpoint does not cover the operation, so replay
	// still places it.
	coverNone coverOutcome = iota
	// coverApplied means the checkpoint covers the operation and applied it.
	coverApplied
	// coverRefused means the checkpoint covers the operation and refused it.
	coverRefused
	// coverUnknown means the checkpoint covers the operation without saying
	// whether it applied: its signer records no outcomes, an installed
	// checkpoint between the two went unseen, or the covered chain is not this
	// operation's.
	coverUnknown
)

// checkpointCover is what the installed checkpoint says about this device's
// operations: which it covers, and of those it newly covers, which it refused.
//
// Coverage is contiguous by hash chain: each operation links its author's
// previous one by hash, and a checkpoint covers an operation only after it
// covers its links, so an author entry at nonce N with hash H covers the whole
// chain ending at H. This device's operations form one chain, since
// AddLocalOperation takes each nonce and link from the durable head under the
// provider lock. A checkpoint that records outcomes lists every refused
// operation it newly covers, so any other operation it newly covers applied.
type checkpointCover struct {
	// height is the installed checkpoint's height.
	height uint64
	// from is the nonce of this device's last operation that the checkpoint
	// installed before this one covered, or zero.
	from uint64
	// head is this device's last covered operation, or nil when the
	// checkpoint covers none.
	head *sobject.SOOperationPosition
	// refused are the hashes of the refused operations the checkpoint newly
	// covers, nil when it does not know them.
	refused map[string]struct{}
}

// nextCover returns the cover of checkpoint for peerID, installed after prev,
// or after none when prev is nil.
func nextCover(prev *checkpointCover, checkpoint *sobject.SOCheckpointInner, peerID string) *checkpointCover {
	// Find this device's covered head.
	c := &checkpointCover{height: checkpoint.GetHeight()}
	for _, author := range checkpoint.GetAuthors() {
		if author.GetPeerId() == peerID {
			c.head = author
			break
		}
	}

	// Know the refusals only for a checkpoint that records them and directly
	// follows the one installed before it, whose coverage it extends.
	if prev == nil || checkpoint.GetOutcomes() == nil || c.height != prev.height+1 {
		return c
	}
	c.from = prev.head.GetNonce()
	c.refused = make(map[string]struct{}, len(checkpoint.GetOutcomes().GetRefused()))
	for _, refused := range checkpoint.GetOutcomes().GetRefused() {
		c.refused[string(refused.GetOpHash())] = struct{}{}
	}
	return c
}

// outcome returns what the cover decides about this device's operation at pos.
func (c *checkpointCover) outcome(pos *sobject.SOOperationPosition) coverOutcome {
	// An operation above the covered head waits for replay.
	if c == nil || pos == nil || c.head == nil || pos.GetNonce() > c.head.GetNonce() {
		return coverNone
	}

	// Decide only an operation the checkpoint newly covers, from outcomes it
	// records, on this operation's own chain.
	if c.refused == nil || pos.GetNonce() <= c.from {
		return coverUnknown
	}
	if pos.GetNonce() == c.head.GetNonce() && !bytes.Equal(pos.GetOpHash(), c.head.GetOpHash()) {
		return coverUnknown
	}
	if _, ok := c.refused[string(pos.GetOpHash())]; ok {
		return coverRefused
	}
	return coverApplied
}
