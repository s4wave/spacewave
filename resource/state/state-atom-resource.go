package resource_state

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
)

// StateAtomResource wraps a StateAtomStore for resource access.
// It implements the StateAtomResourceService RPC interface.
type StateAtomResource struct {
	store StateAtomStore
	mux   srpc.Invoker
}

// NewStateAtomResource creates a new StateAtomResource.
func NewStateAtomResource(store StateAtomStore) *StateAtomResource {
	r := &StateAtomResource{store: store}
	mux := srpc.NewMux()
	_ = SRPCRegisterStateAtomResourceService(mux, r)
	r.mux = mux
	return r
}

// GetMux returns the RPC mux.
func (r *StateAtomResource) GetMux() srpc.Invoker {
	return r.mux
}

// GetState returns the current state.
func (r *StateAtomResource) GetState(
	ctx context.Context,
	req *GetStateRequest,
) (*GetStateResponse, error) {
	stateJson, seqno, err := r.store.Get(ctx)
	if err != nil {
		return nil, err
	}
	return &GetStateResponse{
		StateJson: stateJson,
		Seqno:     seqno,
	}, nil
}

// SetState updates the state.
func (r *StateAtomResource) SetState(
	ctx context.Context,
	req *SetStateRequest,
) (*SetStateResponse, error) {
	seqno, err := r.store.Set(ctx, req.GetStateJson())
	if err != nil {
		return nil, err
	}
	return &SetStateResponse{Seqno: seqno}, nil
}

// WatchState watches for state changes.
func (r *StateAtomResource) WatchState(
	req *WatchStateRequest,
	strm SRPCStateAtomResourceService_WatchStateStream,
) error {
	ctx := strm.Context()
	var prevSeqno uint64

	for {
		stateJson, seqno, err := r.store.Get(ctx)
		if err != nil {
			return err
		}

		if err := strm.Send(&WatchStateResponse{
			StateJson: stateJson,
			Seqno:     seqno,
		}); err != nil {
			return err
		}

		prevSeqno = seqno

		// Wait for next change
		if _, err := r.store.WaitSeqno(ctx, prevSeqno+1); err != nil {
			return err
		}
	}
}

// _ is a type assertion
var _ SRPCStateAtomResourceServiceServer = (*StateAtomResource)(nil)
