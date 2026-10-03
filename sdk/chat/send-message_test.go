package spacewave_chat

import (
	"errors"
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/world"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/s4wave/spacewave/net/peer"
	chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
)

// TestChatOperationsRequireSigner rejects authored replay with no authenticated device.
func TestChatOperationsRequireSigner(t *testing.T) {
	ctx := world.WithOperationPerson(t.Context(), "person")
	for _, op := range []world.Operation{
		&SendChatMessageOp{ObjectKey: GeneralChannelKey, Request: &chat_rpc.SendMessageRequest{TransactionId: "send"}, Timestamp: timestamppb.Now()},
		&UpdateChatReadPositionOp{ObjectKey: GeneralChannelKey, NextIndex: 1, Timestamp: timestamppb.Now()},
		&CreateChatChannelOp{ObjectKey: GeneralChannelKey, Timestamp: timestamppb.Now(), InitialState: []*ChatStateChange{{Type: "m.room.create", ContentJson: `{}`}}},
	} {
		if _, err := op.ApplyWorldOp(ctx, nil, nil, ""); !errors.Is(err, ErrChatAuthorIdentityRequired) {
			t.Fatalf("%s without signer: %v", op.GetOperationTypeId(), err)
		}
	}
}

// TestSendChatMessageReplay assigns positions from the accepted World, not the preparing Session.
func TestSendChatMessageReplay(t *testing.T) {
	// Create an isolated channel for authenticated append replay.
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	ws := world.NewEngineWorldState(tb.Engine, true)
	if _, _, err := ws.ApplyWorldOp(ctx, &CreateChatChannelOp{
		ObjectKey: GeneralChannelKey, Name: "General", Timestamp: timestamppb.Now(),
	}, tb.Volume.GetPeerID()); err != nil {
		t.Fatal(err)
	}

	// Replay append intents from two accepted signing devices.
	for _, send := range []struct {
		device, person, text string
	}{
		{"device-a", "person-a", "first"},
		{"device-b", "person-b", "second"},
	} {
		// Serialize a channel append intent for the selected device.
		intent := &SendChatMessageOp{
			ObjectKey: GeneralChannelKey,
			Request:   &chat_rpc.SendMessageRequest{Text: send.text, TransactionId: "shared-transaction"},
			Timestamp: timestamppb.Now(),
		}
		tx, err := world_block_tx.NewTxApplyWorldOp(intent, tb.Volume.GetPeerID())
		if err != nil {
			t.Fatal(err)
		}

		// Require authenticated replay to omit a delegated sender.
		if tx.GetTxApplyWorldOp().GetOpSender() != "" {
			t.Fatal("authenticated chat operation retained a delegated sender")
		}

		// Execute the serialized append with the accepted device and person.
		replayed, err := tx.LocateTx()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := replayed.ExecuteTx(world.WithOperationPerson(ctx, send.person), peer.ID(send.device), LookupSendChatMessageOp, ws); err != nil {
			t.Fatal(err)
		}
	}

	// Read the channel history after both replayed appends.
	reader := newChatResource(t, ws, nil, GeneralChannelKey, "")
	page, err := reader.ListMessages(ctx, &chat_rpc.ListMessagesRequest{})
	if err != nil {
		t.Fatal(err)
	}

	// Verify replay retained message count, order, and attribution.
	if len(page.GetMessages()) != 2 {
		t.Fatalf("replayed history has %d messages, want 2", len(page.GetMessages()))
	}
	for i, want := range []struct{ device, person, text string }{
		{"device-a", "person-a", "first"},
		{"device-b", "person-b", "second"},
	} {
		// Compare the retained message with its accepted device, person, and position.
		message := page.GetMessages()[i]
		if message.GetIndex() != uint64(i) || message.GetSenderPeerId() != peer.ID(want.device).String() || message.GetPersonId() != want.person || message.GetText() != want.text {
			t.Fatalf("message %d lost accepted order or attribution: %+v", i, message)
		}
	}
}
