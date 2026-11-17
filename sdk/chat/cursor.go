package spacewave_chat

import (
	"context"
	"maps"
	"slices"
)

// CursorTailLen bounds the delivered keys a ChannelCursor remembers below its
// position. A reorder whose lowest changed index lies within this many entries
// of the position is found exactly; a deeper one is found as lost, below.
const CursorTailLen = chatMessagePageSize

// ChannelKeys reads the keys of a channel's entries in the half-open interval
// [start, end) of its current order.
type ChannelKeys func(ctx context.Context, start, end uint64) ([]string, error)

// ChannelCursor is a reader's place in a channel's history, for every reader
// that follows a channel: a watch, a sync, a scan.
//
// Replay assigns each entry its index from the agreed order of the Space's
// operations, and a merge can place a peer's concurrent entry below entries a
// reader already passed, moving them up. An index alone then points past the
// peer's entry. The cursor keeps its invariant across any reorder a replay
// makes: its position never passes an entry it has not delivered.
//
// The cursor holds its index with the keys it delivered just below it, the
// tail. Before each read it compares the tail with the channel's keys at the
// same indexes. The lowest index where they differ is the lowest changed
// index: the cursor moves back to it and re-reads from there, skipping the
// keys it delivered. The channel keeps no record of its previous order, for
// its state is a function of the order alone, so the cursor derives the
// changed index from what it delivered.
//
// The tail holds at most CursorTailLen keys and the skip set at most another
// CursorTailLen, since each skipped key left the tail. A reorder that changes
// the bottom of a full tail may reach below it; the cursor then cannot name
// what it delivered there, so it reports itself lost and re-reads from zero,
// skipping only its tail. A lost cursor may deliver an entry twice; it never
// skips one. A reorder that leaves the whole tail in place but moves an entry
// from below the tail to above the position is not seen; that needs a single
// reorder spanning more than CursorTailLen entries.
//
// A ChannelCursor is not safe for concurrent use.
type ChannelCursor struct {
	// next is the index of the first entry not yet passed.
	next uint64
	// tail holds the keys at [next-len(tail), next) in the order last read.
	tail []string
	// skip holds delivered keys a reorder moved to next or above, which the
	// cursor passes without delivering.
	skip map[string]struct{}
}

// NewChannelCursor returns a cursor at index next with no delivered keys, as
// for a position stored as a bare index: it re-reads from next and never
// below.
func NewChannelCursor(next uint64) *ChannelCursor {
	return &ChannelCursor{next: next}
}

// Next returns the index of the first entry the cursor has not passed.
func (c *ChannelCursor) Next() uint64 {
	return c.next
}

// Clone returns a copy of the cursor, which a reader keeps to return to this
// place when it cannot finish handling the entries a Read returns.
func (c *ChannelCursor) Clone() *ChannelCursor {
	clone := &ChannelCursor{next: c.next, tail: slices.Clone(c.tail)}
	if c.skip != nil {
		clone.skip = maps.Clone(c.skip)
	}
	return clone
}

// Read returns up to limit entries of a channel holding count entries that the
// cursor has not delivered, with their indexes, in index order, and passes
// them. It returns none when the cursor has delivered every entry. lost
// reports that a reorder reached below the tail, so the entries returned may
// include ones delivered before.
func (c *ChannelCursor) Read(ctx context.Context, count, limit uint64, keysAt ChannelKeys) (indexes []uint64, keys []string, lost bool, err error) {
	// Move back to the lowest changed index.
	_, lost, err = c.Rebase(ctx, count, keysAt)
	if err != nil {
		return nil, nil, false, err
	}

	// Read forward, passing the delivered keys a reorder moved.
	for c.next < count && len(keys) == 0 {
		read, err := keysAt(ctx, c.next, min(c.next+max(limit, 1), count))
		if err != nil {
			return nil, nil, false, err
		}
		if len(read) == 0 {
			break
		}
		for _, key := range read {
			index := c.next
			c.pass(key)
			if _, ok := c.skip[key]; ok {
				delete(c.skip, key)
				continue
			}
			indexes, keys = append(indexes, index), append(keys, key)
		}
	}

	// Every present entry is passed, so a skipped key not seen is absent; if
	// it returns it is delivered again, never lost.
	if c.next >= count {
		c.skip = nil
	}
	return indexes, keys, lost, nil
}

// Rebase moves the cursor back to the lowest index of a channel holding count
// entries whose key differs from the one it delivered there, moving the
// delivered keys at and above it to skip, and returns that index: Next when
// nothing it delivered moved. lost reports a change that may reach below the
// tail, after which the cursor is at zero. Read rebases first; a reader that
// caches entries by index calls Rebase to learn which of them a replay moved.
func (c *ChannelCursor) Rebase(ctx context.Context, count uint64, keysAt ChannelKeys) (uint64, bool, error) {
	// Read the channel's keys under the tail.
	base := c.next - uint64(len(c.tail))
	var current []string
	if end := min(c.next, count); end > base {
		var err error
		current, err = keysAt(ctx, base, end)
		if err != nil {
			return 0, false, err
		}
	}

	// Find the first index where they differ.
	same := 0
	for same < len(current) && same < len(c.tail) && current[same] == c.tail[same] {
		same++
	}
	if same == len(c.tail) {
		return c.next, false, nil
	}

	// A change at the bottom of the tail may reach below it.
	if same == 0 && base != 0 {
		c.next, c.tail, c.skip = 0, nil, toSet(c.tail)
		return 0, true, nil
	}
	if c.skip == nil {
		c.skip = make(map[string]struct{}, len(c.tail)-same)
	}
	for _, key := range c.tail[same:] {
		c.skip[key] = struct{}{}
	}
	c.next, c.tail = base+uint64(same), c.tail[:same]
	return c.next, false, nil
}

// pass advances the cursor past key, keeping the tail bounded.
func (c *ChannelCursor) pass(key string) {
	c.next++
	if len(c.tail) == CursorTailLen {
		c.tail = append(c.tail[:0], c.tail[1:]...)
	}
	c.tail = append(c.tail, key)
}

// toSet returns the keys as a set.
func toSet(keys []string) map[string]struct{} {
	set := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		set[key] = struct{}{}
	}
	return set
}
