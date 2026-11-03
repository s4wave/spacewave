import { useCallback, useMemo, useState } from 'react'
import {
  LuArrowLeft,
  LuArrowRight,
  LuCheck,
  LuCopy,
  LuMerge,
  LuMoveRight,
  LuSquare,
  LuSquareCheck,
  LuX,
} from '@s4wave/web/images/icons.js'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import { useNavigate } from '@s4wave/web/router/router.js'
import {
  SessionContext,
  useSessionIndex,
} from '@s4wave/web/contexts/contexts.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { useSessionList } from '@s4wave/app/hooks/useSessionList.js'
import { RadioOption } from '@s4wave/web/ui/RadioOption.js'
import { TransferMode } from '@s4wave/core/provider/transfer/transfer.pb.js'
import { TransferPhase } from '@s4wave/core/provider/transfer/transfer.pb.js'
import type { SpaceSoListEntry } from '@s4wave/core/space/space.pb.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  motion as themeMotion,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    flex: 1,
    flexDirection: 'column',
    overflowY: 'auto',
    backgroundColor: themeColors['--color-background-landing'],
    padding: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@media (min-width: 48rem)': `calc(${themeLayout['--spacing']} * 10)`,
    },
  },
  rule2: {
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-lg)',
  },
  rule3: {
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule4: { marginBottom: `calc(${themeLayout['--spacing']} * 6)` },
  rule5: {
    fontSize: 'var(--text-lg)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
    color: themeColors['--color-foreground'],
  },
  rule6: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule7: {
    overflow: 'hidden',
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    backgroundColor: themeColors['--color-background-get-started'],
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
  },
  rule8: {
    display: 'flex',
    justifyContent: 'flex-end',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule9: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule10: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-brand'],
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule11: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-destructive'],
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      },
    },
  },
  rule12: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule13: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    paddingBlock: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule14: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 4)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule15: {
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule16: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule17: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
  },
  rule18: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 20%, transparent)`,
    opacity: 0.6,
  },
  rule19: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '.25rem',
  },
  rule20: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule21: {
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule22: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule23: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule24: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  rule25: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule26: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule27: {
    marginTop: `calc(${themeLayout['--spacing']} * 1.5)`,
    height: themeLayout['--spacing'],
    overflow: 'hidden',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
  },
  rule28: {
    width: 'var(--progress-width)',
    height: '100%',
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-brand'],
    transitionProperty: 'width',
  },
  rule29: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    paddingBlock: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule30: {
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 12)`,
    height: `calc(${themeLayout['--spacing']} * 12)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule31: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-brand'],
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule5: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-brand'],
  },
  rule6: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-destructive'],
  },
  rule7: {
    width: `calc(${themeLayout['--spacing']} * 6)`,
    height: `calc(${themeLayout['--spacing']} * 6)`,
    color: themeColors['--color-brand'],
  },
})

// WizardStep defines the steps in the transfer wizard.
type WizardStep = 'select' | 'inventory' | 'progress' | 'complete'

// TransferRecovery is the transfer state recovered from a previous run.
interface TransferRecovery {
  sourceIdx: number | null
  targetIdx: number | null
  mode: TransferMode
  // started is true when a transfer is running or was resumed.
  started: boolean
}

// loadTransferRecovery checks for an active transfer or checkpoint (crash
// recovery) and resumes a checkpointed transfer. It returns null when there is
// nothing to recover or the status check fails.
async function loadTransferRecovery(
  session: Session,
  signal: AbortSignal,
): Promise<TransferRecovery | null> {
  try {
    const status = await session.getTransferStatus(signal)
    if (!status.active && !status.hasCheckpoint) return null

    const state = status.state
    const recovery: TransferRecovery = {
      sourceIdx: state?.sourceSessionIndex ?? null,
      targetIdx: state?.targetSessionIndex ?? null,
      mode: state?.mode ?? TransferMode.TransferMode_MERGE,
      started: status.active ?? false,
    }
    if (status.active) return recovery

    // Checkpoint exists: auto-resume the transfer.
    if (state?.sourceSessionIndex && state.targetSessionIndex && state.mode) {
      await session.startTransfer({
        sourceSessionIndex: state.sourceSessionIndex,
        targetSessionIndex: state.targetSessionIndex,
        mode: state.mode,
      })
      recovery.started = true
    }
    return recovery
  } catch {
    // Ignore errors during status check.
    return null
  }
}

// TransferWizard renders the multi-step transfer wizard. It restarts the
// wizard from the recovered transfer state once the recovery check finishes.
export function TransferWizard() {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const { data: recovery } = usePromise(
    useCallback(
      (signal: AbortSignal) =>
        session ? loadTransferRecovery(session, signal) : undefined,
      [session],
    ),
  )

  return (
    <TransferWizardSteps
      key={recovery ? 'recovered' : 'fresh'}
      recovery={recovery ?? null}
    />
  )
}

// useTransferProgress watches the transfer progress once the transfer has
// started. The stream is not connected before the start RPC has returned.
function useTransferProgress(started: boolean) {
  const sessionResource = SessionContext.useContext()
  const progress = useStreamingResource(
    sessionResource,
    (session, signal) =>
      started ? session.watchTransferProgress(signal) : emptyProgress(),
    [started],
  )
  return progress.value?.state ?? null
}

async function* emptyProgress(): AsyncGenerator<never> {}

// useSpaceSelection tracks which of the inventory spaces are selected for
// transfer. Every space starts selected.
function useSpaceSelection(spaces: SpaceSoListEntry[]) {
  const [deselected, setDeselected] = useState<ReadonlySet<string>>(new Set())

  const selectedSpaces = useMemo(
    () =>
      new Set(
        spaces
          .map((sp) => sp.entry?.ref?.providerResourceRef?.id ?? '')
          .filter((id) => !deselected.has(id)),
      ),
    [spaces, deselected],
  )

  const toggleSpace = (id: string) => {
    const next = new Set(deselected)
    if (!next.delete(id)) next.add(id)
    setDeselected(next)
  }

  return { selectedSpaces, toggleSpace }
}

// useTransferDraft holds the source, target, and mode the user is choosing. The
// source defaults to the current session.
function useTransferDraft(recovery: TransferRecovery | null) {
  const currentIdx = useSessionIndex()
  const [sourceIdx, setSourceIdx] = useState<number | null>(
    recovery?.sourceIdx ?? currentIdx ?? null,
  )
  const [targetIdx, setTargetIdx] = useState<number | null>(
    recovery?.targetIdx ?? null,
  )
  const [mode, setMode] = useState<TransferMode>(
    recovery?.mode ?? TransferMode.TransferMode_MERGE,
  )
  return { sourceIdx, setSourceIdx, targetIdx, setTargetIdx, mode, setMode }
}

// useTransferInventory fetches the spaces of the source session while the
// wizard is at the inventory or progress step.
function useTransferInventory(
  session: Session | null | undefined,
  step: WizardStep,
  sourceIdx: number | null,
) {
  const inventoryIdx =
    step === 'inventory' || step === 'progress' ? sourceIdx : null
  const { data: inventory, loading } = usePromise(
    useCallback(
      (signal?: AbortSignal) => {
        if (!session || inventoryIdx == null) return Promise.resolve(null)
        return session.getTransferInventory(inventoryIdx, signal)
      },
      [session, inventoryIdx],
    ),
  )
  const spaces: SpaceSoListEntry[] = useMemo(
    () => inventory?.spaces ?? [],
    [inventory?.spaces],
  )
  return { spaces, loading }
}

// TransferWizardSteps runs the wizard steps from the initial recovery state.
function TransferWizardSteps({
  recovery,
}: {
  recovery: TransferRecovery | null
}) {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const navigate = useNavigate()
  const draft = useTransferDraft(recovery)
  const { sourceIdx, targetIdx, mode } = draft

  const [step, setStep] = useState<WizardStep>(
    recovery?.started ? 'progress' : 'select',
  )
  const [error, setError] = useState<string | null>(null)
  const [transferStarted, setTransferStarted] = useState(
    recovery?.started ?? false,
  )

  const handleBack = () => navigate({ path: '../../' })

  const { spaces, loading: inventoryLoading } = useTransferInventory(
    session,
    step,
    sourceIdx,
  )
  const { selectedSpaces, toggleSpace } = useSpaceSelection(spaces)

  const handleStartTransfer = async () => {
    if (!session || sourceIdx == null || targetIdx == null) return
    setError(null)
    try {
      await session.startTransfer({
        sourceSessionIndex: sourceIdx,
        targetSessionIndex: targetIdx,
        mode,
        spaceIds: [...selectedSpaces],
      })
      setTransferStarted(true)
      setStep('progress')
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  const handleCancel = async () => {
    if (!session) return
    try {
      await session.cancelTransfer()
    } catch {
      // ignore cancel errors
    }
    handleBack()
  }

  // Navigate to target session on complete.
  const handleComplete = () => {
    if (targetIdx != null) {
      navigate({ path: `/u/${targetIdx}/` })
    } else {
      handleBack()
    }
  }

  const transferState = useTransferProgress(transferStarted)
  const { shownStep, shownError } = resolveShownStep(step, error, transferState)

  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <button
          type="button"
          onClick={
            shownStep === 'select' ? handleBack : () => setStep('select')
          }
          disabled={shownStep === 'progress' || shownStep === 'complete'}
          sx={stylexStyles2.rule3}
        >
          <LuArrowLeft sx={stylexStyles.rule1} />
          {shownStep === 'select' ? 'Back to dashboard' : 'Back'}
        </button>

        <div sx={stylexStyles2.rule4}>
          <h1 sx={stylexStyles2.rule5}>Transfer Sessions</h1>
          <p sx={stylexStyles2.rule6}>{stepDescription(shownStep)}</p>
        </div>

        <div sx={stylexStyles2.rule7}>
          <div
            className={
              /* TODO(stylex): a descendant, group, peer or compound selector: space-y-4 */ 'space-y-4 p-6'
            }
          >
            <WizardBody
              step={shownStep}
              draft={draft}
              spaces={spaces}
              inventoryLoading={inventoryLoading}
              selectedSpaces={selectedSpaces}
              onToggleSpace={toggleSpace}
              transferState={transferState}
              error={shownError}
            />
          </div>

          <div sx={stylexStyles2.rule8}>
            <WizardAction
              step={shownStep}
              canProceed={
                sourceIdx != null &&
                targetIdx != null &&
                sourceIdx !== targetIdx
              }
              hasSelection={selectedSpaces.size > 0}
              onNext={() => setStep('inventory')}
              onStart={() => void handleStartTransfer()}
              onCancel={() => void handleCancel()}
              onComplete={handleComplete}
            />
          </div>
        </div>

        {shownError && shownStep !== 'progress' && (
          <p sx={stylexStyles2.rule9}>{shownError}</p>
        )}
      </div>
    </div>
  )
}

type TransferState = ReturnType<typeof useTransferProgress>

// resolveShownStep advances the progress step to complete when the transfer
// finishes, and reports the failure message when it fails.
function resolveShownStep(
  step: WizardStep,
  error: string | null,
  transferState: TransferState,
): { shownStep: WizardStep; shownError: string | null } {
  if (step !== 'progress') return { shownStep: step, shownError: error }

  switch (transferState?.phase) {
    case TransferPhase.TransferPhase_COMPLETE:
      return { shownStep: 'complete', shownError: error }
    case TransferPhase.TransferPhase_FAILED:
      return {
        shownStep: step,
        shownError: transferState.errorMessage ?? 'Transfer failed',
      }
    default:
      return { shownStep: step, shownError: error }
  }
}

// WizardBody renders the content of a wizard step.
function WizardBody({
  step,
  draft,
  spaces,
  inventoryLoading,
  selectedSpaces,
  onToggleSpace,
  transferState,
  error,
}: {
  step: WizardStep
  draft: ReturnType<typeof useTransferDraft>
  spaces: SpaceSoListEntry[]
  inventoryLoading: boolean
  selectedSpaces: ReadonlySet<string>
  onToggleSpace: (id: string) => void
  transferState: TransferState
  error: string | null
}) {
  const sessions = useSessionList().value?.sessions
  const sessionOptions = useMemo(
    () =>
      (sessions ?? []).map((s) => ({
        index: s.sessionIndex ?? 0,
        label: `Session ${s.sessionIndex ?? 0}`,
        providerId: s.sessionRef?.providerResourceRef?.providerId ?? '',
      })),
    [sessions],
  )

  switch (step) {
    case 'select':
      return (
        <SelectStep
          sessionOptions={sessionOptions}
          sourceIdx={draft.sourceIdx}
          targetIdx={draft.targetIdx}
          mode={draft.mode}
          onSourceChange={draft.setSourceIdx}
          onTargetChange={draft.setTargetIdx}
          onModeChange={draft.setMode}
        />
      )
    case 'inventory':
      return (
        <InventoryStep
          spaces={spaces}
          loading={inventoryLoading}
          selectedSpaces={selectedSpaces}
          onToggle={onToggleSpace}
        />
      )
    case 'progress':
      return <ProgressStep transferState={transferState} error={error} />
    case 'complete':
      return (
        <CompleteStep
          spaceCount={transferState?.spaces?.length ?? spaces.length}
        />
      )
  }
}

// stepDescription returns the subtitle for a wizard step.
function stepDescription(step: WizardStep): string {
  switch (step) {
    case 'select':
      return 'Choose source and target sessions.'
    case 'inventory':
      return 'Review spaces to transfer.'
    case 'progress':
      return 'Transfer in progress…'
    case 'complete':
      return 'Transfer complete.'
  }
}

// WizardAction renders the footer button for a wizard step.
function WizardAction({
  step,
  canProceed,
  hasSelection,
  onNext,
  onStart,
  onCancel,
  onComplete,
}: {
  step: WizardStep
  canProceed: boolean
  hasSelection: boolean
  onNext: () => void
  onStart: () => void
  onCancel: () => void
  onComplete: () => void
}) {
  switch (step) {
    case 'select':
      return (
        <button
          type="button"
          onClick={onNext}
          disabled={!canProceed}
          sx={[stylexStyles2.rule10]}
        >
          Next
          <LuArrowRight sx={stylexStyles.rule2} />
        </button>
      )
    case 'inventory':
      return (
        <button
          type="button"
          onClick={onStart}
          disabled={!hasSelection}
          sx={[stylexStyles2.rule10]}
        >
          <LuMerge sx={stylexStyles.rule2} />
          Start Transfer
        </button>
      )
    case 'progress':
      return (
        <button type="button" onClick={onCancel} sx={[stylexStyles2.rule11]}>
          <LuX sx={stylexStyles.rule2} />
          Cancel
        </button>
      )
    case 'complete':
      return (
        <button type="button" onClick={onComplete} sx={[stylexStyles2.rule10]}>
          <LuCheck sx={stylexStyles.rule2} />
          Go to Session
        </button>
      )
  }
}

// SelectStep renders the source/target session picker and mode selector.
function SelectStep({
  sessionOptions,
  sourceIdx,
  targetIdx,
  mode,
  onSourceChange,
  onTargetChange,
  onModeChange,
}: {
  sessionOptions: { index: number; label: string; providerId: string }[]
  sourceIdx: number | null
  targetIdx: number | null
  mode: TransferMode
  onSourceChange: (idx: number) => void
  onTargetChange: (idx: number) => void
  onModeChange: (mode: TransferMode) => void
}) {
  return (
    <>
      <div>
        <span sx={stylexStyles2.rule12}>Source session (merge from)</span>
        <div>
          {sessionOptions.map((s) => (
            <RadioOption
              sx={nativeChildSpacing.s1_5}
              key={`src-${s.index}`}
              selected={sourceIdx === s.index}
              onSelect={() => onSourceChange(s.index)}
              label={s.label}
              description={
                s.providerId === 'local' ? 'Local storage' : 'Spacewave Cloud'
              }
            />
          ))}
        </div>
      </div>

      <div>
        <span sx={stylexStyles2.rule12}>Target session (merge into)</span>
        <div>
          {sessionOptions.flatMap((s) =>
            s.index !== sourceIdx
              ? [
                  <RadioOption
                    sx={nativeChildSpacing.s1_5}
                    key={`tgt-${s.index}`}
                    selected={targetIdx === s.index}
                    onSelect={() => onTargetChange(s.index)}
                    label={s.label}
                    description={
                      s.providerId === 'local'
                        ? 'Local storage'
                        : 'Spacewave Cloud'
                    }
                  />,
                ]
              : [],
          )}
        </div>
      </div>

      <div>
        <span sx={stylexStyles2.rule12}>Transfer mode</span>
        <div>
          <RadioOption
            sx={nativeChildSpacing.s1_5}
            selected={mode === TransferMode.TransferMode_MERGE}
            onSelect={() => onModeChange(TransferMode.TransferMode_MERGE)}
            icon={<LuMerge sx={stylexStyles.rule1} />}
            label="Merge"
            description="Move all spaces to the target and delete the source session"
          />
          <RadioOption
            sx={nativeChildSpacing.s1_5}
            selected={mode === TransferMode.TransferMode_MIGRATE}
            onSelect={() => onModeChange(TransferMode.TransferMode_MIGRATE)}
            icon={<LuMoveRight sx={stylexStyles.rule1} />}
            label="Migrate"
            description="Move all spaces to a different provider and transfer the keypair"
          />
          <RadioOption
            sx={nativeChildSpacing.s1_5}
            selected={mode === TransferMode.TransferMode_MIRROR}
            onSelect={() => onModeChange(TransferMode.TransferMode_MIRROR)}
            icon={<LuCopy sx={stylexStyles.rule1} />}
            label="Mirror"
            description="Copy all spaces to the target without deleting the source"
          />
        </div>
      </div>
    </>
  )
}

// InventoryStep shows the spaces that will be transferred with selection checkboxes.
function InventoryStep({
  spaces,
  loading,
  selectedSpaces,
  onToggle,
}: {
  spaces: SpaceSoListEntry[]
  loading: boolean
  selectedSpaces: ReadonlySet<string>
  onToggle: (id: string) => void
}) {
  if (loading) {
    return (
      <div sx={stylexStyles2.rule13}>
        <Spinner size="md" variant="muted" />
      </div>
    )
  }

  if (spaces.length === 0) {
    return (
      <p sx={stylexStyles2.rule14}>No spaces found on the source session.</p>
    )
  }

  const selectedCount = selectedSpaces.size

  return (
    <div>
      <p sx={stylexStyles2.rule15}>
        {selectedCount} of {spaces.length} space
        {spaces.length !== 1 ? 's' : ''} selected for transfer:
      </p>
      <div>
        {spaces.map((sp) => {
          const id = sp.entry?.ref?.providerResourceRef?.id ?? ''
          const name = sp.spaceMeta?.name || id || 'Unnamed'
          const checked = selectedSpaces.has(id)
          return (
            <button
              key={id}
              type="button"
              onClick={() => onToggle(id)}
              sx={[
                nativeChildSpacing.s1_5,
                [
                  stylexStyles2.rule16,
                  checked ? stylexStyles2.rule17 : stylexStyles2.rule18,
                ],
              ]}
            >
              <div sx={stylexStyles2.rule19}>
                {checked ? (
                  <LuSquareCheck sx={stylexStyles.rule3} />
                ) : (
                  <LuSquare sx={stylexStyles.rule4} />
                )}
              </div>
              <p sx={stylexStyles2.rule20}>{name}</p>
            </button>
          )
        })}
      </div>
    </div>
  )
}

// phaseLabel returns a human-readable label for a transfer phase.
function phaseLabel(phase: TransferPhase): string {
  switch (phase) {
    case TransferPhase.TransferPhase_IDLE:
      return 'Waiting'
    case TransferPhase.TransferPhase_SCANNING:
      return 'Scanning'
    case TransferPhase.TransferPhase_COPYING_BLOCKS:
      return 'Copying blocks'
    case TransferPhase.TransferPhase_COPYING_SO:
      return 'Copying data'
    case TransferPhase.TransferPhase_CLEANUP:
      return 'Cleaning up'
    case TransferPhase.TransferPhase_COMPLETE:
      return 'Complete'
    case TransferPhase.TransferPhase_FAILED:
      return 'Failed'
    default:
      return 'Unknown'
  }
}

// ProgressStep shows the transfer progress with per-space details.
function ProgressStep({
  transferState,
  error,
}: {
  transferState:
    | {
        phase?: TransferPhase
        spaces?: {
          sharedObjectId?: string
          phase?: TransferPhase
          blocksCopied?: bigint
          blocksTotal?: bigint
          meta?: { bodyType?: string; bodyMeta?: Uint8Array }
        }[]
        errorMessage?: string
      }
    | null
    | undefined
  error: string | null
}) {
  const phase = transferState?.phase ?? TransferPhase.TransferPhase_IDLE
  const spaceStates = transferState?.spaces ?? []

  return (
    <div>
      <div sx={stylexStyles2.rule21}>
        {phase !== TransferPhase.TransferPhase_COMPLETE &&
          phase !== TransferPhase.TransferPhase_FAILED && (
            <Spinner variant="brand" />
          )}
        {phase === TransferPhase.TransferPhase_COMPLETE && (
          <LuCheck sx={stylexStyles.rule5} />
        )}
        {phase === TransferPhase.TransferPhase_FAILED && (
          <LuX sx={stylexStyles.rule6} />
        )}
        <p sx={stylexStyles2.rule22}>{phaseLabel(phase)}</p>
      </div>

      {spaceStates.length > 0 && (
        <div>
          {spaceStates.map((sp) => {
            const spPhase = sp.phase ?? TransferPhase.TransferPhase_IDLE
            const copied = Number(sp.blocksCopied ?? 0n)
            const total = Number(sp.blocksTotal ?? 0n)
            const pct = total > 0 ? Math.round((copied / total) * 100) : 0

            return (
              <div
                key={sp.sharedObjectId}
                sx={[nativeChildSpacing.s2, stylexStyles2.rule23]}
              >
                <div sx={stylexStyles2.rule24}>
                  <p sx={stylexStyles2.rule25}>{sp.sharedObjectId}</p>
                  <p sx={stylexStyles2.rule26}>{phaseLabel(spPhase)}</p>
                </div>
                {total > 0 && (
                  <div sx={stylexStyles2.rule27}>
                    <div
                      sx={stylexStyles2.rule28}
                      style={{ '--progress-width': `${pct}%` }}
                    />
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}

      {error && <p sx={stylexStyles2.rule9}>{error}</p>}
    </div>
  )
}

// CompleteStep shows the transfer completion message.
function CompleteStep({ spaceCount }: { spaceCount: number }) {
  return (
    <div sx={stylexStyles2.rule29}>
      <div sx={stylexStyles2.rule30}>
        <LuCheck sx={stylexStyles.rule7} />
      </div>
      <p sx={stylexStyles2.rule22}>Transfer complete</p>
      <p sx={stylexStyles2.rule31}>
        {spaceCount} space{spaceCount !== 1 ? 's' : ''} transferred
        successfully.
      </p>
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s1_5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
