package block_gc

import (
	"context"
	"encoding/binary"
	"maps"
	"slices"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// Owner edges and releases. A block's owner edges from nodes other than the
// unreferenced queue are holders of it, so once the shared counts are
// complete each write keeps them by its owner edges too: a graph over one
// transaction, after CountOwnerEdges, nets the owner edges its writes add and
// remove for each block, and SettleOwnerCounts writes the net before the
// transaction commits. A rise adds to the block's shared count at once. A fall
// adds a release record instead, which ResolveReleases resolves later, one
// block at a time: a release takes one from the count, and the release of a
// block with no count left frees it, which releases each block its child
// list names.
//
// A release waits for the resolver, rather than taking from the count at
// once, so the resolver alone decides that a block frees, between deadlist
// records: a count that fell while a record applied would turn the record's
// decision that a page lives into a free of a page the record's new holders
// name.
//
// An edge marked Uncounted changes no count: its writer counts the holder by
// other means, as a put flush does through the Volume's batch counts.

// ownerNet nets the owner edges a graph's writes add and remove per block.
type ownerNet struct {
	// mtx guards net.
	mtx sync.Mutex
	// net is the change to each block's holders.
	net map[string]int64
}

// count adds delta to the net of edge's block, unless the edge is uncounted,
// from the unreferenced queue, or to a node that is not a block. A nil net
// counts nothing.
func (n *ownerNet) count(edge RefEdge, delta int64) {
	if n == nil || edge.Uncounted || edge.Subject == NodeUnreferenced || !isBlockNode(edge.Object) {
		return
	}
	n.mtx.Lock()
	n.net[edge.Object] += delta
	n.mtx.Unlock()
}

// take returns the nonzero nets and starts over.
func (n *ownerNet) take() map[string]int64 {
	// Swap in an empty net, dropping the edges that cancelled.
	n.mtx.Lock()
	defer n.mtx.Unlock()
	net := n.net
	n.net = make(map[string]int64)
	maps.DeleteFunc(net, func(_ string, change int64) bool { return change == 0 })
	return net
}

// CountOwnerEdges makes the graph net the owner edges its writes add and
// remove, for SettleOwnerCounts. Call it once, before any write, and only on
// a graph whose store is one transaction, so the net commits or discards with
// the edges it counts.
func (g *RefGraph) CountOwnerEdges() {
	g.owners = &ownerNet{net: make(map[string]int64)}
}

// SettleOwnerCounts writes the owner edges' net change since the last settle
// to the shared counts: a rise adds to the block's count, and a fall adds as
// many releases. A fall at a block without a child list adds nothing: the
// write untracked the block, or it was never tracked, and a release there
// could only free a later copy of it. While the counts are not complete it
// drops the net, since the sharing walk counts the edges from the graph.
func (g *RefGraph) SettleOwnerCounts(ctx context.Context) error {
	// Take the net, which nothing may count twice.
	if g.owners == nil {
		return nil
	}
	net := g.owners.take()
	if len(net) == 0 {
		return nil
	}

	// Write each change in key order.
	nodes := slices.Sorted(maps.Keys(net))
	return g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		s, err := readSharingTx(ctx, tx)
		if err != nil || !s.Phase.Counting() {
			return err
		}
		for _, node := range nodes {
			change := net[node]
			if change > 0 {
				if err := addCountTx(ctx, tx, kindHold, node, uint64(change)); err != nil { // #nosec G115 -- change is positive.
					return err
				}
				continue
			}
			tracked, err := tx.Exists(ctx, graphKey(kindChildren, node))
			if err != nil {
				return err
			}
			if tracked {
				if err := addCountTx(ctx, tx, kindRelease, node, uint64(-change)); err != nil { // #nosec G115 -- change is negative.
					return err
				}
			}
		}
		return nil
	})
}

// ReleaseResult reports what one ResolveReleases call did.
type ReleaseResult struct {
	// Done reports that no release was left when the call ended.
	Done bool
	// Resolved is the number of releases the call consumed.
	Resolved uint64
	// Freed is the number of blocks the call freed.
	Freed uint64
	// Underflows is the number of releases beyond a block's count and the
	// one that freed it; any means a holder was never counted.
	Underflows uint64
}

