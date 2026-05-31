//go:build !js

package plugin_host_process

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/pipesock"
	"github.com/pkg/errors"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	host_controller "github.com/s4wave/spacewave/bldr/plugin/host/controller"
	"github.com/s4wave/spacewave/bldr/util/tailwriter"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	"github.com/s4wave/spacewave/net/util/randstring"
	"github.com/sirupsen/logrus"
)

const hostExecutableDirEnv = "BLDR_PLUGIN_HOST_EXECUTABLE_DIR"

// Controller is the plugin host controller tytpe.
type Controller = host_controller.Controller

// ProcessHost implements the plugin host with native processes.
type ProcessHost struct {
	// le is the logger
	le *logrus.Entry
	// stateDir is the directory to use for state
	stateDir string
	// binsDir is the directory to use for binaries
	distDir string
	// pluginPlatformID is the plugin platform to use
	pluginPlatformID string
	// packageStatusMtx guards packageStatus.
	packageStatusMtx sync.Mutex
	// packageStatus stores read-only dist materialization facts by plugin ID.
	packageStatus map[string]PluginPackageStatus
	// packageStatusCtr publishes packageStatus snapshots.
	packageStatusCtr *ccontainer.CContainer[*PluginPackageStatusSnapshot]
}

// PluginPackageStatus describes native dist materialization state for one
// plugin. It deliberately excludes plugin state-dir contents.
type PluginPackageStatus struct {
	PluginID     string
	DistDir      string
	Materialized bool
	Invalidated  bool
	LastAction   string
	LastError    string
	UpdatedAt    time.Time
}

// PluginPackageStatusSnapshot is the read-only native package recovery status
// snapshot.
type PluginPackageStatusSnapshot struct {
	Packages []PluginPackageStatus
}

