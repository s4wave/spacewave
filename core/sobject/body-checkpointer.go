package sobject

import (
	"bytes"
	"context"
	"slices"

	"github.com/pkg/errors"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
)

// errCheckpointStale reports that the held state moved past the stable point
// a checkpoint was built for.
var errCheckpointStale = errors.New("held state moved past the stable point")

// bodyCheckpointer signs a BodyDriver's checkpoints through the state host.
type bodyCheckpointer struct {
	// host holds the state.
	host *SOHost
	// privKey signs as the checkpointer.
	privKey crypto.PrivKey
	// sfs builds the key epoch transforms.
	sfs *block_transform.StepFactorySet
}

// adopt signs the checkpoint covering prefix with the body state data after
// it, encrypted with state's current key epoch, and adopts it into state. It
// returns errCheckpointStale when state no longer holds the checkpoint at
// height or prefix is no longer its stable prefix.
func (c *bodyCheckpointer) adopt(ctx context.Context, le *logrus.Entry, so SharedObject, state *SOState, prefix [][]byte, data []byte, height uint64) error {
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

	// Sign and adopt the checkpoint.
	inner, err := state.stableCheckpointInner(soID, held, prefix, dataEnc)
	if err != nil {
		return err
	}
	checkpoint, err := BuildSOCheckpoint(c.privKey, inner)
	if err != nil {
		return err
	}
	return c.host.GetOperationVerifier().AdoptCheckpoint(state, checkpoint)
}

// verified returns the operations state holds as a verified set, through the
// host's verifier, which checks only the signatures of operations it has not
// verified before: the held ones were verified when the host's state took
// them, so a checkpoint or a vote checks none of them again.
func (c *bodyCheckpointer) verified(state *SOState) (*SOOperationSet, error) {
	return c.host.GetOperationVerifier().OperationSet(state)
}

// handle returns the local participant's view of state.
func (c *bodyCheckpointer) handle(le *logrus.Entry, so SharedObject, state *SOState) *SOStateParticipantHandle {
	return NewSOStateParticipantHandle(le, c.sfs, so.GetSharedObjectID(), state, c.privKey, so.GetPeerID()).
		WithGrantKeyring(c.host.GetGrantKeyring())
}

// encrypt encrypts body state data with state's current key epoch.
func (c *bodyCheckpointer) encrypt(ctx context.Context, le *logrus.Entry, so SharedObject, state *SOState, data []byte) ([]byte, error) {
	xfrm, err := c.handle(le, so, state).GetTransformer(ctx)
	if err != nil {
		return nil, err
	}
	return xfrm.EncodeBlock(data)
}

// decode returns inner with its state data decoded by state's keys.
func (c *bodyCheckpointer) decode(le *logrus.Entry, so SharedObject, state *SOState, inner *SOCheckpointInner) (*SOCheckpointInner, error) {
	return c.handle(le, so, state).DecodeCheckpoint(inner)
}
