package blockenc

// Method is a block encryption method.
// do not write to src
type Method interface {
	// Encrypt encrypts the block and returns the encrypted buf.
	Encrypt(alloc AllocFn, src []byte) ([]byte, error)
	// Decrypt decrypts the whole block and returns the decrypted buf.
	Decrypt(alloc AllocFn, src []byte) ([]byte, error)
}
