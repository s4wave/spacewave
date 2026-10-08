package spacewave_chat

import (
	"testing"

	"github.com/s4wave/spacewave/db/world"
	db_world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	spacewave_chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
)

// TestChatResourceSendAuthor proves an author attributes a message within its sender's peer.
func TestChatResourceSendAuthor(t *testing.T) {
	// Send through the real World-backed channel owner.
	ctx := t.Context()
	tb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	ws := world.NewEngineWorldState(tb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")
	resource := newChatResource(t, ws, tb.Engine, GeneralChannelKey, "alice")

	// Send as a person, then as two authors under the same person.
	for _, request := range []*spacewave_chat_rpc.SendMessageRequest{
		{Text: "person"},
		{Text: "first", Author: "deadlock"},
		{Text: "second", Author: "codex-1"},
	} {
		if _, err := resource.SendMessage(ctx, request); err != nil {
			t.Fatal(err)
		}
	}

	// Verify each message names its author and keeps the sender's peer and person.
	history, err := resource.ListMessages(ctx, &spacewave_chat_rpc.ListMessagesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	messages := history.GetMessages()
	if len(messages) != 3 {
		t.Fatalf("history contains %d messages, want 3", len(messages))
	}
	for i, want := range []string{"", "deadlock", "codex-1"} {
		message := messages[i]
		if message.GetAuthor() != want || message.GetPersonId() != "alice" {
			t.Fatalf("message %d author %q person %q, want %q for alice", i, message.GetAuthor(), message.GetPersonId(), want)
		}
	}

	// Reject an author that is not a DNS label, since the gateway joins it to a username with a dot.
	for _, author := range []string{"Dead.Lock", "a:b", "-lead"} {
		if _, err := resource.SendMessage(ctx, &spacewave_chat_rpc.SendMessageRequest{Text: "bad", Author: author}); err == nil {
			t.Fatalf("accepted author %q", author)
		}
	}

	// A retry under another author conflicts with the accepted send.
	request := &spacewave_chat_rpc.SendMessageRequest{Text: "once", TransactionId: "send-author", Author: "deadlock"}
	if _, err := resource.SendMessage(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err := resource.SendMessage(ctx, request); err != nil {
		t.Fatalf("identical retry: %v", err)
	}
	other := request.CloneVT()
	other.Author = "codex-1"
	if _, err := resource.SendMessage(ctx, other); err == nil {
		t.Fatal("retry accepted a different author")
	}
}
