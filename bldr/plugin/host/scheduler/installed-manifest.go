package plugin_host_scheduler

import (
	"sync"

	manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
)

// installedManifests retains the ordered artifacts from one installation demand.
// The scheduler may recover with an earlier artifact only when no worker survives.
type installedManifests struct {
	refs []*manifest.ManifestRef
}

// addManifestSelection retains the caller's installation choice. Replacing a
// LoadPlugin demand adds the new selection before releasing the old demand.
func (t *pluginInstance) addManifestSelection(refs ...*manifest.ManifestRef) func() {
	// Clone the caller's manifest references into a new selection.
	selection := &installedManifests{refs: make([]*manifest.ManifestRef, len(refs))}
	for i, ref := range refs {
		selection.refs[i] = ref.CloneVT()
	}

	// Register the selection under the next sequence number and project it.
	t.pluginUpdateMtx.Lock()
	t.selectionSequence++
	id := t.selectionSequence
	t.selections[id] = selection
	t.updateManifestSelectionLocked()
	t.pluginUpdateMtx.Unlock()

	// Return the release function that removes this selection again.
	return sync.OnceFunc(func() {
		t.pluginUpdateMtx.Lock()
		delete(t.selections, id)
		t.updateManifestSelectionLocked()
		t.pluginUpdateMtx.Unlock()
	})
}

// updateManifestSelectionLocked projects the newest retained installation demand.
func (t *pluginInstance) updateManifestSelectionLocked() {
	// Find the selection with the highest sequence number.
	var latest uint64
	var selected *installedManifests
	for id, ref := range t.selections {
		if id > latest {
			latest, selected = id, ref
		}
	}

	// Store it and reselect the installed manifest for the current hosts.
	t.selectedManifest.Store(selected)
	t.selectInstalledManifestLocked(t.c.pluginHostsCtr.GetValue())
}

// selectInstalledManifestLocked sends the selected artifact through the normal
// materialization and prepared-admission owner. Missing hosts retain the admitted worker.
func (t *pluginInstance) selectInstalledManifestLocked(hosts *pluginHostSet) {
	// Wait for the installation and the hosts it can run on.
	selected := t.selectedManifest.Load()
	if selected == nil || hosts == nil || len(selected.refs) == 0 {
		return
	}

	// Select the newest runnable artifact; older ones are its fallbacks.
	candidates := installedManifestCandidates(selected, hosts.toPluginPlatformIDsMap(t.c.conf, t.pluginID))
	candidates[0].fallbacks = candidates[1:]
	t.setExecutePluginStateLocked(candidates[0])
}

// installedManifestCandidates lists the installation's artifacts newest first
// for the platforms that have a host. An installation spans platforms, so
// artifacts for other platforms are skipped. If no artifact can run, it returns
// the newest one without a host to report it as unrunnable.
func installedManifestCandidates(selected *installedManifests, platforms map[string]bldr_plugin_host.PluginHost) []*executePluginArgs {
	// Keep the artifacts that one of the hosts can execute.
	candidates := make([]*executePluginArgs, 0, len(selected.refs))
	for _, ref := range selected.refs {
		pluginHost := platforms[ref.GetMeta().GetPlatformId()]
		if pluginHost == nil {
			continue
		}
		candidates = append(candidates, &executePluginArgs{
			manifestSnapshot: &manifest.ManifestSnapshot{ManifestRef: ref.GetManifestRef()},
			pluginHost:       pluginHost,
			installation:     selected,
		})
	}
	if len(candidates) != 0 {
		return candidates
	}

	// Report the newest artifact as unrunnable and keep the admitted worker.
	return []*executePluginArgs{{
		manifestSnapshot: &manifest.ManifestSnapshot{ManifestRef: selected.refs[0].GetManifestRef()},
		installation:     selected,
	}}
}

// acceptsManifest prevents catalog updates and superseded preparation from
// replacing an explicitly installed artifact. The admitted worker may remain
// older while its selected replacement prepares or reports an error.
func (t *pluginInstance) acceptsManifest(args *executePluginArgs) bool {
	selected := t.selectedManifest.Load()
	return selected == nil || args != nil && args.installation == selected
}
