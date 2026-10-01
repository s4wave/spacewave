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
	"github.com/pkg/errors"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	host_controller "github.com/s4wave/spacewave/bldr/plugin/host/controller"
	bldr_pipesock "github.com/s4wave/spacewave/bldr/util/pipesock"
	"github.com/s4wave/spacewave/bldr/util/tailwriter"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	"github.com/s4wave/spacewave/net/util/randstring"
	"github.com/sirupsen/logrus"
)

// Controller is the plugin host controller type.
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
	// distRefsMtx guards distRefs and pruning of unused dist checkouts.
	distRefsMtx sync.Mutex
	// distRefs counts executing instances by plugin artifact ID.
	distRefs map[string]int
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
	// Validate the controller config before building the host.
	if err := c.Validate(); err != nil {
		return nil, nil, err
	}

	// Construct the process host from the configured state and dist dirs.
	stateDir, distDir := c.GetStateDir(), c.GetDistDir()
	processHost, err := NewProcessHost(le, stateDir, distDir)
	if err != nil {
		return nil, nil, err
	}

	// Register the process host as a host controller on the bus.
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

	// Collect the valid plugin IDs from the dist directories.
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
	pluginID, instanceKey, _, manifestRoot, entrypoint string,
	pluginDist, pluginAssets *unixfs.FSHandle,
	hostMux srpc.Mux,
	rpcInit plugin_host.PluginRpcInitCb,
) error {
	// Cancel the derived context when the plugin execution returns.
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	// Resolve the entrypoint file inside the plugin dist handle.
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

	// Read the entrypoint file info and require an executable regular file.
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

	// Create the plugin state directory.
	pluginStateDir, err := h.ensurePluginStateDir(pluginID)
	if err != nil {
		return err
	}

	// Mark the checkout in use before syncing so pruning cannot remove it.
	releaseDist := h.acquirePluginDist(pluginID, manifestRoot)
	defer releaseDist()

	// Sync the plugin dist checkout to disk and prune unused checkouts.
	pluginDistDir, err := h.syncPluginDist(ctx, pluginID, manifestRoot, entrypoint, pluginDist)
	if err != nil {
		return err
	}
	h.pruneUnusedPluginDists(pluginID)
	entrypointPath := filepath.Join(pluginDistDir, entrypoint)

	// Configure the entrypoint process to run from the plugin bin dir.
	entrypointProc := exec.CommandContext(ctx, entrypointPath, "exec-plugin")
	entrypointProc.Dir = pluginDistDir

	// Build the plugin start info and its JSON payload.
	pluginInstanceID := randstring.RandomIdentifier(0)
	pluginStartInfo := bldr_plugin.NewPluginStartInfo(pluginInstanceID, pluginID, instanceKey, manifestRoot)
	pluginStartInfoJsonB64, err := pluginStartInfo.MarshalJsonBase64()
	if err != nil {
		return err
	}

	// Pass the start info and state path to the plugin environment.
	// NOTE: the pluginID is validated to be a valid-dns-identifier
	entrypointProc.Env = append(
		os.Environ(),
		"BLDR_PLUGIN_START_INFO="+pluginStartInfoJsonB64,
		"BLDR_PLUGIN_STATE_PATH="+pluginStateDir,
	)
	if exe, err := os.Executable(); err == nil {
		entrypointProc.Env = append(entrypointProc.Env, bldr_plugin.HostExecutableEnv+"="+exe)
	}

	// Write the start info to a file next to the entrypoint.
	instanceDetailsPath := filepath.Join(pluginDistDir, ".plugin-start-info")
	if err := os.WriteFile(instanceDetailsPath, []byte(pluginStartInfoJsonB64), 0o600); err != nil {
		return err
	}

	// Pipe stderr to the debug log and capture the last lines for error reporting.
	debugWriter := le.WriterLevel(logrus.DebugLevel)
	stderrTail := tailwriter.New(debugWriter, 20)
	entrypointProc.Stderr = stderrTail

	// Apply any os-specific pre-start adjustment.
	preStartObj, err := preStartCmd(entrypointProc)
	if err != nil {
		return err
	}

	// Listen on the plugin pipe socket and advertise its root to the plugin.
	pipeListener, err := bldr_pipesock.Listen(le, pluginDistDir, pluginInstanceID)
	if err != nil {
		return err
	}
	defer pipeListener.Close()
	entrypointProc.Env = append(entrypointProc.Env, "BLDR_PIPE_ROOT="+pipeListener.GetRootDir())

	// Log the entrypoint command that is about to execute.
	le.
		WithField("entrypoint", entrypoint).
		Debugf("executing plugin entrypoint: %s", entrypointProc.String())

	// Start the plugin process.
	startObj, err := startCmd(entrypointProc, preStartObj)
	if err != nil {
		return err
	}

	// Accept plugin IPC connections until the context is canceled.
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

	// Wait for the plugin process to exit and report through the error channel.
	exited := make(chan struct{})
	go func() {
		errCh <- entrypointProc.Wait()
		close(exited)
	}()

	// Shut the plugin down gracefully, then kill it, before returning.
	defer func() {
		// Cancel the context and close the pipe listener.
		ctxCancel()
		_ = pipeListener.Close()

		// Request a graceful shutdown of the plugin process.
		_ = shutdownCmd(entrypointProc, preStartObj, startObj)

		// Wait up to the graceful shutdown window for the process to exit.
		shutdownTimeout := time.NewTimer(time.Second * 3)
		select {
		case <-exited:
			_ = shutdownTimeout.Stop()
		case <-shutdownTimeout.C:
		}

		// Kill the process after the graceful shutdown window expires.
		_ = killCmd(entrypointProc, preStartObj, startObj)

		// Wait for the process exit to be confirmed.
		<-exited
	}()

	// Return the first startup error, logging captured stderr lines.
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
				return startupFailure(pluginID, manifestRoot, entrypointPath, lines, err)
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
	// Close the muxed connection when the IPC session ends.
	defer muxedConn.Close()

	// Construct the srpc client over the muxed connection.
	client := srpc.NewClientWithMuxedConn(muxedConn)

	// Initialize the plugin rpc with the client.
	err := rpcInit(client)
	if err != nil {
		return err
	}

	// Serve incoming requests on the host mux until an error occurs.
	srv := srpc.NewServer(hostMux)
	return srv.AcceptMuxedConn(ctx, muxedConn)
}

