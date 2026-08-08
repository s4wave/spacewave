package forge_lib_git_commit

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	resource_git "github.com/s4wave/spacewave/sdk/git/resource"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "forge/lib/git/commit"

const (
	// inputNameWorld is the name of the Input for the target World.
	inputNameWorld = "world"
	// outputNameCommit is the name of the Output for the commit result.
	outputNameCommit = "commit"
)

// Controller implements the git commit controller.
type Controller struct {
	// le is the log entry
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the configuration
	conf *Config
	// inputVals is the input values map
	inputVals forge_target.InputMap
	// handle contains the controller handle
	handle forge_target.ExecControllerHandle
}

// NewController constructs a new git commit controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	return &Controller{
		le:   le,
		bus:  bus,
		conf: conf,
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"git commit controller",
	)
}

// InitForgeExecController initializes the Forge execution controller.
// This is called before Execute().
// Any error returned cancels execution of the controller.
func (c *Controller) InitForgeExecController(
	ctx context.Context,
	inputVals forge_target.InputMap,
	handle forge_target.ExecControllerHandle,
) error {
	c.inputVals, c.handle = inputVals, handle
	return c.conf.Validate()
}

// Execute executes the controller goroutine.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	inWorld := c.inputVals[inputNameWorld]
	if inWorld == nil || inWorld.IsEmpty() {
		return errors.New("target world input must be set")
	}
	ipv, err := forge_target.InputValueToWorld(inWorld)
	if err != nil {
		return errors.Wrap(err, "world")
	}
	if ipv == nil {
		return errors.New("target world input must be set")
	}

	resource := resource_git.NewGitWorktreeResource(
		ipv.GetWorldState(),
		ipv.GetWorldEngine(),
		c.conf.GetWorktreeObjectKey(),
		&resource_git.WorktreeSnapshot{RepoObjectKey: c.conf.GetRepoObjectKey()},
	)
	resp, err := resource.CommitFiles(ctx, c.conf.GetCommitRequest())
	if err != nil {
		return err
	}
	data, err := resp.MarshalVT()
	if err != nil {
		return err
	}
	out, err := forge_target.StoreBlobValueFromBytes(ctx, c.handle, data)
	if err != nil {
		return err
	}
	out.Name = outputNameCommit

	c.le.WithFields(logrus.Fields{
		"commit": resp.GetCommitHash(),
		"base":   resp.GetBaseCommitHash(),
		"branch": resp.GetBranchRef(),
	}).Info("committed staged git worktree files")

	return c.handle.SetOutputs(ctx, forge_value.ValueSlice{out}, true)
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ forge_target.ExecController = (*Controller)(nil)
