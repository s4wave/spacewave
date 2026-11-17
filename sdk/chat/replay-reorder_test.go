package spacewave_chat

import (
	"context"
	"slices"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	db_world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	spacewave_chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
)

// reorderChannel builds one channel's history in several orders on one block
// engine, whose head a test installs directly as the Space replay does after
// a merge.
type reorderChannel struct {
	t      *testing.T
	ctx    context.Context
	eng    *world_block.Engine
	ws     world.WorldState
	people map[string]*ChatResource
	// base is the common history: Bob's first entry.
	base *bucket.ObjectRef
	// keys maps each send's name to its stable message key.
	keys map[string]string
}

// reorderSend is one send by a named participant, named for the test.
type reorderSend struct{ person, name string }

// newReorderChannel starts the engine, creates the channel, and accepts Bob's
// first entry as the common history.
func newReorderChannel(t *testing.T) *reorderChannel {
	// Start a block World engine on a World testbed's storage.
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(wtb.Release)
	cursor, err := wtb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	eng, err := world_block.NewEngine(ctx, wtb.Logger, cursor, nil, nil, false)
	if err != nil {
		cursor.Release()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Close() })

	// Create the channel with three participants and Bob's first entry.
	ws := world.NewEngineWorldState(eng, true)
	createChatChannel(t, ctx, ws, GeneralChannelKey, "General")
	c := &reorderChannel{t: t, ctx: ctx, eng: eng, ws: ws, people: map[string]*ChatResource{}, keys: map[string]string{}}
	for _, person := range []string{"bob", "alice", "carol"} {
		resource := newChatResource(t, ws, eng, GeneralChannelKey, "peer-"+person)
		t.Cleanup(resource.Close)
		c.people[person] = resource
	}
	c.send(reorderSend{"bob", "b0"})
	c.base = eng.GetRootRef().Clone()
	return c
}

// send accepts one send with a stable transaction ID, so each order holds it
// under the same key.
func (c *reorderChannel) send(s reorderSend) {
	c.t.Helper()
	resp, err := c.people[s.person].SendMessage(c.ctx, &spacewave_chat_rpc.SendMessageRequest{Text: s.name, TransactionId: s.name})
	if err != nil {
		c.t.Fatal(err)
	}
	if prior, ok := c.keys[s.name]; ok && prior != resp.GetMessageKey() {
		c.t.Fatalf("send %s changed key between orders", s.name)
	}
	c.keys[s.name] = resp.GetMessageKey()
}

// order builds the common history followed by sends and returns its head.
func (c *reorderChannel) order(sends ...reorderSend) *bucket.ObjectRef {
	c.t.Helper()
	c.install(c.base)
	for _, s := range sends {
		c.send(s)
	}
	return c.eng.GetRootRef().Clone()
}

// install moves the engine to head, as the Space replay's bind does.
func (c *reorderChannel) install(head *bucket.ObjectRef) {
	c.t.Helper()
	if err := c.eng.SetRootRef(c.ctx, head); err != nil {
		c.t.Fatal(err)
	}
}

// named returns the keys of the named sends.
func (c *reorderChannel) named(names ...string) []string {
	keys := make([]string, 0, len(names))
	for _, name := range names {
		keys = append(keys, c.keys[name])
	}
	return keys
}

// reorderWatch is Bob's watch on the channel.
type reorderWatch struct {
	c      *reorderChannel
	stream *chatWatchStream[*spacewave_chat_rpc.WatchMessagesResponse]
	// sent holds every key the watch has sent.
	sent map[string]bool
}

// watch starts Bob's watch on the channel's current order.
func (c *reorderChannel) watch() *reorderWatch {
	watchCtx, cancel := context.WithCancel(c.ctx)
	stream := newChatWatchStream[*spacewave_chat_rpc.WatchMessagesResponse](watchCtx)
	done := make(chan error, 1)
	go func() {
		done <- c.people["bob"].WatchMessages(&spacewave_chat_rpc.WatchMessagesRequest{}, stream)
	}()
	c.t.Cleanup(func() {
		cancel()
		<-done
	})
	return &reorderWatch{c: c, stream: stream, sent: map[string]bool{}}
}

// expect requires the watch's next batch to hold the named sends in order,
// besides only keys it sent before, which a client keeping messages by key
// takes as no-ops.
func (w *reorderWatch) expect(step string, names ...string) {
	w.c.t.Helper()
	var got []string
	for _, msg := range recvChatWatchValue(w.c.t, w.stream.sent).GetMessages() {
		if key := msg.GetObjectKey(); !w.sent[key] {
			got = append(got, key)
		}
	}
	if want := w.c.named(names...); !slices.Equal(got, want) {
		w.c.t.Fatalf("%s: watch sent new %v, want %v %v", step, got, names, want)
	}
	for _, key := range got {
		w.sent[key] = true
	}
}

