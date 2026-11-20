// Package transport defines how a peer reaches other peers.
//
// A Transport owns one way of carrying packets, such as UDP, WebSocket,
// WebRTC or an in-process network, and yields authenticated Links to its
// TransportHandler: HandleLinkEstablished when a link is up and HandleLinkLost
// once when it ends. Its UUID is unique on the host and is part of every link
// it produces. The handler is usually the transport controller
// (transport/controller), which mounts each link, negotiates the protocol of
// each stream, and admits or refuses a remote peer through its PeerAuthorizer
// before any stream on the link reaches a handler.
//
// Every transport ends in a QUIC link: common/pconn runs QUIC over a packet
// connection (udp, inproc), common/conn over a stream connection (websocket),
// and webrtc over a data channel. The TLS handshake binds the link to both
// peers' keys, so a Link's remote peer ID is authenticated by construction.
// LookupTransport finds a running transport by peer or transport ID.
package transport
