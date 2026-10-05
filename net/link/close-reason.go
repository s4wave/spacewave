package link

// ReasonCloser is a Link that can tell the remote peer why it closed.
type ReasonCloser interface {
	// CloseWithReason closes the link like Close and sends reason to the
	// remote peer.
	CloseWithReason(reason string) error
}

// CloseWithReason closes lnk and sends reason to the remote peer when the link
// can carry it. Other links close without one.
func CloseWithReason(lnk Link, reason string) error {
	if rc, ok := lnk.(ReasonCloser); ok {
		return rc.CloseWithReason(reason)
	}
	return lnk.Close()
}
