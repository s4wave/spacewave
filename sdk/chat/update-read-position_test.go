package spacewave_chat

import (
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/world"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/s4wave/spacewave/net/peer"
	chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
)

// TestReadPositionReplaySharesPerson verifies serialized receipts use the signer
// and accepted person supplied by replay instead of an operation body.
func TestReadPositionReplaySharesPerson(t *testing.T) {
	// Create a channel with one retained message for receipt replay.
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	ws := world.NewEngineWorldState(tb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")
	writer := newChatResourceForPerson(t, ws, tb.Engine, GeneralChannelKey, "writer-device", "writer")
	if _, err := writer.SendMessage(ctx, &chat_rpc.SendMessageRequest{Text: "message"}); err != nil {
		t.Fatal(err)
	}

	// Replay receipt advances from two devices of the same person.
	for _, device := range []peer.ID{"device-a", "device-b"} {
		// Serialize the receipt operation for the accepted device.
		op := &UpdateChatReadPositionOp{ObjectKey: GeneralChannelKey, NextIndex: 1, Timestamp: timestamppb.Now()}
		tx, err := world_block_tx.NewTxApplyWorldOp(op, tb.Volume.GetPeerID())
		if err != nil {
			t.Fatal(err)
		}

		// Require the serialized receipt to omit a delegated sender.
		if tx.GetTxApplyWorldOp().GetOpSender() != "" {
			t.Fatal("receipt retained a delegated sender")
		}

		// Replay the receipt with its accepted device and person.
		replayed, err := tx.LocateTx()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := replayed.ExecuteTx(world.WithOperationPerson(ctx, "person"), device, LookupUpdateChatReadPositionOp, ws); err != nil {
			t.Fatal(err)
		}
	}

	// Read the retained person receipt after both device replays.
	positions, err := writer.GetReadPositions(ctx, &chat_rpc.GetReadPositionsRequest{})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the replayed devices share one read position.
	if len(positions.GetPositions()) != 1 || positions.GetPositions()["person"].GetNextIndex() != 1 {
		t.Fatalf("replayed devices did not share one receipt: %v", positions)
	}
}