// pluginDistDir returns the dist checkout directory for a plugin.
func (h *ProcessHost) pluginDistDir(pluginID string) string {
	return filepath.Join(h.distDir, pluginID)
}

// pluginStateDir returns the state directory for a plugin.
func (h *ProcessHost) pluginStateDir(pluginID string) string {
	return filepath.Join(h.stateDir, pluginID)
}

// ensurePluginStateDir creates the state directory for a plugin if missing.
func (h *ProcessHost) ensurePluginStateDir(pluginID string) (string, error) {
	// Create the plugin state directory if missing.
	pluginStateDir := h.pluginStateDir(pluginID)
	if err := os.MkdirAll(pluginStateDir, 0o755); err != nil {
		return "", err
	}
	return pluginStateDir, nil
}

// acquirePluginDist marks the dist checkout for a manifest root as in use by
// an executing instance. The returned func releases the reference.
func (h *ProcessHost) acquirePluginDist(pluginID, manifestRoot string) func() {
	// Count a reference on the plugin artifact under the lock.
	artifactID := bldr_plugin.PluginArtifactID(pluginID, manifestRoot)
	h.distRefsMtx.Lock()
	if h.distRefs == nil {
		h.distRefs = make(map[string]int)
	}
	h.distRefs[artifactID]++
	h.distRefsMtx.Unlock()

	// Return a release function that drops the reference once.
	var once sync.Once
	return func() {
		once.Do(func() {
			h.distRefsMtx.Lock()
			if h.distRefs[artifactID]--; h.distRefs[artifactID] <= 0 {
				delete(h.distRefs, artifactID)
			}
			h.distRefsMtx.Unlock()
		})
	}
}

// pruneUnusedPluginDists removes per-manifest-root dist checkouts for a
// plugin that no executing instance of this host is using.
func (h *ProcessHost) pruneUnusedPluginDists(pluginID string) {
	// List the plugin's per-manifest-root dist checkouts under the lock.
	manifestsDir := filepath.Join(h.pluginDistDir(pluginID), "manifest")
	h.distRefsMtx.Lock()
	defer h.distRefsMtx.Unlock()
	dirents, err := os.ReadDir(manifestsDir)
	if err != nil {
		if !os.IsNotExist(err) {
			h.le.WithError(err).WithField("plugin-id", pluginID).Warn("unable to list plugin dist checkouts")
		}
		return
	}

	// Remove each unreferenced checkout directory.
	for _, ent := range dirents {
		if !ent.IsDir() {
			continue
		}
		artifactID := bldr_plugin.PluginArtifactID(pluginID, ent.Name())
		if _, _, err := bldr_plugin.ParsePluginArtifactID(artifactID, false); err != nil {
			continue
		}
		if h.distRefs[artifactID] > 0 {
			continue
		}
		dir := filepath.Join(manifestsDir, ent.Name())
		if err := os.RemoveAll(dir); err != nil {
			h.le.WithError(err).WithField("dist-dir", dir).Warn("unable to remove unused plugin dist checkout")
		}
	}
}

