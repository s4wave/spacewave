/* eslint-disable react-doctor/rerender-state-only-in-handlers */
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
import { accountStorageChoice } from '@s4wave/app/session/storage/storage-backend.js'

import {
  getQuickstartOption,
  isQuickstartId,
  type QuickstartOption,
  type QuickstartSpaceCreateId,
} from './options.js'
import {
  buildQuickstartSpaceRoutePath,
  createQuickstartSetupFromSession,
  executeDynamicQuickstart,
  getQuickstartSpaceName,
  populateSpace,
  type QuickstartSetup,
} from './create.js'
import { useVisibleQuickstartOptions } from './useQuickstartOptions.js'

import * as stylex from '@stylexjs/stylex'
import {
  fonts as themeFonts,
  colors as themeColors,
  layout as themeLayout,
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexDirection: 'column',
  },
  rule3: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    alignItems: 'center',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule4: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 40%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    alignItems: 'center',
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
  },
})

function isQuickstartSpaceCreateId(id: string): id is QuickstartSpaceCreateId {
  if (!isQuickstartId(id)) return false
  if (id === 'account' || id === 'pair' || id === 'local') return false
  const opt = getQuickstartOption(id)
  return !opt.path
}

function isSpaceCreatingOption(opt: QuickstartOption): boolean {
  return (
    !opt.path && opt.id !== 'account' && opt.id !== 'pair' && opt.id !== 'local'
  )
}

