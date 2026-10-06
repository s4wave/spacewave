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
} from 'react-icons/lu'

import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { cn } from '@s4wave/web/style/utils.js'
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
    <div className="bg-background-landing flex flex-1 flex-col overflow-y-auto p-6 md:p-10">
      <div className="mx-auto w-full max-w-lg">
        <button
          type="button"
          onClick={
            shownStep === 'select' ? handleBack : () => setStep('select')
          }
          disabled={shownStep === 'progress' || shownStep === 'complete'}
          className="text-foreground-alt hover:text-foreground mb-6 flex items-center gap-1.5 text-sm transition-colors disabled:opacity-50"
        >
          <LuArrowLeft className="size-4" />
          {shownStep === 'select' ? 'Back to dashboard' : 'Back'}
        </button>

        <div className="mb-6">
          <h1 className="text-foreground text-lg font-semibold tracking-wide">
            Transfer Sessions
          </h1>
          <p className="text-foreground-alt mt-1 text-sm">
            {stepDescription(shownStep)}
          </p>
        </div>

        <div className="border-foreground/20 bg-background-get-started overflow-hidden rounded-lg border shadow-lg backdrop-blur-sm">
          <div className="space-y-4 p-6">
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

          <div className="border-foreground/10 flex justify-end gap-2 border-t p-4">
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
          <p className="text-destructive mt-3 text-sm">{shownError}</p>
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

