package block_gc

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// isBlockNode reports whether node is a block IRI.
func isBlockNode(node string) bool {
	_, _, ok := parseBlockIRI(node)
	return ok
}

// childEdge reports whether edge belongs to a child list: an edge between two
// blocks, or a staging edge from the unreferenced queue to a block. It returns
// the block whose list holds the edge and the child it names, empty for a
// staging edge.
func childEdge(edge RefEdge) (parent, child string, ok bool) {
	if !isBlockNode(edge.Object) {
		return "", "", false
	}
	switch {
	case edge.Subject == NodeUnreferenced:
		return edge.Object, "", true
	case isBlockNode(edge.Subject):
		return edge.Subject, edge.Object, true
	}
	return "", "", false
}

// readChildrenTx returns the children in the child list of block, sorted, and
// whether the list exists.
func readChildrenTx(ctx context.Context, tx kvtx.TxOps, block string) ([]string, bool, error) {
	value, found, err := tx.Get(ctx, graphKey(kindChildren, block))
	if err != nil || !found {
		return nil, found, err
	}
	children, err := decodeChildren(value)
	return children, true, err
}

// decodeChildren decodes the packed value of a child list record.
func decodeChildren(value []byte) ([]string, error) {
	var children []string
	for len(value) != 0 {
		child, rest, err := readNode(value)
		if err != nil {
			return nil, err
		}
		if !isBlockNode(child) {
			return nil, errChildNotBlock
		}
		children = append(children, child)
		value = rest
	}
	return children, nil
}

// encodeChildren packs the encodings of the sorted, distinct children.
func encodeChildren(children []string) []byte {
	value := make([]byte, 0, len(children)*(1+32))
	for _, child := range children {
		value = append(value, child[len(prefixBlock):]...)
	}
	return value
}

// addChildrenTx adds children to the child list of block, creating the list
// when it is missing, and returns the children the list did not hold and
// whether it created the list. The list is written only when it changes.
func addChildrenTx(ctx context.Context, tx kvtx.TxOps, block string, children []string) (added []string, created bool, err error) {
	// Find the children the stored list does not hold.
	stored, found, err := readChildrenTx(ctx, tx, block)
	if err != nil {
		return nil, false, err
	}
	for _, child := range children {
		if _, held := slices.BinarySearch(stored, child); !held {
			added = append(added, child)
		}
	}
	slices.Sort(added)
	added = slices.Compact(added)
	if found && len(added) == 0 {
		return nil, false, nil
	}

	// Write the merged list.
	merged := append(slices.Clone(stored), added...)
	slices.Sort(merged)
	return added, !found, tx.Set(ctx, graphKey(kindChildren, block), encodeChildren(merged))
}

// removeChildTx removes child from the child list of block and reports whether
// the list held it. The list stays, empty or not: the block's references
// remain known.
func removeChildTx(ctx context.Context, tx kvtx.TxOps, block, child string) (bool, error) {
	// Find the child in the stored list.
	stored, _, err := readChildrenTx(ctx, tx, block)
	if err != nil {
		return false, err
	}
	i, found := slices.BinarySearch(stored, child)
	if !found {
		return false, nil
	}

	// Write the list without it.
	stored = slices.Delete(stored, i, i+1)
	return true, tx.Set(ctx, graphKey(kindChildren, block), encodeChildren(stored))
}

// errChildNotBlock reports a child list naming a node that is not a block.
var errChildNotBlock = errors.New("child list names a node that is not a block")
