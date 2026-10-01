package resource_root

import (
	"context"
	"maps"
	"slices"

	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/keyed"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/session"
)

// backgroundSpace identifies a Space a Session keeps mounted for its confirmed
// background plugins.
type backgroundSpace struct {
	// providerID, accountID, and sessionID identify the Session.
	providerID, accountID, sessionID string
	// spaceID identifies the Space.
	spaceID string
}

// backgroundSpaceHold carries the plugins a background Space hold keeps
// running. The hold updates its runtime demand in place, so a change to the
// set does not restart the plugins that stay.
type backgroundSpaceHold struct {
	// bcast guards pluginIDs.
	bcast broadcast.Broadcast
	// pluginIDs is the sorted set of confirmed plugins that are not suspended.
	pluginIDs []string
}

// setPluginIDs replaces the held plugins and wakes the hold.
func (h *backgroundSpaceHold) setPluginIDs(ids []string) {
	h.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if !slices.Equal(h.pluginIDs, ids) {
			h.pluginIDs = ids
			broadcast()
		}
	})
}

// getPluginIDs returns the held plugins and a channel closed when they change.
func (h *backgroundSpaceHold) getPluginIDs() ([]string, <-chan struct{}) {
	var ids []string
	var ch <-chan struct{}
	h.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ids, ch = h.pluginIDs, getWaitCh()
	})
	return ids, ch
}

// RunBackgroundPlugins keeps each Space with a confirmed background plugin that
// is not suspended mounted, with its plugin runtime running those plugins, for
// as long as the confirmation stands in its Session's metadata. The Space's
// other plugins run only while an application mounts its contents. Holding the
// Session mount keeps the Session running. Returns when ctx ends.
func (s *CoreRootServer) RunBackgroundPlugins(ctx context.Context) error {
	// Watch the Session metadata owner.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.b, "", false, nil)
	if err != nil {
		return err
	}
	defer sessionCtrlRef.Release()

	// Run one retrying hold per Space; a failed mount retries with backoff.
	holds := keyed.NewKeyedWithLogger(
		s.newBackgroundSpaceHold,
		s.le.WithField("subsystem", "background-plugins"),
		keyed.WithRetry[backgroundSpace, *backgroundSpaceHold](&backoff.Backoff{
			BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
		}),
	)
	holds.SetContext(ctx, true)
	defer holds.ClearContext()

	// Resync the holds after every metadata change.
	bcast := sessionCtrl.GetSessionBroadcast()
	for {
		// Take the wait channel before reading so no metadata change is missed.
		var ch <-chan struct{}
		bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
		})

		// Hold exactly the Spaces and plugins the Sessions currently confirm.
		spaces, err := listBackgroundSpaces(ctx, sessionCtrl)
		if err != nil {
			return err
		}
		holds.SyncKeys(slices.Collect(maps.Keys(spaces)), false)
		for space, pluginIDs := range spaces {
			if hold, ok := holds.GetKey(space); ok {
				hold.setPluginIDs(pluginIDs)
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ch:
		}
	}
}

// listBackgroundSpaces returns the sorted confirmed plugins that are not
// suspended for each Space in each registered Session's metadata.
func listBackgroundSpaces(
	ctx context.Context,
	sessionCtrl session.SessionController,
) (map[backgroundSpace][]string, error) {
	// Snapshot the registered Sessions.
	entries, err := sessionCtrl.ListSessions(ctx)
	if err != nil {
		return nil, err
	}

	// Collect each Session's running confirmations by Space.
	spaces := make(map[backgroundSpace][]string)
	for _, entry := range entries {
		meta, err := sessionCtrl.GetSessionMetadata(ctx, entry.GetSessionIndex())
		if err != nil {
			return nil, err
		}

		ref := entry.GetSessionRef().GetProviderResourceRef()
		for _, plugin := range meta.GetBackgroundPlugins() {
			if plugin.GetSuspended() {
				continue
			}
			space := backgroundSpace{
				providerID: ref.GetProviderId(),
				accountID:  ref.GetProviderAccountId(),
				sessionID:  ref.GetId(),
				spaceID:    plugin.GetSpaceId(),
			}
			spaces[space] = append(spaces[space], plugin.GetPluginId())
		}
	}
	return spaces, nil
}

// newBackgroundSpaceHold builds the routine holding one background Space: it
// mounts the Session and the Space as an application would, acquires the Space
// plugin runtime, and demands the held plugins until ctx ends.
func (s *CoreRootServer) newBackgroundSpaceHold(key backgroundSpace) (keyed.Routine, *backgroundSpaceHold) {
	hold := &backgroundSpaceHold{}
	return func(ctx context.Context) error {
		// Mount the Session, keeping it running.
		sessRef := &session.SessionRef{ProviderResourceRef: &provider.ProviderResourceRef{
			ProviderId:        key.providerID,
			ProviderAccountId: key.accountID,
			Id:                key.sessionID,
		}}
		sess, sessMountRef, err := session.ExMountSession(ctx, s.b, sessRef, false, nil)
		if err != nil {
			return err
		}
		defer sessMountRef.Release()

		// Serve the Session as this root would to an app.
		sessResource := s.newSessionResource(sess)
		defer sessResource.Close()

		// Mount the Space and run its plugins.
		spaceResource, releaseSpace, err := sessResource.MountSpace(ctx, key.spaceID)
		if err != nil {
			return err
		}
		defer releaseSpace()

		// Hold the shared Space plugin runtime.
		_, runtime, runtimeRef, err := spaceResource.AcquireContentsRuntime(ctx)
		if err != nil {
			return err
		}
		defer runtimeRef.Release()

		// Demand the held plugins.
		pluginIDs, changed := hold.getPluginIDs()
		demand := runtime.DemandPlugins(pluginIDs)
		defer demand.Release()

		// Follow changes to the held plugins until the hold ends.
		le := s.le.WithField("space-id", key.spaceID)
		for {
			le.WithField("plugin-ids", pluginIDs).Info("running background plugins")
			select {
			case <-ctx.Done():
				return nil
			case <-changed:
			}
			pluginIDs, changed = hold.getPluginIDs()
			demand.SetPluginIDs(pluginIDs)
		}
	}, hold
}
