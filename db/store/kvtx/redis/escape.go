package store_kvtx_redis

// escapeKey escapes the key for matching.
func escapeKey(key []byte, extraCap int) []byte {
	// Return empty keys unchanged.
	if len(key) == 0 {
		return key
	}

	// Check the key for any characters requiring escapes, building the escaped
	// buffer on the slow path.
	var esc []byte
	for i := range key {
		// anything outside of basic chars should be escaped
		c := key[i]
		if !IsBasicRune(c) {
			// escape
			if cap(esc) == 0 {
				esc = make([]byte, 0, (len(key)-i)+len(key)+extraCap)
				esc = append(esc, key[:i]...)
			}
			esc = append(esc, '\\')
		}
		if cap(esc) != 0 { // slow path: escape
			esc = append(esc, c)
		}
	}
	if cap(esc) != 0 { // escaped
		return esc
	}
	d := make([]byte, len(key), len(key)+extraCap) // fast case
	copy(d, key)
	return d
}
