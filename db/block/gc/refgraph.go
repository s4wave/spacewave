package block_gc

import (
	"context"
	"encoding/binary"
	"maps"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// maxBatchEdges bounds the input edges committed in one transaction.
const maxBatchEdges = 128

// Record kinds. Each kind byte begins a key followed by encoded nodes, each of
// which delimits itself, so no node can be read as a prefix of another.
// Edge, node and root records hold no value: a scan reads the last node from
// each key.
const (
	// kindAdopt keys a record of the adopt log by a big-endian sequence. Its
	// value packs the encodings of the blocks one write adopted while a block
	// pass ran.
	kindAdopt byte = 'a'
	// kindChildren keys the child list of a block by the block. Its value
	// packs the encodings of the blocks it references, sorted and distinct.
	kindChildren byte = 'c'
	// kindForward keys an edge by subject then object.
	kindForward byte = 'f'
	// kindYoungHead is the one record of the young log's range, as
	// MarkYoung describes.
	kindYoungHead byte = 'g'
	// kindIncoming keys an edge by object then subject.
	kindIncoming byte = 'i'
	// kindNode keyed the node inventory of the mark-sweep collector, which
	// no reader uses; InventoryRecord recognizes its records.
	kindNode byte = 'n'
	// kindPass is the one record of a running block pass.
	kindPass byte = 'p'
	// kindRoot keys the registered roots.
	kindRoot byte = 'r'
	// kindYoungReady is the one record that reports the young log complete,
	// as YoungReady describes.
	kindYoungReady byte = 'm'
	// kindYoung keys a record of the young log by a big-endian sequence.
	// Its value lists the blocks whose child lists one write created or
	// changed.
	kindYoung byte = 'y'
)

// RefGraph stores the garbage collection ownership graph as plain records in
// a transactional key-value store. Opening a RefGraph reads nothing.
//
// An edge from a node that is not a block is one record per direction keyed by
// the full node IRIs, so membership is a point read, traversal is a prefix scan
// and a removed edge leaves nothing behind.
//
// The edges between blocks are one child list record per parent block, written
// once when the block is stored, with no record under each child. A block's
// child list also records that its references are known, so a leaf has an
// empty one. Blocks are collected by marking from the roots, so a block never
// joins the unreferenced queue: the queue's staging edge to a block creates
// the block's child list, exists while the list does, and is never removed on
// its own. Incoming queries of a block report its owners that are not blocks,
// and the unreferenced queue while the block is tracked. While a block pass
// runs, each write also logs the blocks it adopts, as MarkBlocks describes.
// Each write also logs the blocks whose child lists it creates or changes in
// the young log, so a minor pass can sweep the blocks written since the last
// pass without reading the rest, as MarkYoung describes.
type RefGraph struct {
	// store holds the graph records.
	store kvtx.Store
}

// NewRefGraph constructs a reference graph over store.
func NewRefGraph(store kvtx.Store) *RefGraph {
	return &RefGraph{store: store}
}

// InventoryRecord reports whether record, a graph key without its prefix,
// belongs to the node inventory the mark-sweep collector kept.
func InventoryRecord(record []byte) bool {
	return len(record) != 0 && record[0] == kindNode
}

// textNode begins the encoding of a node that is not a block.
const textNode byte = 0

// graphKey encodes a record kind followed by its nodes.
func graphKey(kind byte, nodes ...string) []byte {
	// Size the key buffer for the kind byte and the longest node encodings.
	size := 1
	for _, node := range nodes {
		size += 1 + binary.MaxVarintLen64 + len(node)
	}

	// Append the kind byte and each node.
	key := make([]byte, 1, size)
	key[0] = kind
	for _, node := range nodes {
		key = appendNode(key, node)
	}
	return key
}

// appendNode appends the encoding of node to key. A block node is its
// BlockIRI after the prefix: the hash type byte, never zero, and the digest,
// whose length the type fixes. Any other node is textNode, a uvarint length
// and the IRI.
func appendNode(key []byte, node string) []byte {
	if _, _, ok := parseBlockIRI(node); ok {
		return append(key, node[len(prefixBlock):]...)
	}
	key = append(key, textNode)
	key = binary.AppendUvarint(key, uint64(len(node)))
	return append(key, node...)
}

// lastNode returns the node that follows prefix in a record key, where
// prefix is a graphKey with one node fewer than key.
func lastNode(prefix, key []byte) string {
	enc := key[len(prefix):]
	if enc[0] == textNode {
		_, n := binary.Uvarint(enc[1:])
		return string(enc[1+n:])
	}
	return prefixBlock + string(enc)
}

// transaction retries complete graph operations against a fresh consistent view.
func (g *RefGraph) transaction(ctx context.Context, write bool, body func(context.Context, kvtx.Tx) error) error {
	return kvtx.RunTransaction(ctx, write, func(ctx context.Context) (kvtx.Tx, error) {
		return g.store.NewTransaction(ctx, write)
	}, body)
}

// RegisterEntityChain registers a chain of gc/ref edges between nodes.
// Each adjacent pair gets an AddRef call: nodes[0]->nodes[1],
// nodes[1]->nodes[2], etc. At least 2 nodes required. Idempotent.
func RegisterEntityChain(ctx context.Context, rg RefGraphOps, nodes ...string) error {
	// Require enough nodes to define a chain.
	if len(nodes) < 2 {
		return errors.New("RegisterEntityChain requires at least 2 nodes")
	}

	// Register every adjacent pair in chain order.
	for i := 0; i < len(nodes)-1; i++ {
		if err := rg.AddRef(ctx, nodes[i], nodes[i+1]); err != nil {
			return err
		}
	}
	return nil
}

// AddRef adds one edge as a batch of one addition. Idempotent.
func (g *RefGraph) AddRef(ctx context.Context, subject, object string) error {
	return g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		return g.applyRefBatchTx(ctx, tx, []RefEdge{{Subject: subject, Object: object}}, nil)
	})
}

