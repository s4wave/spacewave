package forge_lib_skiffos

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
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
	// le is the log entry
	le *logrus.Entry
	// conf is the configuration
	conf *Config
	// handle contains the controller handle
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
	c.handle = handle
	return c.conf.Validate()
}

// Execute builds the workspace and stores its images directory as the
// output. A failed build returns its error, which retries the Execution with
// backoff; the workspace keeps its build tree, so a retry resumes the build.
func (c *Controller) Execute(ctx context.Context) error {
	// Create the workspace, which keeps the build log beside its tree.
	conf := c.conf
	wsDir := filepath.Join(conf.GetSkiffDir(), "workspaces", conf.GetWorkspace())
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		return err
	}
	logPath := filepath.Join(wsDir, "forge-build.log")

	// Announce the build in the host log and the Execution log.
	skiffConfig := strings.Join(conf.GetConfigs(), ",")
	le := c.le.WithField("workspace", conf.GetWorkspace()).WithField("skiff-config", skiffConfig)
	le.WithField("log", logPath).Info("building SkiffOS workspace")
	if err := c.handle.WriteLog(ctx, "info", "building SkiffOS "+skiffConfig+" in workspace "+conf.GetWorkspace()+", log "+logPath); err != nil {
		return err
	}

	// Build the workspace, writing the log tail to the Execution on failure.
	start := time.Now()
	if err := runMake(ctx, conf.GetSkiffDir(), logPath, []string{
		"SKIFF_WORKSPACE=" + conf.GetWorkspace(),
		"SKIFF_CONFIG=" + skiffConfig,
		"SKIFF_NO_INTERACTIVE=1",
	}, "configure", "compile"); err != nil {
		if ctx.Err() == nil {
			_ = c.handle.WriteLog(ctx, "error", "SkiffOS build failed: "+err.Error()+"\n"+readLogTail(logPath))
		}
		return errors.Wrap(err, "skiffos build")
	}
	dur := time.Since(start).Round(time.Second)
	le.WithField("duration", dur.String()).Info("built SkiffOS workspace")

	// Store the boot set as a UnixFS directory in the World.
	imagesDir := filepath.Join(wsDir, "images")
	root, err := c.storeImages(ctx, imagesDir)
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

// storeImages writes the directory as a UnixFS tree into the World's bucket
// and returns its root. File contents stream through the blob builder.
func (c *Controller) storeImages(ctx context.Context, dir string) (*block.BlockRef, error) {
	// Refuse a build that produced no images directory.
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		if err == nil {
			err = errors.New("not a directory")
		}
		return nil, errors.Wrap(err, dir)
	}

	// Build the tree under the Execution's World stage and write it.
	var root *block.BlockRef
	err := c.handle.AccessStorage(ctx, nil, func(cs *bucket_lookup.Cursor) error {
		// Stream the directory into a new tree, then write its blocks.
		btx, bcs := cs.BuildTransactionAtRef(nil, nil)
		err := unixfs_block.CreateFromFS(ctx, bcs, os.DirFS(dir), c.handle.GetTimestamp())
		if err != nil {
			return err
		}
		root, _, err = btx.Write(ctx, true)
		return err
	})
	return root, err
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
