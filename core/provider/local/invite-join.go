package provider_local

import (
	"context"
	"net"
	"slices"
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
//  1. Ensures a session transport is running (starts one if needed), and
//     dials the owner over UDP when ownerUDPAddr names its address
//  2. Holds a link to the owner until the join returns
//  3. Opens an SRPC stream to the owner and sends AcceptInviteRequest
//  4. Receives the SOGrant from the owner, or returns early when the owner
//     queued the redemption for approval
//  5. Mounts the shared object with the grant
//  6. Records ownerUDPAddr on the listed object, so every later P2P sync
//     generation dials the owner there
//  7. Starts P2P sync so SolicitSync delivers state
//
// Only the wait for the owner's link is bounded. The owner consumes the
// invite when it answers, so the handshake and the local install that follows
// run until the owner replies or ctx ends, and the install is not canceled
// once the grant arrived.
//
// The inviteMsg is the out-of-band SOInviteMessage from the owner.
// sessionKey is the invitee's session private key. ownerUDPAddr is the
// owner's static UDP host:port, or empty to reach it through signaling.
func (a *ProviderAccount) JoinViaInvite(
	ctx context.Context,
	sessionKey crypto.PrivKey,
	inviteMsg *sobject.SOInviteMessage,
	ownerUDPAddr string,
) (*sobject_invite.JoinResult, error) {
	// Reject a malformed address before the join contacts anyone.
	if ownerUDPAddr != "" {
		if _, _, err := net.SplitHostPort(ownerUDPAddr); err != nil {
			return nil, errors.Wrap(err, "server address")
		}
	}

	// Reach the owner the invite names.
	st, ownerPeerID, err := a.reachInviteOwner(ctx, sessionKey, inviteMsg)
	if err != nil {
		return nil, err
	}

	// Dial the owner at its static address on the P2P sync generation, whose
	// link the sync that follows the join keeps using.
	if ownerUDPAddr != "" {
		if err := a.StartPersistentP2PSync(ctx, st); err != nil {
			return nil, errors.Wrap(err, "start P2P sync")
		}
		if err := a.dialJoiningOwner(ownerPeerID, ownerUDPAddr); err != nil {
			return nil, errors.Wrap(err, "dial invite owner over udp")
		}
	}

	// Hold the owner's link for the whole join. Releasing it at once leaves
	// the link to the hold-open timer, which closes it under a slow owner.
	childBus := st.GetChildBus()
	relLink, err := a.holdDirectInviteOwnerLink(
		ctx,
		childBus,
		st.GetPeerID(),
		ownerPeerID.String(),
	)
	if err != nil {
		return nil, err
	}
	defer relLink()

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
	// reachable. A link failure closes the stream and ends the call.
	result, err := sobject_invite.JoinViaInvite(
		ctx,
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

	// Install the grant even if ctx ends now: the invite is already consumed and
	// only this install lets a retry resume from the listed shared object.
	if err := a.mountInvitedSO(context.WithoutCancel(ctx), result); err != nil {
		return nil, errors.Wrap(err, "mount invited shared object")
	}
	if ownerUDPAddr != "" {
		if err := a.setSharedObjectUDPPeer(context.WithoutCancel(ctx), result.SharedObjectID, ownerPeerID, ownerUDPAddr); err != nil {
			return nil, errors.Wrap(err, "record invite owner udp address")
		}
	}

	// P2P sync belongs to the account and stops with the account, not with the
	// enrollment request.
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
	st, ownerPeerID, err := a.reachInviteOwner(ctx, sessionKey, inviteMsg)
	if err != nil {
		return err
	}

	// Hold the owner's link until the owner answers.
	relLink, err := a.holdDirectInviteOwnerLink(
		ctx,
		st.GetChildBus(),
		st.GetPeerID(),
		ownerPeerID.String(),
	)
	if err != nil {
		return err
	}
	defer relLink()

	// Withdraw the request on the owner's host.
	return sobject_invite.WithdrawJoinRequest(
		ctx,
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
) (*transport.SessionTransport, peer.ID, error) {
	// Verify the owner peer the invite names.
	if inviteMsg == nil {
		return nil, "", errors.New("invite message is nil")
	}
	ownerPeerID, err := inviteMsg.VerifyTransportPeer()
	if err != nil {
		return nil, "", errors.Wrap(err, "parse invite owner peer id")
	}

	// A local account rendezvouses through the configured trusted cloud
	// endpoint so WebRTC reconnects after restarts.
	relay := a.fallbackSignalingEndpoint()

	// Enrollment outlives this RPC. Bind its transport to the mounted account
	// rather than to the invite request that happened to create it.
	ownerCtx := a.lifecycleCtx
	if ownerCtx == nil {
		ownerCtx = ctx
	}
	if _, _, err := a.ensureSessionTransportWithOwner(
		ctx, ownerCtx, sessionKey, relay.url, relay.signingEnvPrefix, true,
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

// holdDirectInviteOwnerLink establishes a link to the owner and holds it until
// the returned release is called. It returns ErrDirectInviteOwnerMustBeOnline
// when the owner is not reachable within directInviteOwnerWaitTimeout.
func (a *ProviderAccount) holdDirectInviteOwnerLink(
	ctx context.Context,
	childBus bus.Bus,
	localPeerID peer.ID,
	ownerPeerIDStr string,
) (func(), error) {
	// Parse the owner the invite names.
	if ownerPeerIDStr == "" {
		return nil, errors.New("invite owner peer id is required")
	}
	ownerPeerID, err := peer.IDB58Decode(ownerPeerIDStr)
	if err != nil {
		return nil, errors.Wrap(err, "parse invite owner peer id")
	}

	// Wait for the link, then keep its reference for the caller.
	waitCtx, waitCancel := context.WithTimeout(ctx, directInviteOwnerWaitTimeout)
	defer waitCancel()
	_, rel, err := link.EstablishLinkWithPeerEx(waitCtx, childBus, localPeerID, ownerPeerID, true)
	if err == nil {
		if rel == nil {
			rel = func() {}
		}
		return rel, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	a.le.WithError(err).
		WithField("owner-peer-id", ownerPeerIDStr).
		Debug("direct invite owner not reachable")
	return nil, ErrDirectInviteOwnerMustBeOnline
}

// mountInvitedSO mounts a shared object after receiving an invite grant.
// The grant is stored in the SO state and the SO is persisted to the
// account's SO list so it survives restarts and is picked up by P2P sync.
func (a *ProviderAccount) mountInvitedSO(
	ctx context.Context,
	result *sobject_invite.JoinResult,
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
	_, err := a.mountEnrolledSO(ctx, soID, &sobject.SharedObjectMeta{BodyType: "space"}, "shared", nil, result.SharedObjectState, result.ConfigLineage)
	return err
}

// mountEnrolledSO persists a checkpoint received through an authorized enrollment
// exchange with the config lineage leading to its config, oldest first, and
// returns the generation the object is listed under. The SharedObject host
// validates the state and the lineage.
//
// generation is the catalog entry's generation for an install from the
// account catalog, or nil for a join. A join into a listed object merges into
// it and keeps its generation; otherwise the list write mints one. An install
// merges only into the same generation: a listed object of another
// generation is kept untouched and logged, and nothing is installed.
//
// The state write and the list write run in one critical section under the
// account lock, after a check that the id is still listed as it was when the
// enrollment decided between merge and install. A create, delete or other
// enrollment of the id in between fails the enrollment before it writes, so
// it never writes one lifetime's state into another. The placeholder and the
// mount precede the lock: an enrollment that fails leaves the id unlisted
// and a retry reuses the placeholder. A create or delete of the id replaces
// or removes the placeholder and restarts the mount, so no handle builds on
// it afterwards.
func (a *ProviderAccount) mountEnrolledSO(
	ctx context.Context,
	soID string,
	meta *sobject.SharedObjectMeta,
	source string,
	generation *string,
	state *sobject.SOState,
	lineage []*sobject.SOConfigChange,
) (string, error) {
	// Name the object in this account.
	providerID := a.t.accountInfo.GetProviderId()
	accountID := a.t.accountInfo.GetProviderAccountId()
	blockStoreID := SobjectBlockStoreID(soID)
	ref := sobject.NewSharedObjectRef(providerID, accountID, soID, blockStoreID)
	if err := ref.Validate(); err != nil {
		return "", errors.Wrap(err, "enrolled shared object ref")
	}

	// Decide under the account lock whether the state merges into a listed
	// object or installs a new one.
	relMtx, err := a.mtx.Lock(ctx)
	if err != nil {
		return "", errors.Wrap(err, "lock account mutex")
	}
	listed := a.listedSharedObjectEntry(soID).CloneVT()
	relMtx()
	if listed != nil && generation != nil && listed.GetGeneration() != *generation {
		a.le.WithField("sobject-id", soID).Warn("keeping local shared object of another generation than the enrolled one")
		return listed.GetGeneration(), nil
	}

	// Store a placeholder genesis for the received state to replace, unless
	// the account already holds state for the object, then mount it. The
	// install below goes through the mount.
	if err := a.writeGenesisSOState(ctx, soID); err != nil {
		return "", errors.Wrap(err, "store placeholder shared object state")
	}
	so, relSO, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return "", errors.Wrap(err, "mount shared object")
	}
	defer relSO()

	// Resolve the local shared object the invited state installs into.
	localSO, ok := so.(*SharedObject)
	if !ok {
		return "", errors.New("unexpected shared object type")
	}

	// Hold the account lock from the check through the list write, so no
	// create, delete or other enrollment of the id runs in between.
	relMtx, err = a.mtx.Lock(ctx)
	if err != nil {
		return "", errors.Wrap(err, "lock account mutex")
	}
	listedGen, err := a.writeEnrolledSO(ctx, localSO, ref, meta, source, generation, listed, state, lineage)
	relMtx()
	if err != nil {
		return "", err
	}

	// Denials answered the previous grant, not the checkpoint just installed.
	localSO.run.swapHealth((*sobject.SharedObjectHealth).WithoutSyncDenials)

	// Admission is complete only when body mounts can observe the accepted grant.
	if err := localSO.lsoHost.waitPublishedConfig(ctx, state.GetConfig()); err != nil {
		return "", errors.Wrap(err, "publish invited shared object state")
	}
	return listedGen, nil
}

// writeEnrolledSO writes the enrolled state into the mounted object and lists
// a newly installed one, if the id is still listed as listed was. Assumes mtx
// is locked.
func (a *ProviderAccount) writeEnrolledSO(
	ctx context.Context,
	localSO *SharedObject,
	ref *sobject.SharedObjectRef,
	meta *sobject.SharedObjectMeta,
	source string,
	generation *string,
	listed *sobject.SharedObjectListEntry,
	state *sobject.SOState,
	lineage []*sobject.SOConfigChange,
) (string, error) {
	// Refuse a lifetime other than the one the enrollment decided against.
	soID := ref.GetProviderResourceRef().GetId()
	current := a.listedSharedObjectEntry(soID)
	if (current == nil) != (listed == nil) || (current != nil && current.GetGeneration() != listed.GetGeneration()) {
		return "", errors.Errorf("shared object %s changed during the enrollment", soID)
	}

	// Install the invited state and lineage, whose grant lets the invitee
	// decrypt the SO data. A listed object of the same lifetime holds it, so
	// the state merges into the held one. Otherwise it replaces the
	// placeholder.
	install := localSO.soHost.InstallInviteSnapshot
	if listed != nil {
		install = localSO.soHost.MergeInviteSnapshot
	}
	if err := install(ctx, state, lineage); err != nil {
		return "", errors.Wrap(err, "install owner shared object state")
	}
	if listed != nil {
		return listed.GetGeneration(), nil
	}

	// List a new object in ID order, so it survives restarts and joins P2P
	// sync. Follows createSharedObjectLocked.
	soList := a.soListCtr.GetValue().CloneVT()
	if soList == nil {
		soList = &sobject.SharedObjectList{}
	}
	entry := newListEntry(ref, meta, source, generation)
	insertListEntry(soList, entry)
	if err := a.writeSharedObjectList(ctx, soList); err != nil {
		return "", errors.Wrap(err, "persist SO list")
	}
	a.soListCtr.SetValue(soList)
	return entry.GetGeneration(), nil
}

// listsSharedObject reports whether the list has an entry for the object.
func listsSharedObject(list *sobject.SharedObjectList, soID string) bool {
	return slices.ContainsFunc(list.GetSharedObjects(), func(entry *sobject.SharedObjectListEntry) bool {
		return entry.GetRef().GetProviderResourceRef().GetId() == soID
	})
}

// setSharedObjectUDPPeer records the static UDP address of a peer on the
// listed object, so every P2P sync generation dials it while the object stays
// listed.
func (a *ProviderAccount) setSharedObjectUDPPeer(ctx context.Context, soID string, remote peer.ID, addr string) error {
	return a.editListedSharedObject(ctx, soID, func(entry *sobject.SharedObjectListEntry) bool {
		if entry.GetUdpPeerAddrs()[remote.String()] == addr {
			return false
		}
		if entry.UdpPeerAddrs == nil {
			entry.UdpPeerAddrs = make(map[string]string, 1)
		}
		entry.UdpPeerAddrs[remote.String()] = addr
		return true
	})
}

// editListedSharedObject applies edit to the object's list entry, and
// persists and publishes the list when edit reports a change.
func (a *ProviderAccount) editListedSharedObject(ctx context.Context, soID string, edit func(*sobject.SharedObjectListEntry) bool) error {
	relMtx, err := a.mtx.Lock(ctx)
	if err != nil {
		return errors.Wrap(err, "lock account mutex")
	}
	defer relMtx()
	return a.editListedSharedObjectLocked(ctx, soID, edit)
}

// editListedSharedObjectLocked is editListedSharedObject with mtx held. An
// unlisted object returns ErrSharedObjectNotFound.
func (a *ProviderAccount) editListedSharedObjectLocked(ctx context.Context, soID string, edit func(*sobject.SharedObjectListEntry) bool) error {
	// Edit a copy of the listed entry.
	soList := a.soListCtr.GetValue().CloneVT()
	idx := slices.IndexFunc(soList.GetSharedObjects(), func(entry *sobject.SharedObjectListEntry) bool {
		return entry.GetRef().GetProviderResourceRef().GetId() == soID
	})
	if idx < 0 {
		return sobject.ErrSharedObjectNotFound
	}
	if !edit(soList.SharedObjects[idx]) {
		return nil
	}

	// Persist the list and publish it.
	if err := a.writeSharedObjectList(ctx, soList); err != nil {
		return errors.Wrap(err, "persist SO list")
	}
	a.soListCtr.SetValue(soList)
	return nil
}
