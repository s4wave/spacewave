import {
  SpacePluginLifecycleState,
  type SpacePluginStatus,
} from '@s4wave/sdk/space/space.pb.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
} from '../../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    borderRadius: '3.40282e38px',
    backgroundColor:
      'color-mix(in oklab, var(--color-blue-900) 50%, transparent)',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-blue-300'],
  },
  rule2: {
    borderRadius: '3.40282e38px',
    backgroundColor:
      'color-mix(in oklab, var(--color-red-950) 70%, transparent)',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: 'var(--color-red-200)',
  },
  rule3: {
    borderRadius: '3.40282e38px',
    backgroundColor:
      'color-mix(in oklab, var(--color-amber-950) 70%, transparent)',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: 'var(--color-amber-200)',
  },
  rule4: {
    borderRadius: '3.40282e38px',
    backgroundColor:
      'color-mix(in oklab, var(--color-zinc-900) 80%, transparent)',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-zinc-400'],
  },
  rule5: {
    borderRadius: '3.40282e38px',
    backgroundColor:
      'color-mix(in oklab, var(--color-emerald-950) 70%, transparent)',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: 'var(--color-emerald-200)',
  },
  rule6: {
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-zinc-800']} 70%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-zinc-300'],
  },
})

// pluginLifecycleLabels names each known lifecycle state.
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

// pluginLifecycleLabel resolves absent state and unknown enum values.
export function pluginLifecycleLabel(plugin: SpacePluginStatus): string {
  const state =
    plugin.state ??
    (plugin.loaded
      ? SpacePluginLifecycleState.SpacePluginLifecycleState_LOADED
      : SpacePluginLifecycleState.SpacePluginLifecycleState_LOADING)
  return pluginLifecycleLabels[state] ?? 'Unknown'
}

// PluginLifecycleBadge presents the plugin label with its native status tone.
export function PluginLifecycleBadge({
  plugin,
  sx,
}: {
  plugin: SpacePluginStatus

  sx?: stylex.StyleXStyles
  className?: never
}) {
  const label = pluginLifecycleLabel(plugin)
  const tone = lifecycleTone(label)
  return <span sx={[tone, sx]}>{label}</span>
}

// lifecycleTone selects the native palette for the resolved plugin label.
function lifecycleTone(label: string): stylex.StyleXStyles {
  switch (label) {
    case 'Loaded':
      return stylexStyles.rule1
    case 'Failed':
      return stylexStyles.rule2
    case 'Retrying':
      return stylexStyles.rule3
    case 'Removed':
      return stylexStyles.rule4
    case 'Upgraded':
      return stylexStyles.rule5
    default:
      return stylexStyles.rule6
  }
}
