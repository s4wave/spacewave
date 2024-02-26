package manifest_fetch_plugin

import (
	"context"

	manifest "github.com/aperturerobotics/bldr/manifest"
	plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
)

// resolveFetchManifest resolves a FetchManifest directive.
func (c *Controller) resolveFetchManifest(
	ctx context.Context,
	di directive.Instance,
	dir manifest.FetchManifest,
) (directive.Resolver, error) {
	manifestMeta := dir.FetchManifestMeta()
	if c.fetchManifestIdRe != nil && manifestMeta.GetManifestId() != "" {
		if !c.fetchManifestIdRe.MatchString(manifestMeta.GetManifestId()) {
			return nil, nil
		}
	}
	return &fetchManifestResolver{c: c, manifestMeta: manifestMeta}, nil
}

// fetchManifestResolver resolves FetchManifest with the controller.
type fetchManifestResolver struct {
	// c is the controller
	c *Controller
	// manifestMeta is the manifest metadata
	manifestMeta *manifest.ManifestMeta
}

// Resolve resolves the values, emitting them to the handler.
func (r *fetchManifestResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	err := plugin.ExPluginLoadAccessClient(
		ctx,
		r.c.bus,
		r.c.conf.GetPluginId(),
		func(ctx context.Context, client srpc.Client) error {
			_ = handler.ClearValues()

			r.c.le.Debugf("fetching manifest %s via plugin %s", r.manifestMeta.GetManifestId(), r.c.conf.GetPluginId())
			fetchClient := manifest.NewSRPCManifestFetchClient(client)
			return manifest.FetchManifestViaRpc(
				ctx,
				manifest.NewFetchManifest(r.manifestMeta),
				fetchClient.FetchManifest,
				handler,
				false,
			)
		},
	)
	if err != nil && err != context.Canceled {
		r.c.le.
			WithError(err).
			WithField("via-plugin-id", r.c.conf.GetPluginId()).
			WithField("manifest-id", r.manifestMeta.GetManifestId()).
			Warn("failed to fetch manifest")
	}
	return err
}

// _ is a type assertion
var _ directive.Resolver = ((*fetchManifestResolver)(nil))
