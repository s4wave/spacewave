// Package forge_lib_skiffos_audit audits the SkiffOS source a build consumes
// against upstream as a Forge Step, and changes nothing.
package forge_lib_skiffos_audit

import (
	"context"
	"os"
	"path/filepath"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/pkg/errors"
	git_world "github.com/s4wave/spacewave/db/git/world"
	forge_lib_util_inputfs "github.com/s4wave/spacewave/forge/lib/util/inputfs"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "forge/lib/skiffos/audit"

// InputBuild names the input holding the Repo the build consumes.
const InputBuild = "build"

// InputUpstream names the input holding the fetched SkiffOS Repo.
const InputUpstream = "upstream"

// InputBuildroot names the input holding the fetched Buildroot fork's Repo.
const InputBuildroot = "buildroot"

// OutputReport names the output tree holding ReportFile.
const OutputReport = "report"

// ReportFile is the report tree's file holding the Report as JSON.
const ReportFile = "report.json"

// Controller audits one build's SkiffOS source.
type Controller struct {
	// le is the log entry
	le *logrus.Entry
	// conf is the configuration
	conf *Config
	// inputVals holds the build, upstream and buildroot inputs.
	inputVals forge_target.InputMap
	// handle contains the controller handle
	handle forge_target.ExecControllerHandle
}

// NewController constructs a new SkiffOS audit controller.
func NewController(le *logrus.Entry, conf *Config) *Controller {
	return &Controller{le: le, conf: conf}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "SkiffOS audit controller")
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

// Execute reads the three Repos one at a time, each read-only, and outputs
// the Report.
func (c *Controller) Execute(ctx context.Context) error {
	// Read the build's commit and its Buildroot pin.
	rep := &Report{}
	err := c.readRepo(ctx, InputBuild, func(repo *git.Repository) error {
		head, err := repo.Head()
		if err != nil {
			return errors.Wrap(err, "read HEAD")
		}
		rep.Build, err = readRevision(repo, head.Hash())
		return err
	})
	if err != nil {
		return err
	}

	// Read upstream's head, relate the build to it, and describe both commits
	// by SkiffOS's release tags.
	err = c.readRepo(ctx, InputUpstream, func(repo *git.Repository) error {
		// Resolve upstream's head and its Buildroot pin.
		ref, err := repo.Reference(plumbing.ReferenceName(c.conf.GetUpstreamRef()), true)
		if err != nil {
			return errors.Wrap(err, c.conf.GetUpstreamRef())
		}
		if rep.Upstream, err = readRevision(repo, ref.Hash()); err != nil {
			return err
		}

		// Relate the build's commit to the head, then describe both.
		build := plumbing.NewHash(rep.Build.Skiffos.Commit)
		if rep.Freshness, err = readFreshness(repo, build, ref.Hash()); err != nil {
			return err
		}
		_, err = describeAll(repo, releaseTagPrefix, rep.Build.Skiffos, rep.Upstream.Skiffos)
		return err
	})
	if err != nil {
		return err
	}

	// Describe both Buildroot pins by the fork's skiff-* tags, counting them.
	err = c.readRepo(ctx, InputBuildroot, func(repo *git.Repository) error {
		count, err := describeAll(repo, skiffTagPrefix, rep.Build.Buildroot, rep.Upstream.Buildroot)
		rep.SkiffTags = uint32(count) // #nosec G115 -- a count of tags, far fewer than 1<<32
		return err
	})
	if err != nil {
		return err
	}
	return c.storeReport(ctx, rep)
}

// readRepo calls cb with the Repo the named input holds, open only within cb.
func (c *Controller) readRepo(ctx context.Context, name string, cb func(*git.Repository) error) error {
	ref, err := c.inputVals.ObjectRef(name)
	if err != nil {
		return err
	}
	return errors.Wrapf(git_world.ReadRepo(ctx, c.handle.AccessStorage, ref, cb), "input %q", name)
}

// storeReport writes rep as the report output's tree.
func (c *Controller) storeReport(ctx context.Context, rep *Report) error {
	// Write the report file into a tree of its own.
	data, err := rep.MarshalJSON()
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "skiffos-audit-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir) //nolint:errcheck
	if err := os.WriteFile(filepath.Join(dir, ReportFile), data, 0o644); err != nil {
		return err
	}

	// Store the tree as the output.
	ref, err := forge_lib_util_inputfs.StoreDir(ctx, c.handle, dir)
	if err != nil {
		return err
	}
	return c.handle.SetOutputs(ctx, forge_value.ValueSlice{
		forge_value.NewValueWithBlockRef(OutputReport, ref),
	}, true)
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
