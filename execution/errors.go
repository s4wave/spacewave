package forge_execution

import "errors"

var (
	// ErrUnexpectedPeerID is returned if the peer id was incorrect.
	ErrUnexpectedPeerID = errors.New("unexpected execution peer id")
	// ErrUnknownState is returned if the state was unknown/unhandled.
	ErrUnknownState = errors.New("unexpected or unhandled execution state")
)
