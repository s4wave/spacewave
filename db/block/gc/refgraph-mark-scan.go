package block_gc

import (
	"context"
	"errors"
	"hash/maphash"
	"slices"
	"time"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/hash"
)

// MarkReadBudget bounds how long MarkBlocksScan holds one read transaction.
// While a transaction is open the store cannot reuse pages freed after it
// began, so a short one keeps freed space reusable during a pass.
const MarkReadBudget = 100 * time.Millisecond

// markBudgetCheck is how many records a scan visits between clock reads.
const markBudgetCheck = 64

// markLimit bounds one read transaction of a scan mark: at most records
// records when nonzero, and at most budget when nonzero. Its tail bounds the
// blocks a scan mark expands by point reads instead of another scan, none
// when zero, as markScan describes.
type markLimit struct {
	records int
	budget  time.Duration
	tail    int
}

// MarkBlocksScan marks like MarkBlocks, but reads the child lists in key order
// in read transactions of at most MarkReadBudget each, so a mark costs about
// one read per index page of the lists instead of one per block, and holds no
// snapshot for long. A block pass must be open, as BeginBlockPass describes:
// its adopt log lets the sweep's rescue keep every block a write adopts
// between the mark's transactions.
//
// It never falls back to MarkBlocks to name the reached blocks without a
// child list: that walk would hold one snapshot for the whole graph.
func (g *RefGraph) MarkBlocksScan(ctx context.Context) (*BlockMark, error) {
	return g.markScan(ctx, markLimit{budget: MarkReadBudget, tail: markTailLimit}, nil)
}

