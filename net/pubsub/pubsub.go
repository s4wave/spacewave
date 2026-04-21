package pubsub

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
)

// PubSub is an implementation of a pub-sub message router.
// The PubSub controller provides a common implementation for pub-sub routers.
// The PubSub interface declares the requirements for a router.
// The router is constructed with a private key which is used for communications.
// Each subscription also has a private key to identify the publisher/subscriber.
// Publishing is performed by first subscribing and then publishing to the subscription.
type PubSub interface {
	// Execute executes the PubSub routines.
	Execute(ctx context.Context) error
	// AddPeerStream adds a negotiated peer stream.
	// Two streams will be negotiated, one outgoing, one incoming.
	// The pubsub should communicate over the stream.
	AddPeerStream(tpl PeerLinkTuple, initiator bool, mstrm link.MountedStream)
	// AddSubscription adds a channel subscription, returning a subscription handle.
	AddSubscription(ctx context.Context, privKey crypto.PrivKey, channelID string) (Subscription, error)
	// Close closes the pubsub.
	Close()
}

// PubSubHandler manages a PubSub and receives event callbacks.
// This is typically fulfilled by the PubSub controller.
type PubSubHandler any

// PeerLinkTuple is the peer-id link-id tuple.
type PeerLinkTuple struct {
	PeerID peer.ID
	LinkID uint64
}

// NewPeerLinkTuple constructs a new peer link tuple.
func NewPeerLinkTuple(lnk link.MountedLink) PeerLinkTuple {
	return PeerLinkTuple{
		PeerID: lnk.GetRemotePeer(),
		LinkID: lnk.GetLinkUUID(),
	}
}

// Message is a pubsub message.
type Message interface {
	// GetFrom returns the peer ID of the sender.
	GetFrom() peer.ID
	// GetAuthenticated indicates if the signature is valid.
	GetAuthenticated() bool
	// GetData returns the message data.
	GetData() []byte
}

// Controller is a PubSub controller.
type Controller interface {
	// Controller is the controllerbus controller interface.
	controller.Controller

	// GetPubSub returns the controlled PubSub router.
	// This may wait for the PubSub to be ready.
	GetPubSub(ctx context.Context) (PubSub, error)
}

// Subscription is a pubsub channel subscription handle.
type Subscription interface {
	// GetPeerId returns the peer ID for this subscription derived from private key.
	GetPeerId() peer.ID
	// GetChannelId returns the channel id.
	GetChannelId() string
	// Publish writes to the channel using the subscription's private key.
	Publish(data []byte) error
	// AddHandler adds a callback that is called with each received message.
	// The callback should not block.
	// Returns a remove function.
	// The handler(s) are also removed when the subscription is released.
	AddHandler(cb func(m Message)) func()
	// Release releases the subscription handle, clearing the handlers.
	Release()
}
