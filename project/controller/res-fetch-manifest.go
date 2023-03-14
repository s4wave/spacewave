package bldr_project_controller

import (
	"context"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	"github.com/aperturerobotics/controllerbus/directive"
)

// resolveFetchManifest resolves a FetchManifest directive.
func (c *Controller) resolveFetchManifest(
	ctx context.Context,
	di directive.Instance,
	dir bldr_manifest.FetchManifest,
) directive.Resolver {
	manifestMeta := dir.FetchManifestMeta()
	manifestID := manifestMeta.GetManifestId()
	manifestSet := c.c.GetProjectConfig().GetManifests()
	if _, ok := manifestSet[manifestID]; !ok {
		return nil
	}
	return &fetchManifestResolver{c: c, di: di, manifestMeta: manifestMeta}
}

// fetchManifestResolver resolves FetchManifest with the controller.
type fetchManifestResolver struct {
	// c is the controller
	c *Controller
	// di is the directive instance
	di directive.Instance
	// manifestMeta is the manifest meta
	manifestMeta *bldr_manifest.ManifestMeta
}

// Resolve resolves the values, emitting them to the handler.
func (r *fetchManifestResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	// Load the manifest builder.
	manifestB58 := r.manifestMeta.MarshalB58()
	ref, _, _ := r.c.manifestBuilders.AddKeyRef(manifestB58)

	// Release the reference when the directive is disposed.
	r.di.AddDisposeCallback(ref.Release)
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*fetchManifestResolver)(nil))
