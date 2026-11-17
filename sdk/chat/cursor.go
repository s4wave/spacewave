package spacewave_chat

import (
	"context"
	"crypto/sha256"
	"math/bits"
)

// ChannelChain is a channel's prefix chain: chain(i) = H(chain(i-1), key(i)),
// with the zero value standing for the empty prefix. Two orders agree on the
// chain at an index exactly when they hold the same keys below it, so one
// value names a whole prefix of the channel.
//
// Replay writes the chain where it numbers each entry, so the chain always
// describes the channel's current order, whatever reorder a merge made.
type ChannelChain [sha256.Size]byte

// Next returns the chain of the prefix extended by key.
func (c ChannelChain) Next(key string) ChannelChain {
	// Hash the prior chain, a fixed width, then the key, so no two
	// prefixes share an input.
	h := sha256.New()
	_, _ = h.Write(c[:])
	_, _ = h.Write([]byte(key))
	var next ChannelChain
	h.Sum(next[:0])
	return next
}

// Fold returns the chain of the prefix extended by keys in order.
func (c ChannelChain) Fold(keys []string) ChannelChain {
	for _, key := range keys {
		c = c.Next(key)
	}
	return c
}

// ChannelRead reads one snapshot of a channel: the chain of the entries below
// index start and up to limit keys from start in index order. ok is false when
// the channel holds fewer than start entries. A read with limit zero returns
// only the chain.
type ChannelRead func(ctx context.Context, start, limit uint64) (base ChannelChain, keys []string, ok bool, err error)

// ChannelCursor is a reader's place in a channel's history, for every reader
// that follows a channel: a watch, a sync, a scan.
//
// Replay assigns each entry its index from the agreed order of the Space's
// operations, and a merge can place a peer's concurrent entry below entries a
// reader already passed, or drop an entry the merged order refuses. An index
// alone then points past an entry the reader never saw.
//
// The cursor is its index with the chain of the entries below it. Each Read
// takes the chain and the next keys from one snapshot; when the snapshot's
// chain at the index differs from the cursor's, the channel changed below
// the index. The cursor then finds the highest place it passed whose chain
// still matches, using a ladder of past places at power-of-two distances, and
// reads again from there. It never skips an entry: a key it folded is a key it
// returned, so a cursor whose chain matches the channel's has returned every
// entry below its index. After a change it may return an entry again, at most
// about twice as many entries as the change moved; a reader that keeps entries
// by key takes the repeat as a no-op.
//
// The cursor holds a fixed amount of state, whatever the channel's length.
// A ChannelCursor is not safe for concurrent use.
type ChannelCursor struct {
	// next is the index of the first entry not yet returned.
	next uint64
	// chain is the chain of the entries below next.
	chain ChannelChain
	// ladder holds, at rung k, the highest multiple of 2^k below next that
	// the cursor has stood at, with its chain. Rung indexes never rise with k,
	// and a rung not yet reached holds index zero and the empty chain.
	ladder [64]channelPlace
}

// channelPlace is an index with the chain of the entries below it.
type channelPlace struct {
	index uint64
	chain ChannelChain
}

// NewChannelCursor returns a cursor at index next whose entries below have
// the given chain, as for a stored position. A cursor at zero takes the zero
// chain. A cursor restored this way knows only its own place and zero: a
// change below next sends it back to zero.
func NewChannelCursor(next uint64, chain ChannelChain) *ChannelCursor {
	return &ChannelCursor{next: next, chain: chain}
}

// Next returns the index of the first entry the cursor has not returned.
func (c *ChannelCursor) Next() uint64 {
	return c.next
}

// Chain returns the chain of the entries below Next, which a reader stores
// with Next to restore the cursor.
func (c *ChannelCursor) Chain() ChannelChain {
	return c.chain
}

// Read returns up to limit entries from one snapshot of the channel, starting
// at index from, and passes them. It returns no keys when the cursor has
// returned every entry. from is below the Next before the call when the
// channel changed below it: the entries at and above from may include ones
// returned before, and a reader that keeps state by index drops what it holds
// at and above from.
func (c *ChannelCursor) Read(ctx context.Context, limit uint64, read ChannelRead) (from uint64, keys []string, err error) {
	for {
		base, keys, ok, err := read(ctx, c.next, max(limit, 1))
		if err != nil {
			return 0, nil, err
		}
		if ok && base == c.chain {
			from = c.next
			for _, key := range keys {
				c.pass(key)
			}
			return from, keys, nil
		}
		if err := c.rewind(ctx, read); err != nil {
			return 0, nil, err
		}
	}
}

// rewind moves the cursor back to the highest rung whose chain the channel
// still holds. Rungs below a matching rung match too, so a binary search over
// the rungs finds it; rung 63 is index zero for any channel shorter than 2^63,
// whose empty chain always matches.
func (c *ChannelCursor) rewind(ctx context.Context, read ChannelRead) error {
	lo, hi := 0, len(c.ladder)-1
	for lo < hi {
		mid := (lo + hi) / 2
		place := c.ladder[mid]
		base, _, ok, err := read(ctx, place.index, 0)
		if err != nil {
			return err
		}
		if ok && base == place.chain {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	c.resume(lo)
	return nil
}

// resume moves the cursor to the place at rung k. The rungs below it stood
// above that place, so they take the place of the next rung up, which lies
// below it.
func (c *ChannelCursor) resume(k int) {
	// Stand at the rung's place.
	place := c.ladder[k]
	c.next, c.chain = place.index, place.chain

	// Lower the rungs at and below k to the next rung's place.
	below := channelPlace{}
	if k+1 < len(c.ladder) {
		below = c.ladder[k+1]
	}
	for j := range k + 1 {
		c.ladder[j] = below
	}
}

// pass advances the cursor past key. The place it leaves becomes each rung
// whose multiple that place is.
func (c *ChannelCursor) pass(key string) {
	place := channelPlace{index: c.next, chain: c.chain}
	for k := range min(bits.TrailingZeros64(c.next), len(c.ladder)-1) + 1 {
		c.ladder[k] = place
	}
	c.next++
	c.chain = c.chain.Next(key)
}
