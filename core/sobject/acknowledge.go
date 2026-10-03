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
	// Watch the state as the local peer.
	ctr, rel, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return err
	}
	defer rel()
	peerID := so.GetPeerID().String()

	// Acknowledge each state that needs it.
	var snap SharedObjectStateSnapshot
	for {
		// Wait for the next state.
		snap, err = ctr.WaitValueChange(ctx, snap, nil)
		if err != nil {
			return err
		}
		if snap == nil {
			continue
		}

		// Acknowledge as a writer, on the roster or dropped from it, so a
		// dropped device that returns answers the checkpointer too.
		cfg, err := snap.GetConfig(ctx)
		if err != nil {
			return err
		}
		if !slices.Contains(cfg.Writers(), peerID) {
			continue
		}
		set, err := snap.GetOperationSet(ctx)
		if err != nil {
			return err
		}
		if !set.NeedsAcknowledgment(cfg.Checkpointer(), peerID, AcknowledgmentLag) {
			continue
		}
		if err := write(ctx); err != nil {
			return err
		}
	}
}
