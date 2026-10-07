package execution_controller

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/backoff"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	execution_transaction "github.com/s4wave/spacewave/forge/execution/tx"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_task "github.com/s4wave/spacewave/forge/task"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

// targetWorldInput is the default input name for the target world.
const targetWorldInput = "world"

// executeWithConfig is the routine to execute the Execution controller.
func (c *Controller) executeWithConfig(rctx context.Context, execConf *ExecConfig) error {
	// Renew the granted claim through its existing World operation.
	claim := execConf.GetExecution().GetClaim()
	lease := NewLease(c.le, claim.GetLeaseExpiresAt().AsTime(), c.claimLease, func(ctx context.Context, expiry time.Time) error {
		return c.applyClaimTx(ctx, execution_transaction.NewTxRenewClaim(claim, expiry), c.peerID)
	})
	return lease.Execute(rctx, func(ctx context.Context) error {
		return c.executeClaimed(ctx, execConf)
	})
}

// executeClaimed executes and settles one target while its Lease retains custody.
func (c *Controller) executeClaimed(claimCtx context.Context, execConf *ExecConfig) error {
	// Interrupt setup and target execution, but retain the routine context for
	// the durable completion after processExec has drained and closed the target.
	ctx, ctxCancel := context.WithCancel(claimCtx)
	defer ctxCancel()
	stopCancel := context.AfterFunc(c.cancelCtx, ctxCancel)
	defer stopCancel()
	if c.cancelCtx.Err() != nil {
		ctxCancel()
	}

	// Keep transport interruptions under the live lease. Reconstructing the
	// target resolves the replacement plugin without completing the Execution.
	var execErr error
	retry := (&backoff.Backoff{}).Construct()
	for {
		// Run and drain each target before deciding whether its stream was interrupted.
		execErr = c.processExec(ctx, execConf)
		if claimCtx.Err() != nil {
			return nil
		}
		if c.cancelCtx.Err() != nil ||
			(!errors.Is(execErr, srpc.ErrReset) && !errors.Is(execErr, srpc.ErrClosedBeforeCompletion)) {
			break
		}

		// Retain claim renewal while waiting to rejoin the plugin service.
		c.le.WithError(execErr).Debug("retrying interrupted execution")
		select {
		case <-ctx.Done():
		case <-time.After(retry.NextBackOff()):
		}
	}

	// Retry the durable completion until it commits. A failed write, such as
	// a full disk, must not leave the Execution running with no routine.
	bo := (&backoff.Backoff{}).Construct()
	for {
		// Settle the target only while this controller retains its claim.
		err := c.completeExecution(claimCtx, execConf, execErr)
		var drainErr *drainError
		if err == nil || errors.As(err, &drainErr) {
			return err
		}
		if execution_transaction.IsClaimFenced(err) {
			return err
		}
		if claimCtx.Err() != nil {
			return nil
		}

		// Retry a transient completion failure within the committed lease.
		c.le.WithError(err).Warn("retrying execution completion")
		select {
		case <-claimCtx.Done():
			return nil
		case <-time.After(bo.NextBackOff()):
		}
	}
}

// drainError reports a canceled execution whose target failed while draining.
// Retrying its completion cannot succeed.
type drainError struct {
	// err is the target's failure while draining a canceled execution.
	err error
}

// Error returns the drain failure message.
func (e *drainError) Error() string {
	return "drain canceled execution: " + e.err.Error()
}

// Unwrap returns the target failure.
func (e *drainError) Unwrap() error {
	return e.err
}

