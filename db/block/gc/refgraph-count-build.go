package block_gc

import (
	"bytes"
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// Count builds. A store that tracked blocks before it kept counts builds them
// in slices, each one write transaction, while writes continue:
//
//   - walking counts each child list, then each owner edge of a block from a
//     node other than the unreferenced queue, in key order. The cursor is the
//     key after the last record counted. A write counts a holder only when its
//     record sorts before the cursor, so each holder is counted once: by the
//     write, or by the walk when it reaches the record as the write left it.
//   - checking reads each block with a count and no child list. Such a block
//     is a hole unless the store holds it: a stored block without a list may
//     hold children nobody counted, so the counts go into doubt.
//
// The counts are then complete.

// StartCounts starts the counts of a store that has none: complete at once
// when the graph tracks no block and no owner edge names one, so every
// block's count starts at its first write, and otherwise a build from the
// first child list. It keeps a state already recorded and returns the state.
func (g *RefGraph) StartCounts(ctx context.Context) (CountState, error) {
	var state CountState
	err := g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		// Keep a state already recorded.
		value, found, err := tx.Get(ctx, countsKey)
		if err != nil || found {
			state = decodeCountState(value, found)
			return err
		}

		// Look for a child list, or an owner edge of a block. An incoming
		// record of a block object begins with its hash type, which sorts
		// after a text node's zero byte.
		var tracked bool
		for _, prefix := range [][]byte{{kindChildren}, {kindIncoming, textNode + 1}} {
			it := tx.Iterate(ctx, prefix[:1], true, false)
			err := it.Seek(prefix)
			tracked = err == nil && it.Valid()
			if err == nil {
				err = it.Err()
			}
			it.Close()
			if err != nil {
				return err
			}
			if tracked {
				break
			}
		}

		// Record complete counts, or a build from the first child list.
		record := []byte{countsComplete}
		if tracked {
			record = []byte{countsWalking, kindChildren}
		}
		state = decodeCountState(record, true)
		return tx.Set(ctx, countsKey, record)
	})
	return state, err
}

// BuildCounts runs one slice of a count build in one write transaction: it
// counts or checks up to limit records, stopping early when yield reports a
// waiting writer, after the first. stored reports whether the store holds a
// block. It returns the state after the slice; a state other than
// CountsWalking and CountsChecking has nothing to build.
func (g *RefGraph) BuildCounts(
	ctx context.Context,
	limit int,
	yield func() bool,
	stored func(context.Context, *block.BlockRef) (bool, error),
) (CountState, error) {
	var state CountState
	err := g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		// Read the state; only a build has a slice to run.
		c, err := openCounter(ctx, tx)
		if err != nil {
			return err
		}
		if !c.on || c.phase == countsComplete {
			value, found, err := tx.Get(ctx, countsKey)
			state = decodeCountState(value, found)
			return err
		}

		// Run the slice, which reads its first record whatever yield says.
		var n int
		more := func() bool {
			n++
			return n == 1 || (n <= limit && !yield())
		}
		if c.phase == countsWalking {
			err = c.walk(ctx, tx, more)
		} else {
			err = c.check(ctx, tx, more, stored)
		}
		if err != nil {
			return err
		}

		// Write the slice's counts and the new state.
		if err := c.flush(ctx, tx); err != nil {
			return err
		}
		if !c.on {
			state = CountState{Phase: CountsDoubt, Doubt: c.doubt}
			return nil
		}
		record := append([]byte{c.phase}, c.cursor...)
		state = decodeCountState(record, true)
		return tx.Set(ctx, countsKey, record)
	})
	return state, err
}

// walk counts the child lists, then the owner edges of blocks, from the
// cursor while more allows, and moves to checking after the last.
func (c *counter) walk(ctx context.Context, tx kvtx.Tx, more func() bool) error {
	// Count the child lists.
	if c.cursor[0] == kindChildren {
		done, err := c.walkRange(ctx, tx, kindChildren, more, func(key, value []byte) error {
			// Count the list as its block's first write would.
			block, _, err := readNode(key[1:])
			if err != nil {
				return err
			}
			children, err := decodeChildren(value)
			if err != nil {
				return err
			}
			c.listCreated(block, children)
			return nil
		})
		if err != nil || !done {
			return err
		}
		c.cursor = []byte{kindIncoming, textNode + 1}
	}

	// Count the owner edges of blocks, which sort after those of text nodes.
	done, err := c.walkRange(ctx, tx, kindIncoming, more, func(key, _ []byte) error {
		// Count an owner other than the unreferenced queue.
		object, rest, err := readNode(key[1:])
		if err != nil {
			return err
		}
		subject, _, err := readNode(rest)
		if err != nil {
			return err
		}
		if subject != NodeUnreferenced {
			c.add(object, 1)
		}
		return nil
	})
	if err == nil && done {
		c.phase, c.cursor = countsChecking, []byte{kindCount}
	}
	return err
}

// walkRange passes each record of kind from the cursor to count, moving the
// cursor past it first, while more allows and the counts stay on. It reports
// whether it passed the last record.
func (c *counter) walkRange(
	ctx context.Context,
	tx kvtx.Tx,
	kind byte,
	more func() bool,
	count func(key, value []byte) error,
) (bool, error) {
	// Resume at the cursor.
	it := tx.Iterate(ctx, []byte{kind}, true, false)
	defer it.Close()
	if err := it.Seek(c.cursor); err != nil {
		return false, err
	}

	// Count each record, moving the cursor past it first.
	for ; it.Valid(); it.Next() {
		if !c.on || !more() {
			return false, nil
		}
		value, err := it.Value()
		if err != nil {
			return false, err
		}
		key := it.Key()
		c.cursor = append(bytes.Clone(key), 0)
		if err := count(key, value); err != nil {
			return false, err
		}
	}
	return c.on, it.Err()
}

// check reads each block with a count and no child list from the cursor while
// more allows, and puts the counts in doubt at the first the store holds. The
// counts are complete after the last.
func (c *counter) check(
	ctx context.Context,
	tx kvtx.Tx,
	more func() bool,
	stored func(context.Context, *block.BlockRef) (bool, error),
) error {
	// Resume at the cursor.
	it := tx.Iterate(ctx, []byte{kindCount}, true, false)
	defer it.Close()
	if err := it.Seek(c.cursor); err != nil {
		return err
	}

	// Check each block, moving the cursor past it first.
	for ; it.Valid(); it.Next() {
		if !more() {
			return nil
		}
		key := it.Key()
		c.cursor = append(bytes.Clone(key), 0)
		node, _, err := readNode(key[1:])
		if err != nil {
			return err
		}

		// Skip a block with a child list.
		listed, err := tx.Exists(ctx, graphKey(kindChildren, node))
		if err != nil {
			return err
		}
		if listed {
			continue
		}

		// Doubt the counts when the store holds a block without a list.
		ref, ok := ParseBlockIRI(node)
		if !ok {
			return errors.Errorf("count record of %q is not a block", node)
		}
		held, err := stored(ctx, ref)
		if err != nil {
			return err
		}
		if held {
			c.distrust("block " + FormatIRI(node) + " is stored without a child list")
			return nil
		}
	}
	if err := it.Err(); err != nil {
		return err
	}
	c.phase, c.cursor = countsComplete, nil
	return nil
}

// GetCountState reads the state of the counts.
func (g *RefGraph) GetCountState(ctx context.Context) (CountState, error) {
	var state CountState
	err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		value, found, err := tx.Get(ctx, countsKey)
		state = decodeCountState(value, found)
		return err
	})
	return state, err
}