// NewProcessHost constructs a new ProcessHost.
func NewProcessHost(le *logrus.Entry, stateDir, distDir string) (*ProcessHost, error) {
	if _, err := os.Stat(stateDir); err != nil {
		return nil, errors.Wrap(err, "state dir")
	}
	if _, err := os.Stat(distDir); err != nil {
		return nil, errors.Wrap(err, "dist dir")
	}

	// determine the platform id for the host
	platformID := (&bldr_platform.NativePlatform{}).GetPlatformID()
	return &ProcessHost{
		le:               le,
		stateDir:         stateDir,
		distDir:          distDir,
		pluginPlatformID: platformID,
		packageStatus:    make(map[string]PluginPackageStatus),
		packageStatusCtr: ccontainer.NewCContainerWithEqual(nil, pluginPackageStatusSnapshotEqual),
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
	processHost, err := NewProcessHost(le, stateDir, distDir)
	if err != nil {
		return nil, nil, err
	}
	hctrl := host_controller.NewController(
		le,
		b,
		controller.NewInfo(ControllerID, Version, "plugin host with native processes"),
		processHost,
	)
	return hctrl, processHost, nil
}

// GetPlatformId returns the plugin platform ID for this host.
func (h *ProcessHost) GetPlatformId() string {
	return h.pluginPlatformID
}

// Execute is a stub as the process host does not need a global management goroutine.
func (h *ProcessHost) Execute(ctx context.Context) error {
	return nil
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
		if err := bldr_plugin.ValidatePluginID(entName, false); err != nil {
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
	pluginID, instanceKey, entrypoint string,
	pluginDist, pluginAssets *unixfs.FSHandle,
	hostMux srpc.Mux,
	rpcInit plugin_host.PluginRpcInitCb,
) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	// double-check the entrypoint exists and is executable
	entrypoint = filepath.Clean(entrypoint)
	le := h.le.WithField("plugin-id", pluginID)
	le.
		WithField("entrypoint", entrypoint).
		Debug("looking up native plugin entrypoint")
	entrypointHandle, _, err := pluginDist.LookupPath(ctx, entrypoint)
	if err != nil {
		return errors.Wrap(err, "entrypoint")
	}
	le.
		WithField("entrypoint", entrypoint).
		Debug("native plugin entrypoint lookup complete")
	le.
		WithField("entrypoint", entrypoint).
		Debug("reading native plugin entrypoint file info")
	entrypointFi, err := entrypointHandle.GetFileInfo(ctx)
	entrypointHandle.Release()
	if err != nil {
		return errors.Wrap(err, "entrypoint")
	}
	le.
		WithField("entrypoint", entrypoint).
		WithField("mode", entrypointFi.Mode().String()).
		Debug("native plugin entrypoint file info ready")
	entrypointFiMode := entrypointFi.Mode()
	if !entrypointFiMode.IsRegular() {
		return errors.Errorf("entrypoint must be an executable regular file: %s", entrypointFiMode.String())
	}

	pluginStateDir, err := h.ensurePluginStateDir(pluginID)
	if err != nil {
		return err
	}

	pluginDistDir, err := h.syncPluginDist(ctx, pluginID, pluginDist)
	if err != nil {
		return err
	}

	// the "embed" io/fs will clear the permissions bits
	// set the executable to chmod +x
	entrypointPath := filepath.Join(pluginDistDir, entrypoint)
	le.
		WithField("entrypoint-path", entrypointPath).
		Debug("setting native plugin entrypoint executable bit")
	if err := os.Chmod(entrypointPath, 0o755); err != nil {
		return err
	}

	// configure entrypoint process
	entrypointProc := exec.CommandContext(ctx, entrypointPath, "exec-plugin")

	// set pwd to plugin bin dir
	entrypointProc.Dir = pluginDistDir

	// create unique plugin instance id
	pluginInstanceID := randstring.RandomIdentifier(0)
	pluginStartInfo := bldr_plugin.NewPluginStartInfo(pluginInstanceID, pluginID, instanceKey)
	pluginStartInfoJsonB64, err := pluginStartInfo.MarshalJsonBase64()
	if err != nil {
		return err
	}

	// NOTE: the pluginID is validated to be a valid-dns-identifier
	entrypointProc.Env = append(
		os.Environ(),
		"BLDR_PLUGIN_START_INFO="+pluginStartInfoJsonB64,
		"BLDR_PLUGIN_STATE_PATH="+pluginStateDir,
	)
	if exe, err := os.Executable(); err == nil {
		entrypointProc.Env = append(entrypointProc.Env, hostExecutableDirEnv+"="+filepath.Dir(exe))
	}

	// write start info to a file as well
	instanceDetailsPath := filepath.Join(pluginDistDir, ".plugin-start-info")
	if err := os.WriteFile(instanceDetailsPath, []byte(pluginStartInfoJsonB64), 0o600); err != nil {
		return err
	}

	// stderr: pipe to debug log and capture last lines for error reporting.
	debugWriter := le.WriterLevel(logrus.DebugLevel)
	stderrTail := tailwriter.New(debugWriter, 20)
	entrypointProc.Stderr = stderrTail
	// entrypointProc.Stdout = debugWriter

	// call any os-specific pre-start adjustment
	preStartObj, err := preStartCmd(entrypointProc)
	if err != nil {
		return err
	}

	// attach to pipe
	pipeListener, err := pipesock.BuildPipeListener(le, pluginDistDir, pluginInstanceID)
	if err != nil {
		return err
	}
	defer pipeListener.Close()

	le.
		WithField("entrypoint", entrypoint).
		Debugf("executing plugin entrypoint: %s", entrypointProc.String())

	startObj, err := startCmd(entrypointProc, preStartObj)
	if err != nil {
		return err
	}

	// execute ipc channel
	errCh := make(chan error, 5)
	go func() {
		// wait for sub-process to connect
		for {
			if ctx.Err() != nil {
				return
			}

			conn, err := pipeListener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
				default:
					le.WithError(err).Warn("error accepting plugin pipe sock")
					errCh <- err
				}
				return
			}
			// disable keep alive (unix socket)
			yamuxConf := srpc.NewYamuxConfig()
			yamuxConf.EnableKeepAlive = false

			// construct mplex
			muxedConn, err := srpc.NewMuxedConn(conn, true, yamuxConf)
			if err != nil {
				le.WithError(err).Warn("error constructing muxed conn for plugin")
				_ = conn.Close()
				continue
			}
			err = h.execPluginIPC(ctx, muxedConn, hostMux, rpcInit)
			_ = rpcInit(nil)
			if err != nil && err != context.Canceled && err != io.EOF {
				le.WithError(err).Warn("plugin ipc exited with error")
			}
			_ = muxedConn.Close()
		}
	}()

	// wait for a non-nil error
	exited := make(chan struct{})
	go func() {
		errCh <- entrypointProc.Wait()
		close(exited)
	}()

	// fully kill & wait for exit to be confirmed when returning
	defer func() {
		ctxCancel()
		_ = pipeListener.Close()

		_ = shutdownCmd(entrypointProc, preStartObj, startObj)

		// wait graceful shutdown max duration
		shutdownTimeout := time.NewTimer(time.Second * 3)
		select {
		case <-exited:
			_ = shutdownTimeout.Stop()
		case <-shutdownTimeout.C:
		}

		_ = killCmd(entrypointProc, preStartObj, startObj)

		// wait for full shutdown
		<-exited
	}()

	// wait for context canceled and/or error
	select {
	case <-ctx.Done():
		return context.Canceled
	case err := <-errCh:
		if err != nil {
			if lines := stderrTail.Lines(); len(lines) > 0 {
				le.WithError(err).Error("plugin stderr (last lines):")
				for _, line := range lines {
					le.Error("  | " + line)
				}
			}
		}
		return err
	}
}

