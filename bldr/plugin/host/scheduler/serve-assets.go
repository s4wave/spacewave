package plugin_host_scheduler

import (
	"context"
	"maps"
	"slices"
	"sync"

	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
)

// assetsPlatformIDs are the platforms whose manifests a browser loads
// directly, so their files are useful without a host to run them.
var assetsPlatformIDs = []string{bldr_platform.PlatformID_JS}

// addAssetsDemand retains the binding's browser files until the release.
func (t *pluginInstance) addAssetsDemand() func() {
	if t.assetsDemand.Add(1) == 1 {
		t.reselectManifests()
	}
	return sync.OnceFunc(func() {
		if t.assetsDemand.Add(-1) == 0 {
			t.reselectManifests()
		}
	})
}

// reselectManifests repeats manifest selection after its platform set changed.
func (t *pluginInstance) reselectManifests() {
	// Restart the catalog routines and repeat the installed selection.
	t.manifestSelectionFingerprint.Store(nil)
	t.fetchWorldManifestRoutine.RestartRoutine()
	t.watchWorldManifestRoutine.RestartRoutine()
	t.pluginUpdateMtx.Lock()
	t.selectInstalledManifestLocked(t.c.pluginHostsCtr.GetValue())
	t.pluginUpdateMtx.Unlock()
}

// platformHosts maps each platform the binding selects manifests for to the
// host allowed to run it. While a reference serves assets, a browser platform
// without such a host maps to nil: its manifests are mounted, not run.
func (t *pluginInstance) platformHosts(hosts *pluginHostSet) map[string]plugin_host.PluginHost {
	// Without demand, only the allowed hosts select manifests.
	platforms := hosts.toPluginPlatformIDsMap(t.c.conf, t.pluginID)
	if t.assetsDemand.Load() == 0 {
		return platforms
	}

	// Serve the browser platforms no allowed host runs.
	if platforms == nil {
		platforms = make(map[string]plugin_host.PluginHost, len(assetsPlatformIDs))
	}
	for _, platformID := range assetsPlatformIDs {
		if _, ok := platforms[platformID]; !ok {
			platforms[platformID] = nil
		}
	}
	return platforms
}

// sortedPlatformIDs returns the platform IDs of platforms in order.
func sortedPlatformIDs(platforms map[string]plugin_host.PluginHost) []string {
	return slices.Sorted(maps.Keys(platforms))
}

// serveAssets mounts a browser manifest's files through an admitted worker
// that runs nothing, until the selection changes.
func (t *pluginInstance) serveAssets(ctx context.Context, args *executePluginArgs) error {
	// A worker can only remain admitted here after its host left.
	t.clearExecution(nil)
	ref, worker, _ := t.executions.AddKeyRef(executionReference{args: args})
	if !t.admitExecution(worker, ref.Release, args) {
		ref.Release()
		return context.Canceled
	}

	// The files serve no capability registration to wait for.
	t.stopStartupWaitBudget()
	<-ctx.Done()
	t.clearExecution(worker)
	return context.Canceled
}
