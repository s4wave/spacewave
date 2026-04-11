package world_block_tx

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
)

// garbageCollectable is implemented by world states that support GC sweep.
type garbageCollectable interface {
	// GarbageCollect sweeps unreferenced nodes from the GC ref graph.
	GarbageCollect(ctx context.Context) error
}

// NewTxGCSweep constructs a new GC_SWEEP transaction.
func NewTxGCSweep() (*Tx, error) {
	return &Tx{
		TxType:    TxType_TxType_GC_SWEEP,
		TxGcSweep: &TxGCSweep{},
	}, nil
}

// IsNil checks if the object is nil.
func (t *TxGCSweep) IsNil() bool {
	return t == nil
}

// GetTxType returns the type of transaction this is.
func (t *TxGCSweep) GetTxType() TxType {
	return TxType_TxType_GC_SWEEP
}

// GetEmpty checks if the tx is empty.
func (t *TxGCSweep) GetEmpty() bool {
	return false
}

// Clone clones the tx object.
func (t *TxGCSweep) Clone() *TxGCSweep {
	if t == nil {
		return nil
	}
	return &TxGCSweep{}
}

// Validate performs a cursory check of the transaction.
func (t *TxGCSweep) Validate() error {
	return nil
}

// ExecuteTx executes the transaction against a world instance.
func (t *TxGCSweep) ExecuteTx(
	ctx context.Context,
	sender peer.ID,
	lookupWorldOp world.LookupOp,
	worldInstance world.WorldState,
) (sysErr bool, rerr error) {
	gc, ok := worldInstance.(garbageCollectable)
	if !ok {
		return true, errors.New("world state does not support garbage collection")
	}
	return false, gc.GarbageCollect(ctx)
}

// _ is a type assertion
var _ Transaction = ((*TxGCSweep)(nil))
