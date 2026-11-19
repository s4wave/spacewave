package world_block_tx

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block/quad"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
)

// NewTxSetGraphQuads constructs a SET_GRAPH_QUADS transaction of quads.
func NewTxSetGraphQuads(quads []*quad.Quad) *Tx {
	return &Tx{
		TxType:          TxType_TxType_SET_GRAPH_QUADS,
		TxSetGraphQuads: &TxSetGraphQuads{Quads: quads},
	}
}

// IsNil checks if the object is nil.
func (t *TxSetGraphQuads) IsNil() bool {
	return t == nil
}

// GetTxType returns the type of transaction this is.
func (t *TxSetGraphQuads) GetTxType() TxType {
	return TxType_TxType_SET_GRAPH_QUADS
}

// GetEmpty checks if the tx is empty.
func (t *TxSetGraphQuads) GetEmpty() bool {
	return len(t.GetQuads()) == 0
}

// Validate performs a cursory check of the transaction. An empty list or an
// empty quad is a *world.OpRefusedError.
func (t *TxSetGraphQuads) Validate() error {
	if len(t.GetQuads()) == 0 {
		return world.NewOpRefusedError(errors.New("cannot set an empty graph quad list"))
	}
	for i, q := range t.GetQuads() {
		if q.IsEmpty() {
			return world.NewOpRefusedError(errors.Errorf("quads[%d]: cannot set empty graph quad", i))
		}
	}
	return nil
}

// ExecuteTx sets the quads in one graph update. A World that cannot set a
// list in one update fails it as a system error rather than set the quads one
// at a time, which would reach another World than every other replica.
func (t *TxSetGraphQuads) ExecuteTx(
	ctx context.Context,
	sender peer.ID,
	lookupWorldOp world.LookupOp,
	worldInstance world.WorldState,
) (sysErr bool, rerr error) {
	// Refuse an empty list or quad.
	if err := t.Validate(); err != nil {
		return false, err
	}

	// Set the list in one update.
	setter, ok := worldInstance.(world.GraphQuadsSetter)
	if !ok {
		return true, errors.New("world cannot set a graph quad list in one update")
	}
	quads := make([]world.GraphQuad, len(t.GetQuads()))
	for i, q := range t.GetQuads() {
		quads[i] = world.QuadToGraphQuad(q)
	}
	return false, setter.SetGraphQuads(ctx, quads)
}

// _ is a type assertion
var _ Transaction = (*TxSetGraphQuads)(nil)
