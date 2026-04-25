package provider_local

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_invite "github.com/s4wave/spacewave/core/sobject/invite"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
)

const directInviteOwnerWaitTimeout = 5 * time.Second

// ErrDirectInviteOwnerMustBeOnline indicates the direct invite path requires
// the owner to be reachable on the live transport.
var ErrDirectInviteOwnerMustBeOnline = errors.New("space owner must be online to accept this invite directly")

// JoinViaInvite executes the full invite join flow:
// 1. Ensures a session transport is running (starts one if needed)
// 2. Opens an SRPC stream to the owner and sends AcceptInviteRequest
// 3. Receives the SOGrant from the owner
// 4. Mounts the shared object with the grant
// 5. Starts P2P sync so SolicitSync delivers state
//
// The inviteMsg is the out-of-band SOInviteMessage from the owner.
// sessionKey is the invitee's session private key.
// signalingURL is the cloud API base URL for signaling (can be empty for local).
func (a *ProviderAccount) JoinViaInvite(
	ctx context.Context,
	sessionKey crypto.PrivKey,
	inviteMsg *sobject.SOInviteMessage,
	signalingURL string,
) (*sobject_invite.JoinResult, error) {
	if inviteMsg == nil {
		return nil, errors.New("invite message is nil")
	}

	// Ensure transport is running so we can reach the owner.
	if err := a.EnsureSessionTransport(ctx, sessionKey, signalingURL); err != nil {
		return nil, errors.Wrap(err, "start session transport")
	}

	st := a.GetSessionTransport()
	if st == nil {
		return nil, errors.New("session transport not available")
	}
	childBus := st.GetChildBus()
	if childBus == nil {
		return nil, errors.New("session transport child bus not available")
	}
	if inviteMsg.GetProviderId() == "spacewave" {
		if err := a.waitDirectInviteOwnerOnline(ctx, childBus, st.GetPeerID(), inviteMsg.GetOwnerPeerId()); err != nil {
			return nil, err
		}
	}

	// Execute the invite handshake over SRPC.
	result, err := sobject_invite.JoinViaInvite(
		ctx,
		childBus,
		st.GetPeerID(),
		sessionKey,
		inviteMsg,
	)
	if err != nil {
		return nil, errors.Wrap(err, "invite handshake")
	}

	// Mount the shared object and apply the grant.
	if err := a.mountInvitedSO(ctx, result); err != nil {
		return nil, errors.Wrap(err, "mount invited shared object")
	}

	// Start P2P sync so SolicitSync delivers state from the owner.
	if err := a.StartP2PSync(ctx, st); err != nil {
		a.le.WithError(err).Warn("failed to start P2P sync after invite join")
	}

	return result, nil
}

func (a *ProviderAccount) waitDirectInviteOwnerOnline(
	ctx context.Context,
	childBus bus.Bus,
	localPeerID peer.ID,
	ownerPeerIDStr string,
) error {
	if ownerPeerIDStr == "" {
		return errors.New("invite owner peer id is required")
	}
	ownerPeerID, err := peer.IDB58Decode(ownerPeerIDStr)
	if err != nil {
		return errors.Wrap(err, "parse invite owner peer id")
	}

	waitCtx, waitCancel := context.WithTimeout(ctx, directInviteOwnerWaitTimeout)
	defer waitCancel()

	_, rel, err := link.EstablishLinkWithPeerEx(waitCtx, childBus, localPeerID, ownerPeerID, true)
	if rel != nil {
		rel()
	}
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	a.le.WithError(err).
		WithField("owner-peer-id", ownerPeerIDStr).
		Debug("direct invite owner not reachable")
	return ErrDirectInviteOwnerMustBeOnline
}

// mountInvitedSO mounts a shared object after receiving an invite grant.
// The grant is stored in the SO state and the SO is persisted to the
// account's SO list so it survives restarts and is picked up by P2P sync.
func (a *ProviderAccount) mountInvitedSO(ctx context.Context, result *sobject_invite.JoinResult) error {
	if result.Grant == nil {
		return errors.New("invite result has no grant")
	}

	soID := result.SharedObjectID
	if soID == "" {
		return errors.New("invite result has no shared object ID")
	}

	providerID := a.t.accountInfo.GetProviderId()
	accountID := a.t.accountInfo.GetProviderAccountId()
	blockStoreID := SobjectBlockStoreID(soID)
	ref := sobject.NewSharedObjectRef(providerID, accountID, soID, blockStoreID)

	// Mount the SO. If it already exists, this is a no-op.
	so, relSO, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return errors.Wrap(err, "mount shared object")
	}
	defer relSO()

	// Store the grant on the SO state so the invitee can decrypt the SO data.
	localSO, ok := so.(*SharedObject)
	if !ok {
		return errors.New("unexpected shared object type")
	}

	if err := localSO.soHost.UpdateSOState(ctx, func(state *sobject.SOState) error {
		// Append the grant to root_grants if not already present.
		grantPeerID := result.Grant.GetPeerId()
		for _, g := range state.GetRootGrants() {
			if g.GetPeerId() == grantPeerID {
				return nil
			}
		}
		state.RootGrants = append(state.RootGrants, result.Grant)
		return nil
	}); err != nil {
		return errors.Wrap(err, "store grant")
	}

	// Persist the SO to the account's SO list so it survives restarts
	// and is included in P2P sync. Follows createSharedObjectLocked pattern.
	relMtx, err := a.mtx.Lock(ctx)
	if err != nil {
		return errors.Wrap(err, "lock account mutex")
	}
	defer relMtx()

	soList := a.soListCtr.GetValue().CloneVT()
	if soList == nil {
		soList = &sobject.SharedObjectList{}
	}

	// Skip if already in the list.
	for _, entry := range soList.GetSharedObjects() {
		if entry.GetRef().GetProviderResourceRef().GetId() == soID {
			return nil
		}
	}

	soList.SharedObjects = append(soList.SharedObjects, &sobject.SharedObjectListEntry{
		Ref:    ref.CloneVT(),
		Source: "shared",
		Meta: &sobject.SharedObjectMeta{
			BodyType: "space",
		},
	})
	slices.SortFunc(soList.SharedObjects, func(a, b *sobject.SharedObjectListEntry) int {
		return strings.Compare(a.GetRef().GetProviderResourceRef().GetId(), b.GetRef().GetProviderResourceRef().GetId())
	})

	if err := a.writeSharedObjectList(ctx, soList); err != nil {
		return errors.Wrap(err, "persist SO list")
	}
	a.soListCtr.SetValue(soList)

	return nil
}
