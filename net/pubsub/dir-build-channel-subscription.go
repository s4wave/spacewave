package pubsub

import (
	"context"
	"errors"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// BuildChannelSubscription is a directive to subscribe to a channel.
type BuildChannelSubscription interface {
	// Directive indicates BuildChannelSubscription is a directive.
	directive.Directive

	// BuildChannelSubscriptionChannelID returns the channel ID constraint.
	// Cannot be empty.
	BuildChannelSubscriptionChannelID() string
	// BuildChannelSubscriptionPrivKey returns the private key to use to subscribe.
	// Cannot be empty.
	BuildChannelSubscriptionPrivKey() crypto.PrivKey
}

// BuildChannelSubscriptionValue is the result type for BuildChannelSubscription.
// The value is removed and replaced when necessary.
type BuildChannelSubscriptionValue = Subscription

// buildChannelSubscription implements BuildChannelSubscription
type buildChannelSubscription struct {
	channelID string
	privKey   crypto.PrivKey
}

// NewBuildChannelSubscription constructs a new BuildChannelSubscription directive.
func NewBuildChannelSubscription(channelID string, privKey crypto.PrivKey) BuildChannelSubscription {
	return &buildChannelSubscription{channelID: channelID, privKey: privKey}
}

// ExBuildChannelSubscription executes the BuildChannelSubscription directive.
// Waits for the channel subscription to be built.
// If values are returned, returns vals, valsRef, nil
// Otherwise returns nil, nil, err
func ExBuildChannelSubscription(
	ctx context.Context,
	b bus.Bus,
	returnIfIdle bool,
	channelID string,
	privKey crypto.PrivKey,
	valDisposeCallback func(),
) (BuildChannelSubscriptionValue, directive.Instance, directive.Reference, error) {
	return bus.ExecWaitValue[BuildChannelSubscriptionValue](
		ctx,
		b,
		NewBuildChannelSubscription(channelID, privKey),
		bus.ReturnIfIdle(returnIfIdle),
		valDisposeCallback,
		nil,
	)
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *buildChannelSubscription) Validate() error {
	if d.channelID == "" {
		return errors.New("channel id cannot be empty")
	}
	if d.privKey == nil {
		return errors.New("priv key cannot be empty")
	}

	return nil
}

// GetValueBuildChannelSubscriptionOptions returns options relating to value handling.
func (d *buildChannelSubscription) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		// UnrefDisposeDur is the duration to wait to dispose a directive after all
		// references have been released.
		UnrefDisposeDur:            time.Second,
		UnrefDisposeEmptyImmediate: true,
	}
}

// BuildChannelSubscriptionChannelID returns the channel ID constraint.
func (d *buildChannelSubscription) BuildChannelSubscriptionChannelID() string {
	return d.channelID
}

// BuildChannelSubscriptionPrivKey returns the private key.
func (d *buildChannelSubscription) BuildChannelSubscriptionPrivKey() crypto.PrivKey {
	return d.privKey
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *buildChannelSubscription) IsEquivalent(other directive.Directive) bool {
	// Note: for ChannelSubscription, we want a unique handle for each directive.
	return false

	/* If we wanted to de-duplicate (can't due to Release()):
	od, ok := other.(BuildChannelSubscription)
	if !ok {
		return false
	}

	return d.BuildChannelSubscriptionChannelID() == od.BuildChannelSubscriptionChannelID() &&
		od.BuildChannelSubscriptionPrivKey().GetPublic().Equals(d.BuildChannelSubscriptionPrivKey().GetPublic())
	*/
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *buildChannelSubscription) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *buildChannelSubscription) GetName() string {
	return "BuildChannelSubscription"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *buildChannelSubscription) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	vals["channel-id"] = []string{d.BuildChannelSubscriptionChannelID()}
	pkey := d.BuildChannelSubscriptionPrivKey()
	if pkey != nil {
		pid, _ := peer.IDFromPrivateKey(pkey)
		if len(pid) != 0 {
			vals["peer-id"] = []string{pid.String()}
		}
	}
	return vals
}

// _ is a type assertion
var _ BuildChannelSubscription = ((*buildChannelSubscription)(nil))
