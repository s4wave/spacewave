package space_exec

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	execution_transaction "github.com/s4wave/spacewave/forge/execution/tx"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"

	uuid "github.com/satori/go.uuid"
)

// ProcessExecution reads an execution object from world state and runs the
// target exec handler through the SpaceExecRegistry. Manages the full
// lifecycle: PENDING -> RUNNING -> COMPLETE/FAILED.
//
// Returns nil when the execution completes (success or failure recorded).
// Returns an error if the execution cannot be processed (object missing, etc.).
func ProcessExecution(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	registry *Registry,
	objectKey string,
	peerID peer.ID,
) error {
	le = le.WithField("execution", objectKey)

	// Read the execution object.
	obj, err := world.MustGetObject(ctx, ws, objectKey)
	if err != nil {
		return errors.Wrap(err, "get execution object")
	}

	objRef, _, err := obj.GetRootRef(ctx)
	if err != nil {
		return errors.Wrap(err, "get execution root ref")
	}

	// Unmarshal execution state.
	var exState *forge_execution.Execution
	_, err = world.AccessObject(ctx, ws.AccessWorldState, objRef, func(bcs *block.Cursor) error {
		var berr error
		exState, berr = forge_execution.UnmarshalExecution(ctx, bcs)
		return berr
	})
	if err != nil {
		return errors.Wrap(err, "unmarshal execution")
	}
	if err := exState.Validate(); err != nil {
		return errors.Wrap(err, "validate execution state")
	}

	// Check state.
	currState := exState.GetExecutionState()
	if currState == forge_execution.State_ExecutionState_COMPLETE {
		le.Debug("execution already complete")
		return nil
	}

	// Promote PENDING -> RUNNING.
	if currState == forge_execution.State_ExecutionState_PENDING {
		txd := execution_transaction.NewTxStart(peerID)
		_, _, err = obj.ApplyObjectOp(ctx, txd, peerID)
		if err != nil {
			return errors.Wrap(err, "promote pending to running")
		}
		le.Debug("promoted execution to running")
		// Re-read after state change.
		return ProcessExecution(ctx, le, ws, registry, objectKey, peerID)
	}

	if currState != forge_execution.State_ExecutionState_RUNNING {
		return errors.Errorf("unexpected execution state: %s", currState.String())
	}

	// Read the target config.
	var tgt *forge_target.Target
	_, err = world.AccessObject(ctx, ws.AccessWorldState, nil, func(bcs *block.Cursor) error {
		bcs = bcs.Detach(true)
		bcs.ClearAllRefs()
		bcs.SetRefAtCursor(exState.GetTargetRef(), true)
		var berr error
		tgt, berr = forge_target.UnmarshalTarget(ctx, bcs)
		return berr
	})
	if err != nil {
		return errors.Wrap(err, "read target config")
	}

	tgtExec := tgt.GetExec()
	ctrlConf := tgtExec.GetController()
	if tgtExec.GetDisable() || ctrlConf.GetId() == "" {
		le.Debug("execution disabled or empty config ID")
		return nil
	}

	configID := ctrlConf.GetId()
	configData := ctrlConf.GetConfig()

	// Build the unique ID (same derivation as the vendored controller).
	uniqueID := buildUniqueID(peerID, objectKey)

	// Resolve inputs.
	inputsValMap, err := forge_value.
		ValueSlice(exState.GetValueSet().GetInputs()).
		BuildValueMap(true, true)
	if err != nil {
		return errors.Wrap(err, "build inputs value map")
	}

	// Build a minimal InputMap with the world input.
	inputsMap := make(forge_target.InputMap, len(inputsValMap)+1)
	inputsMap["world"] = forge_target.NewInputValueWorld(nil, ws)

	// Construct the handle.
	handle := newExecHandle(ctx, ws, objectKey, peerID, uniqueID, exState.GetTimestamp())

	// Create handler via the registry.
	handler, err := registry.CreateHandler(ctx, le, ws, handle, inputsMap, configID, configData)
	if err != nil {
		markFailed(ctx, le, ws, objectKey, peerID, errors.Wrap(err, "create handler"))
		return nil
	}

	// Execute.
	le.WithField("config-id", configID).Info("starting space exec handler")
	execErr := handler.Execute(ctx)

	// If context canceled, propagate without marking.
	if ctx.Err() != nil {
		return context.Canceled
	}

	// Mark completion.
	var res *forge_value.Result
	if execErr != nil {
		le.WithError(execErr).Warn("execution failed")
		res = forge_value.NewResultWithError(execErr)
	} else {
		le.Info("execution complete")
		res = forge_value.NewResultWithSuccess()
	}
	return markComplete(ctx, ws, objectKey, peerID, res)
}

// markComplete applies the TxComplete transaction to the execution object.
func markComplete(
	ctx context.Context,
	ws world.WorldState,
	objectKey string,
	peerID peer.ID,
	res *forge_value.Result,
) error {
	obj, err := world.MustGetObject(ctx, ws, objectKey)
	if err != nil {
		return errors.Wrap(err, "get execution for completion")
	}
	txd := execution_transaction.NewTxComplete(res)
	_, _, err = obj.ApplyObjectOp(ctx, txd, peerID)
	return errors.Wrap(err, "mark execution complete")
}

// markFailed marks the execution as failed with the given error.
func markFailed(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	objectKey string,
	peerID peer.ID,
	execErr error,
) {
	res := forge_value.NewResultWithError(execErr)
	if err := markComplete(ctx, ws, objectKey, peerID, res); err != nil {
		le.WithError(err).Warn("failed to mark execution as failed")
	}
}

// buildUniqueID derives a deterministic unique ID from peer ID and object key.
func buildUniqueID(peerID peer.ID, objectKey string) string {
	h := blake3.NewDeriveKey("forge/execution/controller: config: unique id")
	_, _ = h.WriteString(peerID.String())
	_, _ = h.WriteString(objectKey)
	hsum := h.Sum(nil)
	var id uuid.UUID
	copy(id[:], hsum)
	return id.String()
}
