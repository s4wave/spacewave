package tptaddr_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/tptaddr"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
)

// establishLinkResolver resolves establishLink directives
type establishLinkResolver struct {
	c   *Controller
	ctx context.Context
	di  directive.Instance
	dir link.EstablishLinkWithPeer
}

// resolveEstablishLinkWithPeer resolves a EstablishLinkWithPeer directive.
func (c *Controller) resolveEstablishLinkWithPeer(
	ctx context.Context,
	di directive.Instance,
	dir link.EstablishLinkWithPeer,
) ([]directive.Resolver, error) {
	return directive.R(&establishLinkResolver{
		c:   c,
		ctx: ctx,
		di:  di,
		dir: dir,
	}, nil)
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (o *establishLinkResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	// Create LookupTptAddr directive.
	// When a new address is added, add a directive to dial that address.
	di, ref, err := bus.ExecWatchTransformEffect(
		ctx,
		func(ctx context.Context, val directive.TypedAttachedValue[tptaddr.LookupTptAddrValue]) (tptaddr.DialTptAddr, bool, error) {
			return tptaddr.NewDialTptAddr(&dialer.DialerOpts{
				Address: val.GetValue(),
				Backoff: o.c.conf.GetDialBackoff(),
			}, o.dir.EstablishLinkSourcePeerId(), o.dir.EstablishLinkTargetPeerId()), true, nil
		},
		func(val directive.TransformedAttachedValue[tptaddr.LookupTptAddrValue, tptaddr.DialTptAddr]) func() {
			// spawn a new resolver for this
			// note: we won't return any values to the directive. we expect the link controller to do this.
			return handler.AddResolver(directive.NewTransformResolver(
				o.c.bus,
				val.GetTransformedValue(),
				func(ctx context.Context, val directive.AttachedValue) (rval struct{}, rel func(), ok bool, err error) {
					return struct{}{}, nil, false, nil
				},
			), nil)
		},
		o.c.bus,
		tptaddr.NewLookupTptAddr(o.dir.EstablishLinkTargetPeerId()),
	)
	if err != nil {
		return err
	}
	defer ref.Release()

	// mark idle based on the lookup tpt addr value directive
	// handle any non-nil resolver errors if isIdle
	errCh := make(chan error, 1)
	handleErr := func(err error) {
		select {
		case errCh <- err:
		default:
		}
	}
	defer di.AddIdleCallback(func(isIdle bool, resolverErrs []error) {
		if isIdle {
			for _, err := range resolverErrs {
				if err != nil {
					handleErr(err)
					return
				}
			}
		}
		handler.MarkIdle(isIdle)
	})()
	defer di.AddDisposeCallback(func() {
		handleErr(directive.ErrDirectiveDisposed)
	})()

	select {
	case <-ctx.Done():
		return context.Canceled
	case err := <-errCh:
		return err
	}
}

// _ is a type assertion
var _ directive.Resolver = ((*establishLinkResolver)(nil))