// completeExecution records the execution result in one World transaction.
func (c *Controller) completeExecution(ctx context.Context, execConf *ExecConfig, execErr error) error {
	// Open the write transaction that records the result.
	completeTx, err := c.busEngine.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer completeTx.Discard()

	// Read the durable Execution state.
	exec, execObjState, err := forge_execution.LookupExecution(ctx, completeTx, c.conf.GetObjectKey())
	defer world.ReleaseObjectState(execObjState)
	if err != nil {
		return err
	}

	// The durable state decides cancellation: a cancel committed while the
	// target finished may not have reached CancelWaitCh yet.
	canceling := exec.GetExecutionState() == forge_execution.State_ExecutionState_CANCELING
	if !canceling {
		select {
		case <-c.CancelWaitCh():
			canceling = true
		default:
		}
	}

	// Choose the result from the cancellation state and the target outcome.
	var res *forge_value.Result
	switch {
	case canceling && execErr != nil && !errors.Is(execErr, context.Canceled) && !errors.Is(execErr, srpc.ErrReset):
		return &drainError{err: execErr}
	case canceling:
		c.le.Info("marking execution as canceled after drain")
		res = forge_value.NewResultWithCanceled(errors.New("execution canceled"))
	case execErr != nil:
		c.le.WithError(execErr).Warn("marking execution as failed w/ error")
		res = forge_value.NewResultWithError(execErr)
	default:
		c.le.Info("marking execution as complete")
		res = forge_value.NewResultWithSuccess()
	}

	// Apply the completion under the execution's claim and commit it.
	txd := execution_transaction.NewTxComplete(
		res,
		execConf.GetExecution().GetClaim(),
	)
	if _, _, err := execObjState.ApplyObjectOp(ctx, txd, c.peerID); err != nil {
		return err
	}
	return completeTx.Commit(ctx)
}

