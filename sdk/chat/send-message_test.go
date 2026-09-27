package spacewave_chat

import (
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/world"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
)

// TestSendChatMessageReplay assigns positions from the accepted World, not the preparing Session.
func TestSendChatMessageReplay(t *testing.T) {
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	ws := world.NewEngineWorldState(tb.Engine, true)
	if _, _, err := ws.ApplyWorldOp(ctx, &CreateChatChannelOp{
		ObjectKey: GeneralChannelKey, Name: "General", Timestamp: timestamppb.Now(),
	}, tb.Volume.GetPeerID()); err != nil {
		t.Fatal(err)
	}

	for _, send := range []struct {
		device, person, text string
	}{
		{"device-a", "person-a", "first"},
		{"device-b", "person-b", "second"},
	} {
		intent := &SendChatMessageOp{
			ObjectKey:    GeneralChannelKey,
			Request:      &chat_rpc.SendMessageRequest{Text: send.text, TransactionId: "shared-transaction"},
			Timestamp:    timestamppb.Now(),
			SenderPeerId: send.device,
			PersonPeerId: send.person,
		}
		tx, err := world_block_tx.NewTxApplyWorldOp(intent, "")
		if err != nil {
			t.Fatal(err)
		}
		replayed, err := tx.LocateTx()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := replayed.ExecuteTx(ctx, "", LookupSendChatMessageOp, ws); err != nil {
			t.Fatal(err)
		}
	}

	reader := NewChatResource(ws, nil, GeneralChannelKey, "")
	page, err := reader.ListMessages(ctx, &chat_rpc.ListMessagesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.GetMessages()) != 2 {
		t.Fatalf("replayed history has %d messages, want 2", len(page.GetMessages()))
	}
	for i, want := range []struct{ device, person, text string }{
		{"device-a", "person-a", "first"},
		{"device-b", "person-b", "second"},
	} {
		message := page.GetMessages()[i]
		if message.GetIndex() != uint64(i) || message.GetSenderPeerId() != want.device || message.GetPersonPeerId() != want.person || message.GetText() != want.text {
			t.Fatalf("message %d lost accepted order or attribution: %+v", i, message)
		}
	}
}
