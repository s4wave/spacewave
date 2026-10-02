package provider_local

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_invite "github.com/s4wave/spacewave/core/sobject/invite"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// LeaveSharedObject relinquishes this account's storage and calling device grants.
// The native owner acknowledges removal before this account installs revocation.
// A hosting owner that others depend on first transfers ownership to successor,
// or the default successor when empty, and returns once the promotion commits;
// the successor commits the departure and takes over hosting.
// Existing local data stays retained; this operation does not delete the shared object.
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

	// Each participating key consents separately; no caller-supplied peer label grants authority.
	var keys []crypto.PrivKey
	var departing []string
	for _, key := range []crypto.PrivKey{local.localPriv, sessionKey} {
		id, err := peer.IDFromPrivateKey(key)
		if err != nil {
			return err
		}
		if !slices.Contains(departing, id.String()) && slices.ContainsFunc(state.GetConfig().GetParticipants(), func(p *sobject.SOParticipantConfig) bool { return p.GetPeerId() == id.String() }) {
			keys = append(keys, key)
			departing = append(departing, id.String())
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

	// A hosted object commits a final departure locally and hands others to a successor.
	if entry.GetTransportPeerId() == "" {
		remains := slices.ContainsFunc(state.GetConfig().GetParticipants(), func(p *sobject.SOParticipantConfig) bool {
			return !slices.Contains(departing, p.GetPeerId())
		})
		if !remains {
			_, err := sobject.LeaveSOParticipants(ctx, local.soHost, local.localPriv, request)
			return err
		}
		if _, err := sobject.TransferSOOwnership(ctx, local.soHost, local.localPriv, successor, request); err != nil {
			return err
		}
		return a.reconcileSOHosting(ctx, id, local)
	}
	ownerID, err := peer.IDB58Decode(entry.GetTransportPeerId())
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
		current, err = sobject.VerifyConfigChange(current, change)
		if err != nil {
			return err
		}
	}
	if len(response.GetChanges()) == 0 || slices.ContainsFunc(current.GetParticipants(), func(p *sobject.SOParticipantConfig) bool { return slices.Contains(departing, p.GetPeerId()) }) {
		return errors.New("leave response does not prove participant removal")
	}
	err = local.soHost.ImportPeerSnapshot(ctx, &sobject.SOState{Config: current}, response.GetChanges(), local.GetPeerID(), nil)
	if errors.Is(err, sobject.ErrParticipantRevoked) {
		return nil
	}
	return err
}

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