// execPluginIPC executes the plugin IPC channel.
func (h *ProcessHost) execPluginIPC(
	ctx context.Context,
	muxedConn srpc.MuxedConn,
	hostMux srpc.Mux,
	rpcInit plugin_host.PluginRpcInitCb,
) error {
	defer muxedConn.Close()

	// construct srpc client
	client := srpc.NewClientWithMuxedConn(muxedConn)

	// init rpc
	err := rpcInit(client)
	if err != nil {
		return err
	}

	// construct srpc server & accept incoming requests until an error occurs
	srv := srpc.NewServer(hostMux)
	return srv.AcceptMuxedConn(ctx, muxedConn)
}

func (h *ProcessHost) pluginDistDir(pluginID string) string {
	return filepath.Join(h.distDir, pluginID)
}

func (h *ProcessHost) pluginStateDir(pluginID string) string {
	return filepath.Join(h.stateDir, pluginID)
}

func (h *ProcessHost) ensurePluginStateDir(pluginID string) (string, error) {
	pluginStateDir := h.pluginStateDir(pluginID)
	if err := os.MkdirAll(pluginStateDir, 0o755); err != nil {
		return "", err
	}
	return pluginStateDir, nil
}

func (h *ProcessHost) syncPluginDist(ctx context.Context, pluginID string, pluginDist *unixfs.FSHandle) (string, error) {
	pluginDistDir := h.pluginDistDir(pluginID)
	if err := os.MkdirAll(pluginDistDir, 0o755); err != nil {
		h.recordPluginPackageStatus(pluginID, pluginDistDir, false, false, "sync", err)
		return "", err
	}

	h.le.
		WithField("plugin-id", pluginID).
		WithField("dist-dir", pluginDistDir).
		Debug("syncing native plugin dist to disk")
	if err := unixfs_sync.Sync(
		ctx,
		pluginDistDir,
		pluginDist,
		unixfs_sync.DeleteMode_DeleteMode_BEFORE,
		nil,
	); err != nil {
		h.recordPluginPackageStatus(pluginID, pluginDistDir, false, false, "sync", err)
		return "", err
	}
	h.le.
		WithField("plugin-id", pluginID).
		WithField("dist-dir", pluginDistDir).
		Debug("native plugin dist sync complete")
	h.recordPluginPackageStatus(pluginID, pluginDistDir, true, false, "sync", nil)
	return pluginDistDir, nil
}

