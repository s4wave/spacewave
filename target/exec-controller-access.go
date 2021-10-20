package forge_target

import (
	"context"
	"errors"

	"github.com/aperturerobotics/bifrost/peer"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/world"
)

// accessHandle is an ExecControllerHandle which only implements access.
type accessHandle struct {
	peerID      peer.ID
	targetWorld world.Engine
	accessFunc  world.AccessWorldStateFunc
}

// ExecControllerHandleWithAccess constructs an ExecControllerHandle which only
// implements AccessStorage.
func ExecControllerHandleWithAccess(peerID peer.ID, targetWorld world.Engine, accessFunc world.AccessWorldStateFunc) ExecControllerHandle {
	return &accessHandle{peerID: peerID, targetWorld: targetWorld, accessFunc: accessFunc}
}

// GetPeerId returns the peer id that this exec controller is operating as.
func (a *accessHandle) GetPeerId() peer.ID {
	return a.peerID
}

// GetTargetWorld returns a handle to the target world engine.
// Returns nil, ErrTargetWorldUnset if this was not configured.
func (a *accessHandle) GetTargetWorld() (world.Engine, error) {
	if a.targetWorld == nil {
		return nil, ErrTargetWorldUnset
	}
	return a.targetWorld, nil
}

// AccessStorage builds a bucket lookup cursor located at the given ref.
func (h *accessHandle) AccessStorage(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	return h.accessFunc(ctx, ref, cb)
}

// SetOutputs changes the outputs according to the given ValueSlice.
func (h *accessHandle) SetOutputs(context.Context, forge_value.ValueSlice, bool) error {
	return errors.New("set outputs unavailable in access-only handle")
}

// _ is a type assertion
var _ ExecControllerHandle = ((*accessHandle)(nil))
