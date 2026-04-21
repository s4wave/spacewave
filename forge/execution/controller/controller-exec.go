package execution_controller

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	execution_transaction "github.com/s4wave/spacewave/forge/execution/tx"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

// targetWorldInput is the default input name for the target world.
const targetWorldInput = "world"

// executeWithConfig is the routine to execute the Execution controller.
func (c *Controller) executeWithConfig(rctx context.Context, execConf *ExecConfig) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	// process the execution
	execErr := c.processExec(ctx, execConf)

	// ignore error if context canceled
	if cerr := rctx.Err(); cerr != nil {
		return context.Canceled
	}

	// mark the execution as complete
	var res *forge_value.Result
	if execErr != nil {
		c.le.WithError(execErr).Warn("marking execution as failed w/ error")
		res = forge_value.NewResultWithError(execErr)
	} else {
		c.le.Info("marking execution as complete")
		res = forge_value.NewResultWithSuccess()
	}

	// COMPLETE w/ success=true
	completeTx, err := c.busEngine.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer completeTx.Discard()

	execObjState, err := world.MustGetObject(ctx, completeTx, c.conf.GetObjectKey())
	if err != nil {
		return err
	}

	txd := execution_transaction.NewTxComplete(res)
	_, _, err = execObjState.ApplyObjectOp(ctx, txd, c.peerID)
	if err != nil {
		return err
	}

	return completeTx.Commit(ctx)
}

// processExec processes the exec portion of the Target config.
//
// if any error is returned, that error is set as the execution result.
func (c *Controller) processExec(
	ctx context.Context,
	execConf *ExecConfig,
) error {
	tgt := execConf.GetTarget()
	tgtExecConf := tgt.GetExec()
	ctrlConf := tgtExecConf.GetController()
	exState := execConf.GetExecution()
	if tgtExecConf.GetDisable() || ctrlConf.GetId() == "" {
		// skip - configuration is empty
		return nil
	}

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

	// resolve the controller config
	cconf, err := ctrlConf.Resolve(resolveCtx, c.bus)
	if err != nil {
		if err == context.Canceled {
			return err
		}
		return errors.Wrap(err, "resolve exec controller config")
	}

	// rCtrlConf is the typed controllerbus config for the controller
	rCtrlConf := cconf.GetConfig()
	if err := rCtrlConf.Validate(); err != nil {
		return errors.Wrap(err, "validate exec controller config")
	}

	// ask the handler if it's ok to execute this controller
	if err := c.CheckExecControllerConfig(ctx, rCtrlConf); err != nil {
		return err
	}

	// load the factory for the controller
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

	fac, facOk := factoryAv.GetValue().(resolver.LoadFactoryByConfigValue)
	if !facOk {
		return errors.New("load exec controller factory returned unexpected type")
	}

	// construct the controller, using the ExecutionController logger
	le := c.le
	ctrl, err := fac.Construct(ctx, rCtrlConf, controller.ConstructOpts{
		Logger: le,
	})
	if err != nil {
		return errors.Wrap(err, "construct exec controller")
	}

	// lookup the target world engine if applicable
	var targetWorld forge_target.InputValueWorld
	if tgtWorldID := c.conf.GetInputWorld().GetEngineId(); tgtWorldID != "" {
		// only set if there is not an Input set with the id
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

	// build inputs map for passing to controller
	inputsValMap, err := forge_value.
		ValueSlice(exState.GetValueSet().GetInputs()).
		BuildValueMap(true, true)
	if err != nil {
		return err
	}

	inputsMap, inputsUnresolved, inputsRelease, err := forge_target.ResolveInputMap(
		ctx,
		tgtBus,
		targetWorld,
		tgt,
		inputsValMap,
	)
	if err != nil {
		return err
	}
	defer inputsRelease()

	// we expect all inputs to be resolved at this point.
	if len(inputsUnresolved) != 0 {
		inputNames := forge_target.GetInputsNames(inputsUnresolved)
		return errors.Errorf("found %d unset inputs: %s", len(inputNames), inputNames)
	}

	// ensure the inputs match the ValueSet on the Execution.
	inputValueSet := inputsMap.BuildValueSet()

	// compare the value set with the stored inputs
	var inputSet forge_value.ValueSlice = inputValueSet.GetInputs()
	var exInputSet forge_value.ValueSlice = exState.GetValueSet().GetInputs()
	addedInputs, removedInputs, changedInputs := exInputSet.Compare(inputSet)
	inputsDirty := len(addedInputs)+len(removedInputs)+len(changedInputs) != 0
	if inputsDirty {
		dirtyNames := forge_value.GetValuesNames(addedInputs, removedInputs, changedInputs)
		return errors.Errorf("found %d outdated inputs: %s", len(dirtyNames), dirtyNames)
	}

	// set the default "world" input if not already set
	if _, targetWorldOk := inputsMap[targetWorldInput]; !targetWorldOk && targetWorld != nil {
		inputsMap[targetWorldInput] = targetWorld
	}

	// pass handles to the exec controller
	execCtrlHandle := newExecControllerHandle(ctx, c, c.ws, exState.GetTimestamp())
	if execCtrl, execCtrlOk := ctrl.(forge_target.ExecController); execCtrlOk {
		err = execCtrl.InitForgeExecController(
			ctx,
			inputsMap,
			execCtrlHandle,
		)
	} else {
		if !c.conf.GetAllowNonExecController() {
			_ = ctrl.Close()
			return ErrNotExecController
		} else {
			le.Debug("controller does not implement exec-controller interface")
		}
	}
	if ctx.Err() != nil {
		// note: ignore err if context was canceled
		return context.Canceled
	}
	if err != nil {
		if err == context.Canceled {
			return err
		}
		return errors.Wrap(err, "init exec controller")
	}

	// wait for the execution controller to complete
	le.
		WithField("controller-id", ctrl.GetControllerInfo().Id).
		Info("starting exec controller")
	t1 := time.Now()
	err = tgtBus.ExecuteController(ctx, ctrl)
	_ = ctrl.Close()
	t2 := time.Now()
	durLe := le.WithField("exec-dur", t2.Sub(t1))
	if err != nil {
		// this is an error returned by the exec controller itself.
		durLe.WithError(err).Warn("exec controller failed")
		return err
	}

	durLe.Debug("exec controller completed")
	return nil
}
