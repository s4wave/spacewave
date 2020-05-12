package file

// LessThanRange compares to ranges, returning true if r < other.
func (r *Range) LessThanRange(other *Range) bool {
	// first check if the start is before
	rs := r.GetStart()
	os := other.GetStart()
	if rs == os {
		return r.GetNonce() < other.GetNonce()
	}
	return rs < os
}
