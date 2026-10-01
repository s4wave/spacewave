package provider_local

import (
	"context"
	stderrors "errors"
	"maps"

	"github.com/pkg/errors"
	sobject "github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// AutoStartP2PSyncIfNeeded starts P2P sync when the account has a paired
// Device, a joined Space, or an owned Space with invitations or participants.
// Session mount restores both invitation service and synchronization.
//
// Errors mounting the account settings SO are logged and swallowed so a
// missing or unreadable SO does not abort the session mount.
func (a *ProviderAccount) AutoStartP2PSyncIfNeeded(
	ctx context.Context,
	st *transport.SessionTransport,
) error {
	// Without a Session transport there is nothing to sync over.
	if st == nil {
		return nil
	}

	// Read the paired Devices and account Sessions from account settings.
	settings, err := a.readAccountSettings(ctx)
	if err != nil {
		if isAutoStartReadCanceled(err) {
			return nil
		}
		a.le.WithError(err).Warn("failed to read paired devices for auto-start")
		return nil
	}
	devices := settings.GetPairedDevices()

	// Collect the account's other live Sessions when this Session is a member.
	var accountPeers []peer.ID
	if self := settings.FindAccountSession(st.GetPeerID().String()); self != nil && !self.GetRevoked() {
		for _, member := range settings.GetSessions() {
			if member.GetRevoked() || member.GetPeerId() == st.GetPeerID().String() {
				continue
			}
			remote, _, err := peer.ParsePeerIDWithPubKey(member.GetPeerId())
			if err != nil {
				return err
			}
			accountPeers = append(accountPeers, remote)
		}
	}

	// Count shared Spaces and collect the peers their invitations target.
	sharedSpaceCount := 0
	invitedPeers := make(map[string]struct{})
	if soList := a.soListCtr.GetValue(); soList != nil {
		for _, entry := range soList.GetSharedObjects() {
			if entry.GetSource() == "shared" {
				sharedSpaceCount++
				if endpoint := entry.GetTransportPeerId(); endpoint != "" {
					invitedPeers[endpoint] = struct{}{}
				}
				continue
			}

			// Owners must remain reachable before the first recipient accepts.
			so, release, err := a.MountSharedObject(ctx, entry.GetRef(), nil)
			if err != nil {
				return errors.Wrap(err, "inspect shared object for invitation service")
			}
			local, ok := so.(*SharedObject)
			if !ok {
				release()
				continue
			}
			state, err := local.soHost.GetHostState(ctx)
			release()
			if err != nil {
				return errors.Wrap(err, "inspect invitation state")
			}
			shared := len(state.GetRootGrants()) > 1
			for _, invite := range state.GetInvites() {
				target := invite.GetTargetPeerId()
				active := sobject.ValidateInviteUsable(invite) == nil
				for _, grant := range state.GetRootGrants() {
					if target != "" && grant.GetPeerId() == target {
						active = true
						break
					}
				}
				if active {
					shared = true
					if target != "" {
						invitedPeers[target] = struct{}{}
					}
				}
			}
			if shared {
				sharedSpaceCount++
			}
		}
	}

	// Stay idle when nothing needs P2P sync.
	if len(devices) == 0 && len(accountPeers) == 0 && sharedSpaceCount == 0 {
		return nil
	}

	// Start the persistent P2P sync on the Session transport.
	a.le.WithFields(logrus.Fields{
		"paired-device-count": len(devices),
		"shared-space-count":  sharedSpaceCount,
	}).Debug("auto-starting P2P sync")
	if err := a.StartPersistentP2PSync(ctx, st); err != nil {
		return errors.Wrap(err, "auto-start P2P sync")
	}

	// Restore links to the account's other Sessions.
	for _, remote := range accountPeers {
		if err := a.RetainP2PPeer(ctx, remote); err != nil {
			return errors.Wrap(err, "restore account Session")
		}
	}

	// Snapshot pending enrollments after the invitation scan. Approval marks a
	// Device pending before storing its invite, so every scanned SpaceLink
	// invite has its mark here. The Device dials the owner through that invite;
	// the owner must not dial a Device that has not connected once in this
	// process.
	var pendingEnroll map[string]struct{}
	a.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if len(a.p2pPendingEnrollPeers) != 0 {
			pendingEnroll = make(map[string]struct{}, len(a.p2pPendingEnrollPeers))
			maps.Copy(pendingEnroll, a.p2pPendingEnrollPeers)
		}
	})

	// Both targeted peers retain the link so deterministic WebRTC offers can start.
	for target := range invitedPeers {
		targetPeer, err := peer.IDB58Decode(target)
		if err != nil {
			return errors.Wrap(err, "parse invitation recipient")
		}
		if _, pending := pendingEnroll[targetPeer.String()]; pending {
			continue
		}
		if err := a.RetainP2PPeer(ctx, targetPeer); err != nil {
			return errors.Wrap(err, "restore invitation recipient")
		}
	}

	// Retain paired Devices that are not waiting to enroll.
	for _, device := range devices {
		remotePeerID, _, err := peer.ParsePeerIDWithPubKey(device.GetPeerId())
		if err != nil {
			return errors.Wrap(err, "parse paired Device peer id")
		}
		if remotePeerID == st.GetPeerID() {
			continue
		}
		if _, pending := pendingEnroll[remotePeerID.String()]; pending {
			a.le.WithField("peer-id", remotePeerID.String()).Debug("skipping auto-start retain for pending enrollment")
			continue
		}
		if err := a.RetainP2PPeer(ctx, remotePeerID); err != nil {
			return errors.Wrap(err, "retain paired Device peer")
		}
	}
	return nil
}

func isAutoStartReadCanceled(err error) bool {
	return stderrors.Is(err, context.Canceled)
}