// ResolveReleases resolves pending releases in one write transaction, in
// block order, until none is left or yield reports a spent budget or a
// waiting writer, after the first block. Each release of a block takes one
// from its shared count; one with no count left frees the block, calling
// visit with it, and adds a release to each block its child list names. It
// deletes nothing else: the collector's sweep removes a freed block. It
// refuses with ErrSharingWalk while the counts are not complete.
//
// The caller must not run it while a deadlist record applies, between the
// record's read of the counts and its write of their changes.
func (g *RefGraph) ResolveReleases(ctx context.Context, yield func() bool, visit func(node string) error) (ReleaseResult, error) {
	var res ReleaseResult
	err := g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		// Refuse unless the counts are complete.
		res = ReleaseResult{}
		s, err := readSharingTx(ctx, tx)
		if err != nil {
			return err
		}
		if !s.Phase.Counting() {
			return ErrSharingWalk
		}

		// Resolve the first block with releases until none is left. A freed
		// block's children gain releases, which the loop finds in turn.
		for first := true; ; first = false {
			if !first && yield() {
				return nil
			}
			node, releases, found, err := firstReleaseTx(ctx, tx)
			if err != nil {
				return err
			}
			if !found {
				res.Done = true
				return nil
			}
			if err := tx.Delete(ctx, graphKey(kindRelease, node)); err != nil {
				return err
			}
			res.Resolved += releases

			// Take the releases from the count while it lasts.
			n, err := readHoldsTx(ctx, tx, node)
			if err != nil {
				return err
			}
			if n >= releases {
				if err := setCountTx(ctx, tx, kindHold, node, n-releases); err != nil {
					return err
				}
				continue
			}

			// Free the block and release its children.
			res.Freed++
			res.Underflows += releases - n - 1
			if err := tx.Delete(ctx, graphKey(kindHold, node)); err != nil {
				return err
			}
			if err := visit(node); err != nil {
				return err
			}
			children, _, err := readChildrenTx(ctx, tx, node)
			if err != nil {
				return err
			}
			for _, child := range children {
				if err := addCountTx(ctx, tx, kindRelease, child, 1); err != nil {
					return err
				}
			}
		}
	})
	return res, err
}

// GetReleases returns the pending releases of block, or 0 when it has none.
func (g *RefGraph) GetReleases(ctx context.Context, block string) (uint64, error) {
	var n uint64
	err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		var err error
		n, err = readCountTx(ctx, tx, kindRelease, block)
		return err
	})
	return n, err
}

// firstReleaseTx returns the first block with pending releases and their
// number.
func firstReleaseTx(ctx context.Context, tx kvtx.Tx) (string, uint64, bool, error) {
	// Read the first release record.
	prefix := []byte{kindRelease}
	it := tx.Iterate(ctx, prefix, false, false)
	defer it.Close()
	if !it.Next() {
		return "", 0, false, it.Err()
	}
	node := lastNode(prefix, it.Key())
	value, err := it.Value()
	if err != nil {
		return "", 0, false, err
	}

	// Decode the whole value as one uvarint.
	n, size := binary.Uvarint(value)
	if size <= 0 || size != len(value) || n == 0 {
		return "", 0, false, errors.Errorf("releases of %s are malformed", node)
	}
	return node, n, true, nil
}

// addCountTx adds delta to the count of kind at block.
func addCountTx(ctx context.Context, tx kvtx.Tx, kind byte, block string, delta uint64) error {
	n, err := readCountTx(ctx, tx, kind, block)
	if err != nil {
		return err
	}
	return setCountTx(ctx, tx, kind, block, n+delta)
}

// setCountTx sets the count of kind at block, deleting a count of zero.
func setCountTx(ctx context.Context, tx kvtx.Tx, kind byte, block string, n uint64) error {
	key := graphKey(kind, block)
	if n == 0 {
		return tx.Delete(ctx, key)
	}
	return tx.Set(ctx, key, binary.AppendUvarint(nil, n))
}

// readCountTx reads the count of kind at block, or 0 when it has none.
func readCountTx(ctx context.Context, tx kvtx.TxOps, kind byte, block string) (uint64, error) {
	// Read the record, which is absent at zero.
	value, found, err := tx.Get(ctx, graphKey(kind, block))
	if err != nil || !found {
		return 0, err
	}

	// Decode the whole value as one uvarint.
	n, size := binary.Uvarint(value)
	if size <= 0 || size != len(value) {
		return 0, errors.Errorf("count %c of %s is malformed", kind, block)
	}
	return n, nil
}