// RemoveRef removes an exact edge without creating an orphan mark.
func (g *RefGraph) RemoveRef(ctx context.Context, subject, object string) error {
	return g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		_, err := g.removeRefTx(ctx, tx, subject, object)
		return err
	})
}

// batchRemainderError reports the suffix left after an earlier slice committed.
type batchRemainderError struct {
	// err is the failure that stopped the ownership transition.
	err error
	// adds is the uncommitted suffix of the original additions.
	adds []RefEdge
	// removes is the uncommitted suffix of the original removals.
	removes []RefEdge
}

// Error describes the failed partially committed ownership transition.
func (e *batchRemainderError) Error() string {
	return "apply ref batch after partial commit: " + e.err.Error()
}

// Unwrap returns the storage failure that interrupted the transition.
func (e *batchRemainderError) Unwrap() error {
	return e.err
}

// RefBatchRemainder returns copies of the uncommitted input suffixes.
func (e *batchRemainderError) RefBatchRemainder() ([]RefEdge, []RefEdge) {
	return slices.Clone(e.adds), slices.Clone(e.removes)
}

// ApplyRefBatch commits a bounded ownership transition atomically. Larger
// transitions commit slices in add-before-remove order and report an exact
// uncommitted suffix if a later slice fails.
func (g *RefGraph) ApplyRefBatch(ctx context.Context, adds, removes []RefEdge) error {
	addIndex, removeIndex := 0, 0
	committed := false
	for addIndex < len(adds) || removeIndex < len(removes) {
		// Take the next slice, finishing the additions before any removal.
		startAdds, startRemoves := addIndex, removeIndex
		remaining := maxBatchEdges
		addIndex = min(addIndex+remaining, len(adds))
		remaining -= addIndex - startAdds
		if addIndex == len(adds) {
			removeIndex = min(removeIndex+remaining, len(removes))
		}

		// Commit the slice, reporting the uncommitted suffix after the first.
		err := g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
			return g.applyRefBatchTx(ctx, tx, adds[startAdds:addIndex], removes[startRemoves:removeIndex])
		})
		if err != nil {
			if !committed {
				return err
			}
			return &batchRemainderError{
				err:     err,
				adds:    slices.Clone(adds[startAdds:]),
				removes: slices.Clone(removes[startRemoves:]),
			}
		}
		committed = true
	}
	return nil
}

