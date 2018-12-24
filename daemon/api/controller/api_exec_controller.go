package api_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	ce "github.com/aperturerobotics/controllerbus/controller/exec"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
)

// executeController executes a controller and calls the callback with state.
func (a *API) executeController(
	ctx context.Context,
	conf config.Config,
	cb func(ce.ControllerStatus),
) error {
	if cb == nil {
		cb = func(ce.ControllerStatus) {}
	}
	dir := resolver.NewLoadControllerWithConfig(conf)

	cb(ce.ControllerStatus_ControllerStatus_CONFIGURING)
	_, valRef, err := bus.ExecOneOff(ctx, a.bus, dir, nil)
	if err != nil {
		return err
	}
	defer valRef.Release()

	cb(ce.ControllerStatus_ControllerStatus_RUNNING)
	<-ctx.Done()
	return nil
}
