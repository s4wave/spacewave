package refgraph

import (
	"context"
	"slices"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
)

// maxBatchEdges bounds the input edges committed in one transaction.
const maxBatchEdges = 128

// batchRemainderError reports the suffix left after an earlier slice committed.
type batchRemainderError struct {
	// err is the failure that stopped the ownership transition.
	err error
	// adds is the uncommitted suffix of the original additions.
	adds []block_gc.RefEdge
	// removes is the uncommitted suffix of the original removals.
	removes []block_gc.RefEdge
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
func (e *batchRemainderError) RefBatchRemainder() ([]block_gc.RefEdge, []block_gc.RefEdge) {
	return slices.Clone(e.adds), slices.Clone(e.removes)
}

// ApplyRefBatch commits a bounded ownership transition atomically. Larger
// transitions commit slices in add-before-remove order and report an exact
// uncommitted suffix if a later slice fails.
func (g *Graph) ApplyRefBatch(
	ctx context.Context,
	adds, removes []block_gc.RefEdge,
) error {
	addIndex, removeIndex := 0, 0
	committed := false
	for addIndex < len(adds) || removeIndex < len(removes) {
		startAdds, startRemoves := addIndex, removeIndex
		remaining := maxBatchEdges

		addIndex = min(addIndex+remaining, len(adds))
		remaining -= addIndex - startAdds
		if addIndex == len(adds) {
			removeIndex = min(removeIndex+remaining, len(removes))
		}

		err := kvtx.RunTransaction(
			ctx,
			true,
			func(ctx context.Context) (kvtx.Tx, error) {
				return g.store.NewTransaction(ctx, true)
			},
			func(ctx context.Context, tx kvtx.Tx) error {
				return g.applyRefBatchTx(
					ctx,
					tx,
					adds[startAdds:addIndex],
					removes[startRemoves:removeIndex],
				)
			},
		)
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

// applyRefBatchTx applies one complete transaction slice in ownership order.
func (g *Graph) applyRefBatchTx(
	ctx context.Context,
	tx kvtx.Tx,
	adds, removes []block_gc.RefEdge,
) error {
	for _, edge := range adds {
		if err := g.addRefTx(ctx, tx, edge.Subject, edge.Object); err != nil {
			return err
		}
	}

	for _, edge := range removes {
		existed, err := g.removeRefTx(ctx, tx, edge.Subject, edge.Object)
		if err != nil {
			return err
		}
		if !existed || edge.Subject == block_gc.NodeUnreferenced || block_gc.IsPermanentRoot(edge.Object) {
			continue
		}

		hasOwner, err := g.hasIncomingTx(ctx, tx, edge.Object)
		if err != nil {
			return err
		}
		if hasOwner {
			continue
		}
		if err := g.addRefTx(ctx, tx, block_gc.NodeUnreferenced, edge.Object); err != nil {
			return err
		}
	}
	return nil
}

// addRefTx adds one edge and both endpoint inventory records unless the exact
// forward edge already exists.
func (g *Graph) addRefTx(ctx context.Context, tx kvtx.Tx, subject, object string) error {
	// Skip the edge when its exact forward record already exists.
	forward := graphKey('f', subject, object)
	exists, err := tx.Exists(ctx, forward)
	if err != nil || exists {
		return err
	}

	// Write both endpoint inventory records and the two edge directions.
	if err := tx.Set(ctx, graphKey('n', subject), []byte(subject)); err != nil {
		return err
	}
	if err := tx.Set(ctx, graphKey('n', object), []byte(object)); err != nil {
		return err
	}
	if err := tx.Set(ctx, forward, []byte(object)); err != nil {
		return err
	}
	if err := tx.Set(ctx, graphKey('i', object, subject), []byte(subject)); err != nil {
		return err
	}
	return nil
}

// removeRefTx removes both directions of an exact existing edge.
func (g *Graph) removeRefTx(
	ctx context.Context,
	tx kvtx.Tx,
	subject, object string,
) (bool, error) {
	// Skip edges that do not already exist.
	forward := graphKey('f', subject, object)
	exists, err := tx.Exists(ctx, forward)
	if err != nil || !exists {
		return exists, err
	}
	if err := tx.Delete(ctx, forward); err != nil {
		return false, err
	}
	if err := tx.Delete(ctx, graphKey('i', object, subject)); err != nil {
		return false, err
	}
	return true, nil
}

// hasIncomingTx reports whether object has a non-staging owner, stopping at
// the first matching incoming edge.
func (g *Graph) hasIncomingTx(
	ctx context.Context,
	tx kvtx.Tx,
	object string,
) (bool, error) {
	iter := tx.Iterate(ctx, graphKey('i', object), true, false)
	defer iter.Close()

	for iter.Next() {
		subject, err := iter.Value()
		if err != nil {
			return false, err
		}
		if string(subject) != block_gc.NodeUnreferenced {
			return true, nil
		}
	}
	return false, iter.Err()
}

// _ verifies the collector can preserve the exact uncommitted suffix.
var _ block_gc.RefBatchRemainderError = (*batchRemainderError)(nil)
