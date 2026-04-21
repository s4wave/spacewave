package store_kvtx_redis

// IsBasicRune checks if the rune is 1-byte long and within A-Z, a-z, 0-9.
//
// Used for escaping strings for Redis.
func IsBasicRune(c byte) bool {
	switch {
	case c >= 48 && c <= 57:
		fallthrough
		// A-Z
	case c >= 65 && c <= 89:
		fallthrough
		// a-z
	case c >= 97 && c <= 122:
		// no escape
		return true
	default:
		return false
	}
}
