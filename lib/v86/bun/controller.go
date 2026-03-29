package forge_lib_v86_bun

import (
	"context"
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	forge_target "github.com/aperturerobotics/forge/target"
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

//go:embed boot.ts
var bootScriptData []byte

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
//  2. Launch a bun subprocess that connects to the socket.
//  3. The bun script boots v86 with v86fs mounts, runs commands, reports exit.
//
// Input resolution (rootfs -> FSHandle) is deferred to a later iteration
// when the full forge pipeline wiring is integrated.
func (c *Controller) Execute(ctx context.Context) error {
	execID := c.handle.GetExecutionUniqueId()

	// Create temp directory for the unix socket and working state.
	tmpDir, err := os.MkdirTemp("", "forge-v86-"+execID+"-")
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

	// Create v86fs relay server.
	// TODO: resolve rootfs input to FSHandle and wire into mount resolver.
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

	socketAddr := lis.Addr().String()
	c.le.WithFields(logrus.Fields{
		"bun":        bunPath,
		"socket":     socketAddr,
		"memory-mb":  memoryMb,
		"output-dir": outputDir,
		"commands":   len(c.conf.GetCommands()),
	}).Debug("launching v86 bun subprocess")

	// Write embedded boot script to tmpDir.
	bootScript := filepath.Join(tmpDir, "boot.ts")
	if err := os.WriteFile(bootScript, bootScriptData, 0o644); err != nil {
		return errors.Wrap(err, "write boot script")
	}

	// Build command arguments.
	args := []string{"run", bootScript,
		"--socket", socketAddr,
		"--memory", strconv.FormatUint(uint64(memoryMb), 10),
		"--output-dir", outputDir,
	}
	for _, cmd := range c.conf.GetCommands() {
		args = append(args, "--cmd", cmd)
	}

	cmd := exec.CommandContext(ctx, bunPath, args...)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "bun subprocess")
	}

	return nil
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
