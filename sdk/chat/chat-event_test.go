package spacewave_chat

import (
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/world"
	db_world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/s4wave/spacewave/net/peer"
	chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
)

// TestProtocolEventHistory retains extension bodies without changing state or encryption policy.
func TestProtocolEventHistory(t *testing.T) {
	// Start an isolated World for extension event history.
	ctx := t.Context()
	tb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Create the channel and attach the accepted event author.
	ws := world.NewEngineWorldState(tb.Engine, true)
	const key = "chat/channel/events"
	if _, _, err := ws.ApplyWorldOp(ctx, &CreateChatChannelOp{ObjectKey: key, Name: "Events", Timestamp: timestamppb.Now()}, tb.Volume.GetPeerID()); err != nil {
		t.Fatal(err)
	}
	channel := newChatResourceForPerson(t, ws, tb.Engine, key, "device", "person")
	t.Cleanup(channel.Close)

	// Send the extension event and verify its retry identity.
	request := &chat_rpc.SendMessageRequest{TransactionId: "event", Content: &ChatMessageContent{Content: &ChatMessageContent_Event{Event: &ChatEvent{Type: "m.room.test", ContentJson: `{"nested":{"values":[1,true,null,"text"]}}`}}}}
	sent, err := channel.SendMessage(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := channel.SendMessage(ctx, request)
	if err != nil || retry.GetMessageKey() != sent.GetMessageKey() {
		t.Fatalf("retry changed identity: %v %v", retry, err)
	}

	// A new Resource reads the stored body and attribution through ordinary history.
	reader := newChatResourceForPerson(t, ws, tb.Engine, key, "other-device", "other-person")
	t.Cleanup(reader.Close)
	history, err := reader.ListMessages(ctx, &chat_rpc.ListMessagesRequest{})
	if err != nil || len(history.GetMessages()) != 1 {
		t.Fatalf("history: %v %v", history, err)
	}
	message := history.GetMessages()[0]
	if !message.GetContent().EqualVT(request.GetContent()) || message.GetPersonId() != "person" || message.GetSenderPeerId() != peer.ID("device").String() || message.GetText() != "Unsupported message" {
		t.Fatalf("stored event changed: %v", message)
	}
	state, err := reader.GetState(ctx, &chat_rpc.GetStateRequest{})
	if err != nil || len(state.GetMessages()) != 0 {
		t.Fatalf("timeline event replaced channel state: %v %v", state, err)
	}

	// Extension events retain native relationships and reject unavailable targets.
	related := request.CloneVT()
	related.TransactionId = "related-event"
	related.Content.GetEvent().Type = "m.room.related"
	related.Content.GetEvent().ContentJson = `{"body":"related"}`
	related.Content.GetEvent().Relation = &ChatRelation{Type: "m.thread", TargetKey: sent.GetMessageKey()}
	relationResult, err := channel.SendMessage(ctx, related)
	if err != nil {
		t.Fatal(err)
	}

	// Read and verify the stored extension event relationship.
	relationRead, err := reader.GetMessage(ctx, &chat_rpc.GetMessageRequest{MessageKey: relationResult.GetMessageKey()})
	if err != nil || !relationRead.GetMessage().GetContent().EqualVT(related.GetContent()) {
		t.Fatalf("stored event relationship changed: %v %v", relationRead, err)
	}

	// Require an unavailable event relation target to fail.
	invalidRelation := related.CloneVT()
	invalidRelation.TransactionId = "invalid-relation"
	invalidRelation.Content.GetEvent().Relation.TargetKey = key + "/message/missing"
	if _, err := channel.SendMessage(ctx, invalidRelation); err == nil {
		t.Fatal("accepted an unavailable event relationship")
	}

	// Conflicting retries and malformed bodies leave history unchanged.
	conflict := request.CloneVT()
	conflict.Content.GetEvent().ContentJson = `{}`
	if _, err := channel.SendMessage(ctx, conflict); err == nil {
		t.Fatal("changed event reused an accepted transaction")
	}
	for _, body := range []string{"[]", "null", "{"} {
		invalid := request.CloneVT()
		invalid.TransactionId = "invalid"
		invalid.Content.GetEvent().ContentJson = body
		if _, err := channel.SendMessage(ctx, invalid); err == nil {
			t.Fatal("accepted malformed event body")
		}
	}
	info, err := reader.GetChannelInfo(ctx, &chat_rpc.GetChannelInfoRequest{})
	if err != nil || info.GetMessageCount() != 2 {
		t.Fatalf("rejected sends advanced history: %v %v", info, err)
	}

	// An extension body cannot bypass encryption by naming another protocol type.
	const encryptedKey = "chat/channel/encrypted-events"
	if _, _, err := ws.ApplyWorldOp(ctx, &CreateChatChannelOp{ObjectKey: encryptedKey, Name: "Encrypted", Timestamp: timestamppb.Now(), EncryptionAlgorithm: "m.megolm.v1.aes-sha2"}, tb.Volume.GetPeerID()); err != nil {
		t.Fatal(err)
	}
	encrypted := newChatResourceForPerson(t, ws, tb.Engine, encryptedKey, "device", "person")
	t.Cleanup(encrypted.Close)
	if _, err := encrypted.SendMessage(ctx, request); err == nil {
		t.Fatal("accepted a plaintext extension body in an encrypted channel")
	}
}
