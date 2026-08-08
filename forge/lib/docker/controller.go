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
	// le is the log entry
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the configuration
	conf *Config
	// runner executes docker CLI commands
	runner DockerRunner
	// inputVals is the input values map
	inputVals forge_target.InputMap
	// handle contains the controller handle
	handle forge_target.ExecControllerHandle
}

// NewController constructs a docker controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	return &Controller{
		le:     le,
		bus:    bus,
		conf:   conf,
		runner: NewExecDockerRunner(),
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
func (c *Controller) Execute(ctx context.Context) error {
	if c.handle == nil {
		return errors.New("forge exec controller not initialized")
	}
	if err := c.conf.Validate(); err != nil {
		return err
	}

	dockerPath := c.dockerPath()
	dockerEnv := buildDockerEnv(c.conf)

	var containerID string
	defer func() {
		if ctx.Err() == nil || containerID == "" {
			return
		}
		if err := c.stopContainer(context.Background(), dockerPath, dockerEnv, containerID); err != nil {
			c.le.WithError(err).Warn("stop docker container after cancellation")
		}
	}()

	out, err := c.runner.Run(ctx, dockerPath, buildCreateArgs(c.conf), dockerEnv)
	if err != nil {
		return errors.Wrap(err, "docker create")
	}
	containerID = strings.TrimSpace(string(out))
	if containerID == "" {
		return errors.New("docker create returned empty container id")
	}

	if _, err := c.runner.Run(ctx, dockerPath, []string{"start", containerID}, dockerEnv); err != nil {
		return errors.Wrap(err, "docker start")
	}

	out, err = c.runner.Run(ctx, dockerPath, []string{"wait", containerID}, dockerEnv)
	if err != nil {
		if ctx.Err() != nil {
			return context.Canceled
		}
		return errors.Wrap(err, "docker wait")
	}
	if err := ctx.Err(); err != nil {
		return context.Canceled
	}

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

func (c *Controller) dockerPath() string {
	if path := c.conf.GetDockerPath(); path != "" {
		return path
	}
	return "docker"
}

func (c *Controller) stopContainer(ctx context.Context, dockerPath string, dockerEnv []string, containerID string) error {
	_, err := c.runner.Run(ctx, dockerPath, buildStopArgs(c.conf, containerID), dockerEnv)
	return err
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
