package forge_lib_docker

import (
	"bytes"
	"context"
	"os"

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
	var (
		mounts    []*Mount
		outputDir string
	)
	if hd != nil {
		mounts, outputDir = hd.mounts, hd.outputDir
		defer func() {
			if err := os.RemoveAll(hd.root); err != nil {
				c.le.WithError(err).Warn("remove docker host directory")
			}
		}()
	}

	// Debit a deterministic runtime name and launch the container under it.
	// Reserve waits while the Worker admits no work. A drain that fences the
	// launch voids the debit, so release it and reserve again.
	var (
		grant Reservation
		ctr   *Container
	)
	for {
		grant, err = c.admission.Reserve(ctx, c.handle.GetExecutionObjectKey(), c.conf)
		if err != nil {
			return errors.Wrap(err, "reserve docker capacity")
		}
		ctr, err = c.launch(ctx, grant, mounts)
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
	return c.storeOutputs(ctx, outputDir)
}

// launch creates and starts the container under the reservation's runtime name.
func (c *Controller) launch(ctx context.Context, grant Reservation, mounts []*Mount) (*Container, error) {
	var ctr *Container
	err := grant.Launch(ctx, func(name string) error {
		var err error
		ctr, err = CreateContainer(ctx, c.runner, c.conf, name, mounts)
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
