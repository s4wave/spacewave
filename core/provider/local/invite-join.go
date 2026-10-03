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
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
)

const directInviteOwnerWaitTimeout = 5 * time.Second

// ErrDirectInviteOwnerMustBeOnline indicates the direct invite path requires
// the owner to be reachable on the live transport.
var ErrDirectInviteOwnerMustBeOnline = errors.New("space owner must be online to accept this invite directly")

// JoinViaInvite executes the full invite join flow.
//
//  1. Ensures a session transport is running (starts one if needed)
//  2. Opens an SRPC stream to the owner and sends AcceptInviteRequest
//  3. Receives the SOGrant from the owner, or returns early when the owner
//     queued the redemption for approval
//  4. Mounts the shared object with the grant
//  5. Starts P2P sync so SolicitSync delivers state
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
	// Reach the owner the invite names.
	st, ownerPeerID, err := a.reachInviteOwner(ctx, sessionKey, inviteMsg, signalingURL)
	if err != nil {
		return nil, err
	}

	// Wait for the owner to be reachable on the session transport.
	childBus := st.GetChildBus()
	joinCtx, joinCancel := context.WithTimeout(ctx, directInviteOwnerWaitTimeout)
	defer joinCancel()
	if err := a.waitDirectInviteOwnerOnline(
		joinCtx,
		childBus,
		st.GetPeerID(),
		ownerPeerID.String(),
	); err != nil {
		return nil, err
	}

	// Read the storage peer key, which joins alongside the session.
	volumePeer, err := a.vol.GetPeer(ctx, true)
	if err != nil {
		return nil, errors.Wrap(err, "get storage peer")
	}
	storageKey, err := volumePeer.GetPrivKey(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get storage peer key")
	}

	// Execute the invite handshake over SRPC while the verified owner remains
	// reachable. A signaling or link failure must not hold the enrollment RPC
	// forever.
	result, err := sobject_invite.JoinViaInvite(
		joinCtx,
		childBus,
		st.GetPeerID(),
		sessionKey,
		storageKey,
		inviteMsg,
	)
	if err != nil {
		return nil, errors.Wrap(err, "invite handshake")
	}
	if result.Pending {
		return result, nil
	}

	// Mount the shared object and apply the grant.
	if err := a.mountInvitedSO(ctx, result, ownerPeerID); err != nil {
		return nil, errors.Wrap(err, "mount invited shared object")
	}

	// The bounded join context ends with this call. P2P sync belongs to the
	// account and stops with the account, not with the enrollment request.
	if err := a.StartPersistentP2PSync(ctx, st); err != nil {
		a.le.WithError(err).Warn("failed to start P2P sync after invite join")
	} else {
		// Freshen a cached SO's solicitation after installing its new grant. A
		// newly listed SO may still be waiting for normal list reconciliation.
		a.RetrySharedObjectSync(result.SharedObjectID)
	}
	if err := a.RetainP2PPeer(ctx, ownerPeerID); err != nil {
		return nil, errors.Wrap(err, "retain invite owner link")
	}

	return result, nil
}

// WithdrawJoinRequest asks the owner the invite names to drop this session's
// pending request to join the invite's shared object. The owner must be
// reachable on the live transport.
func (a *ProviderAccount) WithdrawJoinRequest(
	ctx context.Context,
	sessionKey crypto.PrivKey,
	inviteMsg *sobject.SOInviteMessage,
) error {
	// Reach the owner the invite names.
	st, ownerPeerID, err := a.reachInviteOwner(ctx, sessionKey, inviteMsg, "")
	if err != nil {
		return err
	}

	// Wait for the owner to be reachable on the session transport.
	withdrawCtx, withdrawCancel := context.WithTimeout(ctx, directInviteOwnerWaitTimeout)
	defer withdrawCancel()
	if err := a.waitDirectInviteOwnerOnline(
		withdrawCtx,
		st.GetChildBus(),
		st.GetPeerID(),
		ownerPeerID.String(),
	); err != nil {
		return err
	}

	// Withdraw the request on the owner's host.
	return sobject_invite.WithdrawJoinRequest(
		withdrawCtx,
		st.GetChildBus(),
		st.GetPeerID(),
		ownerPeerID,
		inviteMsg.GetSharedObjectId(),
	)
}

