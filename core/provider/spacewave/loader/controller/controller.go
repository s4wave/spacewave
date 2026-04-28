package spacewave_loader_controller

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	bldr_dist_entrypoint "github.com/s4wave/spacewave/bldr/dist/entrypoint"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	launcher_helper "github.com/s4wave/spacewave/core/launcher/helper"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller ID.
const ControllerID = "spacewave/loader/controller"

// Version is the controller version.
var Version = semver.MustParse("0.0.1")

// defaultHelperBinaryName is the default helper binary name on non-Windows.
const defaultHelperBinaryName = "spacewave-helper"

const hostExecutableDirEnv = "BLDR_PLUGIN_HOST_EXECUTABLE_DIR"

const defaultAppIconPath = "../Resources/app.icns"

// Controller spawns the spacewave-helper in --loading mode and drives its
// progress bar by observing LoadPlugin directive state for each configured
// watch plugin. The helper is terminated on context cancellation.
type Controller struct {
	le   *logrus.Entry
	bus  bus.Bus
	conf *Config
}

// NewController constructs a new loader controller.
func NewController(le *logrus.Entry, b bus.Bus, conf *Config) *Controller {
	return &Controller{le: le, bus: b, conf: conf}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "loader controller")
}

// HandleDirective is not implemented: the loader controller does not
// currently expose directive resolvers.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases controller-held resources. Nothing to release yet.
func (c *Controller) Close() error {
	return nil
}

// Execute spawns the helper in --loading mode, watches LoadPlugin directive
// state for the configured plugin set, and forwards progress to the helper
// as each plugin transitions to Running. Blocks until the context is canceled.
// Missing helper binaries are logged and tolerated so the rest of the plugin
// stack still boots on platforms without a helper.
func (c *Controller) Execute(ctx context.Context) error {
	helperPath, ok := resolveHelperPath(c.conf.GetHelperBinaryName())
	if !ok {
		c.le.Warn("spacewave-helper binary not found next to entrypoint or host executable; skipping loader UI")
		return nil
	}
	projectID := c.conf.ResolvedProjectID()
	rootDir, err := bldr_dist_entrypoint.DetermineStorageRoot(projectID)
	if err != nil {
		return errors.Wrap(err, "determine storage root")
	}
	if err := os.MkdirAll(rootDir, 0o700); err != nil {
		return errors.Wrap(err, "create storage root")
	}

	iconPath := resolveIconPath(c.conf.GetIconPath())
	client, err := launcher_helper.NewLoadingClient(ctx, c.le, rootDir, helperPath, iconPath)
	if err != nil {
		return errors.Wrap(err, "start loader helper")
	}
	defer client.Close()

	pluginIDs := c.conf.ResolvedWatchPluginIDs()
	progress := newProgressTracker(client, c.le, pluginIDs)
	progress.render()

	// Drain helper events: user-clicked Retry redirects into a fresh
	// DistConfig fetch; Cancel logs and tears down the loader so the helper
	// window closes even if plugin-level progress never completes.
	go c.drainEvents(ctx, client)

	refs := make([]directive.Reference, 0, len(pluginIDs)+1)
	defer func() {
		for _, ref := range refs {
			ref.Release()
		}
	}()

	// Observe DistConfig fetch status so the helper can switch into a retry
	// message when the launcher can't reach any endpoint. We scope the match
	// by project id so multiple launchers on the same bus do not confuse
	// each other's loaders.
	fetchHandler := directive.NewTypedCallbackHandler[*spacewave_launcher.FetchStatus](
		func(v directive.TypedAttachedValue[*spacewave_launcher.FetchStatus]) {
			progress.setFetchStatus(v.GetValue())
		},
		func(v directive.TypedAttachedValue[*spacewave_launcher.FetchStatus]) {
			// A removed fetch-status value just means the launcher is
			// repushing; the next value add will restore it. Clear local
			// state so stale labels don't persist between transitions.
			progress.setFetchStatus(nil)
		},
		nil, nil,
	)
	_, fetchRef, err := c.bus.AddDirective(
		spacewave_launcher.NewWatchLauncherFetchStatus(projectID),
		fetchHandler,
	)
	if err != nil {
		return errors.Wrap(err, "watch launcher fetch status")
	}
	refs = append(refs, fetchRef)

	if len(pluginIDs) == 0 {
		// Nothing plugin-level to watch: still hold the window open so the
		// fetch-status watcher above can drive the UI, then teardown.
		<-ctx.Done()
		return nil
	}

	// Register one LoadPlugin watch per plugin id. The plugin host scheduler
	// de-duplicates against any active LoadPlugin references, so this only
	// observes state without forcing loads to start.
	for _, pluginID := range pluginIDs {
		id := pluginID
		handler := directive.NewTypedCallbackHandler[bldr_plugin.RunningPlugin](
			func(v directive.TypedAttachedValue[bldr_plugin.RunningPlugin]) {
				progress.markRunning(id, true)
			},
			func(v directive.TypedAttachedValue[bldr_plugin.RunningPlugin]) {
				progress.markRunning(id, false)
			},
			nil, nil,
		)
		_, ref, err := c.bus.AddDirective(bldr_plugin.NewLoadPlugin(id), handler)
		if err != nil {
			return errors.Wrapf(err, "watch LoadPlugin %s", id)
		}
		refs = append(refs, ref)
	}

	// Exit once the tracker signals it has dismissed the helper, or when the
	// controller context is canceled. After dismiss the deferred client.Close
	// is still cheap (the subprocess has already self-exited) and the refs
	// are released so the plugin-host scheduler can drop its LoadPlugin
	// references.
	select {
	case <-ctx.Done():
	case <-progress.Done():
	}
	return nil
}