// processExec processes the exec portion of the Target config.
//
// Transport interruptions are retried by executeClaimed; handler errors become
// the execution result.
func (c *Controller) processExec(
	ctx context.Context,
	execConf *ExecConfig,
) error {
	// Skip target execution when its controller configuration is disabled or empty.
	tgt := execConf.GetTarget()
	tgtExecConf := tgt.GetExec()
	ctrlConf := tgtExecConf.GetController()
	exState := execConf.GetExecution()
	if tgtExecConf.GetDisable() || ctrlConf.GetId() == "" {
		return nil
	}

	// Bound controller configuration resolution by the configured deadline.
	resolveCtx := ctx
	tgtBus := c.bus
	if c.conf.GetResolveControllerConfigTimeout() != "" {
		dur, err := c.conf.ParseResolveControllerConfigTimeout()
		if err != nil {
			return err
		}
		var cancel func()
		resolveCtx, cancel = context.WithTimeout(resolveCtx, dur)
		defer cancel()
	}

	// Resolve and validate the execution controller configuration.
	cconf, err := ctrlConf.Resolve(resolveCtx, c.bus)
	if err != nil {
		if err == context.Canceled {
			return err
		}
		return errors.Wrap(err, "resolve exec controller config")
	}

	// Validate the resolved controller configuration before loading its factory.
	rCtrlConf := cconf.GetConfig()
	if err := rCtrlConf.Validate(); err != nil {
		return errors.Wrap(err, "validate exec controller config")
	}

	// Confirm the handler permits this controller execution.
	if err := c.CheckExecControllerConfig(ctx, rCtrlConf); err != nil {
		return err
	}

	// Load the factory for the configured controller.
	factoryAv, _, factoryRef, err := bus.ExecOneOff(
		ctx,
		c.bus,
		resolver.NewLoadFactoryByConfig(rCtrlConf),
		nil,
		nil,
	)
	if err != nil {
		return errors.Wrap(err, "load exec controller factory")
	}
	defer factoryRef.Release()

	// Require the factory directive to return a controller constructor.
	fac, facOk := factoryAv.GetValue().(resolver.LoadFactoryByConfigValue)
	if !facOk {
		return errors.New("load exec controller factory returned unexpected type")
	}

	// Construct the execution controller with its logger.
	le := c.le
	ctrl, err := fac.Construct(ctx, rCtrlConf, controller.ConstructOpts{
		Logger: le,
	})
	if err != nil {
		return errors.Wrap(err, "construct exec controller")
	}
	defer ctrl.Close()

	// Resolve the target world input when configured.
	var targetWorld forge_target.InputValueWorld
	if tgtWorldID := c.conf.GetInputWorld().GetEngineId(); tgtWorldID != "" {
		// Resolve the world only when the execution input is not already set.
		v, rel, err := c.conf.GetInputWorld().ResolveValue(ctx, tgtBus)
		if err != nil {
			return err
		}
		if rel != nil {
			defer rel()
		}
		if v != nil && !v.IsEmpty() {
			targetWorld = v
		}
	}

	// Resolve the controller input map against the target world.
	inputsValMap, err := forge_value.
		ValueSlice(exState.GetValueSet().GetInputs()).
		BuildValueMap(true, true)
	if err != nil {
		return err
	}

	// Resolve target inputs and retain their resources through execution.
	inputsMap, inputsUnresolved, inputsRelease, err := forge_target.ResolveInputMap(
		ctx,
		tgtBus,
		targetWorld,
		tgt,
		inputsValMap,
		forge_task.ResolveOutput,
	)
	if err != nil {
		return err
	}
	defer inputsRelease()

	// Reject unresolved inputs before controller execution.
	if len(inputsUnresolved) != 0 {
		inputNames := forge_target.GetInputsNames(inputsUnresolved)
		return errors.Errorf("found %d unset inputs: %s", len(inputNames), inputNames)
	}

	// Compare resolved inputs with the execution snapshot.
	inputValueSet := inputsMap.BuildValueSet()
	var inputSet forge_value.ValueSlice = inputValueSet.GetInputs()
	var exInputSet forge_value.ValueSlice = exState.GetValueSet().GetInputs()
	addedInputs, removedInputs, changedInputs := exInputSet.Compare(inputSet)
	inputsDirty := len(addedInputs)+len(removedInputs)+len(changedInputs) != 0
	if inputsDirty {
		dirtyNames := forge_value.GetValuesNames(addedInputs, removedInputs, changedInputs)
		return errors.Errorf("found %d outdated inputs: %s", len(dirtyNames), dirtyNames)
	}

	// Add the default world input when no explicit value exists.
	if _, targetWorldOk := inputsMap[targetWorldInput]; !targetWorldOk && targetWorld != nil {
		inputsMap[targetWorldInput] = targetWorld
	}

	// Stage the values the Execution writes until the run ends, after its
	// outputs have adopted them.
	stage, err := c.ws.StageWorldState(ctx)
	if err != nil {
		return err
	}
	defer stage.Release()

	// Build the execution handle and pass it to the controller.
	execCtx := forge_target.WithExecCancelSignal(ctx, c.CancelWaitCh())
	execCtrlHandle := newExecControllerHandle(
		execCtx,
		c,
		c.ws,
		stage,
		exState.GetTimestamp(),
		exState.GetClaim().GetEpoch(),
	)
	if execCtrl, execCtrlOk := ctrl.(forge_target.ExecController); execCtrlOk {
		err = execCtrl.InitForgeExecController(
			execCtx,
			inputsMap,
			execCtrlHandle,
		)
	} else if !c.conf.GetAllowNonExecController() {
		return ErrNotExecController
	} else {
		le.Debug("controller does not implement exec-controller interface")
	}
	if ctx.Err() != nil {
		return context.Canceled
	}
	if err != nil {
		if err == context.Canceled {
			return err
		}
		return errors.Wrap(err, "init exec controller")
	}

	return c.executeTargetController(execCtx, tgtBus, ctrl)
}

// executeTargetController runs a resolved target controller through its local
// lifecycle.
func (c *Controller) executeTargetController(
	ctx context.Context,
	tgtBus bus.Bus,
	ctrl controller.Controller,
) error {
	// Execute the target controller and measure its lifetime for failure logging.
	c.le.
		WithField("controller-id", ctrl.GetControllerInfo().Id).
		Info("starting exec controller")
	t1 := time.Now()
	err := tgtBus.ExecuteController(ctx, ctrl)
	durLe := c.le.WithField("exec-dur", time.Since(t1))
	if err != nil {
		if ctx.Err() == nil || !errors.Is(err, context.Canceled) {
			durLe.WithError(err).Warn("exec controller failed")
		}
		return err
	}

	// Report successful target completion with its execution duration.
	durLe.Debug("exec controller completed")
	return nil
}
