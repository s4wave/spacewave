package spacewave_chat

import (
	"errors"
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/world"
	db_world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
)

// TestRedactionStripsOwnBodies checks that a person removes only their own bodies and keeps routing.
func TestRedactionStripsOwnBodies(t *testing.T) {
	// Create an encrypted channel so redactions must bypass the body policy.
	ctx := t.Context()
	tb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	ws := world.NewEngineWorldState(tb.Engine, true)
	const channelKey = "chat/channel/redaction"
	const algorithm = "m.megolm.v1.aes-sha2"
	if _, _, err := ws.ApplyWorldOp(ctx, &CreateChatChannelOp{ObjectKey: channelKey, Name: "Redaction", Timestamp: timestamppb.Now(), EncryptionAlgorithm: algorithm}, tb.Volume.GetPeerID()); err != nil {
		t.Fatal(err)
	}

	// Alice and Bob write through their own devices.
	alice := newChatResourceForPerson(t, ws, tb.Engine, channelKey, "alice-device", "alice")
	t.Cleanup(alice.Close)
	bob := newChatResourceForPerson(t, ws, tb.Engine, channelKey, "bob-device", "bob")
	t.Cleanup(bob.Close)

	// Alice sends a thread root and an encrypted reply in it.
	body := &ChatCiphertext{Algorithm: algorithm, Ciphertext: "opaque", SenderKey: "sender", SessionId: "session"}
	root, err := alice.SendMessage(ctx, &chat_rpc.SendMessageRequest{TransactionId: "root", Content: &ChatMessageContent{Content: &ChatMessageContent_Ciphertext{Ciphertext: body}}})
	if err != nil {
		t.Fatal(err)
	}
	reply := body.CloneVT()
	reply.Relation = &ChatRelation{Type: "m.thread", TargetKey: root.GetMessageKey(), ReplyToKey: root.GetMessageKey(), IsFallingBack: true}
	replyRequest := &chat_rpc.SendMessageRequest{TransactionId: "reply", Content: &ChatMessageContent{Content: &ChatMessageContent_Ciphertext{Ciphertext: reply}}}
	sent, err := alice.SendMessage(ctx, replyRequest)
	if err != nil {
		t.Fatal(err)
	}

	// Each redaction names one target with a public reason.
	redaction := func(target string) *chat_rpc.SendMessageRequest {
		return &chat_rpc.SendMessageRequest{Content: &ChatMessageContent{Content: &ChatMessageContent_Redaction{Redaction: &ChatRedaction{TargetKey: target, Reason: "typo"}}}}
	}

	// Another person cannot remove the body.
	if _, err := bob.SendMessage(ctx, redaction(sent.GetMessageKey())); !errors.Is(err, ErrChatRedactionForbidden) {
		t.Fatalf("redacted another person's message: %v", err)
	}

	// The author's redaction keeps the history position and the thread routing.
	redacted, err := alice.SendMessage(ctx, redaction(sent.GetMessageKey()))
	if err != nil {
		t.Fatal(err)
	}
	read, err := alice.GetMessage(ctx, &chat_rpc.GetMessageRequest{MessageKey: sent.GetMessageKey()})
	if err != nil {
		t.Fatal(err)
	}
	message := read.GetMessage()
	want := &ChatMessageContent{Content: &ChatMessageContent_Ciphertext{Ciphertext: &ChatCiphertext{Relation: &ChatRelation{Type: "m.thread", TargetKey: root.GetMessageKey()}}}}
	if message.GetRedactedByKey() != redacted.GetMessageKey() || !message.GetContent().EqualVT(want) || message.GetText() != "Message deleted" {
		t.Fatalf("redacted message kept its body: %v", message)
	}

	// The accepted send identity still resolves after its body is gone.
	retry, err := alice.SendMessage(ctx, replyRequest)
	if err != nil || retry.GetMessageKey() != sent.GetMessageKey() {
		t.Fatalf("retry after redaction: %v %v", retry, err)
	}

	// Redactions themselves are permanent history.
	if _, err := alice.SendMessage(ctx, redaction(redacted.GetMessageKey())); err == nil {
		t.Fatal("redacted a redaction")
	}
}
