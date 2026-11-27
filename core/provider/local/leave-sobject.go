package provider_local

import (
	"context"
	"slices"
	"sync/atomic"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_invite "github.com/s4wave/spacewave/core/sobject/invite"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// LeaveSharedObject relinquishes this account's storage and calling device grants.
// The native owner acknowledges removal before this account installs revocation.
// A hosting owner that others depend on first transfers ownership to successor,
// or the default successor when empty, and returns once the promotion commits.
// The successor commits the departure and takes over hosting.
// Existing local data stays retained. This operation does not delete the shared
// object.
func (a *ProviderAccount) LeaveSharedObject(ctx context.Context, sessionKey crypto.PrivKey, id, successor string) error {
	// Resolve only an object already held by this provider account.
	list := a.soListCtr.GetValue()
	index := slices.IndexFunc(list.GetSharedObjects(), func(entry *sobject.SharedObjectListEntry) bool {
		return entry.GetRef().GetProviderResourceRef().GetId() == id
	})
	if index == -1 {
		return sobject.ErrSharedObjectNotFound
	}

	// Mount the object and read its host state.
	entry := list.GetSharedObjects()[index]
	mounted, release, err := a.MountSharedObject(ctx, entry.GetRef(), nil)
	if err != nil {
		return err
	}
	defer release()
	local, ok := mounted.(*SharedObject)
	if !ok {
		return errors.New("leave requires a local shared object")
	}
	state, err := local.soHost.GetHostState(ctx)
	if err != nil {
		return err
	}

	// Each participating key consents separately. No caller-supplied peer
	// label grants authority.
	participants := participantSet(state.GetConfig().GetParticipants())
	var keys []crypto.PrivKey
	departing := make(map[string]struct{}, 2)
	for _, key := range []crypto.PrivKey{local.localPriv, sessionKey} {
		id, err := peer.IDFromPrivateKey(key)
		if err != nil {
			return err
		}
		_, participates := participants[id.String()]
		_, consented := departing[id.String()]
		if participates && !consented {
			keys = append(keys, key)
			departing[id.String()] = struct{}{}
		}
	}

	// Build the signed leave request from the consenting keys.
	if len(keys) == 0 {
		return nil
	}
	request, err := sobject.BuildSOLeaveRequest(id, state.GetConfig().GetConfigChainHash(), keys...)
	if err != nil {
		return err
	}

	// An owner already handing over waits for its successor to commit the
	// departure it requested. A second transfer would fork the config
	// chain.
	own, err := sobject.ReadSOOwnership(ctx, state.GetConfig(), local.soHost.ReadConfigEntry)
	if err != nil {
		return err
	}
	if own.Departing(local.localPid.String()) {
		return sobject.ErrHandingOver
	}

	// A hosted object commits a final departure locally and hands others to a successor.
	if own.Owns(local.localPid.String()) {
		// Departing keys are a subset of the participants.
		if len(participants) == len(departing) {
			_, err := sobject.LeaveSOParticipants(ctx, local.soHost, local.localPriv, request)
			return err
		}
		if _, err := sobject.TransferSOOwnership(ctx, local.soHost, local.localPriv, successor, request); err != nil {
			return err
		}
		return a.reconcileSOHosting(ctx, id, local)
	}

	// A joined object asks one linked owner to commit the departure.
	ownerID, err := a.waitLinkedOwner(ctx, local.soHost)
	if err != nil {
		return err
	}
	transport := a.GetSessionTransport()
	if transport == nil || transport.GetChildBus() == nil {
		return errors.New("leave requires the mounted session transport")
	}
	response, err := sobject_invite.LeaveSharedObject(ctx, transport.GetChildBus(), transport.GetPeerID(), ownerID, request)
	if err != nil {
		return err
	}

	// Adopt only the owner's signed proof, with no remote channel content or renewed grant.
	current := state.GetConfig()
	for _, change := range response.GetChanges() {
		current, err = sobject.VerifyConfigChange(id, current, change)
		if err != nil {
			return err
		}
	}
	remaining := participantSet(current.GetParticipants())
	removed := len(response.GetChanges()) != 0
	for id := range departing {
		if _, ok := remaining[id]; ok {
			removed = false
		}
	}
	if !removed {
		return errors.New("leave response does not prove participant removal")
	}
	err = local.soHost.ImportPeerSnapshot(ctx, &sobject.SOState{Config: current}, response.GetChanges(), local.GetPeerID(), nil)
	if err != nil && !errors.Is(err, sobject.ErrParticipantRevoked) {
		return err
	}

	// A departed member no longer dials the owner's static address.
	return a.editListedSharedObject(ctx, id, func(entry *sobject.SharedObjectListEntry) bool {
		if len(entry.GetUdpPeerAddrs()) == 0 {
			return false
		}
		entry.UdpPeerAddrs = nil
		return true
	})
}

// waitLinkedOwner returns an owner of host with a live link, waiting until
// one links.
// The owners come from the current state on each wake, because sync may list
// a new owner, such as the Session an owner enrolls after this member joined.
// The object's sync retains every owner, so the wait needs no dial of its own.
// One owner receives the request, because two owners committing the same
// departure would fork the config chain.
func (a *ProviderAccount) waitLinkedOwner(ctx context.Context, host *sobject.SOHost) (peer.ID, error) {
	// Watch the state that lists the owners.
	states, release, err := host.GetSOStateCtr(ctx, nil)
	if err != nil {
		return "", err
	}
	defer release()

	for {
		// Read the owners and their links from the current state.
		state, err := states.WaitValue(ctx, nil)
		if err != nil {
			return "", err
		}
		own, err := sobject.ReadSOOwnership(ctx, state.GetConfig(), host.ReadConfigEntry)
		if err != nil {
			return "", err
		}
		online, linkChs := a.GetOnlinePeerIDsWithWait(own.Owners())
		if len(online) != 0 {
			return peer.IDB58Decode(online[0])
		}
		if hook := leaveOwnerWaitHook.Load(); hook != nil {
			(*hook)()
		}

		// Wake on a new state or on any link change, whichever comes first.
		waitCtx, cancel := context.WithCancel(ctx)
		go func() {
			_ = broadcast.WaitAny(waitCtx, linkChs...)
			cancel()
		}()
		_, err = states.WaitValueChange(waitCtx, state, nil)
		cancel()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		if err != nil && !errors.Is(err, context.Canceled) {
			return "", err
		}
	}
}

// leaveOwnerWaitHook, when set by a test, runs each time a leave finds no
// linked owner and is about to wait. It is nil in production.
// Tests replace it while leave routines run, so each call loads one snapshot.
var leaveOwnerWaitHook atomic.Pointer[func()]

// acceptSharedObjectLeave holds the addressed host until its signed removal is committed.
func (a *ProviderAccount) acceptSharedObjectLeave(ctx context.Context, request *sobject.SOLeaveRequest) (*sobject.SOLeaveResponse, error) {
	// A transport request cannot mount an unlisted object by guessing its identifier.
	list := a.soListCtr.GetValue()
	index := slices.IndexFunc(list.GetSharedObjects(), func(entry *sobject.SharedObjectListEntry) bool {
		return entry.GetRef().GetProviderResourceRef().GetId() == request.GetSharedObjectId()
	})
	if index == -1 {
		return nil, sobject.ErrSharedObjectNotFound
	}
	mounted, release, err := a.MountSharedObject(ctx, list.GetSharedObjects()[index].GetRef(), nil)
	if err != nil {
		return nil, err
	}
	defer release()
	local, ok := mounted.(*SharedObject)
	if !ok {
		return nil, errors.New("leave requires a local shared object")
	}

	// Configuration validation independently requires this provider to hold owner authority.
	return sobject.LeaveSOParticipants(ctx, local.soHost, local.localPriv, request)
}

// participantSet indexes participants by peer ID.
func participantSet(participants []*sobject.SOParticipantConfig) map[string]struct{} {
	set := make(map[string]struct{}, len(participants))
	for _, p := range participants {
		set[p.GetPeerId()] = struct{}{}
	}
	return set
}
