package block_gc

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/hash"
)

// reachChunk bounds the nodes whose forward records one prefetch loads.
const reachChunk = 4096

// Reachable marks every node reachable by forward edges and child lists from
// the graph root and the registered roots, the roots included, in the mark's
// Live: the nodes the mark phase of a complete collection keeps. The
// unreferenced queue marks candidates and owns nothing, so neither it nor its
// edges are included. A reached block without a child list counts in the
// mark's Unknown, as in MarkBlocks: the blocks it names are not reached, so a
// caller that keeps only Live must refuse a mark with any. The walk reads one
// snapshot, popping chunks of nodes from a stack and loading the index pages
// of each chunk's records together in key order, so a cold store costs a
// batch of reads per chunk instead of one read per node, and the work list
// stays small however wide the graph is.
func (g *RefGraph) Reachable(ctx context.Context) (*BlockMark, error) {
	reached := NewNodeSet()
	mark := &BlockMark{Live: reached}
	err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		// Seed the stack with the graph root and the registered roots.
		reached.Add(NodeGCRoot)
		stack := []namedNode{{node: NodeGCRoot}}
		prefix := graphKey(kindRoot)
		err := tx.ScanPrefixKeys(ctx, prefix, func(key []byte) error {
			if node := lastNode(prefix, key); node != NodeUnreferenced && reached.Add(node) {
				stack = append(stack, namedNode{node, NodeGCRoot})
			}
			return nil
		})
		if err != nil {
			return err
		}

		// Walk from them.
		_, err = walkTx(ctx, tx, reached, stack, nil, mark.noteUnknown, nil)
		return err
	})
	if err != nil {
		return nil, err
	}
	return mark, nil
}

// RecordSubject returns the node whose liveness decides whether a complete
// collection keeps the graph record key: the subject of an edge in either
// direction, the block of a child list, or the node of a node or root record.
// The records of a block pass and of the young log name no node, so it
// returns an empty subject for them, which no collection keeps: a collection
// that keeps only reachable records leaves every block it keeps old.
func RecordSubject(key []byte) (string, error) {
	// Split the kind byte from the nodes.
	if len(key) == 0 {
		return "", fmt.Errorf("decode empty reference graph record")
	}
	kind, rest := key[0], key[1:]
	if kind == kindPass || kind == kindAdopt || kind == kindYoung || kind == kindYoungHead {
		return "", nil
	}

	// An incoming record names the object first.
	if kind == kindIncoming {
		var err error
		if _, rest, err = readNode(rest); err != nil {
			return "", fmt.Errorf("decode reference graph record %q: %w", key, err)
		}
	}
	node, _, err := readNode(rest)
	if err != nil {
		return "", fmt.Errorf("decode reference graph record %q: %w", key, err)
	}
	return node, nil
}

// readNode decodes the node at the start of enc, as appendNode wrote it, and
// returns the bytes after it.
func readNode(enc []byte) (string, []byte, error) {
	// Require the first byte of a node.
	if len(enc) == 0 {
		return "", nil, fmt.Errorf("missing node")
	}

	// A text node is a uvarint length and the IRI.
	if enc[0] == textNode {
		size, n := binary.Uvarint(enc[1:])
		if n <= 0 || uint64(len(enc)-1-n) < size { //nolint:gosec // the length is checked nonnegative first.
			return "", nil, fmt.Errorf("truncated text node")
		}
		end := 1 + n + int(size) //nolint:gosec // size is bounded by len(enc) above.
		return string(enc[1+n : end]), enc[end:], nil
	}

	// A block node is its hash type and a digest of that type's length.
	t := hash.HashType(enc[0])
	size := t.GetHashLen()
	if size <= 0 || len(enc)-1 < size {
		return "", nil, fmt.Errorf("truncated block node of hash type %d", enc[0])
	}
	end := 1 + size
	return prefixBlock + string(enc[:end]), enc[end:], nil
}
