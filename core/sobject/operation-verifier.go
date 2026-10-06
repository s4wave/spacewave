package sobject

import (
	"sync"

	"github.com/pkg/errors"
)

// SOOperationVerifier builds the operation sets of the states of one shared
// object. It remembers the operations it verified for the last set, so a state
// that gains an operation verifies only that one, and a state that loses
// operations to a checkpoint or is replaced by another drops them. An operation
// is reused only when its bytes and signature are unchanged, so a set equals one
// built from nothing. It is safe for concurrent use.
type SOOperationVerifier struct {
	// sharedObjectID is the shared object the operations are bound to.
	sharedObjectID string
	// mtx guards verified.
	mtx sync.Mutex
	// verified holds the operations of the last set built, by hash.
	verified map[string]verifiedOperation
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
	for i, op := range state.GetOps() {
		key := string(op.Hash())
		held, ok := v.verified[key]
		if !ok || !held.op.EqualVT(op) {
			inner, err := op.Verify(v.sharedObjectID)
			if err != nil {
				return nil, errors.Wrapf(err, "ops[%d]", i)
			}
			held = verifiedOperation{op: op, inner: inner}
		}
		verified[key] = held
		set.insert(key, held.inner)
	}

	// Resolve the sequence and keep the operations of this set for the next.
	set.setSequence(state.GetConfig().GetSequencer(), state.GetSequence())
	v.verified = verified
	return set, nil
}
