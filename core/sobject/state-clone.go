package sobject

import "slices"

// Clone copies the state while sharing its immutable signed records: the
// operations, the positions and the control messages. Their slices and every
// other field are independent, so a clone costs one pointer copy per record
// and a store can tell an unchanged record by its pointer. Callers must not
// modify a shared record or its signature; CloneVT makes a fully deep copy.
func (s *SOState) Clone() *SOState {
	// Preserve nil snapshots.
	if s == nil {
		return nil
	}

	// Deep-copy the remaining fields without copying the signed records.
	rest := *s
	rest.Ops, rest.Sequence, rest.ControlMessages = nil, nil, nil
	next := rest.CloneVT()
	next.Ops = slices.Clone(s.Ops)
	next.Sequence = slices.Clone(s.Sequence)
	next.ControlMessages = slices.Clone(s.ControlMessages)
	return next
}
