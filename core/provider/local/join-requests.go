package provider_local

import (
	"context"
	"slices"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
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

// QueueJoinRequest holds a redemption as a pending join request. A later
// request by the same peer replaces the earlier one.
func (s *SharedObject) QueueJoinRequest(ctx context.Context, joinResp *sobject.SOJoinResponse) error {
	peerID := joinResp.GetResponderPeerId()
	return s.updateJoinRequests(ctx, func(requests []*sobject.SOJoinRequest) ([]*sobject.SOJoinRequest, error) {
		requests = slices.DeleteFunc(requests, func(req *sobject.SOJoinRequest) bool {
			return req.GetJoinResponse().GetResponderPeerId() == peerID
		})
		return append(requests, &sobject.SOJoinRequest{
			JoinResponse: joinResp.CloneVT(),
			CreatedAt:    timestamppb.Now(),
		}), nil
	})
}

// RemoveJoinRequest removes the pending request of peerID.
func (s *SharedObject) RemoveJoinRequest(ctx context.Context, peerID string) error {
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

// _ is a type assertion.
var _ sobject.JoinRequestHost = (*SharedObject)(nil)
