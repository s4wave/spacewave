package web_pkg_rpc_server

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
)

type webPkgResolver struct {
	c          *Controller
	key        string
	buildValue func(context.Context, *webPkgTracker) (directive.Value, error)
}

func (r *webPkgResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	// Retain the web package tracker for the resolver lifetime.
	ref, data, err := r.c.addWebPkgRef(r.key)
	if err != nil {
		return err
	}
	defer r.c.releaseWebPkgRef(ref)

	// Build the directive value from the resolved package tracker.
	val, err := r.buildValue(ctx, data)
	if err != nil {
		return err
	}
	if val == nil {
		return nil
	}

	// Publish the web package value if the directive accepts it.
	_, accepted := handler.AddValue(val)
	if !accepted {
		return nil
	}

	// Retain the published web package value until the resolver is canceled.
	handler.MarkIdle(true)
	<-ctx.Done()
	handler.ClearValues()
	return context.Canceled
}

var _ directive.Resolver = (*webPkgResolver)(nil)