// applyRefBatchTx applies one transaction slice in ownership order, marking
// each object that is not a block and loses its last owner as unreferenced,
// logs the blocks it adds edges to while a block pass runs, and logs the
// blocks whose child lists it creates or changes in the young log.
func (g *RefGraph) applyRefBatchTx(ctx context.Context, tx kvtx.Tx, adds, removes []RefEdge) error {
	// Prefetch every record the checks below read. A store that reads many
	// keys together then answers each check from its cache instead of
	// reading it in turn while the transaction holds the writer.
	if err := prefetchRefBatchTx(ctx, tx, adds, removes); err != nil {
		return err
	}

	// Add every edge first, so an edge in both lists ends removed. The child
	// edges of each block merge into its list in one write.
	children := make(map[string][]string)
	var adopted []string
	for _, edge := range adds {
		if isBlockNode(edge.Object) {
			adopted = append(adopted, edge.Object)
		}
		if parent, child, ok := childEdge(edge); ok {
			list := children[parent]
			if child != "" {
				list = append(list, child)
			}
			children[parent] = list
			continue
		}
		if err := addRefTx(ctx, tx, edge.Subject, edge.Object); err != nil {
			return err
		}
	}
	var young youngRecord
	for _, parent := range slices.Sorted(maps.Keys(children)) {
		created, changed, err := addChildrenTx(ctx, tx, parent, children[parent])
		if err != nil {
			return err
		}
		switch {
		case created:
			young.created = append(young.created, parent)
		case changed:
			young.changed = append(young.changed, parent)
		}
	}
	if err := logAdoptedTx(ctx, tx, adopted); err != nil {
		return err
	}
	if err := logYoungTx(ctx, tx, young); err != nil {
		return err
	}

	// Remove every edge, noting the objects that lost an owner and the
	// objects whose staging edge the caller removed.
	var released []string
	unstaged := make(map[string]struct{})
	for _, edge := range removes {
		existed, err := g.removeRefTx(ctx, tx, edge.Subject, edge.Object)
		switch {
		case err != nil:
			return err
		case !existed || IsPermanentRoot(edge.Object) || isBlockNode(edge.Object):
		case edge.Subject == NodeUnreferenced:
			unstaged[edge.Object] = struct{}{}
		default:
			released = append(released, edge.Object)
		}
	}

	// Mark each released object left without an owner, unless the caller
	// removed its staging edge.
	for _, object := range released {
		if _, ok := unstaged[object]; ok {
			continue
		}
		owned, err := hasIncomingTx(ctx, tx, object, nil)
		if err != nil {
			return err
		}
		if !owned {
			if err := addRefTx(ctx, tx, NodeUnreferenced, object); err != nil {
				return err
			}
		}
	}
	return nil
}

// prefetchRefBatchTx prefetches the record that holds each edge and the first
// incoming record position of each removed object that is not a block.
func prefetchRefBatchTx(ctx context.Context, tx kvtx.Tx, adds, removes []RefEdge) error {
	// List the records the batch checks.
	keys := make([][]byte, 0, len(adds)+2*len(removes))
	for _, edge := range adds {
		keys = append(keys, edgeKey(edge))
	}
	for _, edge := range removes {
		keys = append(keys, edgeKey(edge))
		if !isBlockNode(edge.Object) {
			keys = append(keys, graphKey(kindIncoming, edge.Object))
		}
	}

	// Prefetch them together.
	return kvtx.Prefetch(ctx, tx, keys)
}

// edgeKey returns the key of the record that holds edge: the child list of its
// block, or its forward record.
func edgeKey(edge RefEdge) []byte {
	if parent, _, ok := childEdge(edge); ok {
		return graphKey(kindChildren, parent)
	}
	return graphKey(kindForward, edge.Subject, edge.Object)
}

// addRefTx adds both directions of an edge that is not a child edge unless
// the exact forward edge already exists.
func addRefTx(ctx context.Context, tx kvtx.Tx, subject, object string) error {
	// Skip the edge when its exact forward record already exists.
	forward := graphKey(kindForward, subject, object)
	exists, err := tx.Exists(ctx, forward)
	if err != nil || exists {
		return err
	}

	// Write the two edge directions.
	if err := tx.Set(ctx, forward, nil); err != nil {
		return err
	}
	return tx.Set(ctx, graphKey(kindIncoming, object, subject), nil)
}

