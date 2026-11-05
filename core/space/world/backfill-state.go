package space_world

// IsComplete reports whether the local store held every block of the covered
// roots.
func (s *BackfillState) IsComplete() bool {
	return s != nil && s.GetHoles() == 0
}
