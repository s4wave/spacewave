package assembly_controller

import (
	"context"

	"github.com/aperturerobotics/bldr/assembly"
	"github.com/aperturerobotics/controllerbus/directive"
)

// applyAssemblyResolver is an ApplyAssembly resolver.
type applyAssemblyResolver struct {
	// c is the controller
	c *Controller
	// ctx is the directive context
	ctx context.Context
	// di is the directive instance
	di directive.Instance
	// dir is the directive
	dir assembly.ApplyAssembly
}

func newApplyAssemblyResolver(
	c *Controller,
	ctx context.Context,
	di directive.Instance,
	dir assembly.ApplyAssembly,
) *applyAssemblyResolver {
	r := &applyAssemblyResolver{
		c:   c,
		ctx: ctx,
		di:  di,
		dir: dir,
	}
	return r
}

// Resolve resolves the values, emitting them to the handler.
func (r *applyAssemblyResolver) Resolve(
	ctx context.Context,
	handler directive.ResolverHandler,
) error {
	conf := r.dir.GetApplyAssembly()
	if conf == nil {
		return nil
	}

	ref, err := r.c.PushAssembly(ctx, conf)
	if err != nil {
		return err
	}
	defer ref.Release()
	id, ok := handler.AddValue(ref)
	if !ok {
		return nil
	}
	<-ctx.Done()
	handler.RemoveValue(id)
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*applyAssemblyResolver)(nil))
