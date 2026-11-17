package spacewave_chat

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	spacewave_chat_rpc "github.com/s4wave/spacewave/sdk/chat/rpc"
)

// TestAdversaryKeylessReorder replays two sends that carry no transaction ID,
// so each takes the positional key channel/message/<index>. A peer's
// concurrent send merges ahead of Bob's: the keys at each index stay the same
// while the messages under them change, so the cursor's tail sees no reorder.
// The watch must still deliver Alice's text.
func TestAdversaryKeylessReorder(t *testing.T) {
	c := newReorderChannel(t)
	keyless := func(person, text string) {
		t.Helper()
		if _, err := c.people[person].SendMessage(c.ctx, &spacewave_chat_rpc.SendMessageRequest{Text: text}); err != nil {
			t.Fatal(err)
		}
	}
	c.install(c.base)
	keyless("alice", "a1")
	keyless("bob", "b1")
	merged := c.eng.GetRootRef().Clone()
	c.install(c.base)
	keyless("bob", "b1")

	w := c.watch()
	var texts []string
	for _, msg := range recvChatWatchValue(t, w.stream.sent).GetMessages() {
		texts = append(texts, msg.GetText())
	}
	c.install(merged)
	for _, msg := range recvChatWatchValue(t, w.stream.sent).GetMessages() {
		texts = append(texts, msg.GetText())
	}
	if !slices.Contains(texts, "a1") {
		t.Fatalf("watch delivered %v, never a1", texts)
	}
}

// modelChannel is a channel's key order that a test rewrites as replay would.
type modelChannel struct {
	keys []string
	// onRead, when set, runs before a read is served, to model a replay
	// installed between a reader's reads.
	onRead func()
}

func (m *modelChannel) keysAt(_ context.Context, start, end uint64) ([]string, error) {
	if m.onRead != nil {
		f := m.onRead
		m.onRead = nil
		f()
	}
	end = min(end, uint64(len(m.keys)))
	if start >= end {
		return nil, nil
	}
	return slices.Clone(m.keys[start:end]), nil
}

// read is the model's ChannelRead: the chain below start and the keys from
// start, from one order.
func (m *modelChannel) read(ctx context.Context, start, limit uint64) (ChannelChain, []string, bool, error) {
	if m.onRead != nil {
		f := m.onRead
		m.onRead = nil
		f()
	}
	if start > uint64(len(m.keys)) {
		return ChannelChain{}, nil, false, nil
	}
	keys, err := m.keysAt(ctx, start, start+limit)
	return ChannelChain{}.Fold(m.keys[:start]), keys, true, err
}

// drain reads the cursor to the channel's end and records what it delivered.
func drain(t *testing.T, cur *ChannelCursor, m *modelChannel, limit uint64, got map[string]int) {
	t.Helper()
	for range 100000 {
		_, keys, err := cur.Read(t.Context(), limit, m.read)
		if err != nil {
			t.Fatal(err)
		}
		if len(keys) == 0 {
			return
		}
		for _, k := range keys {
			got[k]++
		}
	}
	t.Fatal("cursor never reached the end")
}

func checkAll(t *testing.T, m *modelChannel, got map[string]int, step string) {
	t.Helper()
	for _, k := range m.keys {
		if got[k] == 0 {
			t.Fatalf("%s: entry %s in the channel was never delivered", step, k)
		}
	}
}

func seq(n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = fmt.Sprintf("k%03d", i)
	}
	return keys
}

// TestAdversaryDeepMoveBelowTail inserts an undelivered entry below the tail
// and moves a delivered one from below the tail to above the position, so the
// tail's keys stay at their indexes.
func TestAdversaryDeepMoveBelowTail(t *testing.T) {
	m := &modelChannel{keys: seq(100)}
	cur, got := NewChannelCursor(0, ChannelChain{}), map[string]int{}
	drain(t, cur, m, 50, got)
	moved := m.keys[10]
	m.keys = slices.Delete(m.keys, 10, 11)
	m.keys = slices.Insert(m.keys, 5, "U")
	m.keys = append(m.keys, moved)
	drain(t, cur, m, 50, got)
	checkAll(t, m, got, "deep move")
}

// TestAdversaryDropAndInsertBelowTail inserts a peer's entry below the tail
// while the merged order rejects one local entry below the tail, as a state
// send with an expected-state key does once the peer's change lands first.
func TestAdversaryDropAndInsertBelowTail(t *testing.T) {
	m := &modelChannel{keys: seq(100)}
	cur, got := NewChannelCursor(0, ChannelChain{}), map[string]int{}
	drain(t, cur, m, 50, got)
	m.keys = slices.Delete(m.keys, 10, 11)
	m.keys = slices.Insert(m.keys, 5, "U")
	drain(t, cur, m, 50, got)
	checkAll(t, m, got, "drop and insert")
}

