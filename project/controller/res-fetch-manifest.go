//go:build !js

package bldr_project_controller

import (
	"context"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	"github.com/aperturerobotics/controllerbus/directive"
)

// resolveFetchManifest resolves a FetchManifest directive.
func (c *Controller) resolveFetchManifest(di directive.Instance, dir bldr_manifest.FetchManifest) directive.Resolver {
	manifestID := dir.GetManifestId()

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

	// we need to know a platform id
	if len(dir.GetPlatformIds()) == 0 {
		c.le.Debugf("not building manifest %s because list of platform ids is emptyt", manifestID)
		return nil
	}

	return &fetchManifestResolver{
		c:   c,
		di:  di,
		dir: dir,
	}
}

// fetchManifestResolver resolves FetchManifest with the controller.
type fetchManifestResolver struct {
	// c is the controller
	c *Controller
	// di is the directive instance
	di directive.Instance
	//  dir is the directive
	dir bldr_manifest.FetchManifest
}

// Resolve resolves the values, emitting them to the handler.
func (r *fetchManifestResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	manifestMetas := bldr_manifest.NewFetchManifestBuildMatrix(r.dir)

	for _, meta := range manifestMetas {
		rel := handler.AddResolver(&fetchManifestWithMetaResolver{
			c:    r.c,
			di:   r.di,
			dir:  r.dir,
			meta: meta,
		}, nil)
		_ = context.AfterFunc(ctx, rel)
	}
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*fetchManifestResolver)(nil))

// fetchManifestWithMetaResolver resolves FetchManifest with a ManifestMeta.
type fetchManifestWithMetaResolver struct {
	// c is the controller
	c *Controller
	// di is the directive instance
	di directive.Instance
	//  dir is the directive
	dir bldr_manifest.FetchManifest
	// meta is the manifest meta
	meta *bldr_manifest.ManifestMeta
}

// Resolve resolves the values, emitting them to the handler.
func (r *fetchManifestWithMetaResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	manifestBuilderRef, remoteRef, err := r.c.AddFetchManifestBuilderRef(ctx, r.meta)
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
				if result.GetBuilderResult().GetManifest().GetMeta().GetManifestId() == "" {
					// continue to waitChanged
					r.c.le.WithError(err).Warn("FetchManifest: manifest builder returned empty result")
				} else {
					_, _ = handler.AddValue(bldr_manifest.NewFetchManifestValue(
						[]*bldr_manifest.ManifestRef{result.GetBuilderResult().GetManifestRef().CloneVT()},
					))
					if !watch {
						return nil
					}
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
var _ directive.Resolver = ((*fetchManifestWithMetaResolver)(nil))
