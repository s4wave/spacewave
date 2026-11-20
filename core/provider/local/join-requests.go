package provider_local

import (
	"context"
	"slices"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_invite "github.com/s4wave/spacewave/core/sobject/invite"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// readJoinRequests loads the pending join requests of a shared object.
func readJoinRequests(ctx context.Context, objStore object.ObjectStore, sharedObjectID string) (*sobject.SOJoinRequestList, error) {
	key := SobjectObjectStoreJoinRequestsKey(sharedObjectID)
	list := &sobject.SOJoinRequestList{}
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			data, found, err := tx.Get(ctx, key)
			if err != nil || !found {
				return err
			}
			return list.UnmarshalVT(data)
		},
	)
	return list, err
}

// GetJoinRequestsCtr returns the pending join requests, at most one per peer.
func (s *SharedObject) GetJoinRequestsCtr() ccontainer.Watchable[*sobject.SOJoinRequestList] {
	return s.joinRequests
}

// QueueJoinRequest holds a redemption, signed by the requester and its storage
// peer, as a pending join request. A later request by the same peer replaces
// the earlier one.
func (s *SharedObject) QueueJoinRequest(ctx context.Context, joinResp, storageJoinResp *sobject.SOJoinResponse) error {
	peerID := joinResp.GetResponderPeerId()
	return s.updateJoinRequests(ctx, func(requests []*sobject.SOJoinRequest) ([]*sobject.SOJoinRequest, error) {
		requests = slices.DeleteFunc(requests, func(req *sobject.SOJoinRequest) bool {
			return req.GetJoinResponse().GetResponderPeerId() == peerID
		})
		return append(requests, &sobject.SOJoinRequest{
			JoinResponse:        joinResp.CloneVT(),
			CreatedAt:           timestamppb.Now(),
			StorageJoinResponse: storageJoinResp.CloneVT(),
		}), nil
	})
}

// removeJoinRequest removes the pending request of peerID.
func (s *SharedObject) removeJoinRequest(ctx context.Context, peerID string) error {
	return s.updateJoinRequests(ctx, func(requests []*sobject.SOJoinRequest) ([]*sobject.SOJoinRequest, error) {
		next := slices.DeleteFunc(requests, func(req *sobject.SOJoinRequest) bool {
			return req.GetJoinResponse().GetResponderPeerId() == peerID
		})
		if len(next) == len(requests) {
			return nil, errors.New("no pending join request from peer")
		}
		return next, nil
	})
}

// WithdrawJoinRequest removes the pending request of peerID, if any.
func (s *SharedObject) WithdrawJoinRequest(ctx context.Context, peerID string) error {
	return s.updateJoinRequests(ctx, func(requests []*sobject.SOJoinRequest) ([]*sobject.SOJoinRequest, error) {
		return slices.DeleteFunc(requests, func(req *sobject.SOJoinRequest) bool {
			return req.GetJoinResponse().GetResponderPeerId() == peerID
		}), nil
	})
}

// dropInviteJoinRequests removes the pending requests that redeemed inviteID.
func (s *SharedObject) dropInviteJoinRequests(ctx context.Context, inviteID string) error {
	// Skip the write when no request names the invite.
	redeemed := func(req *sobject.SOJoinRequest) bool {
		return req.GetJoinResponse().GetInviteId() == inviteID
	}
	if !slices.ContainsFunc(s.joinRequests.GetValue().GetRequests(), redeemed) {
		return nil
	}
	return s.updateJoinRequests(ctx, func(requests []*sobject.SOJoinRequest) ([]*sobject.SOJoinRequest, error) {
		return slices.DeleteFunc(requests, redeemed), nil
	})
}

// updateJoinRequests persists an edit of the pending requests, then publishes it.
func (s *SharedObject) updateJoinRequests(
	ctx context.Context,
	edit func([]*sobject.SOJoinRequest) ([]*sobject.SOJoinRequest, error),
) error {
	// Serialize edits so each starts from the last published list.
	rel, err := s.joinRequestsMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer rel()

	// Apply the edit to a copy of the current list.
	requests, err := edit(s.joinRequests.GetValue().CloneVT().GetRequests())
	if err != nil {
		return err
	}
	next := &sobject.SOJoinRequestList{Requests: requests}

	// Write the list to the object store before publishing it.
	data, err := next.MarshalVT()
	if err != nil {
		return err
	}
	key := SobjectObjectStoreJoinRequestsKey(s.GetSharedObjectID())
	err = kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return s.objStore.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			return tx.Set(ctx, key, data)
		},
	)
	if err != nil {
		return err
	}
	s.joinRequests.SetValue(next)
	return nil
}

