package sobject_world_engine

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/bucket"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// maxGraphSettingsStep bounds GraphSettingsStepOp.max, keeping one replayed
// step's commit short for every member.
const maxGraphSettingsStep = 65536

// GraphSettingsEngine is a World engine whose Space can change the World's
// graph settings online. world_block.ApplyGraphSettings drives it.
type GraphSettingsEngine interface {
	world_block.GraphSettingsStepper
}

// StepGraphSettings reports whether the head World has the target graph
// settings, and if not adds one GraphSettingsStepOp to the operation set and
// waits for its replay outcome.
func (e *soEngine) StepGraphSettings(ctx context.Context, target *world_block.GraphSettings, max int) (bool, error) {
	// Refuse a step replay would reject.
	if err := validateGraphSettingsStep(target, max); err != nil {
		return false, err
	}
	want, err := normalizeGraphSettings(target)
	if err != nil {
		return false, err
	}

	// Replay the admitted operations, excluding write transactions until the
	// step is placed.
	ctx, unlockWriteMtx, err := e.c.lockStateSettled(ctx, e.so, "graph-settings")
	if err != nil {
		return false, err
	}
	defer unlockWriteMtx()
	snap, err := e.admittedState(ctx)
	if err != nil {
		return false, err
	}
	if _, err := e.advance(ctx, snap, nil); err != nil {
		return false, err
	}
	_, head, _ := e.replay.head()
	if head.GetHeadRef().GetEmpty() {
		return false, errors.New("world is not initialized")
	}

	// Report a World that already has the target settings.
	have, err := e.headGraphSettings(ctx)
	if err != nil {
		return false, err
	}
	if have.EqualVT(want) {
		return true, nil
	}

	// Add one step and wait for its outcome.
	opData, err := (&SOWorldOp{
		Body: &SOWorldOp_GraphSettingsStep{
			GraphSettingsStep: &GraphSettingsStepOp{Target: target, Max: uint32(max)}, //nolint:gosec // validateGraphSettingsStep bounds max to maxGraphSettingsStep.
		},
	}).MarshalVT()
	if err != nil {
		return false, err
	}
	return false, e.queueOperation(ctx, opData, nil, nil)
}

// headGraphSettings returns the graph settings of the replayed head World.
func (e *soEngine) headGraphSettings(ctx context.Context) (*world_block.GraphSettings, error) {
	// Read the root through a snapshot of the block engine.
	btx, err := e.admitted.NewBlockEngineTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer btx.Discard()
	return btx.GetGraphSettings(ctx)
}

// processGraphSettingsStepOp moves the head World one step toward the target
// graph settings.
func processGraphSettingsStepOp(
	ctx context.Context,
	le *logrus.Entry,
	stepOp *GraphSettingsStepOp,
	headState *InnerState,
	peerID peer.ID,
	nonce uint64,
	ws *blkEngine,
) (*InnerState, *sobject.SOOperationResult, error) {
	// Step the World and commit a change.
	var nextRef *bucket.ObjectRef
	aerr := func() error {
		// Fork the head World, step it, and write a changed root.
		btx, err := ws.bengine.NewBlockEngineTransaction(ctx, true)
		if err != nil {
			return err
		}
		defer btx.Discard()
		_, changed, err := btx.StepGraphSettings(ctx, stepOp.GetTarget(), int(stepOp.GetMax()))
		if err != nil || !changed {
			return err
		}
		nextRef, err = btx.CommitBlockTransaction(ctx)
		return err
	}()

	// A canceled context explains any failure, and a block that is not
	// available stops replay instead of rejecting.
	if ctx.Err() != nil {
		return nil, nil, context.Canceled
	}
	if isReplayGap(aerr) {
		return nil, nil, aerr
	}
	if aerr != nil {
		le.WithError(aerr).Warn("rejecting graph settings step")
		return nil, opRejection(peerID, nonce, "graph settings step failed: "+aerr.Error()), nil
	}

	// Accept it, with the committed World as the head when the step changed it.
	nextHeadState := headState.CloneVT()
	if nextRef != nil {
		nextHeadState.HeadRef = nextRef
		nextRef.BucketId = ""
	}
	return nextHeadState, sobject.BuildSOOperationResult(peerID.String(), nonce, true, nil), nil
}

// validateGraphSettingsStep checks a step's target and bound.
func validateGraphSettingsStep(target *world_block.GraphSettings, max int) error {
	if max < 1 || max > maxGraphSettingsStep {
		return errors.Errorf("graph settings step max must be 1 to %d", maxGraphSettingsStep)
	}
	return target.ValidateInitial()
}

// normalizeGraphSettings returns the settings a World root stores for target:
// nil for the default settings.
func normalizeGraphSettings(target *world_block.GraphSettings) (*world_block.GraphSettings, error) {
	if len(target.GetIndexes()) == 0 {
		return nil, nil
	}
	indexes, err := target.QuadIndexes()
	if err != nil {
		return nil, err
	}
	return world_block.NewGraphSettings(indexes)
}

// _ is a type assertion
var _ GraphSettingsEngine = (*soEngine)(nil)
