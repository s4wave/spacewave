import { useCallback, useMemo, useReducer } from 'react'
import { LuActivity } from '@s4wave/web/images/icons.js'

import { isDesktop } from '@aptre/bldr'
import {
  useResourceValue,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
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
import { EmptyState } from '@s4wave/web/ui/EmptyState.js'
import { ErrorState } from '@s4wave/web/ui/ErrorState.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

import { PromPanel } from './PromPanel.js'
import { PromTargetView } from './PromTargetView.js'
import { clockPeriodMs, defaultTimeRange, shortPeer } from './prom-view.js'
import { targetAction, targetState } from './target-state.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: { maxWidth: 'var(--container-2xl)' },
})

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-foreground-alt'],
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

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
      <ErrorState
        title="Scrape target unavailable"
        message={handle.error.message}
        onRetry={handle.retry}
      />
    )
  }
  if (!value) return <LoadingInline label="Opening the Scrape target" />
  switch (value.state) {
    case 'absent':
      return (
        <EmptyState
          icon={<LuActivity sx={stylexStyles.rule1} />}
          title="This Scrape target no longer exists."
        />
      )
    case 'no-handler':
      return isDesktop ? (
        <LoadingInline label="Starting the metrics plugin" />
      ) : (
        <EmptyState
          icon={<LuActivity sx={stylexStyles.rule1} />}
          title="Scrape targets run on a desktop Device."
          description="Open this Space in the Spacewave desktop app to see and approve this target."
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

  // Derive the state now; the clock moves a silent target to stale.
  const info = targets.value?.get(objectKey)
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

  if (targets.error) {
    return (
      <ErrorState
        title="Scrape target unavailable"
        message={targets.error.message}
        onRetry={targets.retry}
      />
    )
  }
  if (!targets.value) return <LoadingInline label="Loading the Scrape target" />
  return (
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
      renderChart={(sx) =>
        chartPanel && (state.state === 'up' || state.state === 'down') ? (
          <div sx={[stylexStyles2.rule1, sx]}>
            <PromPanel
              query={query}
              panel={chartPanel}
              range={defaultTimeRange}
              targets={targets.value}
              nowMs={nowMs}
              hoverMs={null}
              onHover={noop}
            />
          </div>
        ) : null
      }
    />
  )
}

// noop ignores a call.
function noop() {}
