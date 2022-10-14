package slices

// CheckSlicesContentsEqual checks if the contents of both slices are the same,
// ignoring the order and length of the slices (and ignoring duplicates).
func CheckSlicesContentsEqual[T comparable](s1, s2 []T) bool {
	s1Map := make(map[T]struct{}, len(s1))
	for _, val := range s1 {
		s1Map[val] = struct{}{}
	}
	s2Map := make(map[T]struct{}, len(s2))
	for _, val := range s2 {
		if _, ok := s1Map[val]; !ok {
			return false
		}
		s2Map[val] = struct{}{}
	}
	for _, val := range s1 {
		if _, ok := s2Map[val]; !ok {
			return false
		}
	}
	return true
}
