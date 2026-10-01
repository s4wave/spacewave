package resource_root

import (
	"context"

	"github.com/aperturerobotics/util/backoff"
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

// RunBackgroundPlugins keeps each Space with a confirmed background plugin
// mounted, with its plugin runtime running, for as long as the confirmation
// stands in its Session's metadata. Holding the Session mount keeps the
// Session running. Returns when ctx ends.
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
		keyed.WithRetry[backgroundSpace, struct{}](&backoff.Backoff{
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

		// Hold exactly the Spaces the Sessions currently confirm.
		spaces, err := listBackgroundSpaces(ctx, sessionCtrl)
		if err != nil {
			return err
		}
		holds.SyncKeys(spaces, false)

		select {
		case <-ctx.Done():
			return nil
		case <-ch:
		}
	}
}

// listBackgroundSpaces returns the Spaces with a confirmed background plugin
// in each registered Session's metadata.
func listBackgroundSpaces(ctx context.Context, sessionCtrl session.SessionController) ([]backgroundSpace, error) {
	// Snapshot the registered Sessions.
	entries, err := sessionCtrl.ListSessions(ctx)
	if err != nil {
		return nil, err
	}

	// Collect each Session's confirmed Spaces.
	var spaces []backgroundSpace
	for _, entry := range entries {
		meta, err := sessionCtrl.GetSessionMetadata(ctx, entry.GetSessionIndex())
		if err != nil {
			return nil, err
		}

		ref := entry.GetSessionRef().GetProviderResourceRef()
		for _, plugin := range meta.GetBackgroundPlugins() {
			spaces = append(spaces, backgroundSpace{
				providerID: ref.GetProviderId(),
				accountID:  ref.GetProviderAccountId(),
				sessionID:  ref.GetId(),
				spaceID:    plugin.GetSpaceId(),
			})
		}
	}
	return spaces, nil
}

// newBackgroundSpaceHold builds the routine holding one background Space: it
// mounts the Session and the Space as an application would and acquires the
// Space plugin runtime until ctx ends.
func (s *CoreRootServer) newBackgroundSpaceHold(key backgroundSpace) (keyed.Routine, struct{}) {
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
		_, _, runtimeRef, err := spaceResource.AcquireContentsRuntime(ctx)
		if err != nil {
			return err
		}
		defer runtimeRef.Release()

		// Run until the confirmation is withdrawn or the root stops.
		s.le.WithField("space-id", key.spaceID).Info("running background plugins")
		<-ctx.Done()
		return nil
	}, struct{}{}
}
