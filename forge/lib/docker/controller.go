package forge_lib_docker

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	forge_runtime "github.com/s4wave/spacewave/forge/runtime"
	forge_target "github.com/s4wave/spacewave/forge/target"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "forge/lib/docker"

// Controller implements the docker CLI execution controller.
type Controller struct {
	// le reports container cleanup failures.
	le *logrus.Entry
	// bus is the controller bus.
	bus bus.Bus
	// conf configures the container command.
	conf *Config
	// runner executes docker CLI commands.
	runner DockerRunner
	// inputVals is the input values map.
	inputVals forge_target.InputMap
	// handle writes retained execution output.
	handle forge_target.ExecControllerHandle
	// admission reserves and fences Docker runtimes for the owning Worker.
	admission Admission
}

// NewController constructs a docker controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
	admission Admission,
) *Controller {
	return &Controller{
		le:        le,
		bus:       bus,
		conf:      conf,
		runner:    NewExecDockerRunner(),
		admission: admission,
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"docker CLI execution controller",
	)
}

// InitForgeExecController initializes the Forge execution controller.
func (c *Controller) InitForgeExecController(
	ctx context.Context,
	inputVals forge_target.InputMap,
	handle forge_target.ExecControllerHandle,
) error {
	c.inputVals, c.handle = inputVals, handle
	return c.conf.Validate()
}

// Execute executes the docker container lifecycle.
func (c *Controller) Execute(ctx context.Context) (retErr error) {
	// Require the execution handle, container configuration, and runtime capacity service.
	if c.handle == nil {
		return errors.New("forge exec controller not initialized")
	}
	if err := c.conf.Validate(); err != nil {
		return err
	}
	if c.admission == nil {
		return errors.New("docker runtime admission unavailable")
	}

	// Prepare the host directories for the output and input mounts.
	hd, err := c.prepareHostDir(ctx)
	if err != nil {
		return errors.Wrap(err, "prepare docker host directory")
	}
	defer hd.remove(c.le)

	// Debit a deterministic runtime name and launch the container under it.
	// Reserve waits while the Worker admits no work or its capacity is held,
	// reporting the capacity wait on the Execution. A drain that fences the
	// launch voids the debit, so release it and reserve again.
	var (
		grant Reservation
		ctr   *Container
	)
	for {
		grant, err = c.admission.Reserve(ctx, c.handle.GetExecutionObjectKey(), c.conf, c.handle.SetWaiting)
		if err != nil {
			return errors.Wrap(err, "reserve docker capacity")
		}
		ctr, err = c.launch(ctx, grant, hd)
		if !errors.Is(err, forge_runtime.ErrCapacityDraining) {
			break
		}
		if err := grant.Release(context.WithoutCancel(ctx)); err != nil {
			return errors.Wrap(err, "release docker capacity")
		}
	}
	defer func() {
		if err := grant.Release(context.WithoutCancel(ctx)); err != nil {
			if retErr == nil {
				retErr = errors.Wrap(err, "release docker capacity")
			} else {
				c.le.WithError(err).Warn("release docker capacity")
			}
		}
	}()
	if err != nil {
		return err
	}

	// Wait for the container to exit or the execution context to be cancelled.
	status, err := ctr.Wait(ctx)
	if ctx.Err() != nil {
		return context.Canceled
	}
	if err != nil {
		return err
	}

	// Retain both container output streams before recording the exit status.
	var stdout, stderr bytes.Buffer
	if err := ctr.Logs(ctx, false, &stdout, &stderr); err != nil {
		return err
	}
	if stdout.Len() != 0 {
		if err := c.handle.WriteLog(ctx, "info", stdout.String()); err != nil {
			return errors.Wrap(err, "retain docker stdout")
		}
	}
	if stderr.Len() != 0 {
		if err := c.handle.WriteLog(ctx, "error", stderr.String()); err != nil {
			return errors.Wrap(err, "retain docker stderr")
		}
	}

	// Fail on a nonzero exit status, then publish the declared outputs.
	if status != 0 {
		return errors.Errorf("docker container exited with status %d", status)
	}
	return c.storeOutputs(ctx, hd.outputDir)
}

// launch creates and starts the container under the reservation's runtime
// name, with the reserved cards' device nodes mapped. A container that writes
// outputs runs as the Worker's user, with each mapped device's group added.
func (c *Controller) launch(ctx context.Context, grant Reservation, hd *hostDir) (*Container, error) {
	// Map the reserved cards beside the configured devices.
	conf := c.conf
	if cards := grant.Devices(); len(cards) != 0 {
		conf = conf.CloneVT()
		conf.Devices = append(conf.Devices, cards...)
	}

	// Run as the Worker's user when the container writes outputs.
	var user *ContainerUser
	if hd.outputDir != "" {
		var err error
		if user, err = workerUser(conf.GetDevices()); err != nil {
			return nil, err
		}
	}

	// Create and start the container under the reservation's fence.
	var ctr *Container
	err := grant.Launch(ctx, func(name string) error {
		var err error
		ctr, err = CreateContainer(ctx, c.runner, conf, name, hd.mounts, user)
		if err != nil {
			return err
		}
		return ctr.Start(ctx)
	})
	return ctr, err
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ forge_target.ExecController = (*Controller)(nil)
