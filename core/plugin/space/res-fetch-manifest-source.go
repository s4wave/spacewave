package plugin_space

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
)

// resolveSourceFetchManifest relays an approved manifest request to the parent
// bus. The parent directive exists only while the Space approval remains.
func (c *Controller) resolveSourceFetchManifest(
	ctx context.Context,
	handler directive.ResolverHandler,
	dir manifest.FetchManifest,
) error {
	for {
		source, approved, waitCh := c.getManifestSourceApproval(dir.GetManifestId())
		if source == nil || !approved {
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
				if demandCtx.Err() != nil {
					return nil
				}
				refs := make([]*manifest.ManifestRef, 0)
				for _, val := range vals {
					refs = append(refs, val.GetManifestRefs()...)
				}
				_ = handler.ClearValues()
				_, _ = handler.AddValue(&manifest.FetchManifestValue{ManifestRefs: refs})
				handler.MarkIdle(true)
				return nil
			},
			nil,
		)
		if err != nil {
			cancel()
			return err
		}
		select {
		case <-ctx.Done():
			cancel()
			release()
			return ctx.Err()
		case <-waitCh:
			cancel()
			release()
		}
	}
}

// getManifestSourceApproval snapshots the parent source and SpaceSettings gate.
func (c *Controller) getManifestSourceApproval(manifestID string) (bus.Bus, bool, <-chan struct{}) {
	var source bus.Bus
	var approved bool
	var waitCh <-chan struct{}
	c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		source = c.manifestSource
		approved = slices.Contains(c.pluginIDs, manifestID)
		waitCh = getWaitCh()
	})
	return source, approved, waitCh
}
