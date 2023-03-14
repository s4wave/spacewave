package bldr_manifest_builder_controller

import (
	"context"

	manifest "github.com/aperturerobotics/bldr/manifest"
	"github.com/aperturerobotics/controllerbus/directive"
)

// resolveFetchManifest resolves a FetchManifest directive.
func (c *Controller) resolveFetchManifest(
	ctx context.Context,
	di directive.Instance,
	dir manifest.FetchManifest,
) directive.Resolver {
	manifestID := dir.FetchManifestMeta().GetManifestId()
	if c.c.GetBuilderConfig().GetManifestMeta().GetManifestId() != manifestID {
		return nil
	}
	return &fetchManifestResolver{c: c, di: di}
}

// fetchManifestResolver resolves FetchManifest with the controller.
type fetchManifestResolver struct {
	// c is the controller
	c *Controller
	// di is the directive instance
	di directive.Instance
}

// Resolve resolves the values, emitting them to the handler.
func (r *fetchManifestResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	_ = handler.ClearValues()
	res, err := r.c.GetResultPromise().Await(ctx)
	if err != nil {
		return err
	}
	var value manifest.FetchManifestValue = &manifest.FetchManifestResponse{
		ManifestRef: res.ManifestRef,
	}
	_, _ = handler.AddValue(value)
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*fetchManifestResolver)(nil))
