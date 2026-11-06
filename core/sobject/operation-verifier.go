package sobject

import (
	"sync"

	"github.com/pkg/errors"
)

// SOOperationVerifier builds the operation sets of the states of one shared
// object. It remembers the operations it verified for the last set, so a state
// that gains an operation verifies only that one, and a state that loses
// operations to a checkpoint or is replaced by another drops them. An operation
// is immutable while held: state copies share it, and the verifier reuses its
// hash. A separately decoded operation is reused only when its bytes and
// signature match, so a set equals one built from nothing. It is safe for
// concurrent use.
type SOOperationVerifier struct {
	// sharedObjectID is the shared object the operations are bound to.
	sharedObjectID string
	// mtx guards verified and hashes.
	mtx sync.Mutex
	// verified holds the operations of the last set built, by hash.
	verified map[string]verifiedOperation
	// hashes holds the hashes of the last state's immutable operations.
	hashes map[*SOOperation]string
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
// set did not hold unchanged and forgets the rest.
func (v *SOOperationVerifier) OperationSet(state *SOState) (*SOOperationSet, error) {
	// Anchor the set at the checkpoint.
	checkpoint, err := state.GetCheckpointInner()
	if err != nil {
		return nil, err
	}

	// Add every held operation, verifying the ones not yet verified.
	v.mtx.Lock()
	defer v.mtx.Unlock()
	set := NewSOOperationSet(v.sharedObjectID, checkpoint)
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
	v.verified = verified
	v.hashes = hashes
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
