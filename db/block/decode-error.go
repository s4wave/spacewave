package block

// DecodeError reports a block whose bytes were read and checked against their
// ref but do not decode, or do not validate, as the type asked for. The bytes
// are named by the ref, so every reader of the same ref and type fails the same
// way: it is an answer about the block, not a failed read, which returns its
// own error.
type DecodeError struct {
	// Err is the decoder's or the validator's error.
	Err error
}

// Error returns the decoder's error.
func (e *DecodeError) Error() string { return e.Err.Error() }

// Unwrap returns the decoder's error.
func (e *DecodeError) Unwrap() error { return e.Err }
