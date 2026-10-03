package sobject

import (
	"context"

	"github.com/pkg/errors"
)

// FoldOutcome is the replay outcome of one operation.
type FoldOutcome struct {
	// Hash identifies the operation.
	Hash []byte
	// Inner is the verified operation body with its data still encoded.
	Inner *SOOperationInner
	// Reason is empty when the operation applied. Otherwise it says in plain
	// words why the operation was not applied.
	Reason string
}

// FoldResult is the state of a non-World shared object after replaying its
// operation set from the checkpoint.
type FoldResult struct {
	// StateData is the decoded state after the last placed operation.
	StateData []byte
	// Outcomes are the placed operations in replay order.
	Outcomes []FoldOutcome
}

// Outcome returns the outcome of peerID's operation with localID, or nil
// while replay has not placed it.
func (r *FoldResult) Outcome(peerID, localID string) *FoldOutcome {
	for i := range r.Outcomes {
		inner := r.Outcomes[i].Inner
		if inner.GetPeerId() == peerID && inner.GetLocalId() == localID {
			return &r.Outcomes[i]
		}
	}
	return nil
}

// Fold replays the operation set of snap from its checkpoint through process,
// one operation at a time in the set's order. Every member holding the same
// operations computes the same result. A missing epoch key or config stops the
// fold with an error, so members never diverge on what they could read.
func Fold(ctx context.Context, snap SharedObjectStateSnapshot, process ProcessOpsFunc) (*FoldResult, error) {
	// Start from the checkpoint's state.
	checkpoint, err := snap.GetCheckpoint(ctx)
	if err != nil {
		return nil, err
	}
	set, err := snap.GetOperationSet(ctx)
	if err != nil {
		return nil, err
	}
	res := &FoldResult{StateData: checkpoint.GetStateData()}

	// Apply each operation and record its outcome.
	for _, h := range set.Order() {
		inner := set.Get(h)
		reason, err := res.apply(ctx, snap, set, h, inner, process)
		if err != nil {
			return nil, err
		}
		res.Outcomes = append(res.Outcomes, FoldOutcome{Hash: h, Inner: inner, Reason: reason})
	}
	return res, nil
}

// apply replays one operation onto r.StateData. It returns the reason the
// operation was not applied, or an error when replay cannot continue.
func (r *FoldResult) apply(
	ctx context.Context,
	snap SharedObjectStateSnapshot,
	set *SOOperationSet,
	h []byte,
	inner *SOOperationInner,
	process ProcessOpsFunc,
) (string, error) {
	// Authorize and decode the operation as every member does.
	if set.Equivocated(h) {
		return ReasonEquivocated, nil
	}
	_, opData, reason, err := PrepareReplayOp(ctx, snap, inner)
	if err != nil || reason != "" {
		return reason, err
	}

	// Run the processor on the decoded operation.
	decoded := inner.CloneVT()
	decoded.OpData = opData
	next, results, err := process(ctx, snap, r.StateData, []*SOOperationInner{decoded})
	if err != nil {
		return "", errors.Wrap(err, "process operation")
	}
	for _, result := range results {
		if !result.GetSuccess() {
			return result.GetErrorDetails().GetErrorMsg(), nil
		}
	}
	if next != nil {
		r.StateData = *next
	}
	return "", nil
}

// ReasonRemoved is the outcome of an operation whose author was removed from
// the shared object before the operation reached the removing owner.
const ReasonRemoved = "its author was removed from the shared object"

// ReasonEquivocated is the outcome of an operation whose author signed another
// operation at the same sequence.
const ReasonEquivocated = "its author signed another operation at the same sequence"

// PrepareReplayOp authorizes the author of inner under the config the
// operation names, requires the current config to admit it, and decodes its
// data with the key of its epoch. It returns the author's participant entry
// and the decoded data, or the reason replay rejects the operation. A config
// or key the member lacks is an error, not a rejection.
func PrepareReplayOp(ctx context.Context, snap SharedObjectStateSnapshot, inner *SOOperationInner) (*SOParticipantConfig, []byte, string, error) {
	// Authorize the author under the config the operation was written under.
	cfg, err := snap.GetConfigByHash(ctx, inner.GetConfigHash())
	if err != nil {
		return nil, nil, "", errors.Wrap(err, "operation config")
	}
	var writer *SOParticipantConfig
	for _, p := range cfg.GetParticipants() {
		if p.GetPeerId() == inner.GetPeerId() && CanWriteOps(p.GetRole()) {
			writer = p
			break
		}
	}
	if writer == nil {
		return nil, nil, "its author could not write to the shared object", nil
	}

	// An old signature cannot show whether a removed author wrote the operation
	// before or after the removal, so only the operations the removal pinned
	// apply.
	current, err := snap.GetConfig(ctx)
	if err != nil {
		return nil, nil, "", err
	}
	if !current.AdmitsOperation(inner.GetPeerId(), inner.GetNonce()) {
		return nil, nil, ReasonRemoved, nil
	}

	// Decode the operation data. A held key that fails is a rejection every
	// holder agrees on.
	opData, err := snap.DecodeOperation(ctx, inner)
	if errors.Is(err, ErrKeyEpochUnavailable) {
		return nil, nil, "", err
	}
	if err != nil {
		return nil, nil, "its data could not be decoded", nil
	}
	return writer, opData, "", nil
}

// WaitOperation waits until replay places the local peer's operation with
// localID and returns the folded state. It returns ErrRejectedOp wrapped with
// the reason when replay rejected the operation.
func WaitOperation(ctx context.Context, so SharedObject, localID string, process ProcessOpsFunc) (*FoldResult, error) {
	// Watch the state until a fold places the operation.
	ctr, rel, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer rel()
	peerID := so.GetPeerID().String()
	var res *FoldResult
	_, err = ctr.WaitValueWithValidator(ctx, func(snap SharedObjectStateSnapshot) (bool, error) {
		// Fold until the outcome of the local operation appears.
		if snap == nil {
			return false, nil
		}
		folded, err := Fold(ctx, snap, process)
		if err != nil {
			return false, err
		}
		if folded.Outcome(peerID, localID) == nil {
			return false, nil
		}
		res = folded
		return true, nil
	}, nil)
	if err != nil {
		return nil, err
	}

	// Report a rejection with its reason.
	if reason := res.Outcome(peerID, localID).Reason; reason != "" {
		return res, errors.Wrap(ErrRejectedOp, reason)
	}
	return res, nil
}

// WriteOperation queues opData as the local peer and waits for its replay
// outcome. It returns the folded state that holds the operation.
func WriteOperation(ctx context.Context, so SharedObject, opData []byte, process ProcessOpsFunc) (*FoldResult, error) {
	localID, err := so.QueueOperation(ctx, opData)
	if err != nil {
		return nil, err
	}
	return WaitOperation(ctx, so, localID, process)
}
