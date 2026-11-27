package forge_lib_skiffos

import (
	"bytes"
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	forge_lib_util_inputfs "github.com/s4wave/spacewave/forge/lib/util/inputfs"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "forge/lib/skiffos"

// OutputImages is the exec output carrying the boot set.
const OutputImages = "images"

// logTailBytes bounds the build log tail written to the Execution on failure.
const logTailBytes = 4096

// Controller runs one SkiffOS build and stores its boot set.
type Controller struct {
	// le is the log entry.
	le *logrus.Entry
	// conf is the configuration.
	conf *Config
	// inputVals holds the Step inputs, read by name through input_env and
	// extra_config_inputs.
	inputVals forge_target.InputMap
	// handle contains the controller handle.
	handle forge_target.ExecControllerHandle
}

// NewController constructs a new SkiffOS build controller.
func NewController(le *logrus.Entry, conf *Config) *Controller {
	return &Controller{le: le, conf: conf}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "SkiffOS build controller")
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

// Execute builds the workspace and stores its images directory as the
// output. A failed build returns its error, which retries the Execution with
// backoff. The workspace keeps its build tree, so a retry resumes the build.
func (c *Controller) Execute(ctx context.Context) error {
	// Bring the Step's checkout to the source's commit.
	conf := c.conf
	commit, err := c.syncSource(ctx, conf.GetSkiffDir())
	if err != nil {
		_ = c.handle.WriteLog(ctx, "error", err.Error())
		return err
	}

	// Create the workspace, which keeps the build log beside its tree.
	wsDir := filepath.Join(conf.GetSkiffDir(), "workspaces", conf.GetWorkspace())
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		return err
	}
	logPath := filepath.Join(wsDir, "forge-build.log")

	// Announce the build in the host log and the Execution log.
	skiffConfig := strings.Join(conf.GetConfigs(), ",")
	le := c.le.WithField("workspace", conf.GetWorkspace()).WithField("skiff-config", skiffConfig).WithField("skiffos", commit)
	le.WithField("log", logPath).Info("building SkiffOS workspace")
	if err := c.handle.WriteLog(ctx, "info", "building SkiffOS "+commit+" for "+skiffConfig+" in workspace "+conf.GetWorkspace()+", log "+logPath); err != nil {
		return err
	}

	// Write the inputs the build reads into the workspace.
	env := []string{
		"SKIFF_WORKSPACE=" + conf.GetWorkspace(),
		"SKIFF_CONFIG=" + skiffConfig,
		"SKIFF_NO_INTERACTIVE=1",
	}
	inputEnv, err := c.checkoutInputs(ctx, filepath.Join(wsDir, "inputs"))
	if err != nil {
		return err
	}
	env = append(env, inputEnv...)

	// Build the workspace, writing the log tail to the Execution on failure.
	start := time.Now()
	if err := runMake(ctx, conf.GetSkiffDir(), logPath, env, "configure", "compile"); err != nil {
		if ctx.Err() == nil {
			_ = c.handle.WriteLog(ctx, "error", "SkiffOS build failed: "+err.Error()+"\n"+readLogTail(logPath))
		}
		return errors.Wrap(err, "skiffos build")
	}
	dur := time.Since(start).Round(time.Second)
	le.WithField("duration", dur.String()).Info("built SkiffOS workspace")

	// Store the boot set as a UnixFS directory in the World.
	imagesDir := filepath.Join(wsDir, "images")
	root, err := forge_lib_util_inputfs.StoreDir(ctx, c.handle, imagesDir)
	if err != nil {
		return errors.Wrap(err, "store images")
	}
	le.WithField("images", root.MarshalString()).Info("stored SkiffOS boot set")
	if err := c.handle.WriteLog(ctx, "info", "built SkiffOS in "+dur.String()+", boot set "+root.MarshalString()); err != nil {
		return err
	}
	return c.handle.SetOutputs(ctx, forge_value.ValueSlice{
		forge_value.NewValueWithBlockRef(OutputImages, root),
	}, true)
}

// checkoutInputs writes each input the build reads to its directory under
// dir, in name order so the log reads the same each build, and returns the
// variables that point the build at them: one per input_env entry and
// SKIFF_EXTRA_CONFIGS_PATH listing the config inputs.
func (c *Controller) checkoutInputs(ctx context.Context, dir string) ([]string, error) {
	// Check out every input before the build reads any of them.
	inputEnv, configs := c.conf.GetInputEnv(), c.conf.GetExtraConfigInputs()
	names := slices.Concat(slices.Collect(maps.Keys(inputEnv)), configs)
	slices.Sort(names)
	names = slices.Compact(names)
	if err := forge_lib_util_inputfs.CheckoutAll(ctx, c.handle, c.inputVals, dir, names); err != nil {
		return nil, err
	}

	// Point each named variable at its input's directory.
	env := make([]string, 0, len(inputEnv)+1)
	for _, name := range slices.Sorted(maps.Keys(inputEnv)) {
		env = append(env, inputEnv[name]+"="+filepath.Join(dir, name))
	}
	if len(configs) != 0 {
		paths := make([]string, len(configs))
		for i, name := range configs {
			paths[i] = filepath.Join(dir, name)
		}
		env = append(env, "SKIFF_EXTRA_CONFIGS_PATH="+strings.Join(paths, ":"))
	}
	return env, nil
}

// readLogTail returns the end of the build log.
func readLogTail(logPath string) string {
	// Open the log, or return nothing when the build never wrote it.
	f, err := os.Open(logPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	// Seek to the last logTailBytes and read to the end.
	if info, err := f.Stat(); err == nil && info.Size() > logTailBytes {
		if _, err := f.Seek(-logTailBytes, 2); err != nil {
			return ""
		}
	}
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(f)
	return buf.String()
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
