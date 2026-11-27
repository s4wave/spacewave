// Package forge_lib_skiffos_update moves the SkiffOS Repo a build consumes to
// the upstream commit an audit reported, as a Forge Step a person approves.
package forge_lib_skiffos_update

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/pkg/errors"
	git_world "github.com/s4wave/spacewave/db/git/world"
	forge_lib_skiffos_audit "github.com/s4wave/spacewave/forge/lib/skiffos/audit"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "forge/lib/skiffos/update"

// InputReport identifies the input holding the audit's report output.
const InputReport = "report"

// InputApprove identifies the input carrying the person's approval, which the
// Step waits for and does not read.
const InputApprove = "approve"

// OutputRepo identifies the output holding the build Repo after the move.
const OutputRepo = "repo"

// inputWorld identifies the input holding the World the build Repo lives in.
const inputWorld = "world"

// buildrootModule is the name SkiffOS gives its Buildroot submodule, whose
// module in the build Repo holds the commits the build checks out there.
const buildrootModule = "buildroot"

// mainBranch is the branch the build Repo's HEAD and its module's HEAD point
// to.
var mainBranch = plumbing.NewBranchReferenceName("main")

// Controller moves one build's SkiffOS Repo.
type Controller struct {
	// le is the log entry.
	le *logrus.Entry
	// conf is the configuration.
	conf *Config
	// inputVals holds the world, report, upstream and buildroot inputs.
	inputVals forge_target.InputMap
	// handle contains the controller handle.
	handle forge_target.ExecControllerHandle
}

// NewController constructs a new SkiffOS update controller.
func NewController(le *logrus.Entry, conf *Config) *Controller {
	return &Controller{le: le, conf: conf}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "SkiffOS update controller")
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

// Execute reads the reported commits, fetches both into the build Repo in one
// World write, and outputs the moved Repo.
func (c *Controller) Execute(ctx context.Context) error {
	// Read the commits the person approved from the report.
	reportVal, err := forge_target.InputValueToValue(c.inputVals[InputReport])
	if err != nil {
		return errors.Wrapf(err, "input %q", InputReport)
	}
	rep, err := forge_lib_skiffos_audit.ReadReport(ctx, c.handle, reportVal)
	if err != nil {
		return err
	}
	skiffos := rep.GetUpstream().GetSkiffos().GetCommit()
	buildroot := rep.GetUpstream().GetBuildroot().GetCommit()
	if skiffos == "" || buildroot == "" {
		return errors.New("report names no upstream commit and Buildroot pin")
	}

	// Resolve the World holding the build Repo.
	ipv, err := forge_target.InputValueToWorld(c.inputVals[inputWorld])
	if err != nil {
		return errors.Wrap(err, "world")
	}

	// Fetch SkiffOS into the Repo and Buildroot into its module.
	ref, _, err := git_world.AccessWorldObjectRepo(ctx, ipv.GetWorldState(), c.conf.GetObjectKey(), true, nil, nil, nil, func(dst *git.Repository) error {
		if err := c.fetch(ctx, dst, forge_lib_skiffos_audit.InputUpstream, skiffos); err != nil {
			return err
		}
		mod, err := openModule(dst, buildrootModule)
		if err != nil {
			return err
		}
		return c.fetch(ctx, mod, forge_lib_skiffos_audit.InputBuildroot, buildroot)
	})
	if err != nil {
		return errors.Wrap(err, c.conf.GetObjectKey())
	}
	c.le.Infof("moved %s to SkiffOS %s, Buildroot %s", c.conf.GetObjectKey(), skiffos, buildroot)

	// Output the moved Repo.
	return c.handle.SetOutputs(ctx, forge_value.ValueSlice{
		forge_value.NewValueWithBucketRef(OutputRepo, ref),
	}, true)
}

// fetch fetches commit from the Repo the named input holds into dst's main
// branch and points dst's HEAD at it.
func (c *Controller) fetch(ctx context.Context, dst *git.Repository, input, commit string) error {
	ref, err := c.inputVals.ObjectRef(input)
	if err != nil {
		return err
	}
	err = git_world.ReadRepo(ctx, c.handle.AccessStorage, ref, func(src *git.Repository) error {
		return git_world.FetchCommit(ctx, dst, src, plumbing.NewHash(commit), mainBranch)
	})
	return errors.Wrapf(err, "input %q", input)
}

// openModule opens repo's module name, creating it when repo lacks it.
func openModule(repo *git.Repository, name string) (*git.Repository, error) {
	// Open the module's storage, then its repository, initializing it if new.
	store, err := repo.Storer.Module(name)
	if err != nil {
		return nil, errors.Wrapf(err, "module %s", name)
	}
	mod, err := git.Open(store, nil)
	if errors.Is(err, git.ErrRepositoryNotExists) {
		mod, err = git.Init(store)
	}
	return mod, errors.Wrapf(err, "module %s", name)
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
var _ forge_target.ExecController = ((*Controller)(nil))
