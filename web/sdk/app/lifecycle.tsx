import {
  SpacePluginLifecycleState,
  type SpacePluginStatus,
} from '@s4wave/sdk/space/space.pb.js'

export const pluginLifecycleLabels = {
  [SpacePluginLifecycleState.SpacePluginLifecycleState_CONFIGURED]:
    'Configured',
  [SpacePluginLifecycleState.SpacePluginLifecycleState_LOADING]: 'Loading',
  [SpacePluginLifecycleState.SpacePluginLifecycleState_LOADED]: 'Loaded',
  [SpacePluginLifecycleState.SpacePluginLifecycleState_FAILED]: 'Failed',
  [SpacePluginLifecycleState.SpacePluginLifecycleState_RETRYING]: 'Retrying',
  [SpacePluginLifecycleState.SpacePluginLifecycleState_REMOVED]: 'Removed',
  [SpacePluginLifecycleState.SpacePluginLifecycleState_UPGRADED]: 'Upgraded',
  [SpacePluginLifecycleState.SpacePluginLifecycleState_UNKNOWN]: 'Unknown',
} satisfies Record<SpacePluginLifecycleState, string>

export function pluginLifecycleLabel(plugin: SpacePluginStatus): string {
  const state =
    plugin.state ??
    (plugin.loaded
      ? SpacePluginLifecycleState.SpacePluginLifecycleState_LOADED
      : SpacePluginLifecycleState.SpacePluginLifecycleState_LOADING)
  return pluginLifecycleLabels[state] ?? 'Unknown'
}

export function PluginLifecycleBadge({
  plugin,
}: {
  plugin: SpacePluginStatus
}) {
  const label = pluginLifecycleLabel(plugin)
  const tone = lifecycleTone(label)
  return <span className={tone}>{label}</span>
}

function lifecycleTone(label: string): string {
  if (label === 'Loaded') {
    return 'rounded-full bg-blue-900/50 px-2 py-0.5 text-xs text-blue-300'
  }
  if (label === 'Failed') {
    return 'rounded-full bg-red-950/70 px-2 py-0.5 text-xs text-red-200'
  }
  if (label === 'Retrying') {
    return 'rounded-full bg-amber-950/70 px-2 py-0.5 text-xs text-amber-200'
  }
  if (label === 'Removed') {
    return 'rounded-full bg-zinc-900/80 px-2 py-0.5 text-xs text-zinc-400'
  }
  if (label === 'Upgraded') {
    return 'rounded-full bg-emerald-950/70 px-2 py-0.5 text-xs text-emerald-200'
  }
  return 'rounded-full bg-zinc-800/70 px-2 py-0.5 text-xs text-zinc-300'
}
