//go:build !goscript

package spacewave_chat_channel

import (
	"context"
	"errors"
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	db_world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	spacewave_chat "github.com/s4wave/spacewave/sdk/chat"
	spacewave_chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
)

func TestChatResourceListMessagesReadsOnlyRequestedPage(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, spacewave_chat.GeneralChannelKey, "General")
	createBadChatMessageObject(t, ctx, ws, spacewave_chat.GeneralChannelKey, spacewave_chat.GeneralChannelKey+"/message/0")
	createChatMessage(t, ctx, ws, spacewave_chat.GeneralChannelKey, spacewave_chat.GeneralChannelKey+"/message/1", "second", "peer-local")
	createChatMessage(t, ctx, ws, spacewave_chat.GeneralChannelKey, spacewave_chat.GeneralChannelKey+"/message/2", "third", "peer-local")

	resource := NewChatResource(ws, wtb.Engine, spacewave_chat.GeneralChannelKey, "peer-local")
	listResp, err := resource.ListMessages(ctx, &spacewave_chat_rpc.ListMessagesRequest{
		BeforeKey: spacewave_chat.GeneralChannelKey + "/message/2",
		Limit:     1,
	})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	requireChatMessages(t, listResp.GetMessages(), spacewave_chat.GeneralChannelKey+"/message/1", "second", "peer-local")
}

// TestChatResourceNativeReplayIsSenderScoped proves native retry dedupe:
// one sender retrying a client message id resolves to the original message
// without advancing the channel; another sender using the same id creates a
// separate message.
func TestChatResourceNativeReplayIsSenderScoped(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	createChatChannel(t, ctx, ws, spacewave_chat.GeneralChannelKey, "General")

	senderA := NewChatResource(ws, wtb.Engine, spacewave_chat.GeneralChannelKey, "peer-a")
	senderB := NewChatResource(ws, wtb.Engine, spacewave_chat.GeneralChannelKey, "peer-b")

	first, err := senderA.SendMessage(ctx, &spacewave_chat_rpc.SendMessageRequest{
		Text:            "hello",
		ClientMessageId: "retry-1",
	})
	if err != nil {
		t.Fatalf("SendMessage(first): %v", err)
	}

	retry, err := senderA.SendMessage(ctx, &spacewave_chat_rpc.SendMessageRequest{
		Text:            "hello",
		ClientMessageId: "retry-1",
	})
	if err != nil {
		t.Fatalf("SendMessage(retry): %v", err)
	}
	if retry.GetMessageKey() != first.GetMessageKey() {
		t.Fatalf("retry key = %q, want original %q", retry.GetMessageKey(), first.GetMessageKey())
	}

	otherPayload, err := senderA.SendMessage(ctx, &spacewave_chat_rpc.SendMessageRequest{
		Text:            "different text",
		ClientMessageId: "retry-1",
	})
	if !errors.Is(err, ErrChatOriginConflict) {
		t.Fatalf("conflicting payload error = %v, want %v", err, ErrChatOriginConflict)
	}
	if otherPayload != nil {
		t.Fatalf("conflicting payload response = %+v, want no message", otherPayload)
	}

	fromB, err := senderB.SendMessage(ctx, &spacewave_chat_rpc.SendMessageRequest{
		Text:            "hello",
		ClientMessageId: "retry-1",
	})
	if err != nil {
		t.Fatalf("SendMessage(sender b): %v", err)
	}
	if fromB.GetMessageKey() == first.GetMessageKey() {
		t.Fatal("sender b deduped into sender a message, want a separate message")
	}

	channel, err := spacewave_chat.ReadChatChannel(ctx, ws, spacewave_chat.GeneralChannelKey)
	if err != nil {
		t.Fatalf("ReadChatChannel: %v", err)
	}
	if channel.GetMessageCount() != 2 {
		t.Fatalf("message count = %d, want 2 (one per sender)", channel.GetMessageCount())
	}
	listResp, err := senderA.ListMessages(ctx, &spacewave_chat_rpc.ListMessagesRequest{})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(listResp.GetMessages()) != 2 {
		t.Fatalf("messages = %d, want 2", len(listResp.GetMessages()))
	}
	for _, msg := range listResp.GetMessages() {
		if msg.GetClientMessageId() != "retry-1" {
			t.Fatalf("projected client message id = %q, want retry-1", msg.GetClientMessageId())
		}
	}
}

func createBadChatMessageObject(t *testing.T, ctx context.Context, ws world.WorldState, channelKey, msgKey string) {
	t.Helper()

	_, _, err := world.CreateWorldObject(ctx, ws, msgKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(&spacewave_chat.ChatChannel{Name: "wrong block", CreatedAt: timestamppb.Now()}, true)
		return nil
	})
	if err != nil {
		t.Fatalf("CreateWorldObject(%s): %v", msgKey, err)
	}
	appendChatMessageKey(t, ctx, ws, channelKey, msgKey)
}
