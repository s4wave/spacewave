package spacewave_chat

import (
	"context"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	db_world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	spacewave_chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
)

// TestWatchMessagesDeliversAppendReorderedBelowPosition replays one channel
// history in two orders on one engine, as a Space replica does when a peer's
// concurrent send merges ahead of a local send. A watcher that delivered the
// local order must still deliver the peer's send after the engine moves to
// the merged order.
func TestWatchMessagesDeliversAppendReorderedBelowPosition(t *testing.T) {
	// Start a block World engine whose head the test installs directly, as
	// the Space replay does after a merge.
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()
	cursor, err := wtb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	eng, err := world_block.NewEngine(ctx, wtb.Logger, cursor, nil, nil, false)
	if err != nil {
		cursor.Release()
		t.Fatal(err)
	}
	defer eng.Close()
	ws := world.NewEngineWorldState(eng, true)

	// Accept Bob's first entry on the common history.
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")
	bob := newChatResource(t, ws, eng, GeneralChannelKey, "peer-bob")
	defer bob.Close()
	alice := newChatResource(t, ws, eng, GeneralChannelKey, "peer-alice")
	defer alice.Close()
	send := func(r *ChatResource, text, txid string) string {
		t.Helper()
		resp, err := r.SendMessage(ctx, &spacewave_chat_rpc.SendMessageRequest{Text: text, TransactionId: txid})
		if err != nil {
			t.Fatal(err)
		}
		return resp.GetMessageKey()
	}
	first := send(bob, "bob first", "bob-1")
	base := eng.GetRootRef().Clone()

	// Build the merged order: Alice's offline send precedes Bob's second.
	aliceKey := send(alice, "alice offline", "alice-1")
	send(bob, "bob second", "bob-2")
	merged := eng.GetRootRef().Clone()

	// Return to the common history and build Bob's local order.
	if err := eng.SetRootRef(ctx, base); err != nil {
		t.Fatal(err)
	}
	second := send(bob, "bob second", "bob-2")

	// Watch Bob's local order to its end.
	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream := newChatWatchStream[*spacewave_chat_rpc.WatchMessagesResponse](watchCtx)
	done := make(chan error, 1)
	go func() {
		done <- bob.WatchMessages(&spacewave_chat_rpc.WatchMessagesRequest{}, stream)
	}()
	defer func() {
		cancel()
		<-done
	}()
	var delivered []string
	for _, msg := range recvChatWatchValue(t, stream.sent).GetMessages() {
		delivered = append(delivered, msg.GetObjectKey())
	}
	if !slices.Equal(delivered, []string{first, second}) {
		t.Fatalf("local order delivered %v, want [%s %s]", delivered, first, second)
	}

	// Install the merged order. The channel grows by one entry, Alice's send.
	if err := eng.SetRootRef(ctx, merged); err != nil {
		t.Fatal(err)
	}
	var batch []string
	for _, msg := range recvChatWatchValue(t, stream.sent).GetMessages() {
		batch = append(batch, msg.GetObjectKey())
	}
	delivered = append(delivered, batch...)
	if !slices.Contains(delivered, aliceKey) {
		t.Fatalf("watcher never delivered Alice's send %s: the merge renumbered it below the watch position; batch after merge %v", aliceKey, batch)
	}
}
