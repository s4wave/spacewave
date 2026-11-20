// Package signaling carries small messages between two peers that have no link
// yet, so they can set one up, as WebRTC does with its offer and answer.
//
// The SignalPeer directive opens a SignalPeerSession with a remote peer over a
// signaling channel, optionally a named one. Send blocks until the remote peer
// acknowledges the message or the context ends, and Recv acknowledges each
// message it returns. GetPresence reports a presence epoch that is zero while
// the remote peer is absent and takes a new value each time it joins, so a
// caller can tell a rejoin from a peer that stayed. HandleSignalPeer is the
// inbound side: it hands a session that a remote peer opened to the
// controller that serves its signaling ID.
package signaling
