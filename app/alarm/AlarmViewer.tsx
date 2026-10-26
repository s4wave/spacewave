import { useCallback, useReducer } from 'react'
import { LuSiren } from 'react-icons/lu'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { AlarmState, AcknowledgeAlarmOp } from '@s4wave/sdk/alarm/alarm.pb.js'
import { AcknowledgeAlarmOpTypeID } from '@s4wave/sdk/alarm/alarm_typeid.pb.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import type { SpaceSharingState } from '@s4wave/sdk/space/space.pb.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useClock } from '@s4wave/web/hooks/useClock.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { useWorldQuery } from '@s4wave/web/hooks/useWorldQuery.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { EmptyState } from '@s4wave/web/ui/EmptyState.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

import { shortPeer } from '@s4wave/app/prom/prom-view.js'

import { loadAlarm } from './alarm-query.js'
import { AlarmView } from './AlarmView.js'

// clockPeriodMs is how often the viewer advances the holding countdown.
const clockPeriodMs = 5_000

// Ack is the state of the acknowledgement write.
type Ack =
  | { state: 'idle' }
  | { state: 'saving' }
  | { state: 'failed'; reason: string }

// AckEvent moves the acknowledgement write between states.
type AckEvent =
  | { kind: 'saving' }
  | { kind: 'saved' }
  | { kind: 'failed'; reason: string }

// reduceAck applies one event to the acknowledgement write.
function reduceAck(_: Ack, ev: AckEvent): Ack {
  switch (ev.kind) {
    case 'saving':
      return { state: 'saving' }
    case 'saved':
      return { state: 'idle' }
    case 'failed':
      return { state: 'failed', reason: ev.reason }
  }
}

// AlarmViewer shows an alarm object, read from the World on every revision,
// so it works on a browser-only Device.
export function AlarmViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const container = SpaceContainerContext.useContextSafe()
  const canWrite = container?.canDeleteObjects ?? false
  const session = useResourceValue(SessionContext.useContext()) as
    | Session
    | undefined
  const myPeerId = useSessionInfo(session).peerId ?? ''
  const nowMs = useClock(clockPeriodMs)
  const [ack, dispatch] = useReducer(reduceAck, { state: 'idle' })

  // Follow the Alarm. The result wraps it, since a missing Alarm reads as
  // null and a resource that has not loaded holds null.
  const alarm = useWorldQuery(
    worldState,
    async (world, signal) => ({
      alarm: await loadAlarm(world, objectKey, signal),
    }),
    [objectKey],
    { key: `alarm:${objectKey}` },
  )

  // Acknowledge it as this member.
  const world = worldState.value
  const onAcknowledge = useCallback(async () => {
    if (!world) return
    dispatch({ kind: 'saving' })
    try {
      const op = AcknowledgeAlarmOp.toBinary({
        objectKey,
        timeMs: BigInt(Date.now()),
      })
      await world.applyWorldOp(AcknowledgeAlarmOpTypeID, op, myPeerId)
      dispatch({ kind: 'saved' })
    } catch (err) {
      dispatch({
        kind: 'failed',
        reason: err instanceof Error ? err.message : String(err),
      })
    }
  }, [world, objectKey, myPeerId])

  const sharing = container?.spaceSharingState ?? null
  const who = useCallback(
    (peerId: string) => memberName(sharing, peerId, myPeerId),
    [sharing, myPeerId],
  )

  if (alarm.error) {
    return (
      <ErrorState
        title="Alarm unavailable"
        message={alarm.error.message}
        onRetry={alarm.retry}
      />
    )
  }
  if (!alarm.value) {
    return <LoadingInline label="Opening the Alarm" />
  }
  const value = alarm.value.alarm
  if (!value) {
    return (
      <EmptyState
        icon={<LuSiren className="text-foreground-alt size-5" />}
        title="This Alarm no longer exists."
      />
    )
  }
  const firing = value.state === AlarmState.FIRING
  let ackError: string | null = null
  if (ack.state === 'failed') {
    ackError = firing
      ? `Could not acknowledge: ${ack.reason}.`
      : 'This Alarm stopped firing before the acknowledgement.'
  }
  return (
    <AlarmView
      alarm={value}
      nowMs={nowMs}
      who={who}
      canAcknowledge={canWrite}
      saving={ack.state === 'saving'}
      ackError={ackError}
      onAcknowledge={() => void onAcknowledge()}
    />
  )
}

// memberName names the member whose session holds peerId: "you" for this
// member, otherwise their username, or a short peer ID when no participant
// lists it.
export function memberName(
  state: SpaceSharingState | null,
  peerId: string,
  myPeerId: string,
): string {
  if (peerId && peerId === myPeerId) return 'you'
  const info = state?.participantInfo?.find((p) => p.peerIds?.includes(peerId))
  if (info?.isSelf) return 'you'
  return info?.entityId || info?.accountId || `peer ${shortPeer(peerId)}`
}