// ResolveJoinRequest grants or refuses the pending join request of peerID to
// the Space sharedObjectID, then removes the request. A grant admits the
// requester and its storage peer at the role of the invite it redeemed.
//
// A grant is the owner's decision under a usable invite: one revoked or used
// up since the request refuses it. The admitted requester redeems the invite
// again to receive its grant.
func (a *ProviderAccount) ResolveJoinRequest(ctx context.Context, sharedObjectID, peerID string, grant bool) error {
	// Mount the Space and find the peer's request.
	so, relSO, err := a.mountListedSharedObject(ctx, sharedObjectID)
	if err != nil {
		return err
	}
	if so == nil {
		return errors.New("shared object is not held")
	}
	defer relSO()
	idx := slices.IndexFunc(so.joinRequests.GetValue().GetRequests(), func(req *sobject.SOJoinRequest) bool {
		return req.GetJoinResponse().GetResponderPeerId() == peerID
	})
	if idx < 0 {
		return errors.New("no pending join request from peer")
	}
	request := so.joinRequests.GetValue().GetRequests()[idx]
	if !grant {
		return so.removeJoinRequest(ctx, peerID)
	}

	// Verify the queued redemption and the invite it names.
	redeemer, err := sobject_invite.ValidateRedeemer(request.GetJoinResponse(), request.GetStorageJoinResponse())
	if err != nil {
		return errors.Wrap(err, "validate join request")
	}
	state, err := so.soHost.GetHostState(ctx)
	if err != nil {
		return err
	}
	invite := sobject.FindInvite(state, request.GetJoinResponse().GetInviteId())
	if invite == nil {
		return errors.New("redeemed invite no longer exists")
	}
	if err := sobject.ValidateInviteRedeemable(invite, peerID); err != nil {
		return errors.Wrap(err, "invite not usable")
	}

	// Admit the requester and its storage peer, then drop the request.
	result, err := a.inviteLookupResult(ctx, so, invite)
	if err != nil {
		return err
	}
	if _, _, _, err := sobject_invite.Admit(ctx, a.enrollInvitee, result, redeemer); err != nil {
		return err
	}
	return so.removeJoinRequest(ctx, peerID)
}

// inviteLookupResult returns the owner authority over invite on so: the
// Space's host and the volume key that signs its config changes.
func (a *ProviderAccount) inviteLookupResult(ctx context.Context, so *SharedObject, invite *sobject.SOInvite) (*sobject_invite.InviteLookupResult, error) {
	// The volume key owns every Space the account hosts.
	volPeer, err := a.vol.GetPeer(ctx, true)
	if err != nil {
		return nil, err
	}
	volPriv, err := volPeer.GetPrivKey(ctx)
	if err != nil {
		return nil, err
	}
	return &sobject_invite.InviteLookupResult{
		Host:           so.soHost,
		InviteMutator:  so,
		Invite:         invite,
		SharedObjectID: so.GetSharedObjectID(),
		OwnerPrivKey:   volPriv,
	}, nil
}

// enrollInvitee adds an invitee to the Space of result at the invite's role
// under the owner key, and keeps the invitee connected. Local participants
// have no entity.
func (a *ProviderAccount) enrollInvitee(
	ctx context.Context,
	result *sobject_invite.InviteLookupResult,
	inviteePeerID peer.ID,
	inviteePubKey crypto.PubKey,
) (*sobject.SOGrant, sobject.SOParticipantAddOutcome, sobject.SOParticipantRole, error) {
	// Add the invitee under the owner key.
	ownerPeerID, err := peer.IDFromPrivateKey(result.OwnerPrivKey)
	if err != nil {
		return nil, 0, 0, err
	}
	grant, outcome, held, err := sobject.AddSOParticipant(
		ctx,
		result.Host,
		result.SharedObjectID,
		result.OwnerPrivKey,
		ownerPeerID.String(),
		inviteePeerID.String(),
		inviteePubKey,
		result.Invite.GetRole(),
		"",
		"",
	)
	if err != nil {
		return nil, 0, 0, err
	}

	// Keep the invitee connected once enrolled, whether or not the invite
	// targeted it: the invitee's own dial does not hold the link open. The
	// storage peer enrolls here too and waits in signaling until it dials.
	a.clearP2PPendingEnrollPeer(inviteePeerID)
	if err := a.RetainP2PPeer(ctx, inviteePeerID); err != nil {
		return nil, 0, 0, errors.Wrap(err, "retain enrolled peer")
	}
	return grant, outcome, held, nil
}

// _ is a type assertion.
var _ sobject.JoinRequestHost = (*SharedObject)(nil)
