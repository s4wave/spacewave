package common

// SetReadTurns gives s's snapshot reads n turns.
func SetReadTurns[T SQLiteDriverConfig](s *Store[T], n int) {
	s.readTurns = make(readTurns, n)
}

// HoldReadTurns takes every read turn of s until the returned func runs.
func HoldReadTurns[T SQLiteDriverConfig](s *Store[T]) func() {
	n := cap(s.readTurns)
	for range n {
		s.readTurns <- struct{}{}
	}
	return func() {
		for range n {
			<-s.readTurns
		}
	}
}
