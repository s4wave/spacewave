package forge_lib_v86_bun

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	forge_target "github.com/aperturerobotics/forge/target"
	forge_value "github.com/aperturerobotics/forge/value"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	unixfs_block_fs "github.com/aperturerobotics/hydra/unixfs/block/fs"
	unixfs_tar "github.com/aperturerobotics/hydra/unixfs/tar"
	v86fs "github.com/aperturerobotics/hydra/unixfs/v86fs"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/autobun"
	"github.com/aperturerobotics/util/pipesock"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "forge/lib/v86/bun"

// Controller implements the v86 bun subprocess execution controller.
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

// NewController constructs a new v86 bun controller.
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
		"v86 bun subprocess controller",
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

// Execute executes the controller goroutine.
//
// Architecture:
//  1. Create a unix socket (pipesock) and serve v86fs SRPC on it.
//  2. Create a writable output mount backed by a block transaction.
//  3. Launch a bun subprocess that connects to the socket.
//  4. The bun script boots v86 with v86fs mounts, runs commands, reports exit.
//  5. Extract the output BlockRef and set it as the forge task output.
func (c *Controller) Execute(ctx context.Context) error {
	// Create temp directory for the unix socket and working state.
	// Use a short prefix to stay within the unix socket path length limit (104 on macOS).
	tmpDir, err := os.MkdirTemp("", "fv86-")
	if err != nil {
		return errors.Wrap(err, "create temp dir")
	}
	defer os.RemoveAll(tmpDir)

	// Start v86fs SRPC server on a unix socket.
	lis, err := pipesock.BuildPipeListener(c.le, tmpDir, "v86fs")
	if err != nil {
		return errors.Wrap(err, "build pipe listener")
	}
	defer lis.Close()

	// Create v86fs relay server (mounts added dynamically below).
	v86fsSrv := v86fs.NewServer(nil)

	mux := srpc.NewMux()
	if err := v86fs.SRPCRegisterV86FsService(mux, v86fsSrv); err != nil {
		return errors.Wrap(err, "register v86fs service")
	}
	server := srpc.NewServer(mux)

	// Accept SRPC connections in background.
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				c.le.WithError(err).Warn("accept error")
				return
			}
			mp, err := srpc.NewMuxedConn(conn, false, nil)
			if err != nil {
				c.le.WithError(err).Warn("muxed conn error")
				conn.Close()
				continue
			}
			go func() {
				_ = server.AcceptMuxedConn(ctx, mp)
			}()
		}
	}()

	// Resolve bun version and state directory.
	bunVersion := c.conf.GetBunVersion()
	if bunVersion == "" {
		bunVersion = autobun.DefaultBunVersion
	}
	stateDir := c.conf.GetStateDir()
	if stateDir == "" {
		stateDir = filepath.Join(tmpDir, "bun")
	}

	bunPath, err := autobun.EnsureBun(ctx, c.le, stateDir, bunVersion)
	if err != nil {
		return errors.Wrap(err, "ensure bun")
	}

	memoryMb := c.conf.GetMemoryMb()
	if memoryMb == 0 {
		memoryMb = 256
	}

	outputDir := c.conf.GetOutputDir()
	if outputDir == "" {
		outputDir = "/output"
	}

	// Load rootfs tar as v86fs root mount if configured.
	rootfsTarPath := c.conf.GetRootfsTarPath()
	if rootfsTarPath != "" {
		rootfsFile, err := os.Open(rootfsTarPath)
		if err != nil {
			return errors.Wrap(err, "open rootfs tar")
		}
		defer rootfsFile.Close()

		rootfsCursor, err := unixfs_tar.NewTarFSCursorFromReader(rootfsFile)
		if err != nil {
			return errors.Wrap(err, "parse rootfs tar")
		}
		defer rootfsCursor.Release()

		rootfsHandle, err := unixfs.NewFSHandle(rootfsCursor)
		if err != nil {
			return errors.Wrap(err, "create rootfs handle")
		}
		defer rootfsHandle.Release()

		// Empty name = v86fs root mount.
		v86fsSrv.AddMount("", "/", rootfsHandle)
		c.le.WithField("rootfs-tar", rootfsTarPath).Debug("added rootfs mount from tar")
	}

	// Access storage to create mounts and run the VM.
	// The entire subprocess execution happens inside the callback because
	// the bucket_lookup.Cursor is only valid within its scope.
	return c.handle.AccessStorage(ctx, nil, func(cs *bucket_lookup.Cursor) error {
		// Initialize an empty directory as the output root.
		outputHandle, err := initOutputMount(ctx, cs)
		if err != nil {
			return errors.Wrap(err, "init output mount")
		}
		defer outputHandle.Release()

		// Add output mount to v86fs server.
		v86fsSrv.AddMount("output", outputDir, outputHandle)

		// Resolve input mounts from config and add to v86fs server.
		// Each mount maps a guest path to a forge Input name providing a UnixFS tree.
		var inputHandles []*unixfs.FSHandle
		defer func() {
			for _, h := range inputHandles {
				h.Release()
			}
		}()

		for guestPath, inputName := range c.conf.GetMounts() {
			inputHandle, err := c.resolveInputMount(ctx, cs, inputName)
			if err != nil {
				return errors.Wrapf(err, "resolve input mount %s", inputName)
			}
			inputHandles = append(inputHandles, inputHandle)

			// Use the input name as the v86fs mount name.
			v86fsSrv.AddMount(inputName, guestPath, inputHandle)
			c.le.WithFields(logrus.Fields{
				"name": inputName,
				"path": guestPath,
			}).Debug("added input mount")
		}

		socketAddr := lis.Addr().String()

		// Resolve boot script path.
		scriptDir := c.conf.GetScriptDir()
		bootScript := filepath.Join(scriptDir, "boot.ts")
		if scriptDir == "" {
			return errors.New("script_dir must be set (directory containing boot.ts)")
		}
		if _, err := os.Stat(bootScript); err != nil {
			return errors.Wrap(err, "boot script not found")
		}

		c.le.WithFields(logrus.Fields{
			"bun":        bunPath,
			"socket":     socketAddr,
			"script-dir": scriptDir,
			"memory-mb":  memoryMb,
			"output-dir": outputDir,
			"commands":   len(c.conf.GetCommands()),
			"mounts":     len(c.conf.GetMounts()),
		}).Debug("launching v86 bun subprocess")

		// Build command arguments.
		args := []string{"run", bootScript,
			"--socket", socketAddr,
			"--memory", strconv.FormatUint(uint64(memoryMb), 10),
			"--output-dir", outputDir,
			"--mount", "output=" + outputDir,
		}
		for guestPath, inputName := range c.conf.GetMounts() {
			args = append(args, "--mount", inputName+"="+guestPath)
		}
		for _, cmd := range c.conf.GetCommands() {
			args = append(args, "--cmd", cmd)
		}

		cmd := exec.CommandContext(ctx, bunPath, args...)
		cmd.Dir = scriptDir
		cmd.Env = os.Environ()
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return errors.Wrap(err, "bun subprocess")
		}

		// Extract the output BlockRef after the VM has written to the mount.
		outputRef := cs.GetRefWithOpArgs()
		if outputRef != nil && !outputRef.GetRootRef().GetEmpty() {
			outps := forge_value.ValueSlice{
				forge_value.NewValueWithBucketRef("output", outputRef),
			}
			return c.handle.SetOutputs(ctx, outps, true)
		}

		return nil
	})
}