// syncPluginDist materializes or updates the plugin's dist checkout from
// its FS handle and returns the directory.
func (h *ProcessHost) syncPluginDist(ctx context.Context, pluginID, manifestRoot, entrypoint string, pluginDist *unixfs.FSHandle) (string, error) {
	// Create the plugin's dist checkout directory.
	pluginDistDir := h.pluginDistDir(bldr_plugin.PluginArtifactID(pluginID, manifestRoot))
	if err := os.MkdirAll(pluginDistDir, 0o755); err != nil {
		h.recordPluginPackageStatus(pluginID, pluginDistDir, false, false, "sync", err)
		return "", err
	}

	// Sync the dist contents from the FS handle, keeping the entrypoint path.
	h.le.
		WithField("plugin-id", pluginID).
		WithField("dist-dir", pluginDistDir).
		Debug("syncing native plugin dist to disk")
	if err := unixfs_sync.Sync(
		ctx,
		pluginDistDir,
		pluginDist,
		unixfs_sync.DeleteMode_DeleteMode_BEFORE,
		func(_ context.Context, path string, _ unixfs.FSCursorNodeType) (bool, error) {
			return filepath.Clean(path) != entrypoint, nil
		},
	); err != nil {
		h.recordPluginPackageStatus(pluginID, pluginDistDir, false, false, "sync", err)
		return "", err
	}

	// Materialize the entrypoint executable as a fresh inode.
	if err := materializeEntrypoint(ctx, pluginDist, entrypoint, pluginDistDir); err != nil {
		h.recordPluginPackageStatus(pluginID, pluginDistDir, false, false, "sync", err)
		return "", err
	}

	// Record the completed sync status.
	h.le.
		WithField("plugin-id", pluginID).
		WithField("dist-dir", pluginDistDir).
		Debug("native plugin dist sync complete")
	h.recordPluginPackageStatus(pluginID, pluginDistDir, true, false, "sync", nil)
	return pluginDistDir, nil
}

// materializeEntrypoint replaces the executable with a complete new inode.
// Updating a previously executed inode in place leaves macOS code-signing caches
// referring to its old contents, even when the replacement signature is valid.
func materializeEntrypoint(ctx context.Context, dist *unixfs.FSHandle, entrypoint, dir string) error {
	// Open the entrypoint contents from the dist handle.
	handle, _, err := dist.LookupPath(ctx, entrypoint)
	if err != nil {
		return err
	}
	source := unixfs_billy.NewBillyFSFile(ctx, entrypoint, handle, os.O_RDONLY, time.Time{})
	defer source.Close()

	// Stage the new file outside the checkout: another instance may sync its assets.
	destination := filepath.Join(dir, entrypoint)
	file, err := os.CreateTemp(filepath.Dir(dir), ".entrypoint-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()

	// Copy the contents and restore the executable permission bits.
	if _, err := io.Copy(file, source); err != nil {
		return err
	}

	// Embedded distributions can omit executable permission bits.
	if err := file.Chmod(0o755); err != nil {
		return err
	}

	// Swap the staged file into place as a complete new inode.
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), destination)
}

// InvalidatePluginDist clears only the derived native plugin dist checkout.
// Plugin-owned state under the state directory is a separate protected surface.
func (h *ProcessHost) InvalidatePluginDist(ctx context.Context, pluginID string) error {
	// Reject invalidation of a canceled context or invalid plugin ID.
	if err := ctx.Err(); err != nil {
		h.recordPluginPackageStatus(pluginID, h.pluginDistDir(pluginID), false, false, "invalidate", err)
		return err
	}
	if err := bldr_plugin.ValidatePluginID(pluginID, false); err != nil {
		h.recordPluginPackageStatus(pluginID, h.pluginDistDir(pluginID), false, false, "invalidate", err)
		return err
	}

	// Remove the dist checkout and record the result.
	pluginDistDir := h.pluginDistDir(pluginID)
	err := os.RemoveAll(pluginDistDir)
	h.recordPluginPackageStatus(pluginID, pluginDistDir, false, err == nil, "invalidate", err)
	return err
}

// PackageStatusSnapshot returns a read-only copy of native dist
// materialization status. It never inspects or exposes plugin-owned state.
func (h *ProcessHost) PackageStatusSnapshot() []PluginPackageStatus {
	// Clone the recorded package statuses under the status lock.
	h.packageStatusMtx.Lock()
	defer h.packageStatusMtx.Unlock()
	return h.packageStatusSnapshotLocked()
}

// GetPackageStatusCtr returns native package recovery status changes.
func (h *ProcessHost) GetPackageStatusCtr() ccontainer.Watchable[*PluginPackageStatusSnapshot] {
	return h.packageStatusCtr
}

// recordPluginPackageStatus records one package status under the lock.
func (h *ProcessHost) recordPluginPackageStatus(
	pluginID,
	distDir string,
	materialized,
	invalidated bool,
	action string,
	err error,
) {
	// Record the status under the status lock.
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

	// Publish the new snapshot to the watchable container.
	if h.packageStatusCtr != nil {
		h.packageStatusCtr.SetValue(&PluginPackageStatusSnapshot{Packages: snapshot})
	}
}

// packageStatusSnapshotLocked clones the recorded package statuses. Caller
// must hold the status lock.
func (h *ProcessHost) packageStatusSnapshotLocked() []PluginPackageStatus {
	// Clone the statuses and sort them by plugin ID.
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

// pluginPackageStatusSnapshotEqual reports whether two snapshots are equal.
func pluginPackageStatusSnapshotEqual(a, b *PluginPackageStatusSnapshot) bool {
	// Compare the two snapshots field by field.
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
	// Remove the dist checkout and the state directory.
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
