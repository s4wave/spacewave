package spacewave_chat

import (
	"strconv"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/world"
	db_world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	spacewave_chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
)

// TestChatResourceSendRetryRetainsHistory proves that retry identity survives Resource replacement.
func TestChatResourceSendRetryRetainsHistory(t *testing.T) {
	// Send through the real World-backed channel owner.
	ctx := t.Context()
	tb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	ws := world.NewEngineWorldState(tb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")
	first := NewChatResource(ws, tb.Engine, GeneralChannelKey, "alice")
	request := &spacewave_chat_rpc.SendMessageRequest{Text: "retained", TransactionId: "device-a/send-1"}
	accepted, err := first.SendMessage(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	seqno, err := ws.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first.Close()

	// Reattach and retry without publishing another World revision.
	resumed := NewChatResource(ws, tb.Engine, GeneralChannelKey, "alice")
	replayed, err := resumed.SendMessage(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.GetMessageKey() != accepted.GetMessageKey() {
		t.Fatal("retry changed message identity")
	}
	after, err := ws.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after != seqno {
		t.Fatal("retry published a World revision")
	}
	conflict := request.CloneVT()
	conflict.Text = "changed"
	if _, err := resumed.SendMessage(ctx, conflict); err == nil {
		t.Fatal("accepted conflicting reuse of send identity")
	}

	// A second authenticated sender has an independent transaction namespace.
	bob := NewChatResource(ws, tb.Engine, GeneralChannelKey, "bob")
	second, err := bob.SendMessage(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if second.GetMessageKey() == accepted.GetMessageKey() {
		t.Fatal("different senders shared a transaction identity")
	}
	history, err := resumed.ListMessages(ctx, &spacewave_chat_rpc.ListMessagesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(history.GetMessages()) != 2 {
		t.Fatalf("history contains %d messages, want 2", len(history.GetMessages()))
	}
	page, err := resumed.ListMessages(ctx, &spacewave_chat_rpc.ListMessagesRequest{BeforeKey: second.GetMessageKey(), Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	requireChatMessages(t, page.GetMessages(), accepted.GetMessageKey(), "retained", "alice")
}

// parseMessageIndex decodes the numeric keys used by historical message fixtures.
func parseMessageIndex(messageKey string) (uint64, bool) {
	idx := strings.LastIndexByte(messageKey, '/')
	if idx < 0 || idx == len(messageKey)-1 {
		return 0, false
	}
	messageIndex, err := strconv.ParseUint(messageKey[idx+1:], 10, 64)
	return messageIndex, err == nil
}
