package block_rpc_client

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
)

// reconnectDelay bounds the wait between reads retried after a lost
// connection.
const (
	reconnectDelayMin = 25 * time.Millisecond
	reconnectDelayMax = time.Second
)

// ReconnectReads wraps a store reached over a connection that may drop and
// come back, such as a block RPC whose client opens each call on the current
// link. A read that fails because the connection closed is sent again, with
// a growing delay, until it gets an answer or ctx ends. Writes, removals and
// roots pass through unchanged, since a write lost with its connection may
// still have applied.
func ReconnectReads(store block.StoreOps) block.StoreOps {
	return &reconnectReads{StoreOps: store}
}

// reconnectReads retries the reads of the store it embeds.
type reconnectReads struct {
	block.StoreOps
}

// BeginReadOperation returns the retrying store, since each read already
// stands alone.
func (r *reconnectReads) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return r, func() {}, nil
}

// GetBlock reads the block, waiting out a lost connection.
func (r *reconnectReads) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	var data []byte
	var found bool
	err := retryRead(ctx, func() error {
		var err error
		data, found, err = r.StoreOps.GetBlock(ctx, ref)
		return err
	})
	return data, found, err
}

// GetStoredBlock reads the block and its refs, waiting out a lost connection.
func (r *reconnectReads) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	var stored *block.StoredBlock
	err := retryRead(ctx, func() error {
		var err error
		stored, err = r.StoreOps.GetStoredBlock(ctx, ref)
		return err
	})
	return stored, err
}

// GetBlockExists checks the block, waiting out a lost connection.
func (r *reconnectReads) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	var found bool
	err := retryRead(ctx, func() error {
		var err error
		found, err = r.StoreOps.GetBlockExists(ctx, ref)
		return err
	})
	return found, err
}

// GetBlockExistsBatch checks the blocks, waiting out a lost connection.
func (r *reconnectReads) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	var found []bool
	err := retryRead(ctx, func() error {
		var err error
		found, err = r.StoreOps.GetBlockExistsBatch(ctx, refs)
		return err
	})
	return found, err
}

// StatBlock stats the block, waiting out a lost connection.
func (r *reconnectReads) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	var stat *block.BlockStat
	err := retryRead(ctx, func() error {
		var err error
		stat, err = r.StoreOps.StatBlock(ctx, ref)
		return err
	})
	return stat, err
}

// retryRead runs read until it returns anything but a lost connection, or
// ctx ends.
func retryRead(ctx context.Context, read func() error) error {
	delay := reconnectDelayMin
	for {
		// Return any answer, including a store error, that is not a lost
		// connection.
		err := read()
		if err == nil || !IsConnectionLost(err) || ctx.Err() != nil {
			return err
		}

		// Wait before sending the read on the next connection.
		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			t.Stop()
			return context.Cause(ctx)
		case <-t.C:
		}
		delay = min(delay*2, reconnectDelayMax)
	}
}

// IsConnectionLost reports whether err means the call's connection closed or
// was reset before it answered, rather than the store answering with an
// error.
func IsConnectionLost(err error) bool {
	return errors.Is(err, srpc.ErrClosedBeforeCompletion) ||
		errors.Is(err, srpc.ErrReset) ||
		errors.Is(err, srpc.ErrNoAvailableClients) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, io.ErrClosedPipe)
}