// InvalidatePluginDist clears only the derived native plugin dist checkout.
// Plugin-owned state under the state directory is a separate protected surface.
func (h *ProcessHost) InvalidatePluginDist(ctx context.Context, pluginID string) error {
	if err := ctx.Err(); err != nil {
		h.recordPluginPackageStatus(pluginID, h.pluginDistDir(pluginID), false, false, "invalidate", err)
		return err
	}
	if err := bldr_plugin.ValidatePluginID(pluginID, false); err != nil {
		h.recordPluginPackageStatus(pluginID, h.pluginDistDir(pluginID), false, false, "invalidate", err)
		return err
	}
	pluginDistDir := h.pluginDistDir(pluginID)
	err := os.RemoveAll(pluginDistDir)
	h.recordPluginPackageStatus(pluginID, pluginDistDir, false, err == nil, "invalidate", err)
	return err
}

// PackageStatusSnapshot returns a read-only copy of native dist
// materialization status. It never inspects or exposes plugin-owned state.
func (h *ProcessHost) PackageStatusSnapshot() []PluginPackageStatus {
	h.packageStatusMtx.Lock()
	defer h.packageStatusMtx.Unlock()
	return h.packageStatusSnapshotLocked()
}

// GetPackageStatusCtr returns native package recovery status changes.
func (h *ProcessHost) GetPackageStatusCtr() ccontainer.Watchable[*PluginPackageStatusSnapshot] {
	return h.packageStatusCtr
}

func (h *ProcessHost) recordPluginPackageStatus(
	pluginID,
	distDir string,
	materialized,
	invalidated bool,
	action string,
	err error,
) {
	h.packageStatusMtx.Lock()
	if h.packageStatus == nil {
		h.packageStatus = make(map[string]PluginPackageStatus)
	}
	status := PluginPackageStatus{
		PluginID:     pluginID,
		DistDir:      distDir,
		Materialized: materialized,
		Invalidated:  invalidated,
		LastAction:   action,
		UpdatedAt:    time.Now().UTC(),
	}
	if err != nil {
		status.LastError = err.Error()
	}
	h.packageStatus[pluginID] = status
	snapshot := h.packageStatusSnapshotLocked()
	h.packageStatusMtx.Unlock()
	if h.packageStatusCtr != nil {
		h.packageStatusCtr.SetValue(&PluginPackageStatusSnapshot{Packages: snapshot})
	}
}

func (h *ProcessHost) packageStatusSnapshotLocked() []PluginPackageStatus {
	out := make([]PluginPackageStatus, 0, len(h.packageStatus))
	for _, status := range h.packageStatus {
		out = append(out, status)
	}
	slices.SortFunc(out, func(a, b PluginPackageStatus) int {
		if a.PluginID < b.PluginID {
			return -1
		}
		if a.PluginID > b.PluginID {
			return 1
		}
		return 0
	})
	return out
}

func pluginPackageStatusSnapshotEqual(a, b *PluginPackageStatusSnapshot) bool {
	if a == nil || b == nil {
		return a == b
	}
	return slices.EqualFunc(a.Packages, b.Packages, func(a, b PluginPackageStatus) bool {
		return a.PluginID == b.PluginID &&
			a.DistDir == b.DistDir &&
			a.Materialized == b.Materialized &&
			a.Invalidated == b.Invalidated &&
			a.LastAction == b.LastAction &&
			a.LastError == b.LastError &&
			a.UpdatedAt.Equal(b.UpdatedAt)
	})
}

// DeletePlugin clears cached plugin data for the given plugin ID.
func (h *ProcessHost) DeletePlugin(ctx context.Context, pluginID string) error {
	pluginDistDir := h.pluginDistDir(pluginID)
	e1 := os.RemoveAll(pluginDistDir)
	pluginStateDir := h.pluginStateDir(pluginID)
	e2 := os.RemoveAll(pluginStateDir)
	if e1 != nil {
		return e1
	}
	return e2
}

// _ is a type assertion
var _ plugin_host.PluginHost = (*ProcessHost)(nil)
