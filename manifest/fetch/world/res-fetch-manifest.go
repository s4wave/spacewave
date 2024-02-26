package manifest_fetch_world

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
) (directive.Resolver, error) {
	manifestMeta := dir.FetchManifestMeta()
	if c.fetchManifestIdRe != nil && manifestMeta.GetManifestId() != "" {
		if !c.fetchManifestIdRe.MatchString(manifestMeta.GetManifestId()) {
			return nil, nil
		}
	}

	if c.conf.GetDisableWatch() {
		return &fetchManifestResolver{c: c, manifestMeta: manifestMeta}, nil
	}

	return &fetchManifestWatchResolver{c: c, manifestMeta: manifestMeta}, nil
}

// fetchManifestResolver resolves FetchManifest once with the controller.
type fetchManifestResolver struct {
	// c is the controller
	c *Controller
	// manifestMeta is the manifest metadata
	manifestMeta *manifest.ManifestMeta
}

// Resolve resolves the values, emitting them to the handler.
func (r *fetchManifestResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	_ = handler.ClearValues()
	res, err := r.c.FetchManifest(ctx, r.manifestMeta, false)
	if err == nil && res == nil {
		le := r.manifestMeta.Logger(r.c.le)
		le.Debug("manifest not found in world")
		return nil
	}
	if err == nil {
		err = res.Validate()
	}
	if err != nil {
		if err != context.Canceled {
			le := r.manifestMeta.Logger(r.c.le)
			le.
				WithError(err).
				WithField("manifest-id", r.manifestMeta.GetManifestId()).
				Warn("failed to fetch manifest")
		}
		return err
	}

	res.ManifestRef.Meta.Logger(r.c.le).Debug("fetched manifest")
	var val *manifest.FetchManifestValue = res
	_, _ = handler.AddValue(val)
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*fetchManifestResolver)(nil))
