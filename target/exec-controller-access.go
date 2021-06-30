package forge_target

import (
	"context"
	"errors"

	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/world"
)

// accessHandle is an ExecControllerHandle which only implements access.
type accessHandle struct {
	accessFunc world.AccessWorldStateFunc
}

// ExecControllerHandleWithAccess constructs an ExecControllerHandle which only
// implements AccessStorage.
func ExecControllerHandleWithAccess(accessFunc world.AccessWorldStateFunc) ExecControllerHandle {
	return &accessHandle{accessFunc: accessFunc}
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
