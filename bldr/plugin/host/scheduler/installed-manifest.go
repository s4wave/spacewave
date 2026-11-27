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
	// peerID is the peer whose approvals decide the refs' roots.
	peerID string
}

// addManifestSelection retains the caller's installation choice. Replacing a
// LoadPlugin demand adds the new selection before releasing the old demand.
func (t *pluginInstance) addManifestSelection(peerID string, refs ...*manifest.ManifestRef) func() {
	// Clone the caller's manifest references into a new selection.
	selection := &installedManifests{refs: make([]*manifest.ManifestRef, len(refs)), peerID: peerID}
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

	// Return the release function that removes this selection again. Releasing
	// the last one ends the installation's work and returns the binding to its
	// catalog demand.
	return sync.OnceFunc(func() {
		// Drop the selection and take the installation's work out of the routines.
		t.pluginUpdateMtx.Lock()
		delete(t.selections, id)
		catalog, stop := t.updateManifestSelectionLocked()
		t.pluginUpdateMtx.Unlock()
		if !catalog {
			return
		}

		// Stop a worker started for the installation, unless a newer admission
		// replaced it, and select from the catalog again: a catalog pass that
		// ran while the installation was selected decided nothing, even when the
		// installation never placed a candidate.
		if stop != nil {
			t.clearExecution(stop)
		}
		t.reselectManifests()
	})
}

// updateManifestSelectionLocked projects the newest retained installation
// demand. With none left, it calls releaseInstallationLocked and reports that
// the binding is back on its catalog demand, with the worker to stop.
func (t *pluginInstance) updateManifestSelectionLocked() (catalog bool, stop *pluginInstance) {
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
	if selected != nil {
		t.selectInstalledManifestLocked(t.c.pluginHostsCtr.GetValue())
		return false, nil
	}
	return true, t.releaseInstallationLocked()
}

// releaseInstallationLocked removes released installation work while nothing
// is selected. It clears an installation candidate from the guarded update
// routine, so the running worker is not asked to quiesce for it. Leaving the
// execute routine cancels any wait for the candidate's admission: the routine
// returns to the admitted worker's own candidate, which it adopts without a
// restart, or to nothing when that worker ran the installation. It returns
// the worker the caller must stop.
func (t *pluginInstance) releaseInstallationLocked() (stop *pluginInstance) {
	// Drop a guarded replacement waiting to quiesce the running worker.
	if t.updatePluginRoutine != nil {
		if pending := t.updatePluginRoutine.GetState(); pending != nil && pending.installation != nil {
			t.updatePluginRoutine.SetState(nil)
		}
	}

	// Stop a worker the installation started. A catalog worker keeps
	// running.
	restore := t.activeArgs
	if restore != nil && restore.installation != nil {
		restore, stop = nil, t.activeExecution
	}

	// Return the execute routine from the installation's candidate.
	if current := t.executePluginRoutine.GetState(); current != nil && current.installation != nil {
		t.executePluginRoutine.SetState(restore)
	}
	t.clearReleasedSwapLocked()
	return stop
}

// selectInstalledManifestLocked sends the selected artifact through the normal
// materialization and prepared-admission owner. Missing hosts retain the admitted worker.
func (t *pluginInstance) selectInstalledManifestLocked(hosts *pluginHostSet) {
	// Wait for the installation and the hosts it can run on.
	selected := t.selectedManifest.Load()
	if selected == nil || hosts == nil || len(selected.refs) == 0 {
		return
	}

	// Select the preferred runnable artifact. Older artifacts are its
	// fallbacks.
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
	// Rank the artifacts of the selectable platforms. Skip foreign
	// platforms.
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
// older while its selected replacement prepares or reports an error. An
// installation's candidate is accepted only while its installation is the
// selection, so a released installation never starts. A removal or catalog
// candidate is accepted only while nothing is selected.
func (t *pluginInstance) acceptsManifest(args *executePluginArgs) bool {
	selected := t.selectedManifest.Load()
	if args == nil || args.installation == nil {
		return selected == nil
	}
	return args.installation == selected
}
