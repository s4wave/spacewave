package s4db

// valueWrite is a large value awaiting its write.
type valueWrite struct {
	// op indexes the record op holding the value's reference.
	op int
	// data is the value.
	data []byte
}
