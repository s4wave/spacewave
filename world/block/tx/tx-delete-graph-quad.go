package world_block_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block/quad"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
)

// NewTxDeleteGraphQuad constructs a new DELETE_GRAPH_QUAD transaction.
func NewTxDeleteGraphQuad(quad *quad.Quad) (*Tx, error) {
	return &Tx{
		TxType: TxType_TxType_DELETE_GRAPH_QUAD,
		TxDeleteGraphQuad: &TxDeleteGraphQuad{
			Quad: quad,
		},
	}, nil
}

// NewTxDeleteGraphQuadTxn constructs a new CREATE_OBJECT transaction.
func NewTxDeleteGraphQuadTxn() Transaction {
	return &TxDeleteGraphQuad{}
}

// IsNil checks if the object is nil.
func (t *TxDeleteGraphQuad) IsNil() bool {
	return t == nil
}

// GetTxType returns the type of transaction this is.
func (t *TxDeleteGraphQuad) GetTxType() TxType {
	return TxType_TxType_DELETE_GRAPH_QUAD
}

// GetEmpty checks if the tx is empty.
func (t *TxDeleteGraphQuad) GetEmpty() bool {
	return t.GetQuad().IsEmpty()
}

// Clone clones the tx object.
func (t *TxDeleteGraphQuad) Clone() *TxDeleteGraphQuad {
	if t == nil {
		return nil
	}
	return &TxDeleteGraphQuad{
		Quad: t.GetQuad().Clone(),
	}
}

// Validate performs a cursory check of the transaction.
// Note: this should not fetch network data.
func (t *TxDeleteGraphQuad) Validate() error {
	if t.GetQuad().IsEmpty() {
		return errors.New("cannot delete empty graph quad")
	}
	return nil
}

// ExecuteTx executes the transaction against a world instance.
func (t *TxDeleteGraphQuad) ExecuteTx(
	ctx context.Context,
	sender peer.ID,
	lookupWorldOp world.LookupOp,
	worldInstance world.WorldState,
) (sysErr bool, rerr error) {
	if err := t.Validate(); err != nil {
		return false, err
	}

	gq := world.QuadToGraphQuad(t.GetQuad())
	err := worldInstance.DeleteGraphQuad(gq)
	return false, err
}

// _ is a type assertion
var _ Transaction = ((*TxDeleteGraphQuad)(nil))
