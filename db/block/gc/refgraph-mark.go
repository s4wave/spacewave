package block_gc

import (
	"bytes"
	"context"
	"encoding/binary"
	"slices"

	"github.com/s4wave/spacewave/db/kvtx"
)

// unknownSample bounds the blocks of unknown references a mark reports.
const unknownSample = 16

// BlockMark is the result of marking the blocks of a graph in one snapshot.
type BlockMark struct {
	// Live holds every node the mark reached. A rescue adds to it.
	Live *NodeSet
	// Unreached lists the tracked blocks the mark did not reach, in key order.
	Unreached []string
	// Unknown counts the reached blocks without a child list, whose
	// references were never recorded. A mark with any sweeps nothing, since
	// such a block may reference any unreached one.
	Unknown int
	// UnknownSample lists up to unknownSample of those blocks.
	UnknownSample []string
}

// MarkBlocks marks the blocks of the graph in one snapshot. Every node that is
// not a block keeps the reference count and its queue, so the mark treats each
// one as live along with the graph root and the registered roots, and walks
// child lists and forward edges from the blocks they own. The tracked blocks
// it does not reach are the sweep's candidates. Opening a pass with
// BeginBlockPass before the mark lets a sweep rescue the blocks writes adopt
// after the snapshot.
func (g *RefGraph) MarkBlocks(ctx context.Context) (*BlockMark, error) {
	var mark *BlockMark
	err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		// Mark the seeds, stacking the blocks to walk.
		mark = &BlockMark{Live: NewNodeSet()}
		live := mark.Live
		var stack []string
		err := seedMarkTx(ctx, tx, func(node string) {
			if node != NodeUnreferenced && live.Add(node) && isBlockNode(node) {
				stack = append(stack, node)
			}
		})
		if err != nil {
			return err
		}

		// Walk the owned blocks, noting those of unknown references.
		err = walkTx(ctx, tx, live, stack, func(block string) {
			if mark.Unknown++; len(mark.UnknownSample) < unknownSample {
				mark.UnknownSample = append(mark.UnknownSample, block)
			}
		})
		if err != nil {
			return err
		}

		// List the tracked blocks the walk did not reach.
		lists := graphKey(kindChildren)
		return tx.ScanPrefixKeys(ctx, lists, func(key []byte) error {
			if block := lastNode(lists, key); !live.Has(block) {
				mark.Unreached = append(mark.Unreached, block)
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return mark, nil
}

// seedMarkTx calls seed with the graph root, each registered root, and both
// nodes of each forward record whose subject is not a block.
func seedMarkTx(ctx context.Context, tx kvtx.Tx, seed func(string)) error {
	// Seed the graph root and the registered roots.
	seed(NodeGCRoot)
	roots := graphKey(kindRoot)
	err := tx.ScanPrefixKeys(ctx, roots, func(key []byte) error {
		seed(lastNode(roots, key))
		return nil
	})
	if err != nil {
		return err
	}

	// Seed both nodes of each edge from a text node.
	owners := []byte{kindForward, textNode}
	return tx.ScanPrefixKeys(ctx, owners, func(key []byte) error {
		// Decode the subject and the object.
		subject, rest, err := readNode(key[1:])
		if err != nil {
			return err
		}
		object, _, err := readNode(rest)
		if err != nil {
			return err
		}

		// Seed them.
		seed(subject)
		seed(object)
		return nil
	})
}

// walkTx walks child lists and forward edges from the nodes on stack, adding
// each node it reaches to reached. untracked, when set, is called with each
// block it reaches that has no child list. It pops chunks of nodes and loads
// the index pages of each chunk's records together in key order, so a cold
// store costs a batch of reads per chunk instead of one read per node, and the
// work list stays small however wide the graph is.
func walkTx(ctx context.Context, tx kvtx.Tx, reached *NodeSet, stack []string, untracked func(string)) error {
	visit := func(node string) {
		if reached.Add(node) {
			stack = append(stack, node)
		}
	}
	keys := make([][]byte, 0, 2*reachChunk)
	for len(stack) != 0 {
		// Pop a chunk and order the keys of its records.
		chunk := stack[max(0, len(stack)-reachChunk):]
		keys = keys[:0]
		for _, node := range chunk {
			if node == NodeUnreferenced {
				continue
			}
			if isBlockNode(node) {
				keys = append(keys, graphKey(kindChildren, node))
			} else {
				keys = append(keys, graphKey(kindForward, node))
			}
		}
		stack = stack[:len(stack)-len(chunk)]
		slices.SortFunc(keys, bytes.Compare)

		// Load the chunk's index pages together, then push each new child
		// and target.
		if err := kvtx.Prefetch(ctx, tx, keys); err != nil {
			return err
		}
		for _, key := range keys {
			if err := walkRecordsTx(ctx, tx, key, visit, untracked); err != nil {
				return err
			}
		}
	}
	return nil
}

// walkRecordsTx calls visit with each node that key names as a target: the
// children in a child list record, or the targets of the forward records
// under a forward prefix. untracked, when set, is called with the block of a
// missing child list.
func walkRecordsTx(ctx context.Context, tx kvtx.Tx, key []byte, visit, untracked func(string)) error {
	// Read the children of a child list.
	if key[0] == kindChildren {
		value, found, err := tx.Get(ctx, key)
		if err != nil {
			return err
		}
		if !found {
			if untracked != nil {
				untracked(lastNode(key[:1], key))
			}
			return nil
		}
		children, err := decodeChildren(value)
		if err != nil {
			return err
		}
		for _, child := range children {
			visit(child)
		}
		return nil
	}

	// Scan the targets of the forward records.
	return tx.ScanPrefixKeys(ctx, key, func(record []byte) error {
		visit(lastNode(key, record))
		return nil
	})
}

// passKey marks a running block pass.
var passKey = []byte{kindPass}

// BeginBlockPass opens a block pass. From its commit until EndBlockPass, each
// write appends the blocks it adopts to the adopt log, so a sweep after the
// mark can rescue them with RescueAdopted. It first drops the log of an
// earlier pass that never ended. The caller runs one pass at a time per graph.
func (g *RefGraph) BeginBlockPass(ctx context.Context) error {
	return g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		if err := deleteAdoptLogTx(ctx, tx); err != nil {
			return err
		}
		return tx.Set(ctx, passKey, nil)
	})
}

// EndBlockPass closes the block pass and deletes its adopt log, which holds
// only what writes logged since the last rescue.
func (g *RefGraph) EndBlockPass(ctx context.Context) error {
	return g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		if err := tx.Delete(ctx, passKey); err != nil {
			return err
		}
		return deleteAdoptLogTx(ctx, tx)
	})
}

