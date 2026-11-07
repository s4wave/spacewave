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
// signature match, so a set equals one built from nothing. Sequence positions
// are remembered the same way, by hash, since state copies do not share them.
// It is safe for concurrent use.
type SOOperationVerifier struct {
	// sharedObjectID is the shared object the operations are bound to.
	sharedObjectID string
	// mtx guards verified, hashes and sequences.
	mtx sync.Mutex
	// verified holds the operations of the last set built, by hash.
	verified map[string]verifiedOperation
	// hashes holds the hashes of the last state's immutable operations.
	hashes map[*SOOperation]string
	// sequences holds the sequence positions of the last set built, by hash.
	sequences map[string]verifiedSequence
}

// verifiedSequence is a sequence position with the body and signer its
// verification returned.
type verifiedSequence struct {
	// record is the position that was verified.
	record *SOSequence
	// inner is the verified body of record.
	inner *SOSequenceInner
	// signer is the peer that signed record.
	signer string
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

	// Resolve the sequence, verifying the positions not yet verified, and keep
	// the operations and positions of this set for the next.
	records := state.GetSequence()
	sequences := make(map[string]verifiedSequence, len(records))
	set.setSequence(state.GetConfig().GetSequencer(), records, func(record *SOSequence) (string, *SOSequenceInner, string, error) {
		key, inner, signer, err := v.verifySequenceLocked(record)
		if err == nil {
			sequences[key] = verifiedSequence{record: record, inner: inner, signer: signer}
		}
		return key, inner, signer, err
	})
	v.verified = verified
	v.hashes = hashes
	v.sequences = sequences
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

// verifySequence returns the verified body and signer of record, reusing the
// last set's for an unchanged position.
func (v *SOOperationVerifier) verifySequence(record *SOSequence) (*SOSequenceInner, string, error) {
	v.mtx.Lock()
	defer v.mtx.Unlock()
	_, inner, signer, err := v.verifySequenceLocked(record)
	return inner, signer, err
}

// verifySequenceLocked returns the hash, verified body and signer of record,
// reusing the last set's for an unchanged position. The hash names only the
// body, so a held position is reused only when its signature matches too. The
// caller holds mtx.
func (v *SOOperationVerifier) verifySequenceLocked(record *SOSequence) (string, *SOSequenceInner, string, error) {
	key := string(record.Hash())
	if held, ok := v.sequences[key]; ok && held.record.EqualVT(record) {
		return key, held.inner, held.signer, nil
	}
	inner, signer, err := record.Verify(v.sharedObjectID)
	return key, inner, signer, err
}