const wizardPrimaryActionClass = cn(
  'flex items-center gap-1.5 rounded-md px-4 py-2 text-sm font-medium transition-all',
  'bg-brand/10 text-brand border-brand/30 border',
  'hover:bg-brand/20',
  'disabled:cursor-not-allowed disabled:opacity-50',
)

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
          className={wizardPrimaryActionClass}
        >
          Next
          <LuArrowRight className="size-3.5" />
        </button>
      )
    case 'inventory':
      return (
        <button
          type="button"
          onClick={onStart}
          disabled={!hasSelection}
          className={wizardPrimaryActionClass}
        >
          <LuMerge className="size-3.5" />
          Start Transfer
        </button>
      )
    case 'progress':
      return (
        <button
          type="button"
          onClick={onCancel}
          className={cn(
            'flex items-center gap-1.5 rounded-md px-4 py-2 text-sm font-medium transition-all',
            'text-destructive border-destructive/30 border',
            'hover:bg-destructive/10',
          )}
        >
          <LuX className="size-3.5" />
          Cancel
        </button>
      )
    case 'complete':
      return (
        <button
          type="button"
          onClick={onComplete}
          className={wizardPrimaryActionClass}
        >
          <LuCheck className="size-3.5" />
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
        <span className="text-foreground mb-2 block text-xs font-medium">
          Source session (merge from)
        </span>
        <div className="space-y-1.5">
          {sessionOptions.map((s) => (
            <RadioOption
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
        <span className="text-foreground mb-2 block text-xs font-medium">
          Target session (merge into)
        </span>
        <div className="space-y-1.5">
          {sessionOptions.flatMap((s) =>
            s.index !== sourceIdx
              ? [
                  <RadioOption
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
        <span className="text-foreground mb-2 block text-xs font-medium">
          Transfer mode
        </span>
        <div className="space-y-1.5">
          <RadioOption
            selected={mode === TransferMode.TransferMode_MERGE}
            onSelect={() => onModeChange(TransferMode.TransferMode_MERGE)}
            icon={<LuMerge className="size-4" />}
            label="Merge"
            description="Move all spaces to the target and delete the source session"
          />
          <RadioOption
            selected={mode === TransferMode.TransferMode_MIGRATE}
            onSelect={() => onModeChange(TransferMode.TransferMode_MIGRATE)}
            icon={<LuMoveRight className="size-4" />}
            label="Migrate"
            description="Move all spaces to a different provider and transfer the keypair"
          />
          <RadioOption
            selected={mode === TransferMode.TransferMode_MIRROR}
            onSelect={() => onModeChange(TransferMode.TransferMode_MIRROR)}
            icon={<LuCopy className="size-4" />}
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
      <div className="flex items-center justify-center py-8">
        <Spinner size="md" variant="muted" />
      </div>
    )
  }

  if (spaces.length === 0) {
    return (
      <p className="text-foreground-alt py-4 text-center text-sm">
        No spaces found on the source session.
      </p>
    )
  }

  const selectedCount = selectedSpaces.size

  return (
    <div>
      <p className="text-foreground-alt mb-3 text-xs">
        {selectedCount} of {spaces.length} space
        {spaces.length !== 1 ? 's' : ''} selected for transfer:
      </p>
      <div className="space-y-1.5">
        {spaces.map((sp) => {
          const id = sp.entry?.ref?.providerResourceRef?.id ?? ''
          const name = sp.spaceMeta?.name || id || 'Unnamed'
          const checked = selectedSpaces.has(id)
          return (
            <button
              key={id}
              type="button"
              onClick={() => onToggle(id)}
              className={cn(
                'border-foreground/10 flex w-full items-center gap-3 rounded-md border p-2.5 text-left transition-colors',
                checked ? 'bg-brand/5' : 'bg-background/20 opacity-60',
              )}
            >
              <div className="flex size-8 shrink-0 items-center justify-center rounded">
                {checked ? (
                  <LuSquareCheck className="text-brand size-5" />
                ) : (
                  <LuSquare className="text-foreground-alt size-5" />
                )}
              </div>
              <p className="text-foreground text-sm">{name}</p>
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
      <div className="mb-4 flex items-center gap-2">
        {phase !== TransferPhase.TransferPhase_COMPLETE &&
          phase !== TransferPhase.TransferPhase_FAILED && (
            <Spinner variant="brand" />
          )}
        {phase === TransferPhase.TransferPhase_COMPLETE && (
          <LuCheck className="text-brand size-4" />
        )}
        {phase === TransferPhase.TransferPhase_FAILED && (
          <LuX className="text-destructive size-4" />
        )}
        <p className="text-foreground text-sm font-medium">
          {phaseLabel(phase)}
        </p>
      </div>

      {spaceStates.length > 0 && (
        <div className="space-y-2">
          {spaceStates.map((sp) => {
            const spPhase = sp.phase ?? TransferPhase.TransferPhase_IDLE
            const copied = Number(sp.blocksCopied ?? 0n)
            const total = Number(sp.blocksTotal ?? 0n)
            const pct = total > 0 ? Math.round((copied / total) * 100) : 0

            return (
              <div
                key={sp.sharedObjectId}
                className="border-foreground/10 rounded-md border p-2.5"
              >
                <div className="flex items-center justify-between">
                  <p className="text-foreground text-xs">{sp.sharedObjectId}</p>
                  <p className="text-foreground-alt text-xs">
                    {phaseLabel(spPhase)}
                  </p>
                </div>
                {total > 0 && (
                  <div className="bg-foreground/10 mt-1.5 h-1 overflow-hidden rounded-full">
                    <div
                      className="bg-brand progress-width transition-width h-full rounded-full"
                      style={{ '--progress-width': `${pct}%` }}
                    />
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}

      {error && <p className="text-destructive mt-3 text-sm">{error}</p>}
    </div>
  )
}

// CompleteStep shows the transfer completion message.
function CompleteStep({ spaceCount }: { spaceCount: number }) {
  return (
    <div className="flex flex-col items-center py-6">
      <div className="bg-brand/10 mb-4 flex size-12 items-center justify-center rounded-full">
        <LuCheck className="text-brand size-6" />
      </div>
      <p className="text-foreground text-sm font-medium">Transfer complete</p>
      <p className="text-foreground-alt mt-1 text-xs">
        {spaceCount} space{spaceCount !== 1 ? 's' : ''} transferred
        successfully.
      </p>
    </div>
  )
}
