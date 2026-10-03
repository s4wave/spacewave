package forge_lib_docker

import (
	"context"
	"strconv"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
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

	// Debit and activate a deterministic runtime name before Docker can create it.
	grant, err := c.admission.Reserve(ctx, c.handle.GetExecutionObjectKey(), c.conf)
	if err != nil {
		return errors.Wrap(err, "reserve docker capacity")
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
	dockerPath := c.dockerPath()
	dockerEnv := BuildDockerEnv(c.conf)
	var containerID string
	if err := grant.Launch(ctx, func(name string) error {
		// Create the container under the reserved runtime name and start it.
		out, err := c.runner.Run(ctx, dockerPath, buildCreateArgs(c.conf, name), dockerEnv)
		if err != nil {
			return errors.Wrap(err, "docker create")
		}
		containerID = strings.TrimSpace(string(out))
		if containerID == "" {
			return errors.New("docker create returned empty container id")
		}
		_, err = c.runner.Run(ctx, dockerPath, []string{"start", containerID}, dockerEnv)
		return errors.Wrap(err, "docker start")
	}); err != nil {
		return err
	}

	// Wait for the container to exit or the execution context to be cancelled.
	out, err := c.runner.Run(ctx, dockerPath, []string{"wait", containerID}, dockerEnv)
	if err != nil {
		if ctx.Err() != nil {
			return context.Canceled
		}
		return errors.Wrap(err, "docker wait")
	}
	if err := ctx.Err(); err != nil {
		return context.Canceled
	}

	// Retain both container output streams before recording the exit status.
	stdout, stderr, err := c.runner.Logs(ctx, dockerPath, containerID, dockerEnv)
	if err != nil {
		return errors.Wrap(err, "docker logs")
	}
	if len(stdout) != 0 {
		if err := c.handle.WriteLog(ctx, "info", string(stdout)); err != nil {
			return errors.Wrap(err, "retain docker stdout")
		}
	}
	if len(stderr) != 0 {
		if err := c.handle.WriteLog(ctx, "error", string(stderr)); err != nil {
			return errors.Wrap(err, "retain docker stderr")
		}
	}

	// Return the container exit status as the execution outcome.
	statusText := strings.TrimSpace(string(out))
	status, err := strconv.Atoi(statusText)
	if err != nil {
		return errors.Wrap(err, "parse docker wait status")
	}
	if status != 0 {
		return errors.Errorf("docker container exited with status %d", status)
	}
	return nil
}

// dockerPath resolves the configured Docker CLI executable.
func (c *Controller) dockerPath() string {
	if path := c.conf.GetDockerPath(); path != "" {
		return path
	}
	return "docker"
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
