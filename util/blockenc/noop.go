package blockenc

// noop is a no-op method.
type noop struct{}

// NewNoop constructs a new no-op method.
func NewNoop() Method {
	return &noop{}
}

// Encrypt encrypts the block and returns the encrypted buf.
func (n *noop) Encrypt(alloc AllocFn, src []byte) ([]byte, error) {
	out := alloc(len(src))
	copy(out, src)
	return out, nil
}

// Decrypt decrypts the whole block and returns the decrypted buf.
func (n *noop) Decrypt(alloc AllocFn, src []byte) ([]byte, error) {
	out := alloc(len(src))
	copy(out, src)
	return out, nil
}

// _ is a type assertion
var _ Method = ((*noop)(nil))