// TestAdversaryReplayBetweenRebaseAndRead installs a reorder after Read's
// rebase found nothing and before its read, then reads at least a tail's
// worth of entries in that one read.
func TestAdversaryReplayBetweenRebaseAndRead(t *testing.T) {
	for _, limit := range []uint64{50, 64, 100} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			m := &modelChannel{keys: seq(100)}
			cur, got := NewChannelCursor(0, ChannelChain{}), map[string]int{}
			drain(t, cur, m, 50, got)
			m.keys = append(m.keys, seq(300)[100:250]...)
			// Let the rebase's tail read pass, then reorder on the next read.
			m.onRead = func() {
				m.onRead = func() { m.keys = slices.Insert(m.keys, 20, "U") }
			}
			drain(t, cur, m, limit, got)
			checkAll(t, m, got, "replay mid-read")
		})
	}
}

// TestAdversaryRandomReorders runs random insertion-only merges (existing
// entries keep their relative order, as a union of operations does), random
// drops, appends and replays installed mid-read, with random read limits.
func TestAdversaryRandomReorders(t *testing.T) {
	for _, tc := range []struct {
		name     string
		maxLimit uint64
		drops    bool
	}{
		{"insert-limit50", 50, false},
		{"insert-limit128", 128, false},
		{"insert-drop-limit50", 50, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fails := 0
			for trial := range 300 {
				r := rand.New(rand.NewPCG(uint64(trial), 7))
				m := &modelChannel{}
				cur, got := NewChannelCursor(0, ChannelChain{}), map[string]int{}
				next := 0
				newKey := func() string { next++; return fmt.Sprintf("n%05d", next) }
				failed := false
				for range 60 {
					switch r.IntN(4) {
					case 0:
						for range r.IntN(80) {
							m.keys = append(m.keys, newKey())
						}
					case 1:
						if len(m.keys) > 0 {
							at := r.IntN(len(m.keys) + 1)
							m.keys = slices.Insert(m.keys, at, newKey())
						}
					case 2:
						if tc.drops && len(m.keys) > 0 {
							at := r.IntN(len(m.keys))
							m.keys = slices.Delete(m.keys, at, at+1)
						}
					case 3:
						if len(m.keys) > 0 {
							m.onRead = func() {
								m.onRead = func() {
									at := r.IntN(len(m.keys) + 1)
									m.keys = slices.Insert(m.keys, at, newKey())
								}
							}
						}
					}
					limit := 1 + uint64(r.IntN(int(tc.maxLimit)))
					for range 1 + r.IntN(3) {
						_, keys, err := cur.Read(t.Context(), limit, m.read)
						if err != nil {
							t.Fatal(err)
						}
						for _, k := range keys {
							got[k]++
						}
					}
				}
				m.onRead = nil
				drain(t, cur, m, 50, got)
				for _, k := range m.keys {
					if got[k] == 0 {
						failed = true
					}
				}
				if failed {
					fails++
				}
			}
			if fails != 0 {
				t.Errorf("%d of 300 random histories skipped an entry", fails)
			}
		})
	}
}

// TestAdversaryReadRebasesSilently follows the room graph's scan: it reads
// page after page. A replay installed between two pages makes Read move the
// cursor below the place the scan passed; Read must report that move, so a
// graph that caches entries by index can drop the stale ones.
func TestAdversaryReadRebasesSilently(t *testing.T) {
	m := &modelChannel{keys: seq(120)}
	cur := NewChannelCursor(0, ChannelChain{})
	moved := false
	for page := 0; cur.Next() < uint64(len(m.keys)); page++ {
		before := cur.Next()
		if page == 1 {
			// A merge lands a peer's entry inside the page just read.
			m.keys = slices.Insert(m.keys, 30, "U")
		}
		from, keys, err := cur.Read(t.Context(), 50, m.read)
		if err != nil {
			t.Fatal(err)
		}
		if len(keys) == 0 {
			break
		}
		if from < before {
			moved = true
			if from > 30 {
				t.Fatalf("page %d: Read resumed at %d, above the change at 30", page, from)
			}
		}
	}
	if !moved {
		t.Fatal("Read never reported the move below the scan's place")
	}
}
