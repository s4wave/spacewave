package s4wave_world

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/db/coord"
)

// Tx represents a transaction against the world state.
// Tx implements the world state transaction interfaces.
// Provides:
// - WorldState: full state read/write interface (inherited from WorldState)
// - tx.Tx: Commit, Discard operations
//
// A Tx maintains state across multiple RPC calls, enabling complex multi-step
// operations within a single transaction. Always call Discard() when done.
//
// Concurrent calls to WorldState functions should be supported.
type Tx struct {
	*WorldState
	txService SRPCTxResourceServiceClient
}

// NewTx creates a new Tx resource wrapper.
func NewTx(client *resource_client.Client, ref resource_client.ResourceRef, readOnly bool) (*Tx, error) {
	ws, err := NewWorldState(client, ref, readOnly)
	if err != nil {
		return nil, err
	}

	srpcClient, err := ref.GetClient()
	if err != nil {
		return nil, err
	}

	return &Tx{
		WorldState: ws,
		txService:  NewSRPCTxResourceServiceClient(srpcClient),
	}, nil
}

// Commit commits the transaction.
// After commit, the transaction should be discarded and its resource released.
// Commit waits for the server's durable publication, not merely admission. A
// concurrent Engine.NewTransaction may prepare the next revision after this
// transaction seals but before this call completes. Keep every outstanding
// Commit result and join them before reporting success; canceling an RPC does
// not establish that an already admitted write was rolled back.
//
// A commit rejected because another writer advanced the World returns an error
// matching coord.ErrStaleGeneration; the caller may retry from a new
// transaction.
func (tx *Tx) Commit(ctx context.Context) error {
	_, err := tx.txService.Commit(ctx, &CommitRequest{})
	return CommitError(err)
}

// CommitError restores the stale-generation classification that the RPC
// boundary flattens to text in a Commit error.
func CommitError(err error) error {
	if err == nil {
		return nil
	}
	msg, stale := strings.CutSuffix(err.Error(), coord.ErrStaleGeneration.Error())
	if !stale {
		return err
	}
	msg = strings.TrimSuffix(msg, ": ")
	if msg == "" {
		return coord.ErrStaleGeneration
	}
	return errors.Wrap(coord.ErrStaleGeneration, msg)
}

// Discard discards the transaction without committing changes.
// Always call this when done with the transaction.
func (tx *Tx) Discard(ctx context.Context) error {
	_, err := tx.txService.Discard(ctx, &DiscardRequest{})
	return err
}
