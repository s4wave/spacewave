import { AlarmState, type Alarm } from '@s4wave/sdk/alarm/alarm.pb.js'

import { formatAgo, formatClock } from '@s4wave/app/prom/prom-view.js'

// ReadsPredicate links an Alarm to each object its watch reads.
export const ReadsPredicate = '<alarm/reads>'

// AlarmTone is how loudly a status draws.
export type AlarmTone = 'quiet' | 'holding' | 'alert'

// AlarmStatus is what a person sees of an Alarm's state.
export interface AlarmStatus {
  // word is the state's name on screen.
  word: 'Clear' | 'Holding' | 'Firing' | 'Acknowledged'
  // line says where the Alarm stands and since when.
  line: string
  // tone is how loudly the status draws.
  tone: AlarmTone
  // sounding is set while the Alarm is holding, firing or acknowledged.
  sounding: boolean
}

// alarmStatus describes a at nowMs. who names the peer that acknowledged it.
export function alarmStatus(
  a: Alarm,
  nowMs: number,
  who: (peerId: string) => string,
): AlarmStatus {
  const since = Number(a.sinceMs ?? 0n)
  const fired = Number(a.firedMs ?? 0n)
  const hold = Number(a.holdMs ?? 0n)
  switch (a.state ?? AlarmState.CLEAR) {
    case AlarmState.HOLDING: {
      const fireAt = since + hold
      return {
        word: 'Holding',
        line: `Holding: fires at ${formatClock(fireAt)} if this continues (in ${formatAgo(nowMs, fireAt)}).`,
        tone: 'holding',
        sounding: true,
      }
    }
    case AlarmState.FIRING:
      return {
        word: 'Firing',
        line: `Firing since ${formatClock(fired)}.`,
        tone: 'alert',
        sounding: true,
      }
    case AlarmState.ACKNOWLEDGED:
      return {
        word: 'Acknowledged',
        line: `Firing since ${formatClock(fired)}. Acknowledged by ${who(a.acknowledgedBy ?? '')} at ${formatClock(Number(a.acknowledgedMs ?? 0n))}.`,
        tone: 'holding',
        sounding: true,
      }
    default: {
      const cleared = Number(a.clearedMs ?? 0n)
      return {
        word: 'Clear',
        line: cleared ? `Clear since ${formatClock(cleared)}.` : 'Clear.',
        tone: 'quiet',
        sounding: false,
      }
    }
  }
}

// holdText says how long the condition must hold before the Alarm fires.
export function holdText(a: Alarm): string {
  const hold = Number(a.holdMs ?? 0n)
  if (hold <= 0) return 'Fires as soon as this condition holds.'
  return `Fires after ${formatAgo(0, hold)} of this condition.`
}

// chipText is the Alarm's chip on a Dashboard, such as "Firing 4m".
export function chipText(a: Alarm, nowMs: number): string {
  switch (a.state ?? AlarmState.CLEAR) {
    case AlarmState.HOLDING:
      return `Holding ${formatAgo(Number(a.sinceMs ?? 0n), nowMs)}`
    case AlarmState.FIRING:
      return `Firing ${formatAgo(Number(a.firedMs ?? 0n), nowMs)}`
    case AlarmState.ACKNOWLEDGED:
      return `Acknowledged, firing ${formatAgo(Number(a.firedMs ?? 0n), nowMs)}`
    default:
      return 'Clear'
  }
}

// soundOrder sorts firing Alarms first, then acknowledged, then holding.
export function soundOrder(a: Alarm): number {
  switch (a.state ?? AlarmState.CLEAR) {
    case AlarmState.FIRING:
      return 0
    case AlarmState.ACKNOWLEDGED:
      return 1
    case AlarmState.HOLDING:
      return 2
    default:
      return 3
  }
}

// linkedAlarms returns the Alarms whose reads edges reach a key in roots,
// directly or through other Alarms. reads maps each Alarm key to the keys it
// reads.
export function linkedAlarms(
  reads: ReadonlyMap<string, readonly string[]>,
  roots: ReadonlySet<string>,
): Set<string> {
  const reached = new Set(roots)
  const linked = new Set<string>()
  for (let grew = true; grew;) {
    grew = false
    for (const [alarm, keys] of reads) {
      if (linked.has(alarm) || !keys.some((k) => reached.has(k))) continue
      linked.add(alarm)
      reached.add(alarm)
      grew = true
    }
  }
  return linked
}
