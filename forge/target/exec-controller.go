package forge_target

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
)

// ExecController is a controller that implements the target Exec controller.
// The controller will be constructed using the exec.controller config.
type ExecController interface {
	// Controller indicates this is a controllerbus controller.
	controller.Controller
	// InitForgeExecController initializes the Forge execution controller.
	// This is called before Execute().
	// Any error returned cancels execution of the controller.
	InitForgeExecController(
		ctx context.Context,
		inputs InputMap,
		handle ExecControllerHandle,
	) error
}

// ExecControllerHandle is the handle passed to the exec controller during init.
// This contains functions that can be called during execution.
type ExecControllerHandle interface {
	// GetExecutionUniqueId returns a unique identifier for the execution pass.
	GetExecutionUniqueId() string
	// GetPeerId returns the peer id that this exec controller is operating as.
	GetPeerId() peer.ID
	// GetTimestamp returns the timestamp for the execution and all execution ops.
	// Cannot return nil. Do not edit this object.
	GetTimestamp() *timestamp.Timestamp
	// AccessStorage builds a bucket lookup cursor located at the given ref.
	// If the ref is empty, will produce a cursor at the root of the world.
	// The lookup cursor will be released after cb returns.
	AccessStorage(
		ctx context.Context,
		ref *bucket.ObjectRef,
		cb func(*bucket_lookup.Cursor) error,
	) error
	// SetOutputs changes the outputs according to the given ValueSlice.
	// Note: the slice contents will be copied before the call returns.
	// Note: each Value must be named.
	// Use the writeCursor to write output objects, then SetOutputs with the refs.
	// If clearOld is set, all old Output values will be cleared.
	// Returns context.Canceled if the handle ctx is canceled.
	SetOutputs(
		ctx context.Context,
		outps forge_value.ValueSlice,
		clearOld bool,
	) error
	// WriteLog appends log entries to the execution.
	// Each entry is a (level, message) pair. The timestamp is set automatically.
	// Returns context.Canceled if the handle ctx is canceled.
	WriteLog(ctx context.Context, level, message string) error
}
