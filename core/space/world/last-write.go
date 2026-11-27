package space_world

import (
	"bytes"
	"slices"
	"strings"

	"github.com/pkg/errors"
)

// ExpectWriteEnv turns off ExpectedWrite on this device's writes when set to
// "off" before opening the adapter, so they apply whatever their author wrote
// before. Replay checks every operation that carries one either way, as every
// replica must decide it alike.
const ExpectWriteEnv = "SPACEWAVE_WORLD_EXPECT_WRITE"

// reasonExpectedWrite is replay's refusal of an operation whose author's
// LastWrite in the World before it is not the write it expects.
const reasonExpectedWrite = "built after a write of its author that did not apply"

// lastWrite returns the hash of peerID's LastWrite in s, or nil.
func (s *InnerState) lastWrite(peerID string) []byte {
	writes := s.GetLastWrites()
	if i, ok := slices.BinarySearchFunc(writes, peerID, compareWriter); ok {
		return writes[i].GetOpHash()
	}
	return nil
}

// expects reports whether an operation of peerID expecting exp may apply on
// s: it expects nothing, or its author's LastWrite in s is the one it expects.
func (s *InnerState) expects(peerID string, exp *ExpectedWrite) bool {
	return exp == nil || bytes.Equal(s.lastWrite(peerID), exp.GetOpHash())
}

// withLastWrite returns s with w as its author's LastWrite. It leaves s
// unchanged and shares its other fields, as positions share states.
func (s *InnerState) withLastWrite(w *LastWrite) *InnerState {
	writes := slices.Clone(s.GetLastWrites())
	if i, ok := slices.BinarySearchFunc(writes, w.GetPeerId(), compareWriter); ok {
		writes[i] = w
	} else {
		writes = slices.Insert(writes, i, w)
	}
	return &InnerState{HeadRef: s.GetHeadRef(), RetainedRoots: s.GetRetainedRoots(), LastWrites: writes}
}

// recordedWrite returns the LastWrite next records beyond prev, or nil. An
// operation records at most one, its author's.
func recordedWrite(prev, next *InnerState) *LastWrite {
	if prev == next {
		return nil
	}
	for _, w := range next.GetLastWrites() {
		if !bytes.Equal(prev.lastWrite(w.GetPeerId()), w.GetOpHash()) {
			return w
		}
	}
	return nil
}

// compareWriter orders a LastWrite by its author's peer ID.
func compareWriter(w *LastWrite, peerID string) int {
	return strings.Compare(w.GetPeerId(), peerID)
}

// errBaseUnsigned is the queue's failure of an operation built on a candidate
// whose operation was never signed, so no order holds the write it expects.
var errBaseUnsigned = errors.New("built on a write that was never signed")

// expectWrite returns adm's operation with the ExpectedWrite of what its
// candidate was built after: the signed hash of the admission it built on, or
// this device's LastWrite in the installed head it built on. It returns adm's
// operation unchanged when this device's writes carry none, or when adm has no
// candidate, as an operation such as SetRetainedRoot built on no World
// expects no write and records none. It returns errBaseUnsigned when the
// admission it built on was never signed. The queue goroutine calls it once,
// after queueing every earlier admission, so a signed base has its receipt.
func (a *Adapter) expectWrite(adm *spaceAdmission) ([]byte, error) {
	// Take the write the candidate was built after.
	if !a.expectWrites || adm.getCandidate() == nil {
		return adm.op, nil
	}
	hash := adm.afterHead
	if adm.after != nil {
		if hash = adm.after.receipt.OpHash(); len(hash) == 0 {
			return nil, errBaseUnsigned
		}
	}

	// Append the field: a WorldOp's encoding is the concatenation of its
	// fields' encodings.
	expected := &ExpectedWrite{OpHash: hash}
	field, err := (&WorldOp{ExpectedWrite: expected}).MarshalVT()
	if err != nil {
		return nil, err
	}
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		adm.expected = expected
	})
	return slices.Concat(adm.op, field), nil
}

// getExpected returns the ExpectedWrite the queue wrote into the operation of
// s, or nil for a nil s.
func (s *spaceAdmission) getExpected() *ExpectedWrite {
	if s == nil {
		return nil
	}
	var expected *ExpectedWrite
	s.a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		expected = s.expected
	})
	return expected
}
