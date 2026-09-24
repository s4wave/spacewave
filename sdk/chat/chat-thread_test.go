package spacewave_chat

import (
	"context"
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	db_world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
)

// TestChatResourceListThreadsRetainsOrderCountsAndReplyParticipation exercises the native index contract.
func TestChatResourceListThreadsRetainsOrderCountsAndReplyParticipation(t *testing.T) {
	ctx := t.Context()
	tb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	ws := world.NewEngineWorldState(tb.Engine, true)
	const channelKey = "chat/channel/thread-index"
	if _, _, err := ws.ApplyWorldOp(ctx, &CreateChatChannelOp{
		ObjectKey: channelKey,
		Name:      "Threads",
		Timestamp: timestamppb.Now(),
	}, tb.Volume.GetPeerID()); err != nil {
		t.Fatal(err)
	}
	alice := NewChatResourceForPerson(ws, tb.Engine, channelKey, "alice-device", "alice")
	bob := NewChatResourceForPerson(ws, tb.Engine, channelKey, "bob-device", "bob")
	t.Cleanup(alice.Close)
	t.Cleanup(bob.Close)

	rootA := sendThreadTestEvent(t, ctx, alice, "root-a", nil)
	replyA := sendThreadTestEvent(t, ctx, bob, "reply-a", &ChatRelation{Type: "m.thread", TargetKey: rootA})
	rootB := sendThreadTestEvent(t, ctx, bob, "root-b", nil)
	replyB := sendThreadTestEvent(t, ctx, alice, "reply-b", &ChatRelation{Type: "m.thread", TargetKey: rootB})

	page, err := alice.ListThreads(ctx, &chat_rpc.ListThreadsRequest{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.GetThreads()) != 1 || page.GetThreads()[0].GetRoot().GetObjectKey() != rootB ||
		page.GetThreads()[0].GetLatestReply().GetObjectKey() != replyB ||
		page.GetThreads()[0].GetReplyCount() != 1 || !page.GetThreads()[0].GetCurrentUserParticipated() ||
		page.NextBeforeIndex == nil {
		t.Fatalf("first thread page = %v", page)
	}
	second, err := alice.ListThreads(ctx, &chat_rpc.ListThreadsRequest{
		BeforeIndex: page.NextBeforeIndex,
		Limit:       1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.GetThreads()) != 1 || second.GetThreads()[0].GetRoot().GetObjectKey() != rootA ||
		second.GetThreads()[0].GetLatestReply().GetObjectKey() != replyA ||
		second.GetThreads()[0].GetCurrentUserParticipated() || second.NextBeforeIndex != nil {
		t.Fatalf("second thread page = %v", second)
	}
	participated, err := alice.ListThreads(ctx, &chat_rpc.ListThreadsRequest{ParticipatedOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(participated.GetThreads()) != 1 || participated.GetThreads()[0].GetRoot().GetObjectKey() != rootB {
		t.Fatalf("participated threads = %v", participated)
	}

	latestA := sendThreadTestEvent(t, ctx, alice, "reply-a-latest", &ChatRelation{Type: "m.thread", TargetKey: rootA})
	if retry := sendThreadTestEvent(t, ctx, alice, "reply-a-latest", &ChatRelation{Type: "m.thread", TargetKey: rootA}); retry != latestA {
		t.Fatalf("retry key = %q, want %q", retry, latestA)
	}
	updated, err := alice.ListThreads(ctx, &chat_rpc.ListThreadsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.GetThreads()) != 2 || updated.GetThreads()[0].GetRoot().GetObjectKey() != rootA ||
		updated.GetThreads()[0].GetLatestReply().GetObjectKey() != latestA ||
		updated.GetThreads()[0].GetReplyCount() != 2 || !updated.GetThreads()[0].GetCurrentUserParticipated() {
		t.Fatalf("updated threads = %v", updated)
	}

	// Pages and thread summaries carry block types so World walks can decode them.
	threadKey, err := alice.chatThreadKey(rootA)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		alice.messagePageKey(0): ChatMessagePageTypeID,
		threadKey:               ChatThreadTypeID,
	} {
		typeID, err := world_types.GetObjectType(ctx, ws, key)
		if err != nil {
			t.Fatal(err)
		}
		if typeID != want {
			t.Fatalf("object %s type = %q, want %q", key, typeID, want)
		}
	}
}

// TestChatResourceListThreadsMigratesLegacyHistoryOnce proves compatibility without repeated scans.
func TestChatResourceListThreadsMigratesLegacyHistoryOnce(t *testing.T) {
	ctx := t.Context()
	tb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	ws := world.NewEngineWorldState(tb.Engine, true)
	const channelKey = "chat/channel/legacy-threads"
	createChatChannel(t, ctx, ws, channelKey, "Legacy")
	rootKey := channelKey + "/message/0"
	createChatMessage(t, ctx, ws, channelKey, rootKey, "root", "alice-device")
	replyKey := channelKey + "/message/1"
	createLegacyThreadReply(t, ctx, ws, channelKey, replyKey, rootKey)

	resource := NewChatResourceForPerson(ws, tb.Engine, channelKey, "bob-device", "bob")
	t.Cleanup(resource.Close)
	tx, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	legacyChannel, err := world.LookupObjectBody[*ChatChannel](ctx, tx, channelKey, NewChatChannelBlock)
	if err != nil {
		t.Fatal(err)
	}
	changed, complete, err := resource.updateThreadIndex(ctx, tx, legacyChannel, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || complete || legacyChannel.GetThreadIndexedMessageCount() != 1 {
		t.Fatalf("bounded migration = changed %t, complete %t, channel %v", changed, complete, legacyChannel)
	}
	if err := resource.writeChannel(ctx, tx, legacyChannel); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := tb.Engine.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	page, err := resource.ListThreads(ctx, &chat_rpc.ListThreadsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.GetThreads()) != 1 || page.GetThreads()[0].GetRoot().GetObjectKey() != rootKey ||
		page.GetThreads()[0].GetLatestReply().GetObjectKey() != replyKey ||
		!page.GetThreads()[0].GetCurrentUserParticipated() {
		t.Fatalf("migrated threads = %v", page)
	}
	channel, err := resource.readChannel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if channel.ThreadIndexedMessageCount == nil || channel.GetThreadIndexedMessageCount() != channel.GetMessageCount() ||
		channel.GetThreadHeadKey() == "" {
		t.Fatalf("migrated channel index = %v", channel)
	}
	before, err := ws.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resource.ListThreads(ctx, &chat_rpc.ListThreadsRequest{}); err != nil {
		t.Fatal(err)
	}
	after, err := ws.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("current index read changed world seqno from %d to %d", before, after)
	}
}

func sendThreadTestEvent(
	t *testing.T,
	ctx context.Context,
	resource *ChatResource,
	transactionID string,
	relation *ChatRelation,
) string {
	t.Helper()
	response, err := resource.SendMessage(ctx, &chat_rpc.SendMessageRequest{
		TransactionId: transactionID,
		Content: &ChatMessageContent{Content: &ChatMessageContent_Event{Event: &ChatEvent{
			Type:        "m.room.message",
			ContentJson: `{"body":"test","msgtype":"m.text"}`,
			Relation:    relation,
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return response.GetMessageKey()
}

func createLegacyThreadReply(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	channelKey string,
	messageKey string,
	rootKey string,
) {
	t.Helper()
	createdObject, _, err := world.CreateWorldObject(ctx, ws, messageKey, func(cursor *block.Cursor) error {
		cursor.SetBlock(&ChatMessage{
			SenderPeerId: "bob-device",
			PersonPeerId: "bob",
			Content: &ChatMessageContent{Content: &ChatMessageContent_Event{Event: &ChatEvent{
				Type:        "m.room.message",
				ContentJson: `{"body":"legacy","msgtype":"m.text"}`,
				Relation:    &ChatRelation{Type: "m.thread", TargetKey: rootKey},
			}}},
			CreatedAt: timestamppb.Now(),
			Index:     1,
		}, true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatal(err)
	}
	if err := world_types.SetObjectType(ctx, ws, messageKey, ChatMessageTypeID); err != nil {
		t.Fatal(err)
	}
	appendChatMessageKey(t, ctx, ws, channelKey, messageKey)
	if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(channelKey, PredChannelMessage.String(), messageKey, "")); err != nil {
		t.Fatal(err)
	}
}
