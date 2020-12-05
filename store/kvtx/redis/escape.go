package store_kvtx_redis

import runes "github.com/aperturerobotics/hydra/util/runes"

// escapeKey escapes the key for matching.
func escapeKey(key []byte, extraCap int) []byte {
	if len(key) == 0 {
		return key
	}

	// escaped
	var esc []byte

	// check for any necessary escapes
	for i := 0; i < len(key); i++ {
		// anything outside of basic chars should be escaped
		c := key[i]
		if !runes.IsBasicRune(c) {
			// escape
			if cap(esc) == 0 {
				esc = make([]byte, 0, (len(key)-i)+(len(key))+extraCap)
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