// resolveInputMount resolves a forge input to a read-only FSHandle.
// Clones the cursor, points it at the input's root ref, and creates a
// read-only FS (no FSWriter).
func (c *Controller) resolveInputMount(
	ctx context.Context,
	cs *bucket_lookup.Cursor,
	inputName string,
) (*unixfs.FSHandle, error) {
	// Look up the input value.
	iv, ok := c.inputVals[inputName]
	if !ok {
		return nil, errors.Errorf("input %q not found in input values", inputName)
	}
	val, err := forge_target.InputValueToValue(iv)
	if err != nil {
		return nil, errors.Wrap(err, "resolve input value")
	}
	if val == nil || val.IsEmpty() {
		return nil, errors.Errorf("input %q is empty", inputName)
	}

	// Get the BucketRef from the value.
	bref, err := val.ToBucketRef()
	if err != nil {
		return nil, errors.Wrap(err, "get bucket ref")
	}
	rootRef := bref.GetRootRef()
	if rootRef.GetEmpty() {
		return nil, errors.Errorf("input %q has empty root ref", inputName)
	}

	// Clone the cursor and point at the input's root.
	inputCs := cs.Clone()
	inputCs.SetRootRef(rootRef)

	// Create read-only FS (nil writer).
	fs := unixfs_block_fs.NewFS(
		ctx,
		unixfs_block.NodeType_NodeType_DIRECTORY,
		inputCs,
		nil,
	)

	handle, err := unixfs.NewFSHandle(fs)
	if err != nil {
		fs.Release()
		return nil, errors.Wrap(err, "create fshandle")
	}

	return handle, nil
}

// initOutputMount creates a writable FSHandle backed by a block transaction.
// Seeds an empty directory as the root, creates FS + FSWriter, and returns
// the FSHandle ready for mounting on a v86fs server.
func initOutputMount(ctx context.Context, cs *bucket_lookup.Cursor) (*unixfs.FSHandle, error) {
	// Create a block transaction and seed an empty directory root.
	btx, bcs := cs.BuildTransaction(nil)
	bcs.SetBlock(unixfs_block.NewFSNode(unixfs_block.NodeType_NodeType_DIRECTORY, 0, nil), true)
	if _, err := unixfs_block.NewFSTree(ctx, bcs, unixfs_block.NodeType_NodeType_DIRECTORY); err != nil {
		return nil, errors.Wrap(err, "create root fstree")
	}
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		return nil, errors.Wrap(err, "write root block")
	}
	cs.SetRootRef(rootRef)

	// Create FS with FSWriter for writable access.
	wr := unixfs_block_fs.NewFSWriter()
	fs := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, cs, wr)
	wr.SetFS(fs)

	handle, err := unixfs.NewFSHandle(fs)
	if err != nil {
		fs.Release()
		return nil, errors.Wrap(err, "create fshandle")
	}

	return handle, nil
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
