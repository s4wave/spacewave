//go:build !goscript

package plugin_host_wazero_quickjs_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/go-git/go-billy/v6/memfs"
	billy_util "github.com/go-git/go-billy/v6/util"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host_wazero_quickjs "github.com/s4wave/spacewave/bldr/plugin/host/wazero-quickjs"
	"github.com/s4wave/spacewave/bldr/testbed"
	"github.com/sirupsen/logrus"
)

// TestPluginDist loads a built js plugin dist directory under QuickJS.
//
// BLDR_QUICKJS_PLUGIN_DIST selects the dist directory and
// BLDR_QUICKJS_PLUGIN_ID selects the plugin ID, whose <id>.mjs is the
// entrypoint.
func TestPluginDist(t *testing.T) {
	// Skip unless a dist directory and plugin ID are given.
	distDir := os.Getenv("BLDR_QUICKJS_PLUGIN_DIST")
	pluginID := os.Getenv("BLDR_QUICKJS_PLUGIN_ID")
	if distDir == "" || pluginID == "" {
		t.Skip("BLDR_QUICKJS_PLUGIN_DIST and BLDR_QUICKJS_PLUGIN_ID are unset")
	}

	// Log at debug level so the host's startup steps are visible.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Copy the dist directory into the manifest filesystem.
	distFS := memfs.New()
	err := filepath.WalkDir(distDir, func(path string, d fs.DirEntry, err error) error {
		// Skip directories. WriteFile creates them.
		if err != nil || d.IsDir() {
			return err
		}

		// Copy the file under its path relative to the dist root.
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(distDir, path)
		if err != nil {
			return err
		}
		return billy_util.WriteFile(distFS, filepath.ToSlash(rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}

	// Build the testbed with the QuickJS plugin host running.
	tb, err := testbed.BuildTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	b, sr := tb.GetBus(), tb.GetStaticResolver()
	sr.AddFactory(plugin_host_wazero_quickjs.NewFactory(b))
	quickjsHost, _, quickjsHostRef, err := loader.WaitExecControllerRunningTyped[*plugin_host_wazero_quickjs.Controller](
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(plugin_host_wazero_quickjs.NewConfig()),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer quickjsHostRef.Release()

	// Request the plugin, then publish its manifest for the host platform.
	_, pluginRef, err := b.AddDirective(ctx, bldr_plugin.NewLoadPlugin(pluginID), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer pluginRef.Release()
	platformID := quickjsHost.GetPluginHost().GetPlatformId()
	meta := bldr_manifest.NewManifestMeta(pluginID, bldr_manifest.BuildType_DEV, platformID, 1)
	_, _, err = tb.CreateManifestWithBilly(ctx, meta, pluginID+".mjs", distFS, memfs.New(), timestamppb.Now())
	if err != nil {
		t.Fatal(err)
	}

	// Wait for the plugin to run and report its startup time.
	start := time.Now()
	_, _, runningPluginRef, err := bldr_plugin.ExLoadPlugin(ctx, b, false, pluginID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer runningPluginRef.Release()
	le.Infof("plugin %s running after %v", pluginID, time.Since(start))
}
