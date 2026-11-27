package space_world

import (
	"bytes"
	"os"

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
	// checkpoint between the two went unseen, or this device holds no proof
	// that the covered chain is this operation's.
	coverUnknown
)

// checkpointCover is what the installed checkpoint says about this device's
// operations: which it covers, and of those it newly covers, which it refused.
//
// Coverage is contiguous by hash chain: each operation links its author's
// previous one by hash, and a checkpoint covers an operation only after it
// covers its links, so an author entry at nonce N with hash H covers the whole
// chain ending at H. This device's operations need not form one chain: a
// restore of the same identity can sign another operation at a nonce already
// used. So an operation counts as covered and applied only when the walk from
// H back through the operations this device holds reaches its hash. A
// checkpoint that records outcomes lists every refused operation it newly
// covers, so any other operation it newly covers on H's chain applied. A
// refusal names its operation by hash, so it decides that operation at any
// install. Applied needs the previous checkpoint's head, which says where
// "newly covers" starts: the checkpoint records it as PrevAuthors, and one
// signed before that field leaves it to the checkpoint installed before.
type checkpointCover struct {
	// height is the installed checkpoint's height.
	height uint64
	// from is the nonce of this device's last operation that the checkpoint
	// before this one covered, or zero. It is known only when chain is set.
	from uint64
	// head is this device's last covered operation, or nil when the
	// checkpoint covers none.
	head *sobject.SOOperationPosition
	// refused are the hashes of the refused operations the checkpoint newly
	// covers, nil when it does not know them.
	refused map[string]struct{}
	// chain holds the hashes of the operations this device holds on the
	// covered chain ending at head, above from. It is nil when from is
	// unknown, so the cover proves no operation applied.
	chain map[string]struct{}
}

// nextCover returns the cover of checkpoint for peerID, installed after prev,
// or after none when prev is nil. held returns the verified body of an
// operation this device holds by its hash, or nil.
func nextCover(
	prev *checkpointCover,
	checkpoint *sobject.SOCheckpointInner,
	peerID string,
	held func(h []byte) *sobject.SOOperationInner,
) *checkpointCover {
	// Find this device's covered head.
	c := &checkpointCover{height: checkpoint.GetHeight()}
	for _, author := range checkpoint.GetAuthors() {
		if author.GetPeerId() == peerID {
			c.head = author
			break
		}
	}

	// Know the refusals of a checkpoint that records outcomes. Under
	// checkpointStandaloneOutcomeEnv=off, know them only when it directly
	// follows the one installed before it.
	outcomes := checkpoint.GetOutcomes()
	follows := prev != nil && c.height == prev.height+1
	if outcomes == nil || (!follows && checkpointStandaloneOutcomeOff()) {
		return c
	}
	c.refused = make(map[string]struct{}, len(outcomes.GetRefused()))
	for _, refused := range outcomes.GetRefused() {
		c.refused[string(refused.GetOpHash())] = struct{}{}
	}

	// Prove applied operations only above the previous checkpoint's head:
	// the PrevAuthors it records, or, on a checkpoint signed before that
	// field, the head of the one installed directly before it.
	from, known := prevHead(prev, follows, outcomes.GetPrevAuthors(), peerID)
	if !known {
		return c
	}
	c.from = from
	if c.head != nil {
		c.chain = heldChain(held, peerID, c.head.GetOpHash(), c.from)
	}
	return c
}

