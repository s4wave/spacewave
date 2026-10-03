package spacewave_chat

import (
	"strconv"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/world"
	db_world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/s4wave/spacewave/net/peer"
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

	// Create the channel and precompute the sender's stable send identity.
	ws := world.NewEngineWorldState(tb.Engine, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")
	first := newChatResource(t, ws, tb.Engine, GeneralChannelKey, "alice")
	request := &spacewave_chat_rpc.SendMessageRequest{Text: "retained", TransactionId: "device-a/send-1"}
	messageKey, err := TransactionMessageKey(GeneralChannelKey, peer.ID("alice").String(), request.GetTransactionId())
	if err != nil {
		t.Fatal(err)
	}

	// Send the initial message and verify its transaction identity.
	accepted, err := first.SendMessage(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.GetMessageKey() != messageKey {
		t.Fatal("send changed its precomputed transaction message identity")
	}

	// Record the accepted World revision before replacing the Resource.
	seqno, err := ws.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first.Close()

	// Reattach and retry without publishing another World revision.
	resumed := newChatResource(t, ws, tb.Engine, GeneralChannelKey, "alice")
	replayed, err := resumed.SendMessage(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.GetMessageKey() != accepted.GetMessageKey() {
		t.Fatal("retry changed message identity")
	}

	// Verify the retry left the accepted World revision unchanged.
	after, err := ws.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after != seqno {
		t.Fatal("retry published a World revision")
	}

	// Require changed content to conflict with the accepted send identity.
	conflict := request.CloneVT()
	conflict.Text = "changed"
	if _, err := resumed.SendMessage(ctx, conflict); err == nil {
		t.Fatal("accepted conflicting reuse of send identity")
	}

	// Opt-in replay ignores changed bodies and relationships without weakening author identity.
	conflict.ReuseAcceptedTransaction = true
	conflict.Text = ""
	conflict.ReplyToKey = GeneralChannelKey + "/message/missing"
	conflict.Content = &ChatMessageContent{Content: &ChatMessageContent_Annotation{
		Annotation: &ChatAnnotation{TargetKey: conflict.ReplyToKey, Key: "changed"},
	}}
	replayed, err = resumed.SendMessage(ctx, conflict)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.GetMessageKey() != accepted.GetMessageKey() {
		t.Fatal("opt-in replay changed the accepted message")
	}

	// Verify opt-in replay preserves the World revision and author identity.
	after, err = ws.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after != seqno {
		t.Fatal("opt-in replay published a World revision")
	}
	otherPerson := newChatResourceForPerson(t, ws, tb.Engine, GeneralChannelKey, "alice", "another-person")
	if _, err := otherPerson.SendMessage(ctx, conflict); err == nil {
		t.Fatal("opt-in replay accepted a different person under the same device identity")
	}

	// A second authenticated sender has an independent transaction namespace.
	bob := newChatResource(t, ws, tb.Engine, GeneralChannelKey, "bob")
	second, err := bob.SendMessage(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if second.GetMessageKey() == accepted.GetMessageKey() {
		t.Fatal("different senders shared a transaction identity")
	}

	// Read and verify the two senders' retained history.
	history, err := resumed.ListMessages(ctx, &spacewave_chat_rpc.ListMessagesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(history.GetMessages()) != 2 {
		t.Fatalf("history contains %d messages, want 2", len(history.GetMessages()))
	}

	// Verify pagination locates the first sender's retained message.
	page, err := resumed.ListMessages(ctx, &spacewave_chat_rpc.ListMessagesRequest{BeforeKey: second.GetMessageKey(), Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	requireChatMessages(t, page.GetMessages(), accepted.GetMessageKey(), "retained", peer.ID("alice").String())
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