// removeRefTx removes an exact edge and reports whether it existed. A child
// edge leaves its block's list, and a staging edge stays with the list.
func (g *RefGraph) removeRefTx(ctx context.Context, tx kvtx.Tx, subject, object string) (bool, error) {
	// Remove a child from its block's list.
	if parent, child, ok := childEdge(RefEdge{Subject: subject, Object: object}); ok {
		if child == "" {
			return false, nil
		}
		return removeChildTx(ctx, tx, parent, child)
	}

	// Check the forward record, then delete both directions.
	forward := graphKey(kindForward, subject, object)
	exists, err := tx.Exists(ctx, forward)
	if err != nil || !exists {
		return exists, err
	}
	if err := tx.Delete(ctx, forward); err != nil {
		return false, err
	}
	if err := tx.Delete(ctx, graphKey(kindIncoming, object, subject)); err != nil {
		return false, err
	}
	return true, nil
}

// hasIncomingTx reports whether object has an owner other than the
// unreferenced root and the ignored subjects, stopping at the first one.
func hasIncomingTx(ctx context.Context, tx kvtx.Tx, object string, ignored map[string]struct{}) (bool, error) {
	// Walk the object's incoming records; each key ends in its subject.
	prefix := graphKey(kindIncoming, object)
	it := tx.Iterate(ctx, prefix, true, false)
	defer it.Close()
	for it.Next() {
		subject := lastNode(prefix, it.Key())
		if subject == NodeUnreferenced {
			continue
		}
		if _, skip := ignored[subject]; !skip {
			return true, nil
		}
	}
	return false, it.Err()
}

// RemoveNodeRefs removes the observed outgoing targets in bounded transactions.
// New unrelated edges may be added after this operation's initial snapshot. A
// block's child list goes first, so the block is no longer tracked; its
// children never join the unreferenced queue.
func (g *RefGraph) RemoveNodeRefs(ctx context.Context, node string, markOrphaned bool) ([]string, error) {
	// Drop a block's child list.
	var children []string
	if isBlockNode(node) {
		err := g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
			// Read the list, then delete it.
			var found bool
			var err error
			children, found, err = readChildrenTx(ctx, tx, node)
			if err != nil || !found {
				return err
			}
			return tx.Delete(ctx, graphKey(kindChildren, node))
		})
		if err != nil {
			return nil, err
		}
	}

	// Remove the observed targets of the other edges in chunks.
	targets, err := g.list(ctx, graphKey(kindForward, node))
	if err != nil {
		return nil, err
	}
	for offset := 0; offset < len(targets); offset += maxBatchEdges {
		chunk := targets[offset:min(offset+maxBatchEdges, len(targets))]
		err := g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
			if markOrphaned {
				removes := make([]RefEdge, 0, len(chunk))
				for _, target := range chunk {
					removes = append(removes, RefEdge{Subject: node, Object: target})
				}
				return g.applyRefBatchTx(ctx, tx, nil, removes)
			}
			for _, target := range chunk {
				if _, err := g.removeRefTx(ctx, tx, node, target); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return append(children, targets...), nil
}

// HasIncomingRefs ignores the bookkeeping edge from the unreferenced root.
func (g *RefGraph) HasIncomingRefs(ctx context.Context, node string) (bool, error) {
	return g.HasIncomingRefsExcluding(ctx, node)
}

// HasIncomingRefsExcluding stops at the first owner outside the unreferenced
// root and the caller's exclusions.
func (g *RefGraph) HasIncomingRefsExcluding(ctx context.Context, node string, excluded ...string) (bool, error) {
	// Index the exclusions, then scan the node's owners.
	ignored := make(map[string]struct{}, len(excluded))
	for _, source := range excluded {
		ignored[source] = struct{}{}
	}
	var found bool
	err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		var err error
		found, err = hasIncomingTx(ctx, tx, node, ignored)
		return err
	})
	return found, err
}

