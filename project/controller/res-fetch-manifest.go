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
	manifestRemoteID := c.c.GetFetchManifestRemote()
	if manifestRemoteID == "" {
		return nil
	}

	manifestBundleObjKey := c.c.GetFetchManifestObjectKey()
	if manifestBundleObjKey == "" {
		return nil
	}

	manifestMeta := dir.FetchManifestMeta()
	manifestID := manifestMeta.GetManifestId()
	manifestSet := c.c.GetProjectConfig().GetManifests()
	if _, ok := manifestSet[manifestID]; !ok {
		return nil
	}
	return &fetchManifestResolver{
		c:                    c,
		di:                   di,
		manifestMeta:         manifestMeta,
		manifestRemoteID:     manifestRemoteID,
		manifestBundleObjKey: manifestBundleObjKey,
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
	// manifestRemoteID is the ID to use of the manifest remote
	manifestRemoteID string
	// manifestBundleObjKey is the object key to write the bundle to
	manifestBundleObjKey string
}

// Resolve resolves the values, emitting them to the handler.
func (r *fetchManifestResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	_, _, err := r.c.BuildManifestBundle(
		ctx,
		r.manifestRemoteID,
		r.manifestBundleObjKey,
		[]*ManifestBuilderConfig{{
			ManifestId: r.manifestMeta.GetManifestId(),
			BuildType:  r.manifestMeta.GetBuildType(),
			PlatformId: r.manifestMeta.GetPlatformId(),
		}},
	)
	if err != nil {
		return err
	}

	// Release the references when the directive is disposed.
	// r.di.AddDisposeCallback(ref.Release)
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*fetchManifestResolver)(nil))