// drainEvents pulls HelperEvent messages until the helper or the controller
// ctx exits and routes user actions back into the launcher:
//
//   - RetryRequest dispatches =ExRecheckDistConfig=, so the "Retry" button on
//     the network-error view triggers the same code path a manual
//     =POST /api/release/notify= would.
//   - CancelRequest logs and returns: the helper has closed its window, so
//     the loader has no more UI to push to. The outer =select= on the
//     tracker's done channel is not notified, so Execute stays alive until
//     the plugin host cancels the loader in the normal teardown order.
func (c *Controller) drainEvents(ctx context.Context, client *launcher_helper.Client) {
	projectID := c.conf.ResolvedProjectID()
	for {
		if ctx.Err() != nil {
			return
		}
		evt, err := client.RecvEvent(ctx)
		if err != nil {
			if ctx.Err() == nil {
				c.le.WithError(err).Debug("helper event stream ended")
			}
			return
		}
		switch {
		case evt.GetRetry() != nil:
			c.le.Debug("helper retry requested; rechecking dist config")
			if err := spacewave_launcher.ExRecheckDistConfig(ctx, c.bus, projectID); err != nil && ctx.Err() == nil {
				c.le.WithError(err).Warn("retry dist config fetch failed")
			}
		case evt.GetCancel() != nil:
			c.le.Debug("helper cancel requested; closing event loop")
			return
		}
	}
}

// progressSender is the minimal subset of launcher_helper.Client consumed by
// progressTracker. A narrow interface lets unit tests swap in a fake
// recorder without spawning a real helper process.
type progressSender interface {
	SendProgress(fraction float32, text string) error
	SendDismiss() error
}

// progressTracker maintains running state for the watched plugin ids plus
// the latest launcher fetch status, and pushes progress or retry-state
// messages to the helper whenever either input changes. Once every watched
// plugin has become Running the tracker calls SendDismiss exactly once and
// stops pushing further updates.
type progressTracker struct {
	client    progressSender
	le        *logrus.Entry
	pluginIDs []string

	mu          sync.Mutex
	running     map[string]bool
	fetchStatus *spacewave_launcher.FetchStatus
	dismissed   bool
	doneCh      chan struct{}
}

// newProgressTracker constructs a tracker for the given plugin id order. The
// order is preserved when computing the "next pending" phase label so the UI
// reports plugins in manifest-declared order rather than arbitrary map order.
func newProgressTracker(
	client progressSender,
	le *logrus.Entry,
	pluginIDs []string,
) *progressTracker {
	return &progressTracker{
		client:    client,
		le:        le,
		pluginIDs: pluginIDs,
		running:   make(map[string]bool, len(pluginIDs)),
		doneCh:    make(chan struct{}),
	}
}

// Done returns a channel that closes after the tracker has sent the dismiss
// signal to the helper. Callers can select on it to tear down the loader
// controller once the main app UI has taken over.
func (t *progressTracker) Done() <-chan struct{} {
	return t.doneCh
}

// markRunning flips the per-plugin running state and re-renders progress.
func (t *progressTracker) markRunning(pluginID string, running bool) {
	t.mu.Lock()
	if t.running[pluginID] == running {
		t.mu.Unlock()
		return
	}
	t.running[pluginID] = running
	t.mu.Unlock()
	t.render()
}

// setFetchStatus updates the cached launcher fetch status and re-renders.
func (t *progressTracker) setFetchStatus(status *spacewave_launcher.FetchStatus) {
	t.mu.Lock()
	if t.fetchStatus == status {
		t.mu.Unlock()
		return
	}
	t.fetchStatus = status
	t.mu.Unlock()
	t.render()
}

