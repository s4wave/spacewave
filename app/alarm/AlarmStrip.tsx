import { LuSiren } from 'react-icons/lu'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { useClock } from '@s4wave/web/hooks/useClock.js'
import { useWorldQuery } from '@s4wave/web/hooks/useWorldQuery.js'
import { cn } from '@s4wave/web/style/utils.js'

import { loadDashboardAlarms, type KeyedAlarm } from './alarm-query.js'
import { alarmStatus, chipText } from './alarm-view.js'

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
    <ul
      className="border-foreground/8 flex shrink-0 flex-wrap items-center gap-1.5 border-b px-4 py-1.5"
      aria-label="Alarms"
    >
      {alarms.map(({ key, alarm }) => {
        const tone = alarmStatus(alarm, nowMs, String).tone
        return (
          <li key={key}>
            <button
              type="button"
              onClick={() => onOpen(key)}
              className={cn(
                'flex items-center gap-1.5 rounded px-2 py-0.5 text-xs',
                tone === 'alert' &&
                  'bg-destructive text-destructive-foreground hover:bg-destructive/85',
                tone !== 'alert' &&
                  'bg-warning/15 text-warning hover:bg-warning/25',
              )}
            >
              <LuSiren className="size-3" aria-hidden />
              <span className="font-medium">{alarm.title || 'Alarm'}</span>
              <span className="tabular-nums">{chipText(alarm, nowMs)}</span>
            </button>
          </li>
        )
      })}
    </ul>
  )
}
