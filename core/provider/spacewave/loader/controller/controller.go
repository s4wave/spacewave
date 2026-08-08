package spacewave_loader_controller

import (
	"context"
	"os"
	"path/filepath"
	"runtime"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	bldr_dist_entrypoint "github.com/s4wave/spacewave/bldr/dist/entrypoint"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	launcher_helper "github.com/s4wave/spacewave/core/launcher/helper"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	"github.com/s4wave/spacewave/core/provider/spacewave/loader/ui"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller ID.
const ControllerID = "spacewave/loader/controller"

// Version is the controller version.
var Version = controller.MustParseVersion("0.0.1")

const hostExecutableDirEnv = "BLDR_PLUGIN_HOST_EXECUTABLE_DIR"

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
		c.le.WithError(err).Warn("loader helper unavailable; skipping loader UI")
		return nil
	}
	defer func() {
		if err := client.Close(); err != nil {
			c.le.WithError(err).Warn("close loader helper")
		}
	}()

	pluginIDs := c.conf.ResolvedWatchPluginIDs()
	progress := ui.NewTracker(client, c.le, pluginIDs)
	progress.Render()

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
			progress.SetFetchStatus(loaderFetchStatus(v.GetValue()))
		},
		func(v directive.TypedAttachedValue[*spacewave_launcher.FetchStatus]) {
			// A removed fetch-status value just means the launcher is
			// repushing; the next value add will restore it. Clear local
			// state so stale labels don't persist between transitions.
			progress.SetFetchStatus(nil)
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
				progress.MarkRunning(id, true)
			},
			func(v directive.TypedAttachedValue[bldr_plugin.RunningPlugin]) {
				progress.MarkRunning(id, false)
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

func loaderFetchStatus(status *spacewave_launcher.FetchStatus) *ui.FetchStatus {
	if status == nil {
		return nil
	}
	return &ui.FetchStatus{
		Fetching:    status.Fetching,
		HasConfig:   status.HasConfig,
		LastErr:     status.LastErr,
		Attempts:    status.Attempts,
		NextRetryAt: status.NextRetryAt,
	}
}

// resolveHelperPath looks for the helper binary adjacent to the running
// executable, then beside the host executable when the loader runs as a
// downloaded plugin. Returns false when no binary exists at the expected path.
func resolveHelperPath(overrideName string) (string, bool) {
	exe, err := os.Executable()
	if err != nil {
		return "", false
	}
	return ui.ResolveHelperPathFromDirs(
		[]string{filepath.Dir(exe), os.Getenv(hostExecutableDirEnv)},
		overrideName,
		runtime.GOOS,
	)
}

func resolveIconPath(overridePath string) string {
	if overridePath != "" {
		return overridePath
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return ui.ResolveIconPathFromDirs([]string{
		filepath.Dir(exe),
		os.Getenv(hostExecutableDirEnv),
	})
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
