package sobject

import (
	"slices"
	"sync"

	"github.com/pkg/errors"
)

// SOOperationVerifier builds the operation sets of the states of one shared
// object. It keeps the last set it built: a state with the same operations and
// anchor gets that set back, a state that gained operations gets a copy with
// only those verified and added, and any other state is built again, reusing
// the bodies of the held operations it still holds. An operation is immutable
// while held: state copies share it, and the verifier reuses its hash. A
// separately decoded operation is reused only when its bytes and signature
// match, so a set equals one built from nothing. It is safe for concurrent
// use.
type SOOperationVerifier struct {
	// sharedObjectID is the shared object the operations are bound to.
	sharedObjectID string
	// mtx guards the fields below.
	mtx sync.Mutex
	// verified holds the operations of the last set built, by hash.
	verified map[string]verifiedOperation
	// hashes holds the hashes of the last state's immutable operations.
	hashes map[*SOOperation]string
	// set is the last set built, or nil before the first.
	set *SOOperationSet
	// ops are the operations of the state set was built from, in its order,
	// copied because the state's owner may change its list in place.
	ops []*SOOperation
	// anchor is a deep copy of that state's checkpoint, sequencer and
	// sequence, which state copies may change in place.
	anchor *SOState
}

// verifiedOperation is an operation with the body its verification returned.
type verifiedOperation struct {
	// op is the operation that was verified.
	op *SOOperation
	// inner is the verified body of op, which sets share and never modify.
	inner *SOOperationInner
}

// NewSOOperationVerifier returns a verifier for the operations of sharedObjectID.
func NewSOOperationVerifier(sharedObjectID string) *SOOperationVerifier {
	return &SOOperationVerifier{sharedObjectID: sharedObjectID}
}

// OperationSet returns the operations state holds as a verified set above its
// checkpoint, with the sequence resolved. It verifies the operations the last
// set did not hold unchanged and forgets the rest. The set is shared with
// other callers, which must not modify it.
func (v *SOOperationVerifier) OperationSet(state *SOState) (*SOOperationSet, error) {
	// Extend the last set when state holds its operations above its anchor.
	v.mtx.Lock()
	defer v.mtx.Unlock()
	anchor := &SOState{
		Checkpoint: state.GetCheckpoint(),
		Config:     &SharedObjectConfig{Sequencer: state.GetConfig().GetSequencer()},
		Sequence:   state.GetSequence(),
	}
	if v.set != nil && anchor.GetCheckpoint().EqualVT(v.anchor.GetCheckpoint()) {
		if added, ok := v.added(state.GetOps()); ok {
			return v.extendLocked(state, anchor, added)
		}
	}

	// Anchor a new set at the checkpoint.
	checkpoint, err := state.GetCheckpointInner()
	if err != nil {
		return nil, err
	}
	set := NewSOOperationSet(v.sharedObjectID, checkpoint)

	// Add every held operation, verifying the ones not yet verified.
	verified := make(map[string]verifiedOperation, len(state.GetOps()))
	hashes := make(map[*SOOperation]string, len(state.GetOps()))
	for i, op := range state.GetOps() {
		key, inner, err := v.verifyLocked(op)
		if err != nil {
			return nil, errors.Wrapf(err, "ops[%d]", i)
		}
		verified[key] = verifiedOperation{op: op, inner: inner}
		hashes[op] = key
		set.insert(key, inner)
	}

	// Resolve the sequence and keep the operations of this set for the next.
	set.setSequence(state.GetConfig().GetSequencer(), state.GetSequence())
	v.verified, v.hashes = verified, hashes
	v.set, v.ops, v.anchor = set, slices.Clone(state.GetOps()), anchor.CloneVT()
	return set, nil
}

// added returns the operations of ops the last set does not hold, and whether
// ops holds every operation of the last set. Both lists are in hash order, so
// one pass compares the shared pointers.
func (v *SOOperationVerifier) added(ops []*SOOperation) ([]*SOOperation, bool) {
	var added []*SOOperation
	held := 0
	for _, op := range ops {
		if held < len(v.ops) && v.ops[held] == op {
			held++
			continue
		}
		added = append(added, op)
	}
	return added, held == len(v.ops)
}

// extendLocked returns the set of state, which holds the operations of the
// last set and added above the same checkpoint. It returns the last set when
// nothing changed, and otherwise a copy with added verified and inserted and
// the sequence resolved again when it changed. The caller holds mtx.
func (v *SOOperationVerifier) extendLocked(state, anchor *SOState, added []*SOOperation) (*SOOperationSet, error) {
	// Return the last set when state holds the same operations and sequence.
	sequenced := anchor.GetConfig().GetSequencer().EqualVT(v.anchor.GetConfig().GetSequencer()) &&
		slices.EqualFunc(anchor.GetSequence(), v.anchor.GetSequence(), (*SOSequence).EqualVT)
	if len(added) == 0 && sequenced {
		return v.set, nil
	}

	// Verify the added operations into a copy of the last set.
	set := v.set.clone()
	keys := make([]string, len(added))
	inners := make([]*SOOperationInner, len(added))
	for i, op := range added {
		key, inner, err := v.verifyLocked(op)
		if err != nil {
			return nil, errors.Wrap(err, "added operation")
		}
		keys[i], inners[i] = key, inner
		set.insert(key, inner)
	}
	if !sequenced {
		set.setSequence(anchor.GetConfig().GetSequencer(), anchor.GetSequence())
		v.anchor = anchor.CloneVT()
	} else if len(keys) == 1 {
		set.extendLast(v.set, keys[0])
	}

	// Remember the added operations with the held ones.
	for i, op := range added {
		v.verified[keys[i]] = verifiedOperation{op: op, inner: inners[i]}
		v.hashes[op] = keys[i]
	}
	v.set, v.ops = set, slices.Clone(state.GetOps())
	return set, nil
}

// AdoptCheckpoint adopts next into state as SOState.AdoptCheckpoint does, and
// rebuilds the operations above next from the ones this verifier verified for
// the last set, checking only the signatures of held operations it has not
// verified. The host's state took each held operation verified, so a
// checkpoint the host adopts checks none of them again.
func (v *SOOperationVerifier) AdoptCheckpoint(state *SOState, next *SOCheckpoint) error {
	return state.adoptCheckpoint(v, next)
}

// keep adds each of ops to set and returns the ones set took, in order, in
// the start of ops. An operation set covers or holds, or whose signature
// fails, is left out. It verifies only the operations the last set did not
// hold unchanged, and leaves what the verifier remembers as it was. Verifying
// may suspend under GoScript, so it filters with a plain loop.
func (v *SOOperationVerifier) keep(set *SOOperationSet, ops []*SOOperation) []*SOOperation {
	// Keep each operation the set takes, in place.
	v.mtx.Lock()
	defer v.mtx.Unlock()
	kept := ops[:0]
	for _, op := range ops {
		key, inner, err := v.verifyLocked(op)
		if err == nil && set.insert(key, inner) {
			kept = append(kept, op)
		}
	}
	clear(ops[len(kept):])
	return kept
}

// verifyLocked returns the hash and verified body of op, reusing the last
// set's body for an unchanged operation. The caller holds mtx.
func (v *SOOperationVerifier) verifyLocked(op *SOOperation) (string, *SOOperationInner, error) {
	// Reuse the body of an unchanged operation, else verify it.
	key, ok := v.hashes[op]
	if !ok {
		key = string(op.Hash())
	}
	if held, ok := v.verified[key]; ok && held.op.EqualVT(op) {
		return key, held.inner, nil
	}
	inner, err := op.Verify(v.sharedObjectID)
	return key, inner, err
}