// prevHead returns the nonce of peerID's last operation the previous
// checkpoint covered, zero when it covered none, and whether that is known.
// prevAuthors, nil on a checkpoint signed before the field existed, names it
// at any install unless checkpointStandaloneOutcomeEnv is off. Otherwise it is
// known only when the checkpoint follows prev, the one installed before it.
// When both name it they must agree: a checkpoint that follows prev yet
// records another previous head is on a forked chain, so neither is trusted.
func prevHead(prev *checkpointCover, follows bool, prevAuthors *sobject.SOCheckpointAuthors, peerID string) (uint64, bool) {
	// Without recorded previous heads, only the installed one names it.
	if prevAuthors == nil || checkpointStandaloneOutcomeOff() {
		if !follows {
			return 0, false
		}
		return prev.head.GetNonce(), true
	}

	// Find peerID's entry, absent when the previous checkpoint covered none
	// of its operations.
	var recorded *sobject.SOOperationPosition
	for _, author := range prevAuthors.GetAuthors() {
		if author.GetPeerId() == peerID {
			recorded = author
			break
		}
	}

	// Distrust recorded heads that disagree with the installed one.
	if follows && !recorded.EqualVT(prev.head) {
		return 0, false
	}
	return recorded.GetNonce(), true
}

// heldChain returns the hashes of peerID's operations held that the chain
// ending at tip holds above nonce: tip, then each operation it reaches by its
// previous-operation link, every one signed by peerID one nonce below the
// last. The walk stops at the first link held lacks, so the result proves
// membership of each hash it returns and says nothing about the rest.
func heldChain(held func(h []byte) *sobject.SOOperationInner, peerID string, tip []byte, above uint64) map[string]struct{} {
	chain := make(map[string]struct{})
	var next uint64
	for h := tip; len(h) != 0; {
		// Stop at a link held lacks or that leaves the author's chain.
		inner := held(h)
		if inner == nil || inner.GetPeerId() != peerID || inner.GetNonce() <= above ||
			(next != 0 && inner.GetNonce() != next) {
			break
		}

		// Record it and follow its link.
		chain[string(h)] = struct{}{}
		next, h = inner.GetNonce()-1, inner.GetPrevOpHash()
	}
	return chain
}

// checkpointTipOutcomeEnv disables membership proof from the signed tip alone.
const checkpointTipOutcomeEnv = "SPACEWAVE_WORLD_CHECKPOINT_TIP_OUTCOME"

// checkpointStandaloneOutcomeEnv, set to "off", restores the rule that a
// checkpoint decides nothing unless it directly follows the one installed
// before it, as at the first install after a mount, and ignores PrevAuthors.
const checkpointStandaloneOutcomeEnv = "SPACEWAVE_WORLD_CHECKPOINT_STANDALONE_OUTCOME"

// checkpointStandaloneOutcomeOff reports whether checkpointStandaloneOutcomeEnv
// restores the old rule.
func checkpointStandaloneOutcomeOff() bool {
	return os.Getenv(checkpointStandaloneOutcomeEnv) == "off"
}

// outcome returns what the cover decides about this device's operation at pos.
func (c *checkpointCover) outcome(pos *sobject.SOOperationPosition) coverOutcome {
	// An operation above the covered head waits for replay.
	if c == nil || pos == nil || c.head == nil || pos.GetNonce() > c.head.GetNonce() {
		return coverNone
	}

	// A refusal the checkpoint records names the operation by its hash.
	if _, ok := c.refused[string(pos.GetOpHash())]; ok {
		return coverRefused
	}

	// Decide applied only for an operation the checkpoint newly covers,
	// above the previous head, on the covered chain.
	if c.chain == nil || pos.GetNonce() <= c.from {
		return coverUnknown
	}
	if _, ok := c.chain[string(pos.GetOpHash())]; ok {
		return coverApplied
	}

	// The verified checkpoint names its exact tip even if replay did not
	// see the operation body. This is membership proof only for that hash,
	// and only because the interval and refusal checks above established
	// that the checkpoint records the operation's outcome.
	if pos.GetPeerId() == c.head.GetPeerId() && pos.GetNonce() == c.head.GetNonce() &&
		len(pos.GetOpHash()) != 0 && bytes.Equal(pos.GetOpHash(), c.head.GetOpHash()) &&
		os.Getenv(checkpointTipOutcomeEnv) != "off" {
		return coverApplied
	}
	return coverUnknown
}
