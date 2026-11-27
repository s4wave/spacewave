package sobject

import "slices"

// Clone copies the state while sharing its signed operations and checkpoint.
// The operation slice and every other field are independent; CloneVT makes a
// fully deep copy.
//
// Invariant: an operation or checkpoint never changes once it is in a state.
// To change one, replace it whole with an edited CloneVT copy. A state writer
// diffs the state it stored against the next one by EqualVT, which passes a
// shared pointer as equal at once; an operation or checkpoint edited in place
// is the same pointer on both sides, so the change is never stored and is
// lost at the next load. Sharing the checkpoint, up to MaxSyncStateSize bytes
// of state data, also keeps each write from copying it.
func (s *SOState) Clone() *SOState {
	// Preserve nil snapshots.
	if s == nil {
		return nil
	}

	// Deep-copy the remaining fields without copying operation or checkpoint bytes.
	rest := *s
	rest.Ops, rest.Checkpoint = nil, nil
	next := rest.CloneVT()
	next.Ops = slices.Clone(s.Ops)
	next.Checkpoint = s.Checkpoint
	return next
}
