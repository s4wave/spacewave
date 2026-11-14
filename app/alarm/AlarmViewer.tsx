import { useCallback, useReducer } from 'react'

import { resourceContent } from '@aptre/bldr-sdk/hooks/index.js'
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
import {
  ViewerStatus,
  resourceStatus,
} from '@s4wave/web/object/ViewerStatus.js'

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

  // Acknowledge it as this member. The write needs the live World: while it
  // has failed or reloads, Acknowledge is withheld and the Alarm's failure
  // notice, which carries the World's, says why.
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

  // Past its reconnect backoff the watch fails and keeps the last Alarm it
  // read, drawn under the failure notice. A failure with nothing kept, or
  // with a kept read that found no Alarm, takes the body.
  const value = resourceContent(alarm)?.alarm
  const failure = resourceStatus('the Alarm', alarm)
  if (failure && !value) {
    return <ViewerStatus {...failure} />
  }
  if (!alarm.value && !value) {
    return <ViewerStatus state="waiting" what="the Alarm" />
  }
  if (!value) {
    return (
      <ViewerStatus state="failed" message="This Alarm no longer exists." />
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
      canAcknowledge={canWrite && world !== null}
      saving={ack.state === 'saving'}
      ackError={ackError}
      onAcknowledge={() => void onAcknowledge()}
      notice={failure ? <ViewerStatus {...failure} /> : null}
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
