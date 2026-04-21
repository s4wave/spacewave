package pubsub_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/backoff"
	"github.com/s4wave/spacewave/net/pubsub"
)

// resolveBuildChannelSub is the BuildChannelSubscription resolver.
type resolveBuildChannelSub struct {
	ctx context.Context
	c   *Controller
	di  directive.Instance
	d   pubsub.BuildChannelSubscription
}

// emittedSubscription is a wrapped subscription with proxied calls
type emittedSubscription struct {
	pubsub.Subscription
	relFunc func()
}

// Release releases the subscription handle, clearing the handlers.
func (e *emittedSubscription) Release() {
	if e.relFunc != nil {
		e.relFunc()
	}
}

// resolveBuildChannelSub resolves building a channel subscription.
func (c *Controller) resolveBuildChannelSub(
	ctx context.Context,
	di directive.Instance,
	d pubsub.BuildChannelSubscription,
) ([]directive.Resolver, error) {
	// accept directive always
	backoffConf := &backoff.Backoff{
		BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
		Exponential: &backoff.Exponential{
			MaxInterval:         5000,
			RandomizationFactor: 0.1,
		},
	}
	return directive.R(directive.NewRetryResolver(c.le, &resolveBuildChannelSub{
		ctx: ctx,
		c:   c,
		di:  di,
		d:   d,
	}, backoffConf.Construct()), nil)
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (r *resolveBuildChannelSub) Resolve(
	ctx context.Context,
	handler directive.ResolverHandler,
) error {
	handler.ClearValues()
	ps, err := r.c.GetPubSub(ctx)
	if err != nil {
		return err
	}

	le := r.c.le.WithField("channel-id", r.d.BuildChannelSubscriptionChannelID())
	le.Debug("adding subscription to channel")
	sub, err := ps.AddSubscription(r.ctx, r.d.BuildChannelSubscriptionPrivKey(), r.d.BuildChannelSubscriptionChannelID())
	if err != nil {
		le.WithError(err).Warn("unable to add subscription to channel")
		return err
	}
	le.Info("successfully added subscription to channel")

	var valueID uint32
	var accepted bool
	var val pubsub.BuildChannelSubscriptionValue = &emittedSubscription{
		Subscription: sub,
		relFunc: func() {
			_, _ = handler.RemoveValue(valueID)
			sub.Release()
		},
	}
	valueID, accepted = handler.AddValue(val)
	if !accepted {
		val.Release()
		return nil
	}
	r.di.AddDisposeCallback(func() {
		val.Release()
	})
	return nil
}

// _ is a type assertion
var _ pubsub.Subscription = ((*emittedSubscription)(nil))

// _ is a type assertion
var _ directive.Resolver = ((*resolveBuildChannelSub)(nil))
