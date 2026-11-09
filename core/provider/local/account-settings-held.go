package provider_local

import (
	"context"
	"slices"
	"time"

	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_sync "github.com/s4wave/spacewave/core/sobject/sync"
	"github.com/s4wave/spacewave/net/peer"
)

// settingsHeldTimeout bounds how long a migrated Session waits for another
// account peer to hold the destination settings before it retires the
// temporary enrollment transport that carries them.
const settingsHeldTimeout = 30 * time.Second

// waitAccountSettingsHeld waits until a remote sync peer advertises a settings
// state that this account held at or after the call. Local states only grow
// from that point, so the peer then holds every settings operation committed
// before the call. The advertised digest is the peer's claim and only delays
// the caller.
func (a *ProviderAccount) waitAccountSettingsHeld(ctx context.Context) error {
	// Mount the account settings object.
	ref, err := a.GetAccountSettingsRef(ctx)
	if err != nil {
		return err
	}
	so, release, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return err
	}
	defer release()

	// Use the local object, whose tracker records the peer state digests.
	localSO, ok := so.(*SharedObject)
	if !ok {
		return errors.New("account settings object is not a local shared object")
	}

	// Watch the accepted local settings state.
	states, releaseStates, err := localSO.soHost.GetSOStateCtr(ctx, nil)
	if err != nil {
		return err
	}
	defer releaseStates()

	// Signal each local state change. A peer can advertise a merged state
	// before this account adopts the same merge.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	changed := make(chan struct{}, 1)
	go func() {
		var previous *sobject.SOState
		for {
			current, err := states.WaitValueChange(ctx, previous, nil)
			if err != nil {
				return
			}
			previous = current
			select {
			case changed <- struct{}{}:
			default:
			}
		}
	}()

	// Wait for a remote peer to advertise one of the local states seen here.
	local := localSO.GetPeerID().String()
	held := make(map[string]struct{})
	tkr := localSO.tkr
	for {
		if current := states.GetValue(); current != nil {
			sum, err := sobject_sync.StateHash(current)
			if err != nil {
				return err
			}
			held[string(sum)] = struct{}{}
		}
		var found bool
		var waitCh <-chan struct{}
		tkr.peerStateBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
			for id, sum := range tkr.peerStates {
				if _, ok := held[string(sum)]; ok && id != local {
					found = true
					return
				}
			}
		})
		if found {
			return nil
		}
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-waitCh:
		case <-changed:
		}
	}
}

// hasRemoteAccountMember reports whether settings name an active member other
// than the excluded local Sessions.
func hasRemoteAccountMember(settings *account_settings.AccountSettings, exclude ...peer.ID) bool {
	for _, member := range settings.GetSessions() {
		if member.GetRevoked() || slices.ContainsFunc(exclude, func(id peer.ID) bool { return id.String() == member.GetPeerId() }) {
			continue
		}
		return true
	}
	return false
}
