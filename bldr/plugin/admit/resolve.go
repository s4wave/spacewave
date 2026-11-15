package bldr_plugin_admit

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/backoff"
	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// admitResolver holds an approved value while the owner's stream says the
// root is admitted.
//
// States: asking (no value, not idle) until the owner answers; admitted (a
// value, idle) or refused (no value, idle) while its stream answers; and back to
// asking, with the value removed, whenever the owner exits or its stream
// breaks, until it answers again. A plugin the value admitted keeps running
// when the value goes: stopping it is not this resolver's decision.
type admitResolver struct {
	// c is the controller
	c *Controller
	// dir is the directive
	dir bldr_plugin.AdmitPluginRoot
}

// Resolve watches the root through the owner until ctx ends. It returns an
// error when the owner's stream breaks, so the retry resolver asks again.
func (r *admitResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	req := &WatchPluginRootAdmittedRequest{
		PeerId:       r.dir.AdmitPluginRootPeerID(),
		PluginId:     r.dir.AdmitPluginRootPluginID(),
		ManifestRoot: r.dir.AdmitPluginRootManifestRoot(),
	}
	return bldr_plugin.ExPluginLoadAccessClient(ctx, r.c.bus, r.c.conf.GetPluginId(), func(ctx context.Context, client srpc.Client) error {
		return watchOwner(ctx, handler, NewSRPCPluginRootAdmissionClient(client), req)
	})
}

// watchOwner applies one owner stream's answers to handler. When the stream
// ends for any reason it removes the value and marks the resolver busy, so
// readers see the root as unanswered rather than a stale answer.
func watchOwner(
	ctx context.Context,
	handler directive.ResolverHandler,
	owner SRPCPluginRootAdmissionClient,
	req *WatchPluginRootAdmittedRequest,
) error {
	// On exit, withdraw the answer and mark the root unanswered.
	var valID uint32
	defer func() {
		if valID != 0 {
			_, _ = handler.RemoveValue(valID)
		}
		handler.MarkIdle(false)
	}()

	// Apply each answer from the owner's stream.
	strm, err := owner.WatchPluginRootAdmitted(ctx, req)
	if err != nil {
		return err
	}
	defer strm.Close()
	for {
		resp, err := strm.Recv()
		if err != nil {
			if ctx.Err() != nil {
				return context.Canceled
			}
			return errors.Wrap(err, "watch plugin root admission")
		}
		switch admitted := resp.GetAdmitted(); {
		case admitted && valID == 0:
			if id, accepted := handler.AddValue(bldr_plugin.AdmitSourceApproved); accepted {
				valID = id
			}
		case !admitted && valID != 0:
			_, _ = handler.RemoveValue(valID)
			valID = 0
		}
		handler.MarkIdle(true)
	}
}

// newBackoff returns the backoff between asks after the owner's stream breaks.
func newBackoff() cbackoff.BackOff {
	return (&backoff.Backoff{
		BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
		Exponential: &backoff.Exponential{
			InitialInterval: 100,
			MaxInterval:     2000,
		},
	}).Construct()
}

// _ is a type assertion
var _ directive.Resolver = (*admitResolver)(nil)
