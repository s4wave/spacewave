package spacewave_chat

import (
	"context"
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/world"
	db_world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	spacewave_chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
)

func TestEncryptedChannelRequiresConfiguredAlgorithm(t *testing.T) {
	// Set up a real World and encrypted channel for policy checks.
	ctx := t.Context()
	tb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	ws := world.NewEngineWorldState(tb.Engine, true)
	const (
		channelKey = "chat/channel/encrypted"
		algorithm  = "m.megolm.v1.aes-sha2"
	)

	// Create the channel through its world operation with an immutable algorithm policy.
	_, _, err = ws.ApplyWorldOp(ctx, &CreateChatChannelOp{
		ObjectKey:           channelKey,
		Name:                "Encrypted",
		Timestamp:           timestamppb.Now(),
		EncryptionAlgorithm: algorithm,
	}, tb.Volume.GetPeerID())
	if err != nil {
		t.Fatalf("ApplyWorldOp: %v", err)
	}
	resource := newChatResource(t, ws, tb.Engine, channelKey, "alice")
	info, err := resource.GetChannelInfo(ctx, &spacewave_chat_rpc.GetChannelInfoRequest{})
	if err != nil {
		t.Fatalf("GetChannelInfo: %v", err)
	}
	if info.GetEncryptionAlgorithm() != algorithm {
		t.Fatalf("channel encryption algorithm = %q, want %q", info.GetEncryptionAlgorithm(), algorithm)
	}

	// Reject plaintext and ciphertext using a different algorithm before creating history.
	if _, err := resource.SendMessage(ctx, &spacewave_chat_rpc.SendMessageRequest{
		TransactionId: "plaintext",
		Text:          "must be encrypted",
	}); err == nil {
		t.Fatal("accepted plaintext on encrypted channel")
	}
	mismatched := &spacewave_chat_rpc.SendMessageRequest{
		TransactionId: "mismatched",
		Content: &ChatMessageContent{Content: &ChatMessageContent_Ciphertext{Ciphertext: &ChatCiphertext{
			Algorithm: "other.algorithm", Ciphertext: "opaque", SenderKey: "sender", SessionId: "session",
		}}},
	}
	if _, err := resource.SendMessage(ctx, mismatched); err == nil {
		t.Fatal("accepted ciphertext with mismatched algorithm")
	}

	// Accept matching ciphertext and resolve an identical retry to the same event.
	request := &spacewave_chat_rpc.SendMessageRequest{
		TransactionId: "encrypted",
		Content: &ChatMessageContent{Content: &ChatMessageContent_Ciphertext{Ciphertext: &ChatCiphertext{
			Algorithm: algorithm, Ciphertext: "opaque", SenderKey: "sender", SessionId: "session",
		}}},
	}
	accepted, err := resource.SendMessage(ctx, request)
	if err != nil {
		t.Fatalf("SendMessage encrypted: %v", err)
	}
	retry, err := resource.SendMessage(ctx, request)
	if err != nil {
		t.Fatalf("SendMessage retry: %v", err)
	}
	if retry.GetMessageKey() != accepted.GetMessageKey() {
		t.Fatalf("encrypted retry key = %q, want %q", retry.GetMessageKey(), accepted.GetMessageKey())
	}

	// Confirm rejected attempts and the retry left one retained history entry.
	info, err = resource.GetChannelInfo(ctx, &spacewave_chat_rpc.GetChannelInfoRequest{})
	if err != nil {
		t.Fatalf("GetChannelInfo after retry: %v", err)
	}
	if info.GetMessageCount() != 1 {
		t.Fatalf("channel message count = %d, want 1", info.GetMessageCount())
	}
}

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
	resource := newChatResource(t, ws, tb.Engine, GeneralChannelKey, "alice")
	accepted, err := resource.SendMessage(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	resource.Close()

	// Reattach, preserve send identity, and reject conflicting ciphertext or plaintext.
	resource = newChatResource(t, ws, tb.Engine, GeneralChannelKey, "alice")
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
