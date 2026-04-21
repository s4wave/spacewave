//go:build !js

package coord

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
)

// coordinatorServiceServer implements SRPCCoordinatorServiceServer.
// Registered on the leader's SRPC mux to handle follower requests.
type coordinatorServiceServer struct {
	handler  *WorldRoleHandler
	lookupOp world.LookupOp
}

// NewCoordinatorServiceServer creates a CoordinatorService SRPC server.
// The handler provides access to the world engine. The lookupOp resolves
// operation types when executing submitted transactions.
func NewCoordinatorServiceServer(handler *WorldRoleHandler, lookupOp world.LookupOp) SRPCCoordinatorServiceServer {
	return &coordinatorServiceServer{
		handler:  handler,
		lookupOp: lookupOp,
	}
}

// SubmitWorldOp deserializes and executes a world transaction on the leader.
func (s *coordinatorServiceServer) SubmitWorldOp(ctx context.Context, req *SubmitWorldOpRequest) (*SubmitWorldOpResponse, error) {
	eng := s.handler.GetEngine()
	if eng == nil {
		return &SubmitWorldOpResponse{Error: "not leader"}, nil
	}

	// Deserialize the world tx.
	var wbTx world_block_tx.Tx
	if err := wbTx.UnmarshalVT(req.GetOpData()); err != nil {
		return &SubmitWorldOpResponse{Error: errors.Wrap(err, "unmarshal tx").Error()}, nil
	}
	if err := wbTx.Validate(); err != nil {
		return &SubmitWorldOpResponse{Error: errors.Wrap(err, "validate tx").Error()}, nil
	}
	subTx, err := wbTx.LocateTx()
	if err != nil {
		return &SubmitWorldOpResponse{Error: errors.Wrap(err, "locate tx").Error()}, nil
	}

	// Create a write transaction on the engine.
	etx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		return &SubmitWorldOpResponse{Error: errors.Wrap(err, "new transaction").Error()}, nil
	}
	defer etx.Discard()

	// Execute the sub-transaction against the world state.
	_, txErr := subTx.ExecuteTx(ctx, "", s.lookupOp, etx)
	if txErr != nil {
		return &SubmitWorldOpResponse{Error: txErr.Error()}, nil
	}

	// Commit.
	if err := etx.Commit(ctx); err != nil {
		return &SubmitWorldOpResponse{Error: errors.Wrap(err, "commit").Error()}, nil
	}

	// Return the current seqno.
	seqno, err := eng.GetSeqno(ctx)
	if err != nil {
		return &SubmitWorldOpResponse{Error: errors.Wrap(err, "get seqno").Error()}, nil
	}
	return &SubmitWorldOpResponse{Seqno: seqno}, nil
}

// WatchWorldSeqno streams seqno updates to followers.
func (s *coordinatorServiceServer) WatchWorldSeqno(req *WatchWorldSeqnoRequest, stream SRPCCoordinatorService_WatchWorldSeqnoStream) error {
	ctx := stream.Context()
	eng, err := s.handler.WaitEngine(ctx)
	if err != nil {
		return err
	}

	lastSeen := req.GetLastSeenSeqno()
	for {
		seqno, err := eng.WaitSeqno(ctx, lastSeen+1)
		if err != nil {
			return err
		}
		if err := stream.Send(&WatchWorldSeqnoResponse{Seqno: seqno}); err != nil {
			return err
		}
		lastSeen = seqno
	}
}

// _ is a type assertion.
var _ SRPCCoordinatorServiceServer = (*coordinatorServiceServer)(nil)
