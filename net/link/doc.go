// Package link is the authenticated connection between two peers and the
// directives that open and serve streams over it.
//
// A Link is one encrypted connection from a transport. Its UUID distinguishes
// separate connections to the same peer, and its remote transport UUID is
// learned in the handshake. Close is idempotent, unblocks pending I/O and
// reports the loss to the transport handler exactly once. A MountedLink is a
// Link after the transport controller has admitted it. OpenMountedStream opens
// a stream for one protocol.ID on it, and the controller writes the stream's
// protocol header before the caller writes data.
//
// EstablishLinkWithPeer asks the bus for a MountedLink to a target peer, from
// any local peer when the source is empty. The link stays open while the
// directive is referenced and for a hold-open interval after the last
// reference is released, so a burst of requests reuses one link.
// OpenStreamWithPeerEx establishes the link and opens a stream in one call.
// HandleMountedStream is the inbound side: the controller emits it for each
// accepted stream with the stream's protocol and peers, and the resolving
// MountedStreamHandler owns the stream from then on.
package link