// markScan marks like MarkBlocks, but reads the child lists in key order:
// each scan expands every list whose block is live and was not yet expanded,
// and expands every child it adds after its position. The children it adds
// behind its position are the scan's tail. While the tail stays within
// limit.tail, expandTail expands it and everything it reaches by point
// reads, which ends the mark; otherwise another scan follows. A scan reads
// each index page of the lists once, so the scans cost pages, and the tail
// costs a read a block only once it is small. Without the tail a chain whose
// links sort before their children would cost a scan a link. It counts the
// blocks without a list in mark.Unknown and lists them, as settleUnknown
// describes.
//
// Each read transaction stays within limit, and the next resumes after the
// last record read, calling between first when set. The adopt log of an open
// pass makes that safe: a write that adds an edge to a block after the mark
// read the edge's source logs the block, and the sweep rescues it. Seeding
// takes both ends of every edge from a node that is not a block, so a seed
// read early misses only edges added later, which the log covers.
func (g *RefGraph) markScan(ctx context.Context, limit markLimit, between func() error) (*BlockMark, error) {
	// Mark seeds live, counting the blocks among them.
	mark := &BlockMark{Live: NewNodeSet()}
	live := mark.Live
	var liveBlocks int
	seed := func(node string) {
		if node != NodeUnreferenced && live.Add(node) && isBlockNode(node) {
			liveBlocks++
		}
	}

	// Seed the graph root and the registered roots.
	seed(NodeGCRoot)
	roots := graphKey(kindRoot)
	err := g.scanRecords(ctx, mark, roots, limit, between, func(key []byte, _ kvtx.Iterator) error {
		mark.Roots++
		seed(lastNode(roots, key))
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Seed both ends of each edge from a text node.
	owners := []byte{kindForward, textNode}
	var last string
	err = g.scanRecords(ctx, mark, owners, limit, between, func(key []byte, _ kvtx.Iterator) error {
		// Decode the subject and the object.
		subject, rest, err := readNode(key[1:])
		if err != nil {
			return err
		}
		object, _, err := readNode(rest)
		if err != nil {
			return err
		}

		// Count and seed them.
		mark.countOwner(subject, last)
		last = subject
		seed(subject)
		seed(object)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Scan until a scan's tail is empty or expanded by point reads.
	expanded := NewNodeSet()
	lists := graphKey(kindChildren)
	for {
		mark.Scans++
		var tail []byte
		var behind int
		err := g.scanRecords(ctx, mark, lists, limit, between, func(key []byte, it kvtx.Iterator) error {
			// Expand a live list once, reading its value only then.
			enc := key[len(lists):]
			if !live.hasBlock(enc) || !expanded.addBlock(enc) {
				return nil
			}
			value, err := it.Value()
			if err != nil {
				return err
			}

			// Add its children, keeping those behind the position, which
			// this scan cannot expand, in the tail while it fits.
			for len(value) != 0 {
				n := blockNodeLen(value)
				if n == 0 || n > len(value) {
					return errChildNotBlock
				}
				if child := value[:n]; live.addBlock(child) {
					liveBlocks++
					if string(child) < string(enc) {
						if behind++; behind <= limit.tail {
							tail = append(tail, child...)
						}
					}
				}
				value = value[n:]
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		if behind == 0 {
			break
		}
		if behind > limit.tail {
			continue
		}

		// Expand the tail, scanning again only if it grew past the limit.
		added, done, err := g.expandTail(ctx, mark, expanded, tail, behind, limit, between)
		liveBlocks += added
		if err != nil {
			return nil, err
		}
		if done {
			break
		}
	}

	// A live block without a list has unknown references. Settle those a
	// write gave a list after the scans passed them, then list the
	// unreached blocks with one more key scan.
	if liveBlocks != expanded.Len() {
		added, err := g.settleUnknown(ctx, mark, expanded, limit, between)
		if err != nil {
			return nil, err
		}
		liveBlocks += added
	}
	mark.Unknown = liveBlocks - expanded.Len()
	err = g.scanRecords(ctx, mark, lists, limit, between, func(key []byte, _ kvtx.Iterator) error {
		if block := lastNode(lists, key); !live.Has(block) {
			mark.Unreached = append(mark.Unreached, block)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return mark, nil
}

// markTailLimit is MarkBlocksScan's limit.tail. A point read costs about as
// much as scanning a few dozen records, so the bound is far below the lists
// of a large graph, and its encodings take a few MiB.
const markTailLimit = 1 << 16

// expandTail expands the live blocks queued in tail, the concatenated
// encodings of queued blocks, and each block their lists make live, by point
// reads in transactions within limit. It returns how many blocks it made
// live, and whether it expanded every block it reached: it stops once queued
// blocks would pass limit.tail, leaving the rest live but unexpanded for
// another scan. A block without a list stays unexpanded, for settleUnknown.
func (g *RefGraph) expandTail(ctx context.Context, mark *BlockMark, expanded *NodeSet, tail []byte, queued int, limit markLimit, between func() error) (int, bool, error) {
	var added int
	full := false
	for first := true; len(tail) != 0 && !full; first = false {
		// Let the caller act between transactions.
		if !first && between != nil {
			if err := between(); err != nil {
				return added, false, err
			}
		}

		// Expand queued blocks until the limit.
		err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
			start := time.Now()
			defer func() { mark.LongestRead = max(mark.LongestRead, time.Since(start)) }()
			for n := 0; len(tail) != 0; n++ {
				// Read the next block's list.
				if n != 0 && limitReached(limit, n, start) {
					return nil
				}
				size := blockNodeLen(tail)
				if size == 0 || size > len(tail) {
					return errChildNotBlock
				}
				enc := tail[:size]
				tail = tail[size:]
				if expanded.hasBlock(enc) {
					continue
				}
				value, found, err := tx.Get(ctx, graphKey(kindChildren, prefixBlock+string(enc)))
				if err != nil {
					return err
				}
				if !found {
					continue
				}

				// Expand it, queueing the children it makes live while they
				// fit, and stop once they do not.
				expanded.addBlock(enc)
				for len(value) != 0 {
					size := blockNodeLen(value)
					if size == 0 || size > len(value) {
						return errChildNotBlock
					}
					if child := value[:size]; mark.Live.addBlock(child) {
						added++
						if queued++; queued <= limit.tail {
							tail = append(tail, child...)
						} else {
							full = true
						}
					}
					value = value[size:]
				}
				if full {
					return nil
				}
			}
			return nil
		})
		if err != nil {
			return added, false, err
		}
	}
	return added, !full, nil
}

// settleLimit bounds the unexpanded live blocks settleUnknown checks.
const settleLimit = 4096

// errSettleFull stops findUnknown's scans once it holds settleLimit blocks.
var errSettleFull = errors.New("unknown blocks to settle full")

// unknownBlock is a live block a scan mark left unexpanded, with the node
// whose record made it live: a root, an owner, or a parent block.
type unknownBlock struct {
	enc   []byte
	namer string
}

// settleUnknown checks the child list of each live block the scans left
// unexpanded, up to settleLimit of them, as findUnknown lists them. A write
// can stage a block after the scans passed its key, so its list may exist
// now: settleUnknown expands it, then checks each child it makes live the
// same way. It lists the blocks still without a list in mark, with their
// namers, and returns how many blocks it made live. Reading a list later than
// the scans is safe for the same reason the scans' later transactions are:
// the adopt log covers edges added since.
func (g *RefGraph) settleUnknown(ctx context.Context, mark *BlockMark, expanded *NodeSet, limit markLimit, between func() error) (int, error) {
	// List the unexpanded live blocks.
	pending, err := g.findUnknown(ctx, mark, expanded, limit, between)
	if err != nil {
		return 0, err
	}

	// Check the lists in transactions within limit until none is pending.
	var added int
	for first := true; len(pending) != 0; first = false {
		if !first && between != nil {
			if err := between(); err != nil {
				return 0, err
			}
		}
		var n int
		err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) (err error) {
			pending, n, err = settleTx(ctx, tx, mark, expanded, pending, limit)
			return err
		})
		added += n
		if err != nil {
			return 0, err
		}
	}
	return added, nil
}

// settleTx checks the lists of the pending blocks in order until limit, as
// settleUnknown describes, and returns the blocks still pending and how many
// blocks it made live. It records its duration in mark.LongestRead.
func settleTx(ctx context.Context, tx kvtx.Tx, mark *BlockMark, expanded *NodeSet, pending []unknownBlock, limit markLimit) ([]unknownBlock, int, error) {
	// Time the transaction.
	start := time.Now()
	defer func() { mark.LongestRead = max(mark.LongestRead, time.Since(start)) }()

	// Settle blocks until none is pending or the limit is reached.
	var added int
	for n := 0; len(pending) != 0; n++ {
		// Read the next block's list, sampling the block when it has none.
		if n != 0 && limitReached(limit, n, start) {
			break
		}
		next := pending[0]
		pending = pending[1:]
		block := prefixBlock + string(next.enc)
		value, found, err := tx.Get(ctx, graphKey(kindChildren, block))
		if err != nil {
			return nil, 0, err
		}
		if !found {
			mark.await(namedNode{block, next.namer})
			continue
		}

		// Expand the list, queueing the children it makes live.
		if !expanded.addBlock(next.enc) {
			continue
		}
		for len(value) != 0 {
			size := blockNodeLen(value)
			if size == 0 || size > len(value) {
				return nil, 0, errChildNotBlock
			}
			if child := value[:size]; mark.Live.addBlock(child) {
				added++
				pending = append(pending, unknownBlock{enc: slices.Clone(child), namer: block})
			}
			value = value[size:]
		}
	}
	return pending, added, nil
}

// findUnknown lists up to settleLimit of the live blocks a scan mark left
// unexpanded, each with its namer, and no block the mark did not reach. Only the records that made them live name
// them, so it scans the roots, the edges from text nodes and the expanded
// lists, within limit like the mark, and stops once the list is full.
func (g *RefGraph) findUnknown(ctx context.Context, mark *BlockMark, expanded *NodeSet, limit markLimit, between func() error) ([]unknownBlock, error) {
	// Note each unexpanded live block once. An edge a write added after the
	// seeds or the scans can name a block the mark never made live; the
	// adopt log covers it, and noting it would expand a list liveBlocks does
	// not count, so the mark would count too few unknown blocks.
	var found []unknownBlock
	noted := NewNodeSet()
	note := func(enc []byte, namer string) error {
		if len(enc) == 0 || enc[0] == textNode || !mark.Live.hasBlock(enc) || expanded.hasBlock(enc) || !noted.addBlock(enc) {
			return nil
		}
		found = append(found, unknownBlock{enc: slices.Clone(enc), namer: namer})
		if len(found) == settleLimit {
			return errSettleFull
		}
		return nil
	}

	// Scan the roots, the objects of edges from text nodes, then the
	// children of the expanded lists.
	roots, owners, lists := graphKey(kindRoot), []byte{kindForward, textNode}, graphKey(kindChildren)
	scans := []struct {
		prefix []byte
		visit  func(key []byte, it kvtx.Iterator) error
	}{
		{roots, func(key []byte, _ kvtx.Iterator) error { return note(key[len(roots):], NodeGCRoot) }},
		{owners, func(key []byte, _ kvtx.Iterator) error {
			subject, rest, err := readNode(key[1:])
			if err != nil {
				return err
			}
			if n := blockNodeLen(rest); n != 0 && n <= len(rest) {
				return note(rest[:n], subject)
			}
			return nil
		}},
		{lists, func(key []byte, it kvtx.Iterator) error {
			// Read only the lists the mark expanded.
			enc := key[len(lists):]
			if !expanded.hasBlock(enc) {
				return nil
			}
			value, err := it.Value()
			if err != nil {
				return err
			}
			for len(value) != 0 {
				n := blockNodeLen(value)
				if n == 0 || n > len(value) {
					return errChildNotBlock
				}
				if err := note(value[:n], prefixBlock+string(enc)); err != nil {
					return err
				}
				value = value[n:]
			}
			return nil
		}},
	}
	for _, scan := range scans {
		err := g.scanRecords(ctx, mark, scan.prefix, limit, between, scan.visit)
		if errors.Is(err, errSettleFull) {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return found, nil
}

// scanRecords calls visit with each record under prefix in key order, in read
// transactions within limit, calling between before each transaction after
// the first. It records the longest transaction in mark.LongestRead.
func (g *RefGraph) scanRecords(ctx context.Context, mark *BlockMark, prefix []byte, limit markLimit, between func() error, visit func(key []byte, it kvtx.Iterator) error) error {
	var resume []byte
	for {
		// Let the caller act between transactions.
		if resume != nil && between != nil {
			if err := between(); err != nil {
				return err
			}
		}

		// Visit records after the last one visited until the limit.
		var more bool
		err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
			// Position after the last record visited.
			start := time.Now()
			defer func() { mark.LongestRead = max(mark.LongestRead, time.Since(start)) }()
			it := tx.Iterate(ctx, prefix, true, false)
			defer it.Close()
			valid := it.Next()
			if resume != nil {
				if err := it.Seek(append(slices.Clone(resume), 0)); err != nil {
					return err
				}
				valid = it.Valid()
			}

			// Visit the records, stopping at the limit.
			for n := 0; valid; n++ {
				if n != 0 && limitReached(limit, n, start) {
					more = true
					break
				}
				key := it.Key()
				if err := visit(key, it); err != nil {
					return err
				}
				resume = append(resume[:0], key...)
				valid = it.Next()
			}
			return it.Err()
		})
		if err != nil || !more {
			return err
		}
	}
}

// limitReached reports whether a transaction that began at start and visited
// n records has reached limit. It reads the clock every markBudgetCheck
// records.
func limitReached(limit markLimit, n int, start time.Time) bool {
	if limit.records != 0 && n >= limit.records {
		return true
	}
	return limit.budget != 0 && n%markBudgetCheck == 0 && time.Since(start) >= limit.budget
}

// blockNodeLen returns the length of the block encoding that b starts with,
// or zero when b does not start with one.
func blockNodeLen(b []byte) int {
	if len(b) == 0 || b[0] == textNode {
		return 0
	}
	size := hash.HashType(b[0]).GetHashLen()
	if size <= 0 {
		return 0
	}
	return 1 + size
}

// blockHash returns the hash Has and Add give the block IRI whose encoding
// is enc, without building the IRI.
func (s *NodeSet) blockHash(enc []byte) [2]uint64 {
	h := [2]uint64{maphash.Bytes(s.blockSeeds[0], enc), maphash.Bytes(s.blockSeeds[1], enc)}
	if h == ([2]uint64{}) {
		h[1] = 1
	}
	return h
}

// hasBlock reports whether the block encoded as enc is in the set.
func (s *NodeSet) hasBlock(enc []byte) bool {
	_, found := s.find(s.blockHash(enc))
	return found
}

// addBlock inserts the block encoded as enc and reports whether it was
// absent.
func (s *NodeSet) addBlock(enc []byte) bool {
	return s.addHash(s.blockHash(enc))
}
