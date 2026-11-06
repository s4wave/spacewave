package sobject

import (
	"bytes"
	"context"
)

// OrderEvent is the agreed order of a SharedObject's operations in one state:
// the checkpoint it starts from and every operation above it, in the order
// every member replays them.
type OrderEvent struct {
	// Snapshot is the state the order was read from.
	Snapshot SharedObjectStateSnapshot
	// Config is the accepted config of Snapshot.
	Config *SharedObjectConfig
	// Checkpoint is the checkpoint the order starts from, with its state data
	// decoded.
	Checkpoint *SOCheckpointInner
	// Set holds the operations above Checkpoint.
	Set *SOOperationSet
	// Order is the agreed order of Set's operations, by hash.
	Order [][]byte
	// Stable is how many of the first operations of Order are stable: replay
	// never places a new operation among them, so a checkpoint may cover them.
	Stable int
}

// ReadOrder reads the agreed order of snap.
func ReadOrder(ctx context.Context, snap SharedObjectStateSnapshot) (*OrderEvent, error) {
	// Read the config, the checkpoint and the operations above it.
	cfg, err := snap.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	checkpoint, err := snap.GetCheckpoint(ctx)
	if err != nil {
		return nil, err
	}
	set, err := snap.GetOperationSet(ctx)
	if err != nil {
		return nil, err
	}

	// Order them and find the stable prefix.
	return &OrderEvent{
		Snapshot:   snap,
		Config:     cfg,
		Checkpoint: checkpoint,
		Set:        set,
		Order:      set.Order(),
		Stable:     len(set.StablePoint(cfg.TrimRoster())),
	}, nil
}

// SharedPrefix returns how many leading hashes a and b share.
func SharedPrefix(a, b [][]byte) int {
	n := min(len(a), len(b))
	for i := range n {
		if !bytes.Equal(a[i], b[i]) {
			return i
		}
	}
	return n
}
