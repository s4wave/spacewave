package plugin_space

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// resolveSourceFetchManifest relays a listed manifest request to the parent
// bus. The parent directive exists only while the Space lists the plugin. The
// resolver adds a value only when the parent supplies manifests, so an empty
// parent never masks the Space World resolver. Each root the parent supplies is
// recorded as trusted: the parent serves only the release and the development
// build, never a Space World.
func (c *Controller) resolveSourceFetchManifest(
	ctx context.Context,
	handler directive.ResolverHandler,
	dir manifest.FetchManifest,
) error {
	for {
		source, listed, waitCh := c.getManifestSourceListing(dir.GetManifestId())
		if source == nil || !listed {
			_ = handler.ClearValues()
			handler.MarkIdle(true)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-waitCh:
			}
			continue
		}

		demandCtx, cancel := context.WithCancel(ctx)
		_, release, err := bus.ExecCollectValuesWatch[*manifest.FetchManifestValue](
			demandCtx,
			source,
			dir,
			true,
			func(_ []error, vals []*manifest.FetchManifestValue) error {
				// Skip updates after the demand context canceled.
				if demandCtx.Err() != nil {
					return nil
				}

				// Republish the collected manifest refs as one value.
				refs := make([]*manifest.ManifestRef, 0)
				for _, val := range vals {
					refs = append(refs, val.GetManifestRefs()...)
				}
				c.addSourceRoots(dir.GetManifestId(), refs)
				_ = handler.ClearValues()
				if len(refs) != 0 {
					_, _ = handler.AddValue(&manifest.FetchManifestValue{ManifestRefs: refs})
				}
				handler.MarkIdle(true)
				return nil
			},
			nil,
		)
		if err != nil {
			cancel()
			return err
		}

		// Keep the parent demand until the source or the listing changes.
		// The broadcast also fires for unrelated state such as resolver
		// registration, which must not restart the parent fetch.
		for {
			select {
			case <-ctx.Done():
				cancel()
				release()
				return ctx.Err()
			case <-waitCh:
			}
			var next bus.Bus
			next, listed, waitCh = c.getManifestSourceListing(dir.GetManifestId())
			if next != source || !listed {
				break
			}
		}
		cancel()
		release()
	}
}

// getManifestSourceListing snapshots the parent source and whether the Space
// lists the plugin. A listing is what the Space demands. Whether the plugin
// may run is the device's AdmitPluginRoot decision.
func (c *Controller) getManifestSourceListing(manifestID string) (bus.Bus, bool, <-chan struct{}) {
	// Snapshot the parent source, listing, and wait channel under the lock.
	var source bus.Bus
	var listed bool
	var waitCh <-chan struct{}
	c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		source = c.manifestSource
		listed = slices.Contains(c.pluginIDs, manifestID)
		waitCh = getWaitCh()
	})
	return source, listed, waitCh
}

// addSourceRoots records the manifest roots the parent source supplied for
// pluginID. Roots are only added: each identifies immutable content the release
// or the development build supplied. That provenance stays true after the
// parent drops the root.
func (c *Controller) addSourceRoots(pluginID string, refs []*manifest.ManifestRef) {
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		added := false
		for _, ref := range refs {
			// Skip a ref of another plugin or with no root.
			if ref.GetMeta().GetManifestId() != pluginID {
				continue
			}
			root := ref.GetManifestRef().GetRootRef().GetHash().MarshalString()
			if root == "" {
				continue
			}
			key := sourceRoot{pluginID: pluginID, root: root}
			if _, ok := c.sourceRoots[key]; ok {
				continue
			}
			if c.sourceRoots == nil {
				c.sourceRoots = make(map[sourceRoot]struct{})
			}
			c.sourceRoots[key] = struct{}{}
			added = true
		}
		if added {
			broadcast()
		}
	})
}

// sourceRoot is a plugin ID paired with a manifest root the parent supplied.
type sourceRoot struct {
	pluginID, root string
}

// resolveAdmitPluginRoot admits a root the parent source supplied, for any
// peer. Every other root is left to the device's approvals.
//
// The parent source is trusted. It is the bus of the root resource
// controller, passed through WithManifestSource, whose FetchManifest
// resolvers serve the release's own manifests from its distribution world
// and, in development, the manifests the devtool built. No Space world
// resolves FetchManifest there, so no Space member can place a root in it.
func (c *Controller) resolveAdmitPluginRoot(dir bldr_plugin.AdmitPluginRoot) ([]directive.Resolver, error) {
	key := sourceRoot{pluginID: dir.AdmitPluginRootPluginID(), root: dir.AdmitPluginRootManifestRoot()}
	return directive.R(directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
		for {
			// Admit the root once the parent has supplied it.
			var found bool
			var waitCh <-chan struct{}
			c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
				_, found = c.sourceRoots[key]
				waitCh = getWaitCh()
			})
			if found {
				_, _ = handler.AddValue(bldr_plugin.AdmitSourceTrusted)
				return nil
			}
			handler.MarkIdle(true)
			select {
			case <-ctx.Done():
				return context.Cause(ctx)
			case <-waitCh:
			}
		}
	}), nil)
}
