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

	conf := c.GetConfig()
	isStart := conf.GetStart()
	if isStart && conf.GetProjectConfig().GetStart().GetDisableBuild() {
		c.le.Infof("not building manifest %s because project.start.disableBuild is set", manifestID)
		return nil
	}

	manifestRemoteID := conf.GetFetchManifestRemote()
	if manifestRemoteID == "" {
		return nil
	}

	manifestSet := conf.GetProjectConfig().GetManifests()
	if _, ok := manifestSet[manifestID]; !ok {
		return nil
	}
	return &fetchManifestResolver{
		c:            c,
		di:           di,
		manifestMeta: manifestMeta,
	}
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
	manifestBuilderRef, remoteRef, err := r.c.AddFetchManifestBuilderRef(ctx, r.manifestMeta)
	if err != nil {
		return err
	}
	defer remoteRef.Release()
	defer manifestBuilderRef.Release()

	conf := r.c.GetConfig()
	watch := conf.GetWatch()
	for {
		_ = handler.ClearValues()
		resultPromiseContainer := manifestBuilderRef.GetResultPromiseContainer()
		currResultPromise, waitChanged := resultPromiseContainer.GetPromise()
		if currResultPromise != nil {
			result, err := currResultPromise.AwaitWithCancelCh(ctx, waitChanged)
			if err != nil {
				if !watch {
					return err
				} else {
					r.c.le.WithError(err).Warn("FetchManifest: manifest builder failed")
				}
			} else if result == nil {
				// waitChanged closed
				continue
			} else {
				// result != nil
				var dirResult bldr_manifest.FetchManifestValue = &bldr_manifest.FetchManifestResponse{
					ManifestRef: result.GetBuilderResult().GetManifestRef().CloneVT(),
				}
				_, _ = handler.AddValue(dirResult)
				if !watch {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return context.Canceled
		case <-waitChanged:
		}
	}
}

// _ is a type assertion
var _ directive.Resolver = ((*fetchManifestResolver)(nil))
