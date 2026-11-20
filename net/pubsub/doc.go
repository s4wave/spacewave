// Package pubsub routes messages on named channels between peers.
//
// A PubSub router runs over the links of its controller: for each linked peer
// it is handed two negotiated streams, one outgoing and one incoming. A
// Subscription joins one channel under its own private key, and a message is
// published by subscribing first and calling Publish, which signs the message
// with the subscription's key; a received Message reports its sender and
// whether its signature verified. Handlers on a subscription must not block.
//
// The BuildChannelSubscription directive subscribes to a channel with a given
// private key through the bus; its value can be removed and replaced, so a
// holder watches it rather than keeping one handle. Package floodsub is the
// router in use: under the protocol ID "bifrost/floodsub" it forwards each
// message to every peer on the channel except the hops it came from, drops a
// message ID it saw in the last two minutes, and signs with SHA-256 unless its
// config names another hash.
package pubsub
