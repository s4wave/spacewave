import { useCallback, useEffect, useMemo, useReducer, useState } from 'react'

import { useAbortSignalEffect } from '@aptre/bldr-react'
import {
  type RegisterCleanup,
  useResourceValue,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { useParams } from '@s4wave/web/router/router.js'
import {
  SessionContext,
  useSessionNavigate,
} from '@s4wave/web/contexts/contexts.js'
import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'

import { SetupPageLayout } from '@s4wave/app/session/setup/SetupPageLayout.js'
import { PhaseChecklist } from '@s4wave/app/session/setup/PhaseChecklist.js'

import {
  getQuickstartOption,
  isQuickstartId,
  type QuickstartSpaceCreateId,
} from './options.js'
import {
  createQuickstartSetupFromSession,
  getQuickstartSpaceName,
  populateSpace,
  type QuickstartSetup,
} from './create.js'

function isQuickstartSpaceCreateId(id: string): id is QuickstartSpaceCreateId {
  if (!isQuickstartId(id)) return false
  if (id === 'account' || id === 'pair' || id === 'local') return false
  const opt = getQuickstartOption(id)
  return !opt.path
}

type Phase = 'create' | 'mount' | 'populate' | 'done' | 'failed'

interface PipelineState {
  phase: Phase
  error: string | null
}

type PipelineAction =
  | { type: 'start' }
  | { type: 'advance'; to: Exclude<Phase, 'failed'> }
  | { type: 'fail'; error: string }

function pipelineReducer(
  state: PipelineState,
  action: PipelineAction,
): PipelineState {
  switch (action.type) {
    case 'start':
      return { phase: 'create', error: null }
    case 'advance':
      return { phase: action.to, error: null }
    case 'fail':
      return { phase: 'failed', error: action.error }
  }
}

// CreateSpaceRoute owns the full create-space pipeline for quickstart tiles
// and renders a chromeless full-screen loading UI with phase progress.
export function CreateSpaceRoute() {
  const params = useParams()
  const quickstartId = params.quickstartId ?? ''
  const orgId = params.orgId ?? ''
  const navigateSession = useSessionNavigate()

  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const rootResource = useRootResource()
  const root = useResourceValue(rootResource)

  const validId: QuickstartSpaceCreateId | null = useMemo(
    () => (isQuickstartSpaceCreateId(quickstartId) ? quickstartId : null),
    [quickstartId],
  )

  const backPath = useMemo(() => (orgId ? `org/${orgId}/` : ''), [orgId])

  useEffect(() => {
    if (validId) return
    toast.error('Unknown quickstart: ' + quickstartId)
    navigateSession({ path: backPath, replace: true })
  }, [backPath, navigateSession, quickstartId, validId])

  const [state, dispatch] = useReducer(pipelineReducer, {
    phase: 'create',
    error: null,
  })
  const [runId, setRunId] = useState(0)

  useAbortSignalEffect(
    (signal) => {
      if (!validId || !session || !root) return
      const resources: Array<{ [Symbol.dispose](): void }> = []
      const cleanup: RegisterCleanup = (resource) => {
        if (resource) resources.push(resource)
        return resource
      }
      const disposeAll = () => {
        while (resources.length) {
          const r = resources.pop()
          try {
            r?.[Symbol.dispose]()
          } catch {
            // Dispose errors are non-fatal; swallow to keep cleanup ordered.
          }
        }
      }

      dispatch({ type: 'start' })
      const run = async (): Promise<void> => {
        const spaceResp = await session.createSpace(
          {
            spaceName: getQuickstartSpaceName(validId),
            ...(orgId ? { ownerType: 'organization', ownerId: orgId } : {}),
          },
          signal,
        )
        if (signal.aborted) return
        dispatch({ type: 'advance', to: 'mount' })

        const setup = await createQuickstartSetupFromSession({
          session,
          spaceResp,
          abortSignal: signal,
          cleanup,
        })
        if (signal.aborted) return
        dispatch({ type: 'advance', to: 'populate' })

        await populateSpace(
          validId,
          { session, spaceResp, ...setup } as QuickstartSetup,
          signal,
        )
        if (signal.aborted) return

        const spaceId = spaceResp.sharedObjectRef?.providerResourceRef?.id ?? ''
        if (!spaceId) {
          throw new Error('createSpace did not return a space id')
        }
        dispatch({ type: 'advance', to: 'done' })
        const spacePath = orgId ? `org/${orgId}/so/${spaceId}` : `so/${spaceId}`
        navigateSession({ path: spacePath, replace: true })
      }

      run()
        .catch((err: unknown) => {
          if (signal.aborted) return
          const message =
            err instanceof Error ? err.message : 'Failed to create space'
          dispatch({ type: 'fail', error: message })
        })
        .finally(disposeAll)

      return disposeAll
    },
    [navigateSession, orgId, session, root, validId, runId],
  )

  const handleRetry = useCallback(() => {
    setRunId((n) => n + 1)
  }, [])

  const handleCancel = useCallback(() => {
    // Leave the in-flight SO in the account per design: no deleteSpace.
    navigateSession({ path: backPath })
  }, [backPath, navigateSession])

  if (!validId) return null

  const spaceName = getQuickstartSpaceName(validId)
  const title =
    state.phase === 'failed' ?
      'Failed to create ' + spaceName
    : 'Creating ' + spaceName
  const subtitle =
    state.phase === 'failed' ?
      'Something went wrong during setup.'
    : 'Setting up your new space...'

  const order: Record<Phase, number> = {
    create: 0,
    mount: 1,
    populate: 2,
    done: 3,
    failed: -1,
  }
  const active = state.phase === 'failed' ? -1 : order[state.phase]
  const phases = [
    { label: 'Create', done: active > 0, active: active === 0 },
    { label: 'Mount', done: active > 1, active: active === 1 },
    { label: 'Populate', done: active > 2, active: active === 2 },
  ]

  return (
    <SetupPageLayout title={title} subtitle={subtitle}>
      <PhaseChecklist phases={phases} />

      {state.phase === 'failed' && state.error && (
        <p className="text-destructive text-center text-xs">{state.error}</p>
      )}

      <div className="flex flex-col gap-2">
        {state.phase === 'failed' && (
          <button
            type="button"
            onClick={handleRetry}
            className="border-brand/30 bg-brand/10 hover:bg-brand/20 flex h-10 items-center justify-center rounded-md border text-sm transition-colors"
          >
            Retry
          </button>
        )}
        <button
          type="button"
          onClick={handleCancel}
          className="border-foreground/20 hover:border-foreground/40 flex h-10 items-center justify-center rounded-md border text-sm transition-colors"
        >
          {state.phase === 'failed' ? 'Back to dashboard' : 'Cancel'}
        </button>
      </div>
    </SetupPageLayout>
  )
}
