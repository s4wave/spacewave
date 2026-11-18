package spacewave_chat

import (
	"context"
	"fmt"
	"slices"
	"testing"
)

// TestAdversaryRefusalKeepsCountNoWake installs a merged order in which a
// peer's send lands and the local send is refused, as an expected-state send
// is once the peer's change merges first. The channel then holds as many
// entries as before, written as many times, so a watch keyed on the channel
// object's revision is not woken, and the peer's send goes undelivered until
// the next write to the channel.
func TestAdversaryRefusalKeepsCountNoWake(t *testing.T) {
	c := newReorderChannel(t)
	merged := c.order(reorderSend{"alice", "a1"})
	c.order(reorderSend{"bob", "b1"})

	w := c.watch()
	w.expect("local order", "b0", "b1")
	c.install(merged)
	w.expect("after merge refusing b1", "a1")
}

// TestAdversaryRewindBelowLowestRung reads a long channel to its end, so the
// ladder's low rungs sit close below the cursor, then reorders an entry below
// every rung but the zero rung. The cursor must fall back to a rung deep
// enough to see the change.
func TestAdversaryRewindBelowLowestRung(t *testing.T) {
	for _, at := range []int{0, 1, 3, 1000, 2047, 2048, 3000} {
		m := &modelChannel{keys: seq(4097)}
		cur, got := NewChannelCursor(0, ChannelChain{}), map[string]int{}
		drain(t, cur, m, 7, got)
		m.keys = slices.Insert(m.keys, at, "U")
		drain(t, cur, m, 7, got)
		checkAll(t, m, got, fmt.Sprint("insert at ", at))
	}
}

// TestAdversaryReorderBetweenRungs reads to 1100 in pages of 64, so the
// ladder holds rungs at 1100, 1088, 1024 and below, then changes the channel
// at every index from 960 to 1099: at a rung, between two rungs, and just
// below the cursor, inserting one entry and, in turn, dropping the next.
func TestAdversaryReorderBetweenRungs(t *testing.T) {
	for at := 960; at < 1100; at++ {
		for _, drop := range []bool{false, true} {
			m := &modelChannel{keys: seq(1100)}
			cur, got := NewChannelCursor(0, ChannelChain{}), map[string]int{}
			drain(t, cur, m, 64, got)
			if drop {
				m.keys = slices.Delete(m.keys, at, at+1)
			}
			m.keys = slices.Insert(m.keys, at, "U")
			drain(t, cur, m, 64, got)
			checkAll(t, m, got, fmt.Sprintf("at %d drop %v", at, drop))
		}
	}
}

// TestAdversaryTornRead reads the chain from one order and the keys from
// another, as a reader on a World state with no snapshot can. The cursor may
// deliver a torn page's keys, but must not skip an entry of the order that
// stays.
func TestAdversaryTornRead(t *testing.T) {
	for _, at := range []int{0, 10, 49, 50, 51, 99} {
		m := &modelChannel{keys: seq(100)}
		cur, got := NewChannelCursor(0, ChannelChain{}), map[string]int{}
		drain(t, cur, m, 50, got)
		m.keys = append(m.keys, seq(200)[100:200]...)
		old := slices.Clone(m.keys)
		torn := func(start, limit uint64) (ChannelChain, []string, bool, error) {
			// The chain comes from the old order, the keys from the new.
			chain := ChannelChain{}.Fold(old[:min(start, uint64(len(old)))])
			m.keys = slices.Insert(slices.Clone(old), at, "U")
			end := min(start+limit, uint64(len(m.keys)))
			if start > end {
				return chain, nil, false, nil
			}
			return chain, slices.Clone(m.keys[start:end]), true, nil
		}
		if _, keys, err := cur.Read(t.Context(), 50, func(_ context.Context, start, limit uint64) (ChannelChain, []string, bool, error) {
			return torn(start, limit)
		}); err != nil {
			t.Fatal(err)
		} else {
			for _, key := range keys {
				got[key]++
			}
		}
		drain(t, cur, m, 50, got)
		checkAll(t, m, got, fmt.Sprint("torn at ", at))
	}
}
