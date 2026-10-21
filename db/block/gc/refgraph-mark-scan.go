package block_gc

import (
	"context"
	"hash/maphash"
	"slices"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/hash"
)

// markScanPassHook, when set, is called after each scan with the pass, the
// lists it expanded and the children it added behind the scan position.
var markScanPassHook func(pass, expanded, behind int)

// MarkBlocksScan marks like MarkBlocks in one snapshot, but reads the child
// lists in key order, as markScan describes.
func (g *RefGraph) MarkBlocksScan(ctx context.Context) (*BlockMark, int, error) {
	return g.markScan(ctx, 0, nil)
}

// markScan marks like MarkBlocks, but reads the child lists in key order:
// each scan expands every list whose block is live and was not yet expanded,
// and the scans repeat until one adds no child behind its position, since it
// expanded every child it added after its position. A scan reads each index
// page of the lists once, so a mark costs scans times pages instead of a page
// a block. It returns the number of scans.
//
// When perTx is nonzero, each read transaction visits at most perTx records
// and the next resumes after the last, calling between first when set. The
// adopt log of an open pass makes that safe: a write that adds an edge to a
// block after the mark read the edge's source logs the block, and the sweep
// rescues it.
func (g *RefGraph) markScan(ctx context.Context, perTx int, between func() error) (mark *BlockMark, passes int, err error) {
	// Mark the seeds.
	mark = &BlockMark{Live: NewNodeSet()}
	live := mark.Live
	var liveBlocks int
	err = g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		return seedMarkTx(ctx, tx, func(node string) {
			if node != NodeUnreferenced && live.Add(node) && isBlockNode(node) {
				liveBlocks++
			}
		})
	})
	if err != nil {
		return nil, 0, err
	}

	// Scan until a scan adds no child behind its position.
	expanded := NewNodeSet()
	lists := graphKey(kindChildren)
	for {
		passes++
		var nexp, behind int
		err := g.scanRecords(ctx, lists, perTx, between, func(key []byte, it kvtx.Iterator) error {
			// Expand a live list once, reading its value only then.
			enc := key[len(lists):]
			if !live.hasBlock(enc) || !expanded.addBlock(enc) {
				return nil
			}
			nexp++
			value, err := it.Value()
			if err != nil {
				return err
			}

			// Add its children, counting those behind the position, which
			// only a later scan can expand.
			for len(value) != 0 {
				n := blockNodeLen(value)
				if n == 0 || n > len(value) {
					return errChildNotBlock
				}
				if live.addBlock(value[:n]) {
					liveBlocks++
					if string(value[:n]) < string(enc) {
						behind++
					}
				}
				value = value[n:]
			}
			return nil
		})
		if err != nil {
			return nil, passes, err
		}
		if markScanPassHook != nil {
			markScanPassHook(passes, nexp, behind)
		}
		if behind == 0 {
			break
		}
	}

	// A live block without a list has unknown references. List the
	// unreached blocks with one more key scan.
	mark.Unknown = liveBlocks - expanded.Len()
	err = g.scanRecords(ctx, lists, perTx, between, func(key []byte, _ kvtx.Iterator) error {
		if block := lastNode(lists, key); !live.Has(block) {
			mark.Unreached = append(mark.Unreached, block)
		}
		return nil
	})
	return mark, passes, err
}

// scanRecords calls visit with each record under prefix in key order, in read
// transactions of at most perTx records each when perTx is nonzero, calling
// between before each transaction after the first.
func (g *RefGraph) scanRecords(ctx context.Context, prefix []byte, perTx int, between func() error, visit func(key []byte, it kvtx.Iterator) error) error {
	var resume []byte
	for {
		// Let the caller act between transactions.
		if resume != nil && between != nil {
			if err := between(); err != nil {
				return err
			}
		}

		// Visit up to perTx records after the last one visited.
		var more bool
		err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
			// Position after the last record visited.
			it := tx.Iterate(ctx, prefix, true, false)
			defer it.Close()
			valid := it.Next()
			if resume != nil {
				if err := it.Seek(append(slices.Clone(resume), 0)); err != nil {
					return err
				}
				valid = it.Valid()
			}

			// Visit the records, stopping after perTx.
			for n := 0; valid; n++ {
				if perTx != 0 && n == perTx {
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
