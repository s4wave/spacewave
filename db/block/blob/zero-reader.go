package blob

// ZeroReader supplies an unbounded zero-filled stream without retaining a buffer.
// Use io.LimitReader to represent a finite implicit range.
type ZeroReader struct{}

// Read fills p with zeros and never reaches EOF.
func (ZeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}
