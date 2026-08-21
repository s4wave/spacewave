package signaling_rpc

import "errors"

var (
	// ErrUserpedSession is returned when a session is opened twice for one peer.
	ErrUserpedSession = errors.New("signaling: session can only be called once per peer")
	// ErrUserpedListen is returned when a listen stream is opened twice for one peer.
	ErrUserpedListen = errors.New("signaling: listen can only be called once per peer")
	// ErrUnexpectedSessionMsg is returned on a message that does not fit the session state.
	ErrUnexpectedSessionMsg = errors.New("signaling: unexpected session message")
)
