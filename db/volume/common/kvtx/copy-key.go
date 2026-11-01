package kvtx

// CopyKey is a key and value a copy writes beside a block it keeps.
type CopyKey struct {
	// Key is the key.
	Key []byte
	// Value is the value.
	Value []byte
}
