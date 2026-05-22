package world_block_tx

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
)

// garbageCollectable is implemented by world states that support GC sweep.
type garbageCollectable interface {
	// GarbageCollect sweeps unreferenced nodes from the GC ref graph.
	GarbageCollect(ctx context.Context) error
}

// NewTxGCSweep constructs a legacy GC_SWEEP transaction.
//
// Deprecated: use NewMaintenanceTxGCSweep or NewExplicitTxGCSweep so sweep
// intent is persisted with the transaction.
func NewTxGCSweep() (*Tx, error) {
	return newTxGCSweep(TxGCSweepIntent_TxGCSweepIntent_LEGACY_MAINTENANCE), nil
}

// NewMaintenanceTxGCSweep constructs a maintenance GC_SWEEP transaction.
func NewMaintenanceTxGCSweep() (*Tx, error) {
	return newTxGCSweep(TxGCSweepIntent_TxGCSweepIntent_MAINTENANCE), nil
}

// NewExplicitTxGCSweep constructs an explicit GC_SWEEP transaction.
func NewExplicitTxGCSweep() (*Tx, error) {
	return newTxGCSweep(TxGCSweepIntent_TxGCSweepIntent_EXPLICIT), nil
}

func newTxGCSweep(intent TxGCSweepIntent) *Tx {
	return &Tx{
		TxType:    TxType_TxType_GC_SWEEP,
		TxGcSweep: &TxGCSweep{Intent: intent},
	}
}

// ContainsGCSweep returns true when tx or any nested batch transaction is a GC sweep.
func ContainsGCSweep(tx *Tx) bool {
	if tx == nil {
		return false
	}
	switch tx.GetTxType() {
	case TxType_TxType_GC_SWEEP:
		return true
	case TxType_TxType_BATCH:
		if slices.ContainsFunc(tx.GetTxBatch().GetTxs(), ContainsGCSweep) {
			return true
		}
	}
	return false
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
	return &TxGCSweep{Intent: t.Intent}
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
