package space_world

import (
	"context"
	"encoding/hex"
	"os"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/sirupsen/logrus"
)

// replayCheckEnv identifies the environment variable that turns on the replay
// check when set to 1. Apply then also runs each of this device's operations
// whose candidate World it adopts on the replay engine, as another device
// replays it, and logs an outcome that differs from the adopted World.
// Install logs both Worlds when the replay misses the checkpoint's. It is a
// diagnostic for replicas that reach different Worlds and never changes the
// position Apply takes or what Install keeps. Its log lines carry the full
// refusal reason, which may quote operation data, so it stays off by default.
const replayCheckEnv = "SPACEWAVE_REPLAY_CHECK"

// replayCheck reports whether the replay check is on, read once at start.
var replayCheck = os.Getenv(replayCheckEnv) == "1"

// checkAdopted runs op on the World before index and logs how its outcome
// differs from adopt, the World this device's writer built with op. A World
// before index whose pages left memory unstored is skipped: running on it
// would need the recompute that only the replay's own path may start. The
// replay's writes stay in its stage like any replay's. The next operation
// moves the engine on, as after any adopted candidate.
func (r *spaceReplay) checkAdopted(ctx context.Context, index int, op *sobject.OrderedOperation, adopt *spacePosition) {
	// Run op as another device replays it, on a World whose pages are here.
	if index != 0 && index <= r.floor {
		return
	}
	prev, seqno := r.stateBefore(index)
	pos, err := r.apply(ctx, spacePosition{op: op, state: prev, seqno: seqno}, op)

	// Record the operation and the adopted World.
	wop := &WorldOp{}
	opType := "undecodable"
	if wop.UnmarshalVT(op.Data) == nil {
		opType = opTypeOf(wop)
	}
	le := r.a.le.WithFields(logrus.Fields{
		"op-hash":      hex.EncodeToString(op.Hash),
		"op-type":      opType,
		"op-index":     index,
		"base-root":    prev.GetHeadRef().GetRootRef().MarshalString(),
		"adopted-root": adopt.state.GetHeadRef().GetRootRef().MarshalString(),
	})

	// Log a replay that fails, refuses or reaches another World.
	switch {
	case err != nil:
		le.WithError(err).Warn("replay check: replay of an adopted operation failed")
	case pos.reason != "":
		le.WithField("reason", pos.reason).Warn("replay check: replay refuses an adopted operation")
	case !pos.state.EqualVT(adopt.state):
		le.WithField("replayed-root", pos.state.GetHeadRef().GetRootRef().MarshalString()).
			WithField("replayed-state", pos.state.String()).
			WithField("adopted-state", adopt.state.String()).
			Warn("replay check: replay of an adopted operation reaches another World")
	}
}

// logInstallMismatch logs the checkpoint's World and the one the replay
// reached after the covered positions, when they differ.
func (r *spaceReplay) logInstallMismatch(height uint64, covered int, checkpoint, replayed *InnerState) {
	r.a.le.WithFields(logrus.Fields{
		"checkpoint-height": height,
		"covered":           covered,
		"positions":         len(r.positions),
		"checkpoint-state":  checkpoint.String(),
		"replayed-state":    replayed.String(),
	}).Warn("replay check: checkpoint World differs from the replayed World")
}
