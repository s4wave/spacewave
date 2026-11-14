import { useCallback, useMemo, useReducer } from 'react'
import { LuActivity } from 'react-icons/lu'

import { isDesktop } from '@aptre/bldr'
import {
  useResourceValue,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { resourceContent } from '@aptre/bldr-sdk/hooks/index.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { PromQuery, TargetTypeID } from '@s4wave/sdk/prom/prom.js'
import { PanelKind, PanelUnit, type Panel } from '@s4wave/sdk/prom/prom.pb.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import type { SpaceContents } from '@s4wave/sdk/space/contents.js'
import {
  SessionContext,
  SpaceContentsContext,
} from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { useClock } from '@s4wave/web/hooks/useClock.js'
import { useWatchTypedHandle } from '@s4wave/web/hooks/useWatchTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { ViewerStatus } from '@s4wave/web/object/ViewerStatus.js'

import { PromPanel } from './PromPanel.js'
import { PromTargetView } from './PromTargetView.js'
import { clockPeriodMs, defaultTimeRange, shortPeer } from './prom-view.js'
import { targetAction, targetState } from './target-state.js'

export { TargetTypeID }

// PromTargetViewer shows a prom/target object: its configuration, its scrape
// state, and this Device's approval of it.
export function PromTargetViewer({
  objectInfo,
  worldState,
  spaceContents,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const handle = useWatchTypedHandle(
    worldState,
    objectKey,
    PromQuery,
    TargetTypeID,
  )
  const contextContents = SpaceContentsContext.useContext()

  const value = handle.value
  if (handle.error) {
    return (
      <ViewerStatus
        state="failed"
        message={`Could not read the Scrape target: ${handle.error.message}`}
        retry={handle.retry}
      />
    )
  }
  if (!value) return <ViewerStatus state="waiting" what="the Scrape target" />
  switch (value.state) {
    case 'absent':
      return (
        <ViewerStatus
          state="failed"
          message="This Scrape target no longer exists."
        />
      )
    case 'no-handler':
      return isDesktop ? (
        <ViewerStatus state="waiting" what="the metrics plugin" />
      ) : (
        <ViewerStatus
          state="empty"
          icon={<LuActivity />}
          message="Scrape targets run on a desktop Device. Open this Space in the Spacewave desktop app to see and approve this target."
        />
      )
    case 'ready':
      return (
        <TargetBody
          objectKey={objectKey}
          query={value.handle}
          contents={spaceContents ?? contextContents}
        />
      )
  }
}

// Approval is the state of this Device's approval write.
type Approval =
  | { state: 'idle' }
  | { state: 'saving' }
  | { state: 'failed'; reason: string }

// ApprovalEvent moves the approval write between states.
type ApprovalEvent =
  | { kind: 'saving' }
  | { kind: 'saved' }
  | { kind: 'failed'; reason: string }

// reduceApproval applies one event to the approval write.
function reduceApproval(_: Approval, ev: ApprovalEvent): Approval {
  switch (ev.kind) {
    case 'saving':
      return { state: 'saving' }
    case 'saved':
      return { state: 'idle' }
    case 'failed':
      return { state: 'failed', reason: ev.reason }
  }
}

// TargetBody follows the target's info and this Device's binding.
function TargetBody({
  objectKey,
  query,
  contents,
}: {
  objectKey: string
  query: PromQuery
  contents: Resource<SpaceContents>
}) {
  const container = SpaceContainerContext.useContextSafe()
  const canWrite = container?.canDeleteObjects ?? false
  const session = useResourceValue(SessionContext.useContext()) as
    | Session
    | undefined
  const myPeerId = useSessionInfo(session).peerId ?? ''
  const [approval, dispatch] = useReducer(reduceApproval, { state: 'idle' })

  // Follow the targets and this Device's process bindings.
  const queryResource = useMemo(
    () => ({ value: query, loading: false, error: null, retry: noop }),
    [query],
  )
  const targets = useStreamingResource(
    queryResource,
    useCallback(
      (q: PromQuery, signal: AbortSignal) => q.watchTargets(signal),
      [],
    ),
    [],
    { reconnect: true },
  )
  const contentsState = useStreamingResource(
    contents,
    useCallback(
      (c: SpaceContents, signal: AbortSignal) => c.watchState({}, signal),
      [],
    ),
    [],
  )
  const approved =
    contentsState.value?.processBindings?.find((b) => b.objectKey === objectKey)
      ?.approved ?? false

  // Derive the state now; the clock moves a silent target to stale. Past
  // its reconnect backoff the targets watch fails and keeps its last record,
  // drawn under the failure.
  const targetsValue = resourceContent(targets)
  const info = targetsValue?.get(objectKey)
  const nowMs = useClock(clockPeriodMs)
  const state = targetState(info, approved, myPeerId, nowMs)
  const action = targetAction(state, approved, canWrite && isDesktop)

  // Approve or revoke this Device's binding.
  const contentsValue = contents.value
  const onAction = useCallback(async () => {
    if (!contentsValue) return
    dispatch({ kind: 'saving' })
    try {
      await contentsValue.setProcessBinding(
        objectKey,
        TargetTypeID,
        action !== 'stop',
      )
      dispatch({ kind: 'saved' })
    } catch (err) {
      dispatch({
        kind: 'failed',
        reason: err instanceof Error ? err.message : String(err),
      })
    }
  }, [contentsValue, objectKey, action])

  // Chart up and the scrape duration from this target's own series.
  const chartPanel = useMemo<Panel | null>(() => {
    const labels = info?.meta?.labels?.labels ?? []
    if (labels.length === 0) return null
    const sel = labels.map((l) => `${l.name}="${l.value}"`).join(',')
    return {
      id: 'scrape',
      title: 'Scrape duration',
      expr: `scrape_duration_seconds{${sel}}`,
      unit: PanelUnit.SECONDS,
      kind: PanelKind.CHART,
    }
  }, [info?.meta?.labels])

  const deviceName = useCallback(
    (peerId: string) =>
      peerId === myPeerId ? 'this Device' : `Device ${shortPeer(peerId)}`,
    [myPeerId],
  )

  const failure = targets.error ? (
    <ViewerStatus
      state="failed"
      message={`Could not read the Scrape target: ${targets.error.message}`}
      retry={targets.retry}
    />
  ) : null
  if (!targetsValue) {
    return failure ?? <ViewerStatus state="waiting" what="the Scrape target" />
  }
  return (
    <div className="flex h-full w-full flex-col">
      {failure}
      <div className="min-h-0 flex-1">
        <PromTargetView
          objectKey={objectKey}
          info={info}
          state={state}
          action={action}
          deviceName={deviceName}
          nowMs={nowMs}
          saving={approval.state === 'saving'}
          saveError={approval.state === 'failed' ? approval.reason : null}
          onAction={() => void onAction()}
          chart={
            chartPanel && (state.state === 'up' || state.state === 'down') ? (
              <div className="max-w-2xl">
                <PromPanel
                  query={query}
                  panel={chartPanel}
                  range={defaultTimeRange}
                  targets={targetsValue}
                  nowMs={nowMs}
                  hoverMs={null}
                  onHover={noop}
                />
              </div>
            ) : null
          }
        />
      </div>
    </div>
  )
}

// noop ignores a call.
function noop() {}