// TestWatchMessagesDeliversAppendReorderedBelowPosition replays one channel
// history in two orders, as a Space replica does when a peer's concurrent
// send merges ahead of a local send. A watcher that delivered the local order
// must deliver the peer's send after the merge, repeating only what it sent.
func TestWatchMessagesDeliversAppendReorderedBelowPosition(t *testing.T) {
	c := newReorderChannel(t)
	merged := c.order(reorderSend{"alice", "a1"}, reorderSend{"bob", "b1"})
	c.order(reorderSend{"bob", "b1"})

	// Watch Bob's local order, then install the merged order.
	w := c.watch()
	w.expect("local order", "b0", "b1")
	c.install(merged)
	w.expect("after merge", "a1")
}

// TestWatchMessagesDeliversAcrossSuccessiveReorders installs two merges in
// turn, each placing a send below every entry after the common history.
func TestWatchMessagesDeliversAcrossSuccessiveReorders(t *testing.T) {
	c := newReorderChannel(t)
	second := c.order(reorderSend{"carol", "c1"}, reorderSend{"alice", "a1"}, reorderSend{"bob", "b1"})
	first := c.order(reorderSend{"alice", "a1"}, reorderSend{"bob", "b1"})
	c.order(reorderSend{"bob", "b1"})

	// Watch Bob's local order, then each merge in turn.
	w := c.watch()
	w.expect("local order", "b0", "b1")
	c.install(first)
	w.expect("after first merge", "a1")
	c.install(second)
	w.expect("after second merge", "c1")
}

// TestChannelCursorReorderAtPosition places a peer's send exactly at a
// reader's position, above everything it delivered and below an entry it has
// not read yet: it must read both, the send first.
func TestChannelCursorReorderAtPosition(t *testing.T) {
	c := newReorderChannel(t)
	merged := c.order(reorderSend{"bob", "b1"}, reorderSend{"alice", "a1"}, reorderSend{"bob", "b2"})
	c.order(reorderSend{"bob", "b1"}, reorderSend{"bob", "b2"})

	// Read the local order up to b2, leaving it unread.
	bob := c.people["bob"]
	cursor := NewChannelCursor(0, ChannelChain{})
	read := func(count uint64) []string {
		t.Helper()
		// Read the channel as if it held only count entries.
		upTo := func(ctx context.Context, start, limit uint64) (ChannelChain, []string, bool, error) {
			return bob.readChannelSpan(ctx, start, min(limit, count-min(start, count)))
		}
		var all []string
		for {
			before := cursor.Next()
			from, keys, err := cursor.Read(c.ctx, 50, upTo)
			if err != nil {
				t.Fatal(err)
			}
			if from < before {
				t.Fatalf("cursor moved back from %d to %d in a reorder at its position", before, from)
			}
			if len(keys) == 0 {
				return all
			}
			all = append(all, keys...)
		}
	}
	if got, want := read(2), c.named("b0", "b1"); !slices.Equal(got, want) {
		t.Fatalf("local order read %v, want %v", got, want)
	}

	// Install the merged order, whose lowest changed index is the position.
	c.install(merged)
	channel, err := bob.readChannel(c.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := read(channel.GetMessageCount()), c.named("a1", "b2"); !slices.Equal(got, want) {
		t.Fatalf("after merge read %v, want %v", got, want)
	}
}

// TestChannelChainAcrossPages checks that the chain a span read returns at
// each page edge is the fold of every key below it, as the pages' stored bases
// must make it.
func TestChannelChainAcrossPages(t *testing.T) {
	c := newReorderChannel(t)
	for i := range 2*chatMessagePageSize + 2 {
		c.send(reorderSend{"bob", "m" + strconv.Itoa(i)})
	}
	bob := c.people["bob"]
	channel, err := bob.readChannel(c.ctx)
	if err != nil {
		t.Fatal(err)
	}
	count := channel.GetMessageCount()
	all, err := bob.readMessageKeys(c.ctx, c.ws, 0, count)
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range []uint64{0, 1, chatMessagePageSize - 1, chatMessagePageSize, chatMessagePageSize + 1, 2 * chatMessagePageSize, count} {
		base, keys, ok, err := bob.readChannelSpan(c.ctx, index, 1)
		if err != nil || !ok {
			t.Fatalf("span at %d: ok=%v err=%v", index, ok, err)
		}
		if want := (ChannelChain{}).Fold(all[:index]); base != want {
			t.Fatalf("chain below %d does not fold the keys below it", index)
		}
		if want := all[index:min(index+1, count)]; !slices.Equal(keys, want) {
			t.Fatalf("span at %d read %v, want %v", index, keys, want)
		}
	}
	if _, _, ok, err := bob.readChannelSpan(c.ctx, count+1, 1); err != nil || ok {
		t.Fatalf("span past the end: ok=%v err=%v", ok, err)
	}
}
