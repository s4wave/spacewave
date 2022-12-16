package plugin_host_process

import (
	"context"
	"os"
	"os/exec"
	"path"

	"github.com/aperturerobotics/bldr/plugin"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	host_controller "github.com/aperturerobotics/bldr/plugin/host/controller"
	"github.com/aperturerobotics/bldr/util/pipesock"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_sync "github.com/aperturerobotics/hydra/unixfs/sync"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ControllerID is the process host controller ID.
const ControllerID = "bldr/plugin/host/process"

// Version is the version of this controller.
var Version = semver.MustParse("0.0.1")

// ProcessHost implements the plugin host with native processes.
type ProcessHost struct {
	// le is the logger
	le *logrus.Entry
	// stateDir is the directory to use for state
	stateDir string
	// binsDir is the directory to use for binaries
	distDir string
	// verboseIO enables verbose io logging
	verboseIO bool
}

// NewProcessHost constructs a new ProcessHost.
func NewProcessHost(le *logrus.Entry, stateDir, distDir string, verboseID bool) (*ProcessHost, error) {
	if _, err := os.Stat(stateDir); err != nil {
		return nil, errors.Wrap(err, "state dir")
	}
	if _, err := os.Stat(distDir); err != nil {
		return nil, errors.Wrap(err, "dist dir")
	}
	return &ProcessHost{
		le:        le,
		stateDir:  stateDir,
		distDir:   distDir,
		verboseIO: verboseID,
	}, nil
}

// NewProcessHostController constructs the ProcessHost and PluginHost controller.
func NewProcessHostController(
	le *logrus.Entry,
	b bus.Bus,
	c *Config,
) (*host_controller.Controller, *ProcessHost, error) {
	if err := c.Validate(); err != nil {
		return nil, nil, err
	}
	stateDir, distDir := c.GetStateDir(), c.GetDistDir()
	processHost, err := NewProcessHost(le, stateDir, distDir, c.GetVerboseIo())
	if err != nil {
		return nil, nil, err
	}
	hctrl := host_controller.NewController(
		le,
		b,
		c.ToControllerConfig(),
		controller.NewInfo(ControllerID, Version, "plugin host with native processes"),
		processHost,
	)
	return hctrl, processHost, nil
}

// ListPlugins lists the set of initialized plugins.
func (h *ProcessHost) ListPlugins(ctx context.Context) ([]string, error) {
	// List the directories in the dist directory.
	dirents, err := os.ReadDir(h.distDir)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, ent := range dirents {
		if !ent.IsDir() {
			continue
		}
		entName := ent.Name()
		if err := plugin.ValidatePluginID(entName); err != nil {
			h.le.Warnf("ignoring unknown directory in plugin bins dir: %s", entName)
			continue
		}
		ids = append(ids, entName)
	}
	return ids, nil
}

// ExecutePlugin executes the plugin with the given ID.
// If the plugin was already initialized, existing state can be reused.
// The plugin should be stopped if/when the function exits.
// Return ErrPluginUninitialized if the plugin was not ready.
// Should expect to be called only once (at a time) for a plugin ID.
// pluginDist contains the plugin distribution files (binaries and assets).
func (h *ProcessHost) ExecutePlugin(
	rctx context.Context,
	pluginID, entrypoint string,
	pluginDist *unixfs.FSHandle,
	rpcInit plugin_host.PluginRpcInitCb,
) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	// double-check the entrypoint exists and is executable
	entrypoint = path.Clean(entrypoint)
	entrypointHandle, err := pluginDist.LookupPath(ctx, entrypoint)
	if err != nil {
		return errors.Wrap(err, "entrypoint")
	}
	entrypointFi, err := entrypointHandle.GetFileInfo(ctx)
	entrypointHandle.Release()
	if err != nil {
		return errors.Wrap(err, "entrypoint")
	}
	entrypointFiMode := entrypointFi.Mode()
	if !entrypointFiMode.IsRegular() {
		return errors.Errorf("entrypoint must be an executable regular file: %s", entrypointFiMode.String())
	}

	// create the plugin bin and state dir
	pluginBinDir := path.Join(h.distDir, pluginID)
	if err := os.MkdirAll(pluginBinDir, 0755); err != nil {
		return err
	}
	pluginStateDir := path.Join(h.stateDir, pluginID)
	if err := os.MkdirAll(pluginStateDir, 0755); err != nil {
		return err
	}

	// checkout the plugin dist unixfs to the disk.
	if err := unixfs_sync.Sync(
		ctx,
		pluginBinDir,
		pluginDist,
		unixfs_sync.DeleteMode_DeleteMode_BEFORE,
		nil,
	); err != nil {
		return err
	}

	// the "embed" io/fs will clear the permissions bits
	// set the executable to chmod +x
	entrypointPath := path.Join(pluginBinDir, entrypoint)
	if err := os.Chmod(entrypointPath, 0755); err != nil {
		return err
	}

	// configure entrypoint process
	entrypointProc := exec.CommandContext(ctx, entrypointPath, "exec-plugin")

	// set pwd to plugin bin dir
	entrypointProc.Dir = pluginBinDir

	// NOTE: the pluginID is validated to be a valid-dns-identifier
	entrypointProc.Env = os.Environ()
	entrypointProc.Env = append(entrypointProc.Env, "BLDR_PLUGIN="+pluginID)

	// stderr: contains any logs
	le := h.le.WithField("plugin-id", pluginID)
	debugWriter := le.WriterLevel(logrus.DebugLevel)
	entrypointProc.Stderr = debugWriter
	// entrypointProc.Stdout = debugWriter

	// attach to pipe
	pipeListener, err := pipesock.BuildPipeListener(le, pluginBinDir, "plugin")
	if err != nil {
		return err
	}
	defer pipeListener.Close()

	le.
		WithField("entrypoint", entrypoint).
		Debugf("executing plugin entrypoint: %s", entrypointProc.String())
	if err := entrypointProc.Start(); err != nil {
		return err
	}

	// execute ipc channel
	go func() {
		// wait for sub-process to connect
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			conn, err := pipeListener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
				default:
					le.WithError(err).Warn("error accepting plugin pipe sock")
					ctxCancel()
				}
				return
			}
			// disable keep alive (unix socket)
			yamuxConf := srpc.NewYamuxConfig()
			yamuxConf.EnableKeepAlive = false
			muxedConn, err := srpc.NewMuxedConn(conn, true, yamuxConf)
			if err != nil {
				le.WithError(err).Warn("error constructing muxed conn for plugin")
				_ = conn.Close()
				continue
			}
			err = h.execPluginIPC(ctx, muxedConn, rpcInit)
			if err != nil && err != context.Canceled {
				le.WithError(err).Warn("plugin ipc exited with error")
			}
		}
	}()

	// wait for a non-nil error
	err = entrypointProc.Wait()
	if err != nil && err != context.Canceled {
		select {
		case <-ctx.Done():
		default:
			le.WithError(err).Warn("plugin exited with error")
		}
	}
	return nil
}

// execPluginIPC executes the plugin stdin/stdout IPC channel.
func (h *ProcessHost) execPluginIPC(ctx context.Context, muxedConn network.MuxedConn, rpcInit plugin_host.PluginRpcInitCb) error {
	defer muxedConn.Close()

	// construct srpc client
	client := srpc.NewClientWithMuxedConn(muxedConn)

	// init rpc
	mux, err := rpcInit(client)
	if err != nil {
		return err
	}

	// construct srpc server & accept incoming requests until an error occurs
	srv := srpc.NewServer(mux)
	return srv.AcceptMuxedConn(ctx, muxedConn)
}

// DeletePlugin clears cached plugin data for the given plugin ID.
func (h *ProcessHost) DeletePlugin(ctx context.Context, pluginID string) error {
	pluginBinDir := path.Join(h.distDir, pluginID)
	e1 := os.RemoveAll(pluginBinDir)
	pluginStateDir := path.Join(h.stateDir, pluginID)
	e2 := os.RemoveAll(pluginStateDir)
	if e1 != nil {
		return e1
	}
	return e2
}

// _ is a type assertion
var _ plugin_host.PluginHost = (*ProcessHost)(nil)
