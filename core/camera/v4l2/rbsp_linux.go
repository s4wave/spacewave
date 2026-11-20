//go:build linux

package v4l2

// rbsp reads H.264 syntax fields from one NAL payload.
type rbsp struct {
	// data holds the unread bytes, including emulation prevention bytes.
	data []byte
	// current holds the current byte.
	current byte
	// bits counts the unread bits in current.
	bits uint8
	// zeros counts consecutive zero bytes in the encoded payload.
	zeros int
	// invalid records a truncated or out-of-range field.
	invalid bool
}

// readBit reads the next RBSP bit, skipping emulation prevention bytes.
func (r *rbsp) readBit() uint32 {
	// Advance to the next byte after discarding an emulation prevention byte.
	if r.bits == 0 {
		if r.zeros == 2 && len(r.data) > 0 && r.data[0] == 3 {
			r.data = r.data[1:]
			r.zeros = 0
		}
		if len(r.data) == 0 {
			r.invalid = true
			return 0
		}
		r.current, r.data = r.data[0], r.data[1:]
		r.bits = 8
		if r.current == 0 {
			r.zeros++
		}
		if r.current != 0 {
			r.zeros = 0
		}
	}

	// H.264 packs fields most significant bit first.
	r.bits--
	return uint32(r.current>>r.bits) & 1
}

// readUE reads an unsigned Exp-Golomb field.
func (r *rbsp) readUE() uint32 {
	// Count prefix bits before the first one.
	leading := 0
	for r.readBit() == 0 {
		if r.invalid || leading == 31 {
			r.invalid = true
			return 0
		}
		leading++
	}

	// Read the suffix as the low bits of codeNum plus one.
	value := uint32(1)
	for range leading {
		value = value<<1 | r.readBit()
	}
	return value - 1
}
