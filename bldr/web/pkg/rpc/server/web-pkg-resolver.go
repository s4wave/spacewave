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
	ref, data, err := r.c.addWebPkgRef(r.key)
	if err != nil {
		return err
	}
	defer r.c.releaseWebPkgRef(ref)

	val, err := r.buildValue(ctx, data)
	if err != nil {
		return err
	}
	if val == nil {
		return nil
	}

	_, accepted := handler.AddValue(val)
	if !accepted {
		return nil
	}

	handler.MarkIdle(true)
	<-ctx.Done()
	handler.ClearValues()
	return context.Canceled
}

var _ directive.Resolver = (*webPkgResolver)(nil)