// render pushes the current progress snapshot to the helper. Priority:
//  1. If the launcher has no DistConfig and the last fetch failed: show a
//     retry message with countdown to the next scheduled attempt.
//  2. If the launcher is currently fetching its first DistConfig: show a
//     "Connecting..." indeterminate spinner.
//  3. Otherwise fall through to the plugin-level progress: indeterminate
//     "Preparing..." until anything resolves, then determinate with phase
//     labels for the next pending plugin.
func (t *progressTracker) render() {
	t.mu.Lock()
	if t.dismissed {
		t.mu.Unlock()
		return
	}
	total := len(t.pluginIDs)
	running := 0
	var nextPending string
	for _, id := range t.pluginIDs {
		if t.running[id] {
			running++
			continue
		}
		if nextPending == "" {
			nextPending = id
		}
	}
	status := t.fetchStatus
	// When every watched plugin has resolved, send the final 100% progress
	// snapshot so the helper bar snaps full, then dismiss exactly once.
	if total > 0 && running == total {
		t.dismissed = true
		t.mu.Unlock()
		if err := t.client.SendProgress(1.0, "Ready"); err != nil {
			t.le.WithError(err).Debug("send final progress")
		}
		if err := t.client.SendDismiss(); err != nil {
			t.le.WithError(err).Debug("send dismiss")
		}
		close(t.doneCh)
		return
	}
	t.mu.Unlock()

	if status != nil && !status.HasConfig {
		if status.LastErr != "" {
			text := formatRetryMessage(status.NextRetryAt)
			if err := t.client.SendProgress(-1, text); err != nil {
				t.le.WithError(err).Debug("send retry status")
			}
			return
		}
		if status.Fetching {
			if err := t.client.SendProgress(-1, "Connecting to Spacewave..."); err != nil {
				t.le.WithError(err).Debug("send connecting status")
			}
			return
		}
	}

	if total == 0 {
		if err := t.client.SendProgress(-1, "Preparing Spacewave..."); err != nil {
			t.le.WithError(err).Debug("send preparing status")
		}
		return
	}
	if running == 0 {
		if err := t.client.SendProgress(-1, "Preparing Spacewave..."); err != nil {
			t.le.WithError(err).Debug("send initial progress")
		}
		return
	}
	text := "Loading Spacewave..."
	if nextPending != "" {
		text = "Loading " + nextPending + "..."
	}
	fraction := float32(running) / float32(total)
	if err := t.client.SendProgress(fraction, text); err != nil {
		t.le.WithError(err).Debug("send progress")
	}
}

// formatRetryMessage returns the helper message for a failed DistConfig
// fetch with an optional countdown to the next scheduled retry.
func formatRetryMessage(nextRetryAt time.Time) string {
	const prefix = "Waiting for network"
	if nextRetryAt.IsZero() {
		return prefix + "..."
	}
	remaining := time.Until(nextRetryAt)
	if remaining < time.Second {
		return prefix + " (retrying...)"
	}
	secs := int(remaining.Round(time.Second).Seconds())
	return prefix + " (retry in " + strconv.Itoa(secs) + "s)"
}

// resolveHelperPath looks for the helper binary adjacent to the running
// executable, then beside the host executable when the loader runs as a
// downloaded plugin. Returns false when no binary exists at the expected path.
func resolveHelperPath(overrideName string) (string, bool) {
	exe, err := os.Executable()
	if err != nil {
		return "", false
	}
	return resolveHelperPathFromDirs(
		[]string{filepath.Dir(exe), os.Getenv(hostExecutableDirEnv)},
		overrideName,
		runtime.GOOS,
	)
}

func resolveHelperPathFromDirs(baseDirs []string, overrideName, goos string) (string, bool) {
	for _, baseDir := range baseDirs {
		if baseDir == "" {
			continue
		}
		path, ok := resolveHelperPathIn(baseDir, overrideName, goos)
		if ok {
			return path, true
		}
	}
	return "", false
}

func resolveIconPath(overridePath string) string {
	if overridePath != "" {
		return overridePath
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return resolveIconPathFromDirs([]string{
		filepath.Dir(exe),
		os.Getenv(hostExecutableDirEnv),
	})
}

func resolveIconPathFromDirs(baseDirs []string) string {
	for _, baseDir := range baseDirs {
		if baseDir == "" {
			continue
		}
		path := filepath.Clean(filepath.Join(baseDir, defaultAppIconPath))
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// resolveHelperPathIn resolves the helper binary path within baseDir using the
// given goos to decide whether to append =.exe= when overrideName is empty.
// Split out of resolveHelperPath so the name-resolution logic is testable
// without touching the test binary's adjacent directory.
func resolveHelperPathIn(baseDir, overrideName, goos string) (string, bool) {
	name := overrideName
	if name == "" {
		name = defaultHelperBinaryName
		if goos == "windows" {
			name += ".exe"
		}
	}
	path := filepath.Join(baseDir, name)
	if _, err := os.Stat(path); err != nil {
		return "", false
	}
	return path, true
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
