import { useCallback, useEffect, useMemo, useState } from 'react'
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
import { useSessionList } from '@s4wave/app/hooks/useSessionList.js'
import { RadioOption } from '@s4wave/web/ui/RadioOption.js'
import { TransferMode } from '@s4wave/core/provider/transfer/transfer.pb.js'
import { TransferPhase } from '@s4wave/core/provider/transfer/transfer.pb.js'
import type { SpaceSoListEntry } from '@s4wave/core/space/space.pb.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'

// WizardStep defines the steps in the transfer wizard.
type WizardStep = 'select' | 'inventory' | 'progress' | 'complete'

// TransferWizard renders the multi-step transfer wizard.
export function TransferWizard() {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const navigate = useNavigate()
  const currentIdx = useSessionIndex()
  const sessionsResource = useSessionList()
  const sessions = useMemo(
    () => sessionsResource.value?.sessions ?? [],
    [sessionsResource.value?.sessions],
  )

  const [step, setStep] = useState<WizardStep>('select')
  const [sourceIdx, setSourceIdx] = useState<number | null>(null)
  const [targetIdx, setTargetIdx] = useState<number | null>(null)
  const [mode, setMode] = useState<TransferMode>(
    TransferMode.TransferMode_MERGE,
  )
  const [error, setError] = useState<string | null>(null)
  const [transferStarted, setTransferStarted] = useState(false)
  const [selectedSpaces, setSelectedSpaces] = useState<Set<string> | null>(null)

  // Default source to current session.
  useEffect(() => {
    if (currentIdx != null && sourceIdx === null) {
      setSourceIdx(currentIdx)
    }
  }, [currentIdx, sourceIdx])

  // Check for active transfer or checkpoint on mount (crash recovery).
  useEffect(() => {
    if (!session) return
    const abort = new AbortController()
    void (async () => {
      try {
        const status = await session.getTransferStatus(abort.signal)
        if (abort.signal.aborted) return
        if (status.active || status.hasCheckpoint) {
          const state = status.state
          if (state) {
            setSourceIdx(state.sourceSessionIndex ?? null)
            setTargetIdx(state.targetSessionIndex ?? null)
            setMode(state.mode ?? TransferMode.TransferMode_MERGE)
          }
          if (status.active) {
            setTransferStarted(true)
            setStep('progress')
          } else {
            // Checkpoint exists: auto-resume the transfer.
            if (
              state?.sourceSessionIndex &&
              state?.targetSessionIndex &&
              state?.mode
            ) {
              await session.startTransfer({
                sourceSessionIndex: state.sourceSessionIndex,
                targetSessionIndex: state.targetSessionIndex,
                mode: state.mode,
              })
              setTransferStarted(true)
              setStep('progress')
            }
          }
        }
      } catch {
        // Ignore errors during status check.
      }
    })()
    return () => abort.abort()
  }, [session])

  const handleBack = useCallback(() => {
    navigate({ path: '../../' })
  }, [navigate])

  const handleStepBack = useCallback(() => {
    if (step === 'inventory') setStep('select')
  }, [step])

  // Inventory: fetch spaces for the source session.
  const inventoryIdx =
    step === 'inventory' || step === 'progress' ? sourceIdx : null
  const { data: inventory, loading: inventoryLoading } = usePromise(
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

  // Initialize selected spaces when inventory loads.
  useEffect(() => {
    if (spaces.length > 0 && selectedSpaces === null) {
      setSelectedSpaces(
        new Set(
          spaces.map((sp) => sp.entry?.ref?.providerResourceRef?.id ?? ''),
        ),
      )
    }
  }, [spaces, selectedSpaces])

  const toggleSpace = useCallback((id: string) => {
    setSelectedSpaces((prev) => {
      if (!prev) return prev
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }, [])

  // Start transfer.
  const handleStartTransfer = useCallback(async () => {
    if (!session || sourceIdx == null || targetIdx == null) return
    setError(null)
    try {
      const spaceIds = selectedSpaces ? [...selectedSpaces] : []
      await session.startTransfer({
        sourceSessionIndex: sourceIdx,
        targetSessionIndex: targetIdx,
        mode,
        spaceIds,
      })
      setTransferStarted(true)
      setStep('progress')
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }, [session, sourceIdx, targetIdx, mode, selectedSpaces])

  const handleStartTransferClick = useCallback(() => {
    void handleStartTransfer()
  }, [handleStartTransfer])

  // Watch transfer progress (only after transfer has been started).
  // Uses a separate state to hold the latest transfer state so we avoid
  // connecting the stream before the RPC has returned.
  const [transferState, setTransferState] = useState<{
    phase?: TransferPhase
    spaces?: {
      sharedObjectId?: string
      phase?: TransferPhase
      blocksCopied?: bigint
      blocksTotal?: bigint
      meta?: { bodyType?: string; bodyMeta?: Uint8Array }
    }[]
    errorMessage?: string
  } | null>(null)

  useEffect(() => {
    if (!transferStarted || !session) return
    const abort = new AbortController()
    void (async () => {
      try {
        for await (const msg of session.watchTransferProgress(abort.signal)) {
          if (abort.signal.aborted) break
          setTransferState(msg.state ?? null)
        }
      } catch {
        if (abort.signal.aborted) return
      }
    })()
    return () => abort.abort()
  }, [transferStarted, session])

  const overallPhase = transferState?.phase ?? TransferPhase.TransferPhase_IDLE

  // Auto-advance to complete step.
  useEffect(() => {
    if (
      overallPhase === TransferPhase.TransferPhase_COMPLETE &&
      step === 'progress'
    ) {
      setStep('complete')
    }
    if (
      overallPhase === TransferPhase.TransferPhase_FAILED &&
      step === 'progress'
    ) {
      setError(transferState?.errorMessage ?? 'Transfer failed')
    }
  }, [overallPhase, step, transferState])

  // Cancel transfer.
  const handleCancel = useCallback(async () => {
    if (!session) return
    try {
      await session.cancelTransfer()
    } catch {
      // ignore cancel errors
    }
    handleBack()
  }, [session, handleBack])

  const handleCancelClick = useCallback(() => {
    void handleCancel()
  }, [handleCancel])

  // Navigate to target session on complete.
  const handleComplete = useCallback(() => {
    if (targetIdx != null) {
      navigate({ path: `/u/${targetIdx}/` })
    } else {
      handleBack()
    }
  }, [targetIdx, navigate, handleBack])

  // Resolve session display names.
  const sessionOptions = useMemo(
    () =>
      sessions.map((s) => ({
        index: s.sessionIndex ?? 0,
        label: `Session ${s.sessionIndex ?? 0}`,
        providerId: s.sessionRef?.providerResourceRef?.providerId ?? '',
      })),
    [sessions],
  )

  const canProceedToInventory =
    sourceIdx != null && targetIdx != null && sourceIdx !== targetIdx

  const selectedCount = selectedSpaces?.size ?? 0

  return (
    <div className="bg-background-landing flex flex-1 flex-col overflow-y-auto p-6 md:p-10">
      <div className="mx-auto w-full max-w-lg">
        <button
          onClick={step === 'select' ? handleBack : handleStepBack}
          disabled={step === 'progress' || step === 'complete'}
          className="text-foreground-alt hover:text-foreground mb-6 flex items-center gap-1.5 text-sm transition-colors disabled:opacity-50"
        >
          <LuArrowLeft className="h-4 w-4" />
          {step === 'select' ? 'Back to dashboard' : 'Back'}
        </button>

        <div className="mb-6">
          <h1 className="text-foreground text-lg font-bold tracking-wide">
            Transfer Sessions
          </h1>
          <p className="text-foreground-alt mt-1 text-sm">
            {step === 'select' && 'Choose source and target sessions.'}
            {step === 'inventory' && 'Review spaces to transfer.'}
            {step === 'progress' && 'Transfer in progress...'}
            {step === 'complete' && 'Transfer complete.'}
          </p>
        </div>

        <div className="border-foreground/20 bg-background-get-started overflow-hidden rounded-lg border shadow-lg backdrop-blur-sm">
          <div className="space-y-4 p-6">
            {step === 'select' && (
              <SelectStep
                sessionOptions={sessionOptions}
                sourceIdx={sourceIdx}
                targetIdx={targetIdx}
                mode={mode}
                onSourceChange={setSourceIdx}
                onTargetChange={setTargetIdx}
                onModeChange={setMode}
              />
            )}

            {step === 'inventory' && (
              <InventoryStep
                spaces={spaces}
                loading={inventoryLoading}
                selectedSpaces={selectedSpaces}
                onToggle={toggleSpace}
              />
            )}

            {step === 'progress' && (
              <ProgressStep transferState={transferState} error={error} />
            )}

            {step === 'complete' && (
              <CompleteStep
                spaceCount={transferState?.spaces?.length ?? spaces.length}
              />
            )}
          </div>

          <div className="border-foreground/10 flex justify-end gap-2 border-t p-4">
            {step === 'select' && (
              <button
                onClick={() => setStep('inventory')}
                disabled={!canProceedToInventory}
                className={cn(
                  'flex items-center gap-1.5 rounded-md px-4 py-2 text-sm font-medium transition-all',
                  'bg-brand/10 text-brand border-brand/30 border',
                  'hover:bg-brand/20',
                  'disabled:cursor-not-allowed disabled:opacity-50',
                )}
              >
                Next
                <LuArrowRight className="h-3.5 w-3.5" />
              </button>
            )}

            {step === 'inventory' && (
              <button
                onClick={handleStartTransferClick}
                disabled={selectedCount === 0}
                className={cn(
                  'flex items-center gap-1.5 rounded-md px-4 py-2 text-sm font-medium transition-all',
                  'bg-brand/10 text-brand border-brand/30 border',
                  'hover:bg-brand/20',
                  'disabled:cursor-not-allowed disabled:opacity-50',
                )}
              >
                <LuMerge className="h-3.5 w-3.5" />
                Start Transfer
              </button>
            )}

            {step === 'progress' && (
              <button
                onClick={handleCancelClick}
                className={cn(
                  'flex items-center gap-1.5 rounded-md px-4 py-2 text-sm font-medium transition-all',
                  'text-destructive border-destructive/30 border',
                  'hover:bg-destructive/10',
                )}
              >
                <LuX className="h-3.5 w-3.5" />
                Cancel
              </button>
            )}

            {step === 'complete' && (
              <button
                onClick={handleComplete}
                className={cn(
                  'flex items-center gap-1.5 rounded-md px-4 py-2 text-sm font-medium transition-all',
                  'bg-brand/10 text-brand border-brand/30 border',
                  'hover:bg-brand/20',
                )}
              >
                <LuCheck className="h-3.5 w-3.5" />
                Go to Session
              </button>
            )}
          </div>
        </div>

        {error && step !== 'progress' && (
          <p className="text-destructive mt-3 text-sm">{error}</p>
        )}
      </div>
    </div>
  )
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
        <label className="text-foreground mb-2 block text-xs font-medium">
          Source session (merge from)
        </label>
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
        <label className="text-foreground mb-2 block text-xs font-medium">
          Target session (merge into)
        </label>
        <div className="space-y-1.5">
          {sessionOptions
            .filter((s) => s.index !== sourceIdx)
            .map((s) => (
              <RadioOption
                key={`tgt-${s.index}`}
                selected={targetIdx === s.index}
                onSelect={() => onTargetChange(s.index)}
                label={s.label}
                description={
                  s.providerId === 'local' ? 'Local storage' : 'Spacewave Cloud'
                }
              />
            ))}
        </div>
      </div>

      <div>
        <label className="text-foreground mb-2 block text-xs font-medium">
          Transfer mode
        </label>
        <div className="space-y-1.5">
          <RadioOption
            selected={mode === TransferMode.TransferMode_MERGE}
            onSelect={() => onModeChange(TransferMode.TransferMode_MERGE)}
            icon={<LuMerge className="h-4 w-4" />}
            label="Merge"
            description="Move all spaces to the target and delete the source session"
          />
          <RadioOption
            selected={mode === TransferMode.TransferMode_MIGRATE}
            onSelect={() => onModeChange(TransferMode.TransferMode_MIGRATE)}
            icon={<LuMoveRight className="h-4 w-4" />}
            label="Migrate"
            description="Move all spaces to a different provider and transfer the keypair"
          />
          <RadioOption
            selected={mode === TransferMode.TransferMode_MIRROR}
            onSelect={() => onModeChange(TransferMode.TransferMode_MIRROR)}
            icon={<LuCopy className="h-4 w-4" />}
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
  selectedSpaces: Set<string> | null
  onToggle: (id: string) => void
}) {
  if (loading) {
    return (
      <div className="flex items-center justify-center py-8">
        <Spinner size="md" className="text-foreground-alt" />
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

  const selectedCount = selectedSpaces?.size ?? 0

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
          const checked = selectedSpaces?.has(id) ?? false
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
              <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded">
                {checked ?
                  <LuSquareCheck className="text-brand h-5 w-5" />
                : <LuSquare className="text-foreground-alt h-5 w-5" />}
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
            <Spinner className="text-brand" />
          )}
        {phase === TransferPhase.TransferPhase_COMPLETE && (
          <LuCheck className="text-brand h-4 w-4" />
        )}
        {phase === TransferPhase.TransferPhase_FAILED && (
          <LuX className="text-destructive h-4 w-4" />
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
                      className="bg-brand h-full rounded-full transition-all"
                      style={{ width: `${pct}%` }}
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
      <div className="bg-brand/10 mb-4 flex h-12 w-12 items-center justify-center rounded-full">
        <LuCheck className="text-brand h-6 w-6" />
      </div>
      <p className="text-foreground text-sm font-medium">Transfer complete</p>
      <p className="text-foreground-alt mt-1 text-xs">
        {spaceCount} space{spaceCount !== 1 ? 's' : ''} transferred
        successfully.
      </p>
    </div>
  )
}
