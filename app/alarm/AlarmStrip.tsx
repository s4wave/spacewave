import { LuSiren } from '@s4wave/web/images/icons.js'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { useClock } from '@s4wave/web/hooks/useClock.js'
import { useWorldQuery } from '@s4wave/web/hooks/useWorldQuery.js'

import { loadDashboardAlarms, type KeyedAlarm } from './alarm-query.js'
import { alarmStatus, chipText } from './alarm-view.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    flexShrink: 0,
    flexWrap: 'wrap',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule2: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '.25rem',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-warning']} 15%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-warning']} 25%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-warning'],
  },
  rule3: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '.25rem',
    backgroundColor: {
      default: themeColors['--color-destructive'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 85%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive-foreground'],
  },
  rule4: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: '.25rem',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule5: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule6: {
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

// clockPeriodMs is how often the strip ages its chips.
const clockPeriodMs = 15_000

// AlarmStrip shows a chip for each sounding Alarm linked to the Dashboard key,
// firing first. It takes no space while none sounds.
export function AlarmStrip({
  worldState,
  dashboardKey,
  onOpen,
}: {
  worldState: Resource<IWorldState>
  dashboardKey: string
  onOpen: (key: string) => void
}) {
  const alarms = useWorldQuery(
    worldState,
    (world, signal) => loadDashboardAlarms(world, dashboardKey, signal),
    [dashboardKey],
    { key: `dashboard-alarms:${dashboardKey}` },
  )
  const nowMs = useClock(clockPeriodMs)
  if (!alarms.value?.length) return null
  return <AlarmChips alarms={alarms.value} nowMs={nowMs} onOpen={onOpen} />
}

// AlarmChips draws one chip per Alarm.
export function AlarmChips({
  alarms,
  nowMs,
  onOpen,
}: {
  alarms: readonly KeyedAlarm[]
  nowMs: number
  onOpen: (key: string) => void
}) {
  return (
    <ul sx={stylexStyles2.rule1} aria-label="Alarms">
      {alarms.map(({ key, alarm }) => {
        const tone = alarmStatus(alarm, nowMs, String).tone
        return (
          <li key={key}>
            <button
              type="button"
              onClick={() => onOpen(key)}
              sx={tone === 'alert' ? stylexStyles2.rule3 : stylexStyles2.rule2}
            >
              <LuSiren sx={stylexStyles.rule1} aria-hidden />
              <span sx={stylexStyles2.rule5}>{alarm.title || 'Alarm'}</span>
              <span sx={stylexStyles2.rule6}>{chipText(alarm, nowMs)}</span>
            </button>
          </li>
        )
      })}
    </ul>
  )
}
