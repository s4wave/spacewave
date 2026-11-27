package cluster_tx

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/byteslice"
	"github.com/s4wave/spacewave/db/world"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Transaction is an instance of a transaction object.
type Transaction interface {
	// MarshalVT marshals to binary.
	MarshalVT() ([]byte, error)
	// UnmarshalVT unmarshals from binary.
	UnmarshalVT(data []byte) error

	// GetTxType returns the type of transaction this is.
	GetTxType() TxType
	// Validate checks the transaction cursorily.
	// Note: this should not fetch network data.
	Validate() error
	// ExecuteTx executes the transaction against the cluster instance.
	// bcs is located at the pass state root.
	// The result is written into bcs.
	ExecuteTx(
		ctx context.Context,
		worldState world.WorldState,
		sender peer.ID,
		objKey string,
		bcs *block.Cursor,
		root *forge_cluster.Cluster,
	) error
}

// Validate checks the transaction (cursory checks only).
func (t *Tx) Validate() error {
	// Require a cluster object key before validating the transaction payload.
	if len(t.GetClusterObjectKey()) == 0 {
		return errors.Wrap(world.ErrEmptyObjectKey, "cluster_object_key")
	}

	// Reject transaction types the cluster cannot execute.
	if err := t.GetTxType().Validate(); err != nil {
		return err
	}

	// Locate the transaction payload for its own validation.
	ttx, err := t.LocateTx()
	if err != nil {
		return err
	}

	return ttx.Validate()
}

// Validate checks the execution tx type is in range.
func (t TxType) Validate() error {
	switch t {
	default:
		return errors.Wrap(world.ErrUnhandledOp, t.String())
	}
}

// LocateTx returns the sub-block for the transaction.
func (t *Tx) LocateTx() (Transaction, error) {
	switch t.GetTxType() {
	default:
		return nil, errors.Wrap(world.ErrUnhandledOp, t.String())
	}
}

// ByteSliceToTx converts a byte slice block to a Tx.
// If blk is nil, returns nil, nil
// If the blk is already parsed to a Tx, returns the Tx.
func ByteSliceToTx(blk block.Block) (*Tx, error) {
	// Preserve an absent block as an absent transaction.
	if blk == nil {
		return nil, nil
	}

	// Decode byte slice blocks into a transaction payload.
	var out *Tx
	nr, ok := blk.(*byteslice.ByteSlice)
	if ok && nr != nil {
		// Unmarshal the transaction before exposing it to the caller.
		out = &Tx{}
		if err := out.UnmarshalVT(nr.GetBytes()); err != nil {
			return nil, err
		}

		return out, nil
	}

	// Accept parsed transactions and reject other block types.
	out, ok = blk.(*Tx)
	if !ok {
		return out, block.ErrUnexpectedType
	}

	return out, nil
}

// ApplyWorldOp applies the operation as a world operation.
func (t *Tx) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Locate the transaction payload to apply to the cluster.
	ttx, err := t.LocateTx()
	if err != nil {
		return false, err
	}

	// Apply the transaction within writable access to its cluster object.
	objKey := t.GetClusterObjectKey()
	_, _, err = world.AccessWorldObject(ctx, worldHandle, objKey, true, func(bcs *block.Cursor) error {
		// Decode the cluster state from its World object.
		ps, err := forge_cluster.UnmarshalCluster(ctx, bcs)
		if err != nil {
			return err
		}

		// Execute the transaction and validate the resulting cluster state.
		err = ttx.ExecuteTx(ctx, worldHandle, sender, objKey, bcs, ps)
		if err == nil {
			err = ps.Validate()
		}

		return err
	})

	return false, err
}

// _ is a type assertion
var _ world.Operation = (*Tx)(nil)
