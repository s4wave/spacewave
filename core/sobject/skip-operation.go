package sobject

import (
	"bytes"
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
)

// MaxLiveSkips is the most skips above the checkpoint an owner's skip leaves
// in the config. SkipSOOperation refuses a skip past it. Each skip adds a full
// config copy to the config chain.
const MaxLiveSkips = 8

// MaxSkippedOperations is the most skips a config holds. A merge keeps every
// skip of each owner's concurrent branch, so it may hold more than
// MaxLiveSkips. A merge past this bound fails instead of dropping a skip.
const MaxSkippedOperations = 32

// MaxSkipReasonSize is the largest owner's note on a skip, in bytes.
const MaxSkipReasonSize = 256

// ReasonSkipped is the outcome of an operation an owner skipped.
const ReasonSkipped = "an owner skipped it"

// SkipSOOperation has signer, an owner, skip the operation at pos, which
// replay waits at or cannot apply, with reason as its note to the author.
// Every replica then refuses the operation, including one that already
// applied it. A skip cannot be withdrawn. The operation must be held above
// the checkpoint, so a mistyped position skips nothing. Under group control
// signer agrees to the skip and SkipSOOperation returns ErrAwaitingGroup.
// SkipSOOperation does nothing when the config already skips the operation.
func SkipSOOperation(ctx context.Context, host *SOHost, pos *SOOperationPosition, reason string, signer crypto.PrivKey) error {
	// Find the operation above the checkpoint.
	state, err := host.GetHostState(ctx)
	if err != nil {
		return errors.Wrap(err, "get current SO state")
	}
	set, err := state.OperationSet(host.GetSharedObjectID())
	if err != nil {
		return err
	}
	inner := set.Get(pos.GetOpHash())
	if inner == nil || inner.GetPeerId() != pos.GetPeerId() || inner.GetNonce() != pos.GetNonce() {
		return errors.New("no operation held above the checkpoint at that position")
	}
	current := state.GetConfig()
	if current.SkipsOperation(pos.GetOpHash(), inner) {
		return nil
	}

	// Drop the skips the checkpoint covers, so the cap counts only the skips
	// that still decide an operation, then add the skip in op hash order.
	next := current.CloneVT()
	pruneSkippedOperations(set, next)
	if live := len(next.GetSkippedOperations()); live >= MaxLiveSkips {
		return errors.Errorf("the config already skips %d operations above the checkpoint, the most an owner adds", live)
	}
	next.SkippedOperations = append(next.SkippedOperations, &SOSkippedOperation{Position: pos.CloneVT(), Reason: reason})
	sortSkippedOperations(next.SkippedOperations)
	return ChangeSOConfig(ctx, host, state, next, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_SKIP_OPERATION, signer, nil, nil)
}

// SkipsOperation reports whether c skips the operation inner with hash h. An
// entry matches by hash, author and nonce: the hash keeps a reordered log
// from moving the skip onto another operation, and the author and nonce bind
// it to one position of one author's chain.
func (c *SharedObjectConfig) SkipsOperation(h []byte, inner *SOOperationInner) bool {
	// Find the entry for the hash.
	skips := c.GetSkippedOperations()
	i, ok := slices.BinarySearchFunc(skips, h, func(s *SOSkippedOperation, h []byte) int {
		return bytes.Compare(s.GetPosition().GetOpHash(), h)
	})
	if !ok {
		return false
	}

	// Match the author and nonce the entry binds.
	pos := skips[i].GetPosition()
	return pos.GetPeerId() == inner.GetPeerId() && pos.GetNonce() == inner.GetNonce()
}

// ReplayRefusal returns the reason replay refuses operation h of set under
// cfg before reading its data, or "" to prepare it: an author that signed
// two operations at its sequence, or an owner's skip. Every replay path
// checks these first, so each member reaches the same outcome.
func ReplayRefusal(set *SOOperationSet, cfg *SharedObjectConfig, h []byte, inner *SOOperationInner) string {
	switch {
	case set.Equivocated(h):
		return ReasonEquivocated
	case cfg.SkipsOperation(h, inner):
		return ReasonSkipped
	default:
		return ""
	}
}

// pruneSkippedOperations drops from next each skip whose position set's
// checkpoint covers: the checkpoint holds the order there for good, so the
// entry decides nothing more. It keeps every skip above the checkpoint.
func pruneSkippedOperations(set *SOOperationSet, next *SharedObjectConfig) {
	next.SkippedOperations = slices.DeleteFunc(next.SkippedOperations, func(s *SOSkippedOperation) bool {
		return set.Covers(s.GetPosition().GetPeerId(), s.GetPosition().GetNonce())
	})
}

// sortSkippedOperations sorts skips by op hash.
func sortSkippedOperations(skips []*SOSkippedOperation) {
	slices.SortFunc(skips, func(a, b *SOSkippedOperation) int {
		return bytes.Compare(a.GetPosition().GetOpHash(), b.GetPosition().GetOpHash())
	})
}

// validateSkippedOperations checks that skips holds at most
// MaxSkippedOperations real positions, strictly sorted by op hash, each with
// a reason of at most MaxSkipReasonSize bytes.
func validateSkippedOperations(skips []*SOSkippedOperation) error {
	if len(skips) > MaxSkippedOperations {
		return errors.Errorf("skipped_operations holds %d entries, more than %d", len(skips), MaxSkippedOperations)
	}
	for i, skip := range skips {
		if err := validatePosition(skip.GetPosition()); err != nil {
			return errors.Wrapf(err, "skipped_operations[%d]", i)
		}
		if len(skip.GetReason()) > MaxSkipReasonSize {
			return errors.Errorf("skipped_operations[%d]: reason is longer than %d bytes", i, MaxSkipReasonSize)
		}
		if i > 0 && bytes.Compare(skips[i-1].GetPosition().GetOpHash(), skip.GetPosition().GetOpHash()) >= 0 {
			return errors.New("skipped_operations must be strictly sorted by op_hash")
		}
	}
	return nil
}
