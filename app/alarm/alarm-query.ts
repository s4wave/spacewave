import { Alarm } from '@s4wave/sdk/alarm/alarm.pb.js'
import { AlarmTypeID } from '@s4wave/sdk/alarm/alarm_typeid.pb.js'
import { TargetTypeID } from '@s4wave/sdk/prom/prom.js'
import { iriToKey } from '@s4wave/sdk/world/graph-utils.js'
import { GraphEdgeBucketDirection } from '@s4wave/sdk/world/world.pb.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import { linkedAlarms, ReadsPredicate, soundOrder } from './alarm-view.js'

// readsLimit bounds the reads edges read per Alarm.
const readsLimit = 64

// KeyedAlarm is an Alarm with its object key.
export interface KeyedAlarm {
  key: string
  alarm: Alarm
}

// loadAlarm reads the Alarm key, or null when the object is missing.
export async function loadAlarm(
  world: IWorldState,
  key: string,
  signal?: AbortSignal,
): Promise<Alarm | null> {
  const [body] = await world.getObjectBodies([key], signal)
  if (!body?.exists) return null
  return Alarm.fromBinary(body.body ?? new Uint8Array())
}

// loadDashboardAlarms reads the sounding Alarms linked to the Dashboard key:
// those whose reads edges reach it or any Scrape target, which the Dashboard
// queries, directly or through other Alarms. Firing Alarms come first.
export async function loadDashboardAlarms(
  world: IWorldState,
  dashboardKey: string,
  signal?: AbortSignal,
): Promise<KeyedAlarm[]> {
  // List the Alarms and the targets.
  const [alarmKeys, targetKeys] = await Promise.all([
    world.listObjectsWithType(AlarmTypeID, signal),
    world.listObjectsWithType(TargetTypeID, signal),
  ])
  if (alarmKeys.length === 0) return []

  // Read each Alarm's reads edges and find the linked ones.
  const buckets = await world.listGraphEdgeBuckets(alarmKeys, readsLimit, {
    predicate: ReadsPredicate,
    direction: GraphEdgeBucketDirection.OUT,
    abortSignal: signal,
  })
  const reads = new Map<string, string[]>()
  for (const b of buckets.buckets ?? []) {
    const keys = (b.outgoing ?? []).flatMap((q) =>
      q.obj ? [iriToKey(q.obj)] : [],
    )
    reads.set(b.originObjectKey ?? '', keys)
  }
  const linked = [
    ...linkedAlarms(reads, new Set([dashboardKey, ...targetKeys])),
  ].sort()

  // Read the linked Alarms and keep the sounding ones.
  const bodies = await world.getObjectBodies(linked, signal)
  const out: KeyedAlarm[] = []
  for (const body of bodies) {
    if (!body.exists) continue
    const alarm = Alarm.fromBinary(body.body ?? new Uint8Array())
    if (soundOrder(alarm) < 3) out.push({ key: body.objectKey ?? '', alarm })
  }
  return out.sort((a, b) => soundOrder(a.alarm) - soundOrder(b.alarm))
}
