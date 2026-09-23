package plugin_host_scheduler

import (
	"sync"

	manifest "github.com/s4wave/spacewave/bldr/manifest"
)

// installedManifests retains the ordered artifacts from one installation demand.
// The scheduler may recover with an earlier artifact only when no worker survives.
type installedManifests struct {
	refs []*manifest.ManifestRef
}

// addManifestSelection retains the caller's installation choice. Replacing a
// LoadPlugin demand adds the new selection before releasing the old demand.
func (t *pluginInstance) addManifestSelection(refs ...*manifest.ManifestRef) func() {
	selection := &installedManifests{refs: make([]*manifest.ManifestRef, len(refs))}
	for i, ref := range refs {
		selection.refs[i] = ref.CloneVT()
	}
	t.pluginUpdateMtx.Lock()
	t.selectionSequence++
	id := t.selectionSequence
	t.selections[id] = selection
	t.updateManifestSelectionLocked()
	t.pluginUpdateMtx.Unlock()
	return sync.OnceFunc(func() {
		t.pluginUpdateMtx.Lock()
		delete(t.selections, id)
		t.updateManifestSelectionLocked()
		t.pluginUpdateMtx.Unlock()
	})
}

// updateManifestSelectionLocked projects the newest retained installation demand.
func (t *pluginInstance) updateManifestSelectionLocked() {
	var latest uint64
	var selected *installedManifests
	for id, ref := range t.selections {
		if id > latest {
			latest, selected = id, ref
		}
	}
	t.selectedManifest.Store(selected)
	t.selectInstalledManifestLocked(t.c.pluginHostsCtr.GetValue())
}

// selectInstalledManifestLocked sends the selected artifact through the normal
// materialization and prepared-admission owner. Missing hosts retain the admitted worker.
func (t *pluginInstance) selectInstalledManifestLocked(hosts *pluginHostSet) {
	selected := t.selectedManifest.Load()
	if selected == nil || hosts == nil {
		return
	}
	if len(selected.refs) == 0 {
		return
	}
	platforms := hosts.toPluginPlatformIDsMap(t.c.conf, t.pluginID)
	candidates := make([]*executePluginArgs, 0, len(selected.refs))
	for _, ref := range selected.refs {
		candidates = append(candidates, &executePluginArgs{
			manifestSnapshot: &manifest.ManifestSnapshot{ManifestRef: ref.GetManifestRef()},
			pluginHost:       platforms[ref.GetMeta().GetPlatformId()],
			installation:     selected,
		})
	}
	candidates[0].fallbacks = candidates[1:]
	t.setExecutePluginStateLocked(candidates[0])
}

// acceptsManifest prevents catalog updates and superseded preparation from
// replacing an explicitly installed artifact. The admitted worker may remain
// older while its selected replacement prepares or reports an error.
func (t *pluginInstance) acceptsManifest(args *executePluginArgs) bool {
	selected := t.selectedManifest.Load()
	return selected == nil || args != nil && args.installation == selected
}
