//go:build !js

package bldr_project_controller

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
)

const manifestBuilderRetainAfterFetch = 30 * time.Second

// resolveFetchManifest resolves a FetchManifest directive.
func (c *Controller) resolveFetchManifest(di directive.Instance, dir bldr_manifest.FetchManifest) directive.Resolver {
	// Read the controller configuration.
	manifestID := dir.GetManifestId()
	conf := c.GetConfig()

	// Skip building when the project start config disables builds.
	isStart := conf.GetStart()
	if isStart && conf.GetProjectConfig().GetStart().GetDisableBuild() {
		c.le.Infof("not building manifest %s because project.start.disableBuild is set", manifestID)
		return nil
	}

	// Skip when no remote is configured for fetching manifests.
	manifestRemoteID := conf.GetFetchManifestRemote()
	if manifestRemoteID == "" {
		return nil
	}

	// Skip when the manifest is not declared in the project config.
	manifestSet := conf.GetProjectConfig().GetManifests()
	if _, ok := manifestSet[manifestID]; !ok {
		return nil
	}

	// Skip when the directive names no platform IDs.
	if len(dir.GetPlatformIds()) == 0 {
		c.le.Debugf("not building manifest %s because list of platform ids is empty", manifestID)
		return nil
	}

	// Otherwise resolve the directive with a meta resolver.
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
var _ directive.Resolver = (*fetchManifestResolver)(nil)

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
	// Add a reference to the manifest builder for this meta.
	le := r.meta.Logger(r.c.le)
	manifestBuilderRef, remoteRef, err := r.c.AddFetchManifestBuilderRef(ctx, r.meta)
	if err != nil {
		return err
	}
	defer remoteRef.Release()
	releaseManifestBuilderRef := true
	defer func() {
		if releaseManifestBuilderRef {
			manifestBuilderRef.Release()
		}
	}()

	// Watch the builder's result promise, re-resolving when it changes.
	conf := r.c.GetConfig()
	watch := conf.GetWatch()
	for {
		_ = handler.ClearValues()
		resultPromiseContainer := manifestBuilderRef.GetResultPromiseContainer()
		currResultPromise, waitChanged := resultPromiseContainer.GetPromise()
		if currResultPromise != nil {
			result, err := currResultPromise.AwaitWithCancelCh(ctx, waitChanged)
			if err != nil {
				return err
			}
			if result == nil {
				// waitChanged closed
				continue
			}
			if result.GetBuilderResult().GetManifest().GetMeta().GetManifestId() == "" {
				// continue to waitChanged
				le.Debug("FetchManifest: manifest builder returned empty result, ignoring")
			} else {
				_, _ = handler.AddValue(bldr_manifest.NewFetchManifestValue(
					[]*bldr_manifest.ManifestRef{result.GetBuilderResult().GetManifestRef().CloneVT()},
				))
				if !watch {
					releaseManifestBuilderRef = false
					manifestBuilderRef.ReleaseAfter(ctx, manifestBuilderRetainAfterFetch)
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
var _ directive.Resolver = (*fetchManifestWithMetaResolver)(nil)
