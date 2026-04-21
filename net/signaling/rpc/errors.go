package signaling_rpc

import "errors"

var (
	ErrUserpedSession       = errors.New("signaling: session can only be called once per peer")
	ErrUserpedListen        = errors.New("signaling: listen can only be called once per peer")
	ErrUnexpectedSessionMsg = errors.New("signaling: unexpected session message")
)
