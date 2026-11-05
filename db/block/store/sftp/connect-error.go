package block_store_sftp

// ConnectError is the error of a call that could not open a connection to
// the server after every dial attempt: the server sent no reply to the call.
type ConnectError struct {
	// Err is the last dial attempt's error.
	Err error
	// slot is the slot that could not connect, or nil.
	slot *slot
}

// Error returns the dial error.
func (e *ConnectError) Error() string { return e.Err.Error() }

// Unwrap returns the dial error.
func (e *ConnectError) Unwrap() error { return e.Err }
