package plugin_host_scheduler

import (
	"context"
	"slices"

	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// setExecutePluginState keeps guarded plugins running until they relinquish
// their work. Initial starts and unguarded replacements apply immediately. A
// nil state cancels the candidate but leaves the admitted worker; see
// releaseCatalogExecution.
func (t *pluginInstance) setExecutePluginState(args *executePluginArgs) bool {
	t.pluginUpdateMtx.Lock()
	defer t.pluginUpdateMtx.Unlock()
	return t.setExecutePluginStateLocked(args)
}

// setExecutePluginStateLocked applies candidate ordering under pluginUpdateMtx.
func (t *pluginInstance) setExecutePluginStateLocked(args *executePluginArgs) bool {
	// Reject a candidate the binding no longer accepts.
	if !t.acceptsManifest(args) {
		return false
	}

	// Retain the running generation when the replacement is an older revision.
	current := t.executePluginRoutine.GetState()
	if t.selectedManifest.Load() == nil && current != nil && args != nil {
		currentMeta := current.manifestSnapshot.GetManifest().GetMeta()
		nextMeta := args.manifestSnapshot.GetManifest().GetMeta()
		if currentMeta != nil && nextMeta != nil &&
			currentMeta.GetManifestId() == nextMeta.GetManifestId() &&
			currentMeta.GetBuildType() == nextMeta.GetBuildType() &&
			currentMeta.GetPlatformId() == nextMeta.GetPlatformId() &&
			currentMeta.GetRev() > nextMeta.GetRev() {
			// A remote copy can temporarily leave only an older local fallback.
			// Retain the admitted generation until its replacement is ready.
			return false
		}
	}

	// Replace immediately when unguarded, unchanged, or nothing runs.
	if args == nil || current == nil || executePluginArgsEqual(current, args) ||
		!slices.Contains(t.c.conf.GetUpdateGuardPluginIds(), t.pluginID) {
		if t.updatePluginRoutine != nil {
			t.updatePluginRoutine.SetState(nil)
		}
		_, changed, _, _ := t.executePluginRoutine.SetState(args)
		return changed
	}

	// Route the guarded replacement through the update routine.
	_, changed, _, _ := t.updatePluginRoutine.SetState(args)
	return changed
}

// releaseCatalogExecution ends the catalog's selection once the catalog links
// no manifest the binding may run, on any platform: none of the plugin, or for
// a binding pinned to a manifest root, none at that root, whatever else the
// plugin has. It clears the candidate and releases the worker the catalog
// admitted, running or serving files. A catalog worker is released exactly
// then, never on a host change, which keeps it as an installation's worker is
// kept; while an installation is selected the catalog releases nothing. One
// binding never releases: with the store disabled and no root, the direct
// fetch selects, and its empty result cannot tell a gap from a removal. It
// reports whether the selection or the admitted worker changed.
func (t *pluginInstance) releaseCatalogExecution() bool {
	// Clear the catalog candidate and take the catalog's worker, if admitted.
	t.pluginUpdateMtx.Lock()
	if !t.acceptsManifest(nil) {
		t.pluginUpdateMtx.Unlock()
		return false
	}
	changed := t.setExecutePluginStateLocked(nil)
	t.clearReleasedSwapLocked()
	var worker *pluginInstance
	if t.activeArgs != nil && t.activeArgs.installation == nil {
		worker = t.activeExecution
	}
	t.pluginUpdateMtx.Unlock()

	// Release it unless another admission replaced it meanwhile.
	if worker != nil {
		t.clearExecution(worker)
		changed = true
	}
	return changed
}

// execGuardedPluginUpdate asks the current generation to quiesce before the
// execution owner cancels it. A failed request leaves that generation running.
func (t *pluginInstance) execGuardedPluginUpdate(ctx context.Context, args *executePluginArgs) error {
	// Skip updates that carry no candidate.
	if args == nil {
		return nil
	}

	// Wait for the running plugin and ask it to quiesce, only for a candidate
	// the binding still accepts.
	running, err := t.runningPluginCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}
	if !t.acceptsManifest(args) {
		return context.Canceled
	}

	// Prepare the running generation for replacement.
	client := bldr_plugin.NewSRPCUpdateGuardClient(running.GetRpcClient())
	if _, err := client.Prepare(ctx, &bldr_plugin.PrepareUpdateRequest{}); err != nil {
		return err
	}

	// Apply the replacement only if the binding still accepts it.
	t.pluginUpdateMtx.Lock()
	defer t.pluginUpdateMtx.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if !t.acceptsManifest(args) {
		return context.Canceled
	}
	t.executePluginRoutine.SetState(args)
	return nil
}
