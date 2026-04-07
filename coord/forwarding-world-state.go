package coord

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/world"
	world_block_tx "github.com/aperturerobotics/hydra/world/block/tx"
	"github.com/pkg/errors"
)

// ForwardingWorldState wraps a read-only WorldState and routes write
// operations (ApplyWorldOp) through the leader's SubmitWorldOp SRPC.
// Read operations go through the local follower engine directly.
type ForwardingWorldState struct {
	world.WorldState
	client SRPCCoordinatorServiceClient
}

// NewForwardingWorldState creates a forwarding world state.
// The underlying WorldState should be backed by the follower engine (read-only).
// The client is used to forward write operations to the leader.
func NewForwardingWorldState(ws world.WorldState, client SRPCCoordinatorServiceClient) *ForwardingWorldState {
	return &ForwardingWorldState{WorldState: ws, client: client}
}

// GetReadOnly returns false since this state supports writes (via forwarding).
func (f *ForwardingWorldState) GetReadOnly() bool {
	return false
}

// ApplyWorldOp serializes the operation and forwards it to the leader via SRPC.
// Returns the new seqno and any error from the leader.
func (f *ForwardingWorldState) ApplyWorldOp(
	ctx context.Context,
	op world.Operation,
	sender peer.ID,
) (uint64, bool, error) {
	tx, err := world_block_tx.NewTxApplyWorldOp(op)
	if err != nil {
		return 0, false, errors.Wrap(err, "serialize world op")
	}
	data, err := tx.MarshalVT()
	if err != nil {
		return 0, false, errors.Wrap(err, "marshal world op tx")
	}

	resp, err := f.client.SubmitWorldOp(ctx, &SubmitWorldOpRequest{OpData: data})
	if err != nil {
		return 0, true, errors.Wrap(err, "submit world op to leader")
	}
	if errMsg := resp.GetError(); errMsg != "" {
		return 0, false, errors.New(errMsg)
	}
	return resp.GetSeqno(), false, nil
}

// _ is a type assertion.
var _ world.WorldState = (*ForwardingWorldState)(nil)
