package transport_quic

import (
	"context"
	"errors"
	"io"

	"github.com/quic-go/quic-go"
)

var (
	// ErrDialUnimplemented is returned if dialing the peer is unimplemented.
	ErrDialUnimplemented = errors.New("dial peer not implemented")
	// ErrRemoteUnspecified is returned if the remote addr is unspecified.
	ErrRemoteUnspecified = errors.New("peer id and/or remote addr must be specified")
)

// isCleanAcceptClose returns true if the error is an expected accept-loop
// close: a zero-code application error, cancellation, or stream EOF.
func isCleanAcceptClose(err error) bool {
	var qe *quic.ApplicationError
	if errors.As(err, &qe) {
		return qe != nil && qe.ErrorCode == 0
	}

	return errors.Is(err, context.Canceled) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrClosedPipe)
}