// RescueAdopted consumes the adopt log, adding each block it names and every
// node reachable from one to mark.Live, and returns the number of nodes added.
// A sweep runs it first in each transaction that removes blocks, so no block
// a committed write adopted is removed.
func (g *RefGraph) RescueAdopted(ctx context.Context, mark *BlockMark) (int, error) {
	var rescued int
	err := g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		// Consume the log.
		blocks, err := readAdoptLogTx(ctx, tx)
		if err != nil {
			return err
		}
		if err := deleteAdoptLogTx(ctx, tx); err != nil {
			return err
		}

		// Walk from the blocks the mark and earlier rescues did not reach.
		before := mark.Live.Len()
		var stack []string
		for _, block := range blocks {
			if mark.Live.Add(block) {
				stack = append(stack, block)
			}
		}
		err = walkTx(ctx, tx, mark.Live, stack, nil)
		rescued = mark.Live.Len() - before
		return err
	})
	return rescued, err
}

// readAdoptLogTx returns the blocks of every adopt log record.
func readAdoptLogTx(ctx context.Context, tx kvtx.Tx) ([]string, error) {
	var blocks []string
	err := tx.ScanPrefix(ctx, []byte{kindAdopt}, func(_, value []byte) error {
		logged, err := decodeChildren(value)
		blocks = append(blocks, logged...)
		return err
	})
	return blocks, err
}

// logAdoptedTx appends the blocks a write adopted to the adopt log while a
// block pass runs.
func logAdoptedTx(ctx context.Context, tx kvtx.Tx, blocks []string) error {
	// Log only during a pass.
	if len(blocks) == 0 {
		return nil
	}
	open, err := tx.Exists(ctx, passKey)
	if err != nil || !open {
		return err
	}

	// Number the record after the last.
	var seq uint64
	it := tx.Iterate(ctx, []byte{kindAdopt}, true, true)
	if it.Next() {
		seq = binary.BigEndian.Uint64(it.Key()[1:]) + 1
	}
	err = it.Err()
	it.Close()
	if err != nil {
		return err
	}

	// Write the sorted, distinct blocks.
	blocks = slices.Clone(blocks)
	slices.Sort(blocks)
	return tx.Set(ctx, adoptKey(seq), encodeChildren(slices.Compact(blocks)))
}

// adoptKey returns the key of the adopt log record seq.
func adoptKey(seq uint64) []byte {
	return binary.BigEndian.AppendUint64([]byte{kindAdopt}, seq)
}

// deleteAdoptLogTx deletes the adopt log.
func deleteAdoptLogTx(ctx context.Context, tx kvtx.Tx) error {
	// List the keys, then delete them.
	var keys [][]byte
	err := tx.ScanPrefixKeys(ctx, []byte{kindAdopt}, func(key []byte) error {
		keys = append(keys, slices.Clone(key))
		return nil
	})
	if err != nil {
		return err
	}
	for _, key := range keys {
		if err := tx.Delete(ctx, key); err != nil {
			return err
		}
	}
	return nil
}
