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

	// Select the preferred runnable artifact; older ones are its fallbacks.
	candidates := installedManifestCandidates(selected, t.platformHosts(hosts))
	candidates[0].fallbacks = candidates[1:]
	t.setExecutePluginStateLocked(candidates[0])
}

// installedManifestCandidates returns the artifact to run, then its fallbacks.
//
// An installation spans platforms and lists its artifacts newest first, but a
// deploy repins every platform it publishes, so the order across platforms
// says only which platform was deployed last. The selection therefore takes
// the platform this peer prefers, native before JavaScript as in catalog
// selection, and the newest artifact of that platform. Only older artifacts of
// the same platform are fallbacks: a native plugin that fails to start must
// not fall back to its JavaScript build, which serves a different runtime.
// Artifacts a host only serves are used when nothing runs. Artifacts for other
// platforms are skipped. If no artifact is selectable, it returns the newest
// one without a host to report it as unrunnable.
func installedManifestCandidates(selected *installedManifests, platforms map[string]bldr_plugin_host.PluginHost) []*executePluginArgs {
	// Rank the artifacts of the selectable platforms; skip foreign platforms.
	type rankedCandidate struct {
		args       *executePluginArgs
		platformID string
		rank       int
	}
	var ranked []rankedCandidate
	best := -1
	for _, ref := range selected.refs {
		platformID := ref.GetMeta().GetPlatformId()
		pluginHost, ok := platforms[platformID]
		if !ok {
			continue
		}
		candidate := rankedCandidate{
			args: &executePluginArgs{
				manifestSnapshot: &manifest.ManifestSnapshot{ManifestRef: ref.GetManifestRef()},
				rev:              ref.GetMeta().GetRev(),
				pluginHost:       pluginHost,
				serveAssets:      pluginHost == nil,
				installation:     selected,
			},
			platformID: platformID,
			rank:       candidateRank(platformID, pluginHost),
		}
		if best < 0 || candidate.rank < ranked[best].rank {
			best = len(ranked)
		}
		ranked = append(ranked, candidate)
	}

	// Return the best platform's artifacts, newest first, in installation order.
	var chosen []*executePluginArgs
	for _, candidate := range ranked {
		if candidate.platformID == ranked[best].platformID {
			chosen = append(chosen, candidate.args)
		}
	}
	if len(chosen) != 0 {
		return chosen
	}

	// Report the newest artifact as unrunnable and keep the admitted worker.
	return []*executePluginArgs{{
		manifestSnapshot: &manifest.ManifestSnapshot{ManifestRef: selected.refs[0].GetManifestRef()},
		rev:              selected.refs[0].GetMeta().GetRev(),
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
