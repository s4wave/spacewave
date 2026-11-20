package frametrack

import (
	"io"

	"github.com/pkg/errors"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// maxSubscriptionMessageSize bounds the control stream's subscription packet.
const maxSubscriptionMessageSize = 4096

// ReadSubscription reads the subscription from a control stream.
func ReadSubscription(control io.ReadWriteCloser) (*Subscription, error) {
	// Read and validate the viewer's subscription identifier.
	session := stream_packet.NewSession(control, maxSubscriptionMessageSize)
	subscription := new(Subscription)
	if err := session.RecvMsg(subscription); err != nil {
		return nil, err
	}
	if subscription.GetSubscriptionId() == "" {
		return nil, errors.New("frame subscription ID is empty")
	}
	return subscription, nil
}

// WriteSubscription sends a subscription on a control stream.
func WriteSubscription(control io.ReadWriteCloser, subscription *Subscription) error {
	// Reject a subscription the receiver cannot accept.
	if subscription.GetSubscriptionId() == "" || subscription.SizeVT() > maxSubscriptionMessageSize {
		return errors.New("frame subscription ID is empty or too large")
	}

	// Send the subscription using the same packet framing as group messages.
	return stream_packet.NewSession(control, maxSubscriptionMessageSize).SendMsg(subscription)
}
