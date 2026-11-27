package sobject

import (
	"bytes"
	"context"
	"slices"
	"time"

	"github.com/pkg/errors"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
)

// errCheckpointStale reports that the held state moved past the stable point
// a checkpoint was built for.
var errCheckpointStale = errors.New("held state moved past the stable point")

// BodyCheckpointer signs a BodyDriver's checkpoints through the state host as
// the local participant. It is fixed for the life of the driver or host that
// takes it.
type BodyCheckpointer struct {
	// host holds the state.
	host *SOHost
	// privKey signs as the checkpointer.
	privKey crypto.PrivKey
	// sfs builds the key epoch transforms.
	sfs *block_transform.StepFactorySet
	// deadline is how long the first stable operation above the checkpoint
	// waits before a checkpoint below MinCheckpointOperations, or zero to
	// checkpoint at MinCheckpointOperations only.
	deadline time.Duration
}

// NewBodyCheckpointer returns the checkpointer that signs, through host as
// privKey, the checkpoints a device owes: as the checkpointer of an
// owner-controlled Space, a checkpoint over the stable prefix once
// MinCheckpointOperations operations are stable, or once the first stable one
// has waited deadline. A zero deadline checkpoints at MinCheckpointOperations
// only. The body's Root holds the state each one carries. sfs builds the key
// epoch transforms that encrypt it.
func NewBodyCheckpointer(host *SOHost, privKey crypto.PrivKey, sfs *block_transform.StepFactorySet, deadline time.Duration) *BodyCheckpointer {
	return &BodyCheckpointer{host: host, privKey: privKey, sfs: sfs, deadline: deadline}
}

// adoptHeld adopts the checkpoint covering prefix into the held state, in an
// ordered commit, as adopt describes.
func (c *BodyCheckpointer) adoptHeld(ctx context.Context, le *logrus.Entry, so SharedObject, prefix, refused [][]byte, data []byte, height uint64) error {
	return c.host.UpdateSOState(kvtx.WithOrderedCommitContext(ctx), func(state *SOState) error {
		return c.adopt(ctx, le, so, state, prefix, refused, data, height)
	})
}

// adopt signs the checkpoint covering prefix with the body state data after
// it, encrypted with state's current key epoch, and the outcomes of prefix:
// refused lists the operations of prefix the body refused. It adopts the
// checkpoint into state. It returns errCheckpointStale when state no longer
// holds the checkpoint at height or prefix is no longer its stable prefix.
func (c *BodyCheckpointer) adopt(ctx context.Context, le *logrus.Entry, so SharedObject, state *SOState, prefix, refused [][]byte, data []byte, height uint64) error {
	// The held checkpoint is unchanged and prefix is still stable.
	current, err := state.GetCheckpointInner()
	if err != nil {
		return err
	}
	if current.GetHeight() != height {
		return errCheckpointStale
	}
	soID := so.GetSharedObjectID()
	held, err := c.verified(state)
	if err != nil {
		return err
	}
	stable := held.StablePoint(state.GetConfig().TrimRoster())
	if len(stable) < len(prefix) || !slices.EqualFunc(stable[:len(prefix)], prefix, bytes.Equal) {
		return errCheckpointStale
	}

	// Encrypt the state data with the current key epoch.
	dataEnc, err := c.encrypt(ctx, le, so, state, data)
	if err != nil {
		return err
	}

	// Sign and adopt the checkpoint with the outcomes it covers.
	outcomes, err := held.outcomes(refused)
	if err != nil {
		return err
	}
	checkpoint, err := state.signStableCheckpoint(soID, held, c.privKey, prefix, dataEnc, outcomes)
	if err != nil {
		return err
	}
	return c.host.AdoptCheckpoint(state, checkpoint)
}

// verified returns the operations state holds as a verified set, through the
// host's verifier, which checks only the signatures of operations it has not
// verified before: the held ones were verified when the host's state took
// them, so a checkpoint or a vote checks none of them again.
func (c *BodyCheckpointer) verified(state *SOState) (*SOOperationSet, error) {
	return c.host.OperationSet(state)
}

// handle returns the local participant's view of state.
func (c *BodyCheckpointer) handle(le *logrus.Entry, so SharedObject, state *SOState) *SOStateParticipantHandle {
	return NewSOStateParticipantHandle(le, c.sfs, so.GetSharedObjectID(), state, c.privKey, so.GetPeerID()).
		WithGrantKeyring(c.host.GetGrantKeyring())
}

// encrypt encrypts body state data with state's current key epoch.
func (c *BodyCheckpointer) encrypt(ctx context.Context, le *logrus.Entry, so SharedObject, state *SOState, data []byte) ([]byte, error) {
	xfrm, err := c.handle(le, so, state).GetTransformer(ctx)
	if err != nil {
		return nil, err
	}
	return xfrm.EncodeBlock(data)
}

// decode returns inner with its state data decoded by state's keys.
func (c *BodyCheckpointer) decode(le *logrus.Entry, so SharedObject, state *SOState, inner *SOCheckpointInner) (*SOCheckpointInner, error) {
	return c.handle(le, so, state).DecodeCheckpoint(inner)
}
