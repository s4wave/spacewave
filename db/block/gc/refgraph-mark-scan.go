package block_gc

import (
	"context"
	"hash/maphash"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/hash"
)

// markScanPassHook, when set, is called after each scan with the pass, the
// lists it expanded and the children it added behind the scan position.
var markScanPassHook func(pass, expanded, behind int)

// MarkBlocksScan marks like MarkBlocks, but reads the child lists in key
// order: each scan expands every list whose block is live and was not yet
// expanded, and the scans repeat until one expands nothing. A scan reads each
// index page of the lists once, so a mark costs scans times pages instead of
// a page a block. Passes reports the scans.
func (g *RefGraph) MarkBlocksScan(ctx context.Context) (mark *BlockMark, passes int, err error) {
	err = g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		// Mark the seeds.
		mark = &BlockMark{Live: NewNodeSet()}
		var liveBlocks int
		live := mark.Live
		err := seedMarkTx(ctx, tx, func(node string) {
			if node != NodeUnreferenced && live.Add(node) && isBlockNode(node) {
				liveBlocks++
			}
		})
		if err != nil {
			return err
		}

		// Scan until a scan adds no child behind its position: every child
		// it added after its position it also expanded.
		expanded := NewNodeSet()
		lists := graphKey(kindChildren)
		for {
			passes++
			var grew bool
			var nexp, behind int
			err := tx.ScanPrefix(ctx, lists, func(key, value []byte) error {
				// Expand a live list once.
				enc := key[len(lists):]
				if !live.hasBlock(enc) || !expanded.addBlock(enc) {
					return nil
				}
				grew = true
				nexp++

				// Add its children, counting those behind the position,
				// which only a later scan can expand.
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
				return err
			}
			if markScanPassHook != nil {
				markScanPassHook(passes, nexp, behind)
			}
			if !grew || behind == 0 {
				break
			}
		}

		// A live block without a list has unknown references. List the
		// unreached blocks with one more key scan.
		mark.Unknown = liveBlocks - expanded.Len()
		return tx.ScanPrefixKeys(ctx, lists, func(key []byte) error {
			if block := lastNode(lists, key); !live.Has(block) {
				mark.Unreached = append(mark.Unreached, block)
			}
			return nil
		})
	})
	return mark, passes, err
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
	var h [2]uint64
	for i, seed := range s.seeds {
		var mh maphash.Hash
		mh.SetSeed(seed)
		_, _ = mh.WriteString(prefixBlock)
		_, _ = mh.Write(enc)
		h[i] = mh.Sum64()
	}
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
