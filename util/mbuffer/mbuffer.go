package mbuffer

// MBuffer is a bytes slice that is allocated on-demand.
// The zero value is valid.
type MBuffer struct {
	buf []byte
}

// GetOrAllocate gets or allocates a buffer with given size.
func (b *MBuffer) GetOrAllocate(size int) []byte {
	buf := b.buf
	if cap(buf) < size {
		buf = make([]byte, size)
	} else {
		buf = buf[:size]
	}
	b.buf = buf
	return buf
}
