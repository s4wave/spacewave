package plugin_host_process

import (
	"context"
	"os"
	"path"

	"github.com/aperturerobotics/bldr/plugin"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	host_controller "github.com/aperturerobotics/bldr/plugin/host/controller"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/hydra/unixfs"
	"github.com/blang/semver"
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
}

// NewProcessHost constructs a new ProcessHost.
func NewProcessHost(le *logrus.Entry, stateDir, distDir string) (*ProcessHost, error) {
	if _, err := os.Stat(stateDir); err != nil {
		return nil, errors.Wrap(err, "state dir")
	}
	if _, err := os.Stat(distDir); err != nil {
		return nil, errors.Wrap(err, "dist dir")
	}
	return &ProcessHost{le: le, stateDir: stateDir, distDir: distDir}, nil
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
		c.ToControllerConfig(),
		controller.NewInfo(ControllerID, Version, "plugin host with native processes"),
		processHost,
	)
	return hctrl, processHost, err
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
func (h *ProcessHost) ExecutePlugin(ctx context.Context, pluginID string, pluginDist *unixfs.FSHandle) error {
	// create the plugin bin and state dir
	pluginBinDir := path.Join(h.distDir, pluginID)
	if err := os.MkdirAll(pluginBinDir, 0755); err != nil {
		return err
	}
	pluginStateDir := path.Join(h.stateDir, pluginID)
	if err := os.MkdirAll(pluginStateDir, 0755); err != nil {
		return err
	}

	// sync the plugin dist unixfs to the disk.
	// TODO
	return errors.New("execute plugin: " + pluginID)
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
