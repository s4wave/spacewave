package plugin_host_scheduler

import (
	"context"
	"slices"

	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// setExecutePluginState keeps guarded plugins running until they relinquish
// their work. Initial starts and unguarded replacements apply immediately.
func (t *pluginInstance) setExecutePluginState(args *executePluginArgs) bool {
	t.pluginUpdateMtx.Lock()
	defer t.pluginUpdateMtx.Unlock()
	return t.setExecutePluginStateLocked(args)
}

// setExecutePluginStateLocked applies candidate ordering under pluginUpdateMtx.
func (t *pluginInstance) setExecutePluginStateLocked(args *executePluginArgs) bool {
	if !t.acceptsManifest(args) {
		return false
	}

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

	if args == nil || current == nil || executePluginArgsEqual(current, args) ||
		!slices.Contains(t.c.conf.GetUpdateGuardPluginIds(), t.pluginID) {
		if t.updatePluginRoutine != nil {
			t.updatePluginRoutine.SetState(nil)
		}
		_, changed, _, _ := t.executePluginRoutine.SetState(args)
		return changed
	}

	_, changed, _, _ := t.updatePluginRoutine.SetState(args)
	return changed
}

// execGuardedPluginUpdate asks the current generation to quiesce before the
// execution owner cancels it. A failed request leaves that generation running.
func (t *pluginInstance) execGuardedPluginUpdate(ctx context.Context, args *executePluginArgs) error {
	if args == nil {
		return nil
	}

	running, err := t.runningPluginCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}
	client := bldr_plugin.NewSRPCUpdateGuardClient(running.GetRpcClient())
	if _, err := client.Prepare(ctx, &bldr_plugin.PrepareUpdateRequest{}); err != nil {
		return err
	}

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
