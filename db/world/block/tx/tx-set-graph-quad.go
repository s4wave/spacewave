package world_block_tx

import (
	"context"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/db/block/quad"
	"github.com/s4wave/spacewave/db/world"
	"github.com/pkg/errors"
)

// NewTxSetGraphQuad constructs a new SET_GRAPH_QUAD transaction.
func NewTxSetGraphQuad(quad *quad.Quad) (*Tx, error) {
	return &Tx{
		TxType: TxType_TxType_SET_GRAPH_QUAD,
		TxSetGraphQuad: &TxSetGraphQuad{
			Quad: quad,
		},
	}, nil
}

// NewTxSetGraphQuadTxn constructs a new CREATE_OBJECT transaction.
func NewTxSetGraphQuadTxn() Transaction {
	return &TxSetGraphQuad{}
}

// IsNil checks if the object is nil.
func (t *TxSetGraphQuad) IsNil() bool {
	return t == nil
}

// GetTxType returns the type of transaction this is.
func (t *TxSetGraphQuad) GetTxType() TxType {
	return TxType_TxType_SET_GRAPH_QUAD
}

// GetEmpty checks if the tx is empty.
func (t *TxSetGraphQuad) GetEmpty() bool {
	return t.GetQuad().IsEmpty()
}

// Clone clones the tx object.
func (t *TxSetGraphQuad) Clone() *TxSetGraphQuad {
	if t == nil {
		return nil
	}
	return &TxSetGraphQuad{
		Quad: t.GetQuad().Clone(),
	}
}

// Validate performs a cursory check of the transaction.
// Note: this should not fetch network data.
func (t *TxSetGraphQuad) Validate() error {
	if t.GetQuad().IsEmpty() {
		return errors.New("cannot set empty graph quad")
	}
	return nil
}

// ExecuteTx executes the transaction against a world instance.
func (t *TxSetGraphQuad) ExecuteTx(
	ctx context.Context,
	sender peer.ID,
	lookupWorldOp world.LookupOp,
	worldInstance world.WorldState,
) (sysErr bool, rerr error) {
	if err := t.Validate(); err != nil {
		return false, err
	}

	gq := world.QuadToGraphQuad(t.GetQuad())
	err := worldInstance.SetGraphQuad(ctx, gq)
	return false, err
}

// _ is a type assertion
var _ Transaction = ((*TxSetGraphQuad)(nil))
