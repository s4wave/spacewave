package execution_controller

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	forge_target "github.com/aperturerobotics/forge/target"
	"github.com/pkg/errors"
)

// processExec processes the exec portion of the Target config.
func (c *Controller) processExec(ctx context.Context, t *forge_target.Target) error {
	var ctxCancel func()
	ctx, ctxCancel = context.WithCancel(ctx)
	defer ctxCancel()

	execConf := t.GetExec()
	ctrlConf := execConf.GetController()
	if execConf.GetDisable() || ctrlConf.GetId() == "" {
		// skip - configuration is empty
		return nil
	}

	resolveCtx := ctx
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
	factoryAv, factoryRef, err := bus.ExecOneOff(
		ctx,
		c.bus,
		resolver.NewLoadFactoryByConfig(rCtrlConf),
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
	ctrl, err := fac.Construct(rCtrlConf, controller.ConstructOpts{
		Logger: le,
	})
	if err != nil {
		return errors.Wrap(err, "construct exec controller")
	}

	// pass handles to the exec controller
	execCtrlHandle := newExecControllerHandle(c)
	if execCtrl, execCtrlOk := ctrl.(forge_target.ExecController); execCtrlOk {
		err = execCtrl.InitForgeExecController(ctx, execCtrlHandle)
	} else {
		if !c.conf.GetAllowNonExecController() {
			_ = ctrl.Close()
			return ErrNotExecController
		} else {
			le.Debug("controller does not implement exec-controller interface")
		}
	}

	select {
	case <-ctx.Done():
		// note: ignore err if context was canceled
		return context.Canceled
	default:
	}
	if err != nil {
		return errors.Wrap(err, "init exec controller")
	}

	// wait for the execution controller to complete
	le.Info("starting exec controller")
	t1 := time.Now()
	err = c.bus.ExecuteController(ctx, ctrl)
	_ = ctrl.Close()
	t2 := time.Now()
	durLe := le.WithField("exec-dur", t2.Sub(t1))
	if err != nil {
		// this is an error returned by the exec controller itself.
		durLe.WithError(err).Warn("exec controller failed")
		return err
	}
	durLe.Info("exec controller completed")

	return nil
}
