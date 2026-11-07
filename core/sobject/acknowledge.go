package sobject

import (
	"context"
	"slices"
)

// Acknowledge calls write each time the local peer should acknowledge, as
// NeedsAcknowledgment describes, so the checkpointer can checkpoint and trim
// the history every roster member has built on. write queues an empty
// operation after every operation the local peer has already started. It
// watches the state and returns when ctx ends or write fails.
func Acknowledge(ctx context.Context, so SharedObject, write func(context.Context) error) error {
	return watchStates(ctx, so, func(snap SharedObjectStateSnapshot, cfg *SharedObjectConfig, peerID string) error {
		// Acknowledge as a writer, on the roster or dropped from it, so a
		// dropped device that returns answers the checkpointer too.
		if !slices.Contains(cfg.Writers(), peerID) {
			return nil
		}
		set, err := snap.GetOperationSet(ctx)
		if err != nil || !set.NeedsAcknowledgment(cfg.Checkpointer(), peerID, AcknowledgmentLag) {
			return err
		}
		return write(ctx)
	})
}
