package manifest_fetch_rpc

import (
	"context"

	manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/aperturerobotics/controllerbus/directive"
)

// resolveFetchManifest resolves a FetchManifest directive.
func (c *Controller) resolveFetchManifest(
	ctx context.Context,
	di directive.Instance,
	dir manifest.FetchManifest,
) (directive.Resolver, error) {
	if c.fetchManifestIdRe != nil && dir.GetManifestId() != "" {
		if !c.fetchManifestIdRe.MatchString(dir.GetManifestId()) {
			return nil, nil
		}
	}

	return &fetchManifestResolver{c: c, req: manifest.NewFetchManifestRequest(dir)}, nil
}

// fetchManifestResolver resolves FetchManifest with the controller.
type fetchManifestResolver struct {
	// c is the controller
	c *Controller
	// req is the request
	req *manifest.FetchManifestRequest
}

// Resolve resolves the values, emitting them to the handler.
func (r *fetchManifestResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	handler.ClearValues()
	return r.c.FetchManifest(ctx, r.req.ToDirective(), handler, false)
}

// _ is a type assertion
var _ directive.Resolver = ((*fetchManifestResolver)(nil))
