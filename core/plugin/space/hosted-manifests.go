package plugin_space

import (
	"context"

	manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/world"
)

// hostedManifests selects the manifests a Space hosted in a plugin loads its
// unpinned plugins from. A hosted Space sends its loads to the host's
// scheduler, whose FetchManifest never reaches the Space bus, so a plugin
// deployed only to the Space World would never resolve there. The Space
// resolves the latest manifest per platform itself, as processResolvers does
// for its own bus, and names them in the load.
//
// The watch loop goroutine owns it.
type hostedManifests struct {
	// roots is the manifest objects and roots the selection was read from, or
	// nil before the first read.
	roots []*world.ObjectRootRef
	// byPlugin is the latest manifest per platform of each manifest ID with a
	// manifest.
	byPlugin map[string][]*manifest.ManifestRef
}

// refresh rereads the selection when the reachable manifests changed. It keeps
// the previous selection when the World cannot be read.
func (h *hostedManifests) refresh(ctx context.Context, c *Controller, ws world.WorldState) {
	// Skip the walk when no manifest object changed.
	le := c.GetLogger()
	objKeys, roots, err := c.readManifestRoots(ctx, ws)
	if err != nil {
		warnOnErrorUnlessCanceled(ctx, le, err, "failed to list manifests")
		return
	}
	next := &resolverInputs{listed: true, manifests: roots}
	if h.roots != nil && next.equal(&resolverInputs{listed: true, manifests: h.roots}) {
		return
	}

	// Collect every platform's manifests: the host loads its own.
	collected, manifestErrs, err := bldr_manifest_world.CollectManifests(ctx, ws, nil, objKeys...)
	if err != nil {
		warnOnErrorUnlessCanceled(ctx, le, err, "failed to collect manifests")
		return
	}
	for _, merr := range manifestErrs {
		warnOnErrorUnlessCanceled(ctx, le, merr, "ignoring invalid manifest")
	}

	// Select the latest revision per platform of each manifest ID.
	byPlugin := make(map[string][]*manifest.ManifestRef, len(collected))
	for pid, manifests := range collected {
		var refs []*manifest.ManifestRef
		for _, m := range bldr_manifest_world.FilterCollectedManifestsByLatestRev(manifests) {
			ref, err := space_world.NewSpaceManifestRef(ctx, ws, m.Manifest.GetMeta(), m.ManifestRef)
			if err != nil {
				warnOnErrorUnlessCanceled(ctx, le, err, "failed to resolve plugin manifest")
				continue
			}
			refs = append(refs, ref)
		}
		if len(refs) != 0 {
			byPlugin[pid] = refs
		}
	}
	h.roots, h.byPlugin = roots, byPlugin
}