// storagePlacement maps the route's storage choice to the createSpace fields.
// No choice leaves the account's default storage backend in effect.
function storagePlacement(storage: string) {
  if (!storage) return {}
  if (storage === accountStorageChoice) return { accountStorage: true }
  return { storageBackendId: storage }
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

interface CreateSpacePipelineParams {
  valid: boolean
  quickstartId: string
  isDynamic: boolean
  staticQuickstartId: QuickstartSpaceCreateId | null
  spaceName: string
  orgId: string
  storage: string
}

/**
 * useCreateSpacePipeline runs create, mount, and populate for the selected
 * quickstart, then navigates to the new space. retry restarts a failed run.
 */
function useCreateSpacePipeline({
  valid,
  quickstartId,
  isDynamic,
  staticQuickstartId,
  spaceName,
  orgId,
  storage,
}: CreateSpacePipelineParams) {
  const navigateSession = useSessionNavigate()
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const rootResource = useRootResource()
  const root = useResourceValue(rootResource)
  const [state, dispatch] = useReducer(pipelineReducer, {
    phase: 'create',
    error: null,
  })
  const [runId, setRunId] = useState(0)

  useAbortSignalEffect(
    (signal) => {
      if (!valid || !quickstartId || !session || !root) return
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
        // eslint-disable-next-line react-doctor/async-defer-await
        const spaceResp = await session.createSpace(
          {
            spaceName,
            ...(orgId ? { ownerType: 'organization', ownerId: orgId } : {}),
            ...storagePlacement(storage),
          },
          signal,
        )
        if (signal.aborted) return
        const spaceId = spaceResp.sharedObjectRef?.providerResourceRef?.id ?? ''
        if (!spaceId) {
          throw new Error('createSpace did not return a space id')
        }
        dispatch({ type: 'advance', to: 'mount' })

        // eslint-disable-next-line react-doctor/async-defer-await
        const setup = await createQuickstartSetupFromSession({
          session,
          spaceResp,
          abortSignal: signal,
          cleanup,
          mountContents: staticQuickstartId !== 'drive',
        })
        if (signal.aborted) return
        dispatch({ type: 'advance', to: 'populate' })

        const fullSetup: QuickstartSetup = {
          root,
          session,
          spaceResp,
          ...setup,
        }
        if (isDynamic) {
          await executeDynamicQuickstart(root, quickstartId, fullSetup, signal)
        } else if (staticQuickstartId) {
          await populateSpace(staticQuickstartId, fullSetup, signal)
        }
        if (signal.aborted) return

        dispatch({ type: 'advance', to: 'done' })
        const spaceBasePath = orgId
          ? `org/${orgId}/so/${spaceId}`
          : `so/${spaceId}`
        const spacePath = staticQuickstartId
          ? buildQuickstartSpaceRoutePath(spaceBasePath, staticQuickstartId)
          : spaceBasePath
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
    [
      isDynamic,
      navigateSession,
      orgId,
      quickstartId,
      root,
      runId,
      session,
      spaceName,
      staticQuickstartId,
      storage,
      valid,
    ],
  )

  const retry = useCallback(() => {
    setRunId((n) => n + 1)
  }, [])

  return { state, retry }
}

const phaseOrder: Record<Phase, number> = {
  create: 0,
  mount: 1,
  populate: 2,
  done: 3,
  failed: -1,
}

/** pipelinePhases returns the checklist rows for the pipeline's current phase. */
function pipelinePhases(phase: Phase) {
  const active = phaseOrder[phase]
  return [
    { label: 'Create', done: active > 0, active: active === 0 },
    { label: 'Mount', done: active > 1, active: active === 1 },
    { label: 'Populate', done: active > 2, active: active === 2 },
  ]
}

interface CreateSpaceProgressProps {
  state: PipelineState
  spaceName: string
  onRetry: () => void
  onCancel: () => void
}

/** CreateSpaceProgress renders the phase checklist with the retry and cancel actions. */
function CreateSpaceProgress({
  state,
  spaceName,
  onRetry,
  onCancel,
}: CreateSpaceProgressProps) {
  const failed = state.phase === 'failed'

  return (
    <SetupPageLayout
      title={(failed ? 'Failed to create ' : 'Creating ') + spaceName}
      subtitle={
        failed
          ? 'Something went wrong during setup.'
          : 'Setting up your new space...'
      }
    >
      <PhaseChecklist phases={pipelinePhases(state.phase)} />

      {failed && state.error && <p sx={stylexStyles.rule1}>{state.error}</p>}

      <div sx={stylexStyles.rule2}>
        {failed && (
          <button type="button" onClick={onRetry} sx={stylexStyles.rule3}>
            Retry
          </button>
        )}
        <button type="button" onClick={onCancel} sx={stylexStyles.rule4}>
          {failed ? 'Back to dashboard' : 'Cancel'}
        </button>
      </div>
    </SetupPageLayout>
  )
}

// CreateSpaceRoute owns the full create-space pipeline for quickstart tiles
// and renders a chromeless full-screen loading UI with phase progress.
export function CreateSpaceRoute() {
  const params = useParams()
  const quickstartId = params.quickstartId ?? ''
  const orgId = params.orgId ?? ''
  const storage = params.storage ?? ''
  const navigateSession = useSessionNavigate()
  const quickstartOptions = useVisibleQuickstartOptions()
  const backPath = orgId ? `org/${orgId}/` : ''

  const selectedOption = useMemo(
    () =>
      quickstartOptions.find(
        (opt) => opt.id === quickstartId && isSpaceCreatingOption(opt),
      ) ?? null,
    [quickstartId, quickstartOptions],
  )
  const staticId: QuickstartSpaceCreateId | null = isQuickstartSpaceCreateId(
    quickstartId,
  )
    ? quickstartId
    : null
  const valid = !!selectedOption && (!!selectedOption.dynamic || !!staticId)
  const spaceName = staticId
    ? getQuickstartSpaceName(staticId)
    : selectedOption?.spaceName || selectedOption?.name || ''

  useEffect(() => {
    if (valid) return
    toast.error('Unknown quickstart: ' + quickstartId)
    navigateSession({ path: backPath, replace: true })
  }, [backPath, navigateSession, quickstartId, valid])

  const { state, retry } = useCreateSpacePipeline({
    valid,
    quickstartId: selectedOption?.id ?? '',
    isDynamic: selectedOption?.dynamic ?? false,
    staticQuickstartId: staticId,
    spaceName,
    orgId,
    storage,
  })

  const handleCancel = useCallback(() => {
    // Leave the in-flight SO in the account per design: no deleteSpace.
    navigateSession({ path: backPath })
  }, [backPath, navigateSession])

  if (!valid) return null

  return (
    <CreateSpaceProgress
      state={state}
      spaceName={spaceName}
      onRetry={retry}
      onCancel={handleCancel}
    />
  )
}