// list returns the last node of every record under prefix.
func (g *RefGraph) list(ctx context.Context, prefix []byte) ([]string, error) {
	var answer []string
	err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		answer = nil
		return tx.ScanPrefixKeys(ctx, prefix, func(key []byte) error {
			answer = append(answer, lastNode(prefix, key))
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return answer, nil
}

// GetOutgoingRefs returns all current targets of a node: a block's children
// and the targets of its other edges.
func (g *RefGraph) GetOutgoingRefs(ctx context.Context, node string) ([]string, error) {
	var answer []string
	err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		// Read a block's child list.
		answer = nil
		if isBlockNode(node) {
			children, _, err := readChildrenTx(ctx, tx, node)
			if err != nil {
				return err
			}
			answer = children
		}

		// Add the targets of the forward records.
		prefix := graphKey(kindForward, node)
		return tx.ScanPrefixKeys(ctx, prefix, func(key []byte) error {
			answer = append(answer, lastNode(prefix, key))
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return answer, nil
}

// GetIncomingRefs returns all current owners of a node. A block reports its
// owners that are not blocks, and the unreferenced queue while it is tracked.
func (g *RefGraph) GetIncomingRefs(ctx context.Context, node string) ([]string, error) {
	var answer []string
	err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		// Report the staging edge of a tracked block.
		answer = nil
		if isBlockNode(node) {
			tracked, err := tx.Exists(ctx, graphKey(kindChildren, node))
			if err != nil {
				return err
			}
			if tracked {
				answer = append(answer, NodeUnreferenced)
			}
		}

		// Add the sources of the incoming records.
		prefix := graphKey(kindIncoming, node)
		return tx.ScanPrefixKeys(ctx, prefix, func(key []byte) error {
			answer = append(answer, lastNode(prefix, key))
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return answer, nil
}

// TrackedBlocks reports, for each ref, whether the graph tracks the block, in
// one read. A block is tracked from its first write until a block pass sweeps
// it, which removes its child list and its value together, so a tracked block
// is one no pass has collected, whether or not its value is stored locally.
func (g *RefGraph) TrackedBlocks(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	tracked := make([]bool, len(refs))
	err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		for i, ref := range refs {
			node := BlockIRI(ref)
			if node == "" {
				continue
			}
			ok, err := tx.Exists(ctx, graphKey(kindChildren, node))
			if err != nil {
				return err
			}
			tracked[i] = ok
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return tracked, nil
}

// GetUnreferencedNodes returns the nodes marked for collector reconsideration,
// none of which is a block.
func (g *RefGraph) GetUnreferencedNodes(ctx context.Context) ([]string, error) {
	return g.GetOutgoingRefs(ctx, NodeUnreferenced)
}

// AddBlockRef adds gc/ref from source block to target block.
func (g *RefGraph) AddBlockRef(ctx context.Context, source, target *block.BlockRef) error {
	from, to := BlockIRI(source), BlockIRI(target)
	if from == "" || to == "" {
		return nil
	}
	return g.AddRef(ctx, from, to)
}

// AddObjectRoot links an object identity to its current content root.
func (g *RefGraph) AddObjectRoot(ctx context.Context, objectKey string, ref *block.BlockRef) error {
	target := BlockIRI(ref)
	if target == "" {
		return nil
	}
	return g.AddRef(ctx, ObjectIRI(objectKey), target)
}

// RemoveObjectRoot removes an exact object-to-block ownership edge.
func (g *RefGraph) RemoveObjectRoot(ctx context.Context, objectKey string, ref *block.BlockRef) error {
	target := BlockIRI(ref)
	if target == "" {
		return nil
	}
	return g.RemoveRef(ctx, ObjectIRI(objectKey), target)
}

// AddRoot registers node as a root.
func (g *RefGraph) AddRoot(ctx context.Context, node string) error {
	return g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		key := graphKey(kindRoot, node)
		found, err := tx.Exists(ctx, key)
		if err != nil || found {
			return err
		}
		return tx.Set(ctx, key, nil)
	})
}

// RemoveRoot removes root membership without changing the node's edges.
func (g *RefGraph) RemoveRoot(ctx context.Context, node string) error {
	return g.deleteRecord(ctx, graphKey(kindRoot, node))
}

// deleteRecord treats a missing record as a no-op.
func (g *RefGraph) deleteRecord(ctx context.Context, key []byte) error {
	return g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		found, err := tx.Exists(ctx, key)
		if err != nil || !found {
			return err
		}
		return tx.Delete(ctx, key)
	})
}

// GetRootNodes returns the registered roots.
func (g *RefGraph) GetRootNodes(ctx context.Context) ([]string, error) {
	return g.list(ctx, graphKey(kindRoot))
}

// Close leaves the store's lifetime with its owner.
func (g *RefGraph) Close() error { return nil }

// _ is a type assertion
var (
	_ RefGraphOps            = (*RefGraph)(nil)
	_ RefBatchRemainderError = (*batchRemainderError)(nil)
)
