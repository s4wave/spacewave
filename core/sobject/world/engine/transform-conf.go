package sobject_world_engine

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// TransformEngine is a World engine whose Space can change the transform later
// World blocks are written with. Blocks already written keep their encoding,
// so the new transform must also read them.
type TransformEngine interface {
	// GetTransformConf returns the transform of the installed World head.
	GetTransformConf(ctx context.Context) (*block_transform.Config, error)
	// SetTransformConf changes the World transform to conf and waits for the
	// operation's outcome. conf must read every block the current transform
	// wrote. Setting the current transform does nothing.
	SetTransformConf(ctx context.Context, conf *block_transform.Config) error
}

// GetTransformConf returns the transform of the installed World head.
func (e *soEngine) GetTransformConf(ctx context.Context) (*block_transform.Config, error) {
	// Replay the admitted operations, then read the head.
	unlockWriteMtx, err := e.c.lockSettled(ctx, "transform-conf")
	if err != nil {
		return nil, err
	}
	defer unlockWriteMtx()
	snap, err := e.admittedState(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := e.advance(ctx, snap, nil); err != nil {
		return nil, err
	}
	_, head, _ := e.replay.head()
	return head.GetHeadRef().GetTransformConf().CloneVT(), nil
}

// SetTransformConf adds a SetTransformConfOp to the operation set and waits
// for its replay outcome.
func (e *soEngine) SetTransformConf(ctx context.Context, conf *block_transform.Config) error {
	// Build the operation on the current World, excluding write transactions
	// until it is placed. The SharedObject writes it directly after every
	// pending receipt, so wait for them outside the lock.
	ctx, unlockWriteMtx, err := e.c.lockStateSettled(ctx, e.so, "transform-conf")
	if err != nil {
		return err
	}
	defer unlockWriteMtx()
	snap, err := e.admittedState(ctx)
	if err != nil {
		return err
	}
	if _, err := e.advance(ctx, snap, nil); err != nil {
		return err
	}

	// Skip the current transform and refuse one replay would reject.
	_, head, _ := e.replay.head()
	prev := head.GetHeadRef().GetTransformConf()
	if prev.EqualVT(conf) {
		return nil
	}
	if err := validateTransformSuccessor(prev, conf); err != nil {
		return err
	}

	// Add it and wait for its outcome.
	opData, err := (&SOWorldOp{
		Body: &SOWorldOp_SetTransformConf{
			SetTransformConf: &SetTransformConfOp{TransformConf: conf},
		},
	}).MarshalVT()
	if err != nil {
		return err
	}
	return e.queueOperation(ctx, opData, nil, nil)
}

// processSetTransformConfOp changes the transform of the head World.
func processSetTransformConfOp(
	le *logrus.Entry,
	setOp *SetTransformConfOp,
	headState *InnerState,
	peerID peer.ID,
	nonce uint64,
) (*InnerState, *sobject.SOOperationResult, error) {
	// The World must exist and the new transform must read its blocks.
	if headState.GetHeadRef().GetEmpty() {
		return rejectOp(le, peerID, nonce, "world is not initialized")
	}
	if err := validateTransformSuccessor(headState.GetHeadRef().GetTransformConf(), setOp.GetTransformConf()); err != nil {
		return rejectOp(le, peerID, nonce, err.Error())
	}

	// Accept it.
	nextHeadState := headState.CloneVT()
	nextHeadState.HeadRef.TransformConf = setOp.GetTransformConf().CloneVT()
	return nextHeadState, sobject.BuildSOOperationResult(peerID.String(), nonce, true, nil), nil
}

// validateTransformSuccessor checks that next is a valid World write transform
// that reads every block prev wrote: the same steps in the same order, where
// each step is unchanged or a gzip step became an s2 step that reads gzip.
func validateTransformSuccessor(prev, next *block_transform.Config) error {
	// The new transform must be one the World may write with.
	if err := validateWorldWriteTransform(next); err != nil {
		return err
	}

	// Compare the steps in order.
	prevSteps, nextSteps := prev.GetSteps(), next.GetSteps()
	if len(prevSteps) != len(nextSteps) {
		return errors.New("new world transform must keep the same steps")
	}
	for i, prevStep := range prevSteps {
		if err := validateStepSuccessor(prevStep, nextSteps[i]); err != nil {
			return errors.Wrapf(err, "world transform step %d", i)
		}
	}
	return nil
}

// validateStepSuccessor checks that next decodes every block prev encoded.
func validateStepSuccessor(prev, next *block_transform.StepConfig) error {
	// An unchanged step reads its own output.
	if prev.EqualVT(next) {
		return nil
	}

	// s2 with read_gzip reads gzip output.
	if prev.GetId() != transform_gzip.ConfigID || next.GetId() != transform_s2.ConfigID {
		return errors.Errorf("cannot replace %s with %s", prev.GetId(), next.GetId())
	}
	s2Conf := &transform_s2.Config{}
	if err := block_transform.UnmarshalStepConfig(next.GetConfig(), s2Conf); err != nil {
		return errors.Wrap(err, "s2 config")
	}
	if !s2Conf.GetReadGzip() {
		return errors.New("s2 step replacing gzip must set read_gzip")
	}
	return nil
}

// _ is a type assertion
var _ TransformEngine = (*soEngine)(nil)