// reachInviteOwner verifies the owner peer inviteMsg names and starts the
// session transport that reaches it.
func (a *ProviderAccount) reachInviteOwner(
	ctx context.Context,
	sessionKey crypto.PrivKey,
	inviteMsg *sobject.SOInviteMessage,
	signalingURL string,
) (*transport.SessionTransport, peer.ID, error) {
	// Verify the owner peer the invite names.
	if inviteMsg == nil {
		return nil, "", errors.New("invite message is nil")
	}
	ownerPeerID, err := inviteMsg.VerifyTransportPeer()
	if err != nil {
		return nil, "", errors.Wrap(err, "parse invite owner peer id")
	}

	// A local account with no explicit signaling URL rendezvouses through the
	// configured trusted cloud endpoint so WebRTC reconnects after restarts.
	signingEnvPrefix := ""
	if signalingURL == "" {
		relay := a.fallbackSignalingEndpoint()
		signalingURL = relay.url
		signingEnvPrefix = relay.signingEnvPrefix
	}

	// Enrollment outlives this RPC. Bind its transport to the mounted account
	// rather than to the invite request that happened to create it.
	ownerCtx := a.lifecycleCtx
	if ownerCtx == nil {
		ownerCtx = ctx
	}
	if _, _, err := a.ensureSessionTransportWithOwner(
		ctx, ownerCtx, sessionKey, signalingURL, signingEnvPrefix, true,
	); err != nil {
		return nil, "", errors.Wrap(err, "start session transport")
	}

	// Return the transport the session reaches the owner on.
	st := a.GetSessionTransport()
	if st == nil {
		return nil, "", errors.New("session transport not available")
	}
	if st.GetChildBus() == nil {
		return nil, "", errors.New("session transport child bus not available")
	}
	return st, ownerPeerID, nil
}

// waitDirectInviteOwnerOnline establishes a link to the owner, returning
// ErrDirectInviteOwnerMustBeOnline when the owner is unreachable.
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
func (a *ProviderAccount) mountInvitedSO(
	ctx context.Context,
	result *sobject_invite.JoinResult,
	ownerPeerID peer.ID,
) error {
	// The result carries a grant, a state, and an object ID.
	if result.Grant == nil {
		return errors.New("invite result has no grant")
	}
	if result.SharedObjectState == nil {
		return errors.New("invite result has no shared object state")
	}
	soID := result.SharedObjectID
	if soID == "" {
		return errors.New("invite result has no shared object ID")
	}
	return a.mountEnrolledSO(ctx, soID, &sobject.SharedObjectMeta{BodyType: "space"}, "shared", result.SharedObjectState, result.ConfigLineage, ownerPeerID)
}

// mountEnrolledSO persists a checkpoint received through an authorized enrollment
// exchange with the config lineage leading to its config, oldest first. The
// SharedObject host validates the state and the lineage.
func (a *ProviderAccount) mountEnrolledSO(
	ctx context.Context,
	soID string,
	meta *sobject.SharedObjectMeta,
	source string,
	state *sobject.SOState,
	lineage []*sobject.SOConfigChange,
	ownerPeerID peer.ID,
) error {
	// Name the object in this account.
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

	// Install the invited state and lineage, whose grant lets the invitee
	// decrypt the SO data.
	localSO, ok := so.(*SharedObject)
	if !ok {
		return errors.New("unexpected shared object type")
	}
	if err := localSO.soHost.InstallInviteSnapshot(ctx, state, lineage); err != nil {
		return errors.Wrap(err, "install owner shared object state")
	}

	// Denials answered the previous grant, not the checkpoint just installed.
	localSO.tkr.healthCtr.SwapValue((*sobject.SharedObjectHealth).WithoutSyncDenials)

	// Admission is complete only when body mounts can observe the accepted grant.
	if err := localSO.lsoHost.waitPublishedConfig(ctx, state.GetConfig()); err != nil {
		return errors.Wrap(err, "publish invited shared object state")
	}

	// Persist the SO to the account's SO list so it survives restarts
	// and is included in P2P sync. Follows createSharedObjectLocked pattern.
	relMtx, err := a.mtx.Lock(ctx)
	if err != nil {
		return errors.Wrap(err, "lock account mutex")
	}
	defer relMtx()

	// Load the account's object list.
	soList := a.soListCtr.GetValue().CloneVT()
	if soList == nil {
		soList = &sobject.SharedObjectList{}
	}

	// Refresh the verified endpoint when an existing participant accepts a new invite.
	for _, entry := range soList.GetSharedObjects() {
		if entry.GetRef().GetProviderResourceRef().GetId() == soID {
			entry.TransportPeerId = ownerPeerID.String()
			if err := a.writeSharedObjectList(ctx, soList); err != nil {
				return errors.Wrap(err, "persist invited peer endpoint")
			}
			a.soListCtr.SetValue(soList)
			return nil
		}
	}

	// Otherwise add the object in ID order.
	soList.SharedObjects = append(soList.SharedObjects, &sobject.SharedObjectListEntry{
		Ref:             ref.CloneVT(),
		Source:          source,
		TransportPeerId: ownerPeerID.String(),
		Meta:            meta.CloneVT(),
	})
	slices.SortFunc(soList.SharedObjects, func(a, b *sobject.SharedObjectListEntry) int {
		return strings.Compare(a.GetRef().GetProviderResourceRef().GetId(), b.GetRef().GetProviderResourceRef().GetId())
	})

	// Persist the list and publish it.
	if err := a.writeSharedObjectList(ctx, soList); err != nil {
		return errors.Wrap(err, "persist SO list")
	}
	a.soListCtr.SetValue(soList)
	return nil
}
