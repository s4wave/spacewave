package file

// LessThanRange compares to ranges, returning true if r < other.
func (r *Range) LessThanRange(other *Range) bool {
	return r.GetStart() < other.GetStart() ||
		r.GetNonce() < other.GetNonce()
}
