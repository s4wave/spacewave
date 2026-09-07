package spacewave_chat

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/world"
	db_world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	spacewave_chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
)

// TestEncryptedContentRoundTrip preserves the exact envelope across retry, history, and watch.
func TestEncryptedContentRoundTrip(t *testing.T) {
	// Persist one typed envelope through the real shared channel Resource.
	ctx := t.Context()
	tb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	ws := world.NewEngineWorldState(tb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")
	content := &ChatMessageContent{Content: &ChatMessageContent_Ciphertext{Ciphertext: &ChatCiphertext{
		Algorithm: "m.megolm.v1.aes-sha2", Ciphertext: "opaque-envelope", SenderKey: "sender-public-key", SessionId: "session-id",
	}}}
	request := &spacewave_chat_rpc.SendMessageRequest{TransactionId: "encrypted-send", Content: content}
	resource := NewChatResource(ws, tb.Engine, GeneralChannelKey, "alice")
	accepted, err := resource.SendMessage(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	resource.Close()

	// Reattach, preserve send identity, and reject conflicting ciphertext or plaintext.
	resource = NewChatResource(ws, tb.Engine, GeneralChannelKey, "alice")
	retry, err := resource.SendMessage(ctx, request)
	if err != nil || retry.GetMessageKey() != accepted.GetMessageKey() {
		t.Fatalf("encrypted retry changed identity: %v", err)
	}
	conflict := request.CloneVT()
	conflict.Content.GetCiphertext().Ciphertext = "different-envelope"
	if _, err := resource.SendMessage(ctx, conflict); err == nil {
		t.Fatal("changed ciphertext reused an accepted send identity")
	}
	conflict = request.CloneVT()
	conflict.Text = "plaintext substitution"
	if _, err := resource.SendMessage(ctx, conflict); err == nil {
		t.Fatal("accepted plaintext alongside encrypted content")
	}
	history, err := resource.ListMessages(ctx, &spacewave_chat_rpc.ListMessagesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(history.GetMessages()) != 1 || !history.GetMessages()[0].GetContent().EqualVT(content) || history.GetMessages()[0].GetText() != "" {
		t.Fatal("history changed or substituted the encrypted envelope")
	}

	// Stream the same envelope through the existing client watch contract.
	watchCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	stream := newChatMessageStream(watchCtx)
	done := make(chan error, 1)
	go func() { done <- resource.WatchMessages(&spacewave_chat_rpc.WatchMessagesRequest{}, stream) }()
	batch := recvChatWatchValue(t, stream.sent)
	if len(batch.GetMessages()) != 1 || !batch.GetMessages()[0].GetContent().EqualVT(content) || batch.GetMessages()[0].GetText() != "" {
		t.Fatal("watch changed or substituted the encrypted envelope")
	}
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("watch cancellation: %v", err)
	}
}
