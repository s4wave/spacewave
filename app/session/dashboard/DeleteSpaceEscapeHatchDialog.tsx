import { useCallback, useMemo, useState } from 'react'
import {
  LuBoxes,
  LuCircleAlert,
  LuCircleCheck,
  LuShieldAlert,
  LuTriangleAlert,
} from 'react-icons/lu'
import { useWatchStateRpc } from '@aptre/bldr-react'

import type { Session } from '@s4wave/sdk/session/session.js'
import {
  WatchResourcesListRequest,
  WatchResourcesListResponse,
  WatchSharedObjectHealthRequest,
  WatchSharedObjectHealthResponse,
} from '@s4wave/sdk/session/session.pb.js'
import {
  SharedObjectHealthStatus,
  type SharedObjectHealth,
} from '@s4wave/core/sobject/sobject.pb.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import { cn } from '@s4wave/web/style/utils.js'

import { TypedConfirmField } from './TypedConfirmField.js'

export interface DeleteSpaceEscapeHatchDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  session: Session | null
}

type Step = 'select' | 'warning' | 'final'

interface SpaceChoice {
  id: string
  name: string
  // hasName is true when the space advertises a user-visible display name.
  hasName: boolean
}

// useSpaceChoices watches the session's spaces while the dialog is open.
// loading is true until the first list arrives.
function useSpaceChoices(open: boolean, session: Session | null) {
  const resourcesList = useWatchStateRpc(
    useCallback(
      (req: WatchResourcesListRequest, signal: AbortSignal) =>
        open && session ? session.watchResourcesList(req, signal) : null,
      [open, session],
    ),
    {},
    WatchResourcesListRequest.equals,
    WatchResourcesListResponse.equals,
  )

  const choices = useMemo<SpaceChoice[]>(() => {
    const entries = resourcesList?.spacesList ?? []
    return entries.flatMap((entry): SpaceChoice[] => {
      const id = entry.entry?.ref?.providerResourceRef?.id ?? ''
      if (!id) return []
      const name = entry.spaceMeta?.name?.trim() ?? ''
      return [
        {
          id,
          name: name || id,
          hasName: name.length > 0,
        },
      ]
    })
  }, [resourcesList])

  return { choices, loading: !resourcesList }
}

// useSelectedSpaceHealth watches the health of the chosen space. It only
// watches once the user has picked a space, which avoids fanning out one Watch
// stream per space in the list.
function useSelectedSpaceHealth(
  open: boolean,
  session: Session | null,
  selectedId: string,
): SharedObjectHealth | null {
  const healthResp = useWatchStateRpc(
    useCallback(
      (req: WatchSharedObjectHealthRequest, signal: AbortSignal) =>
        open && session && selectedId
          ? session.watchSharedObjectHealth(req, signal)
          : null,
      [open, session, selectedId],
    ),
    selectedId ? { sharedObjectId: selectedId } : null,
    WatchSharedObjectHealthRequest.equals,
    WatchSharedObjectHealthResponse.equals,
  )
  return healthResp?.health ?? null
}

// useSpaceDeletion deletes the selected space through the session and tracks
// the pending and failed state. onDeleted runs after a successful delete.
function useSpaceDeletion(
  session: Session | null,
  selected: SpaceChoice | null,
  onDeleted: () => void,
) {
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string>()

  const deleteSpace = async () => {
    if (!session || !selected) return
    setSubmitting(true)
    setError(undefined)
    try {
      await session.deleteSpace(selected.id)
      onDeleted()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Delete failed')
      setSubmitting(false)
    }
  }

  return {
    submitting,
    error,
    deleteSpace,
    clearError: () => setError(undefined),
    reset: () => {
      setError(undefined)
      setSubmitting(false)
    },
  }
}

// DeleteSpaceEscapeHatchDialog is a stepwise destructive flow for deleting a
// space without mounting it, used when a space cannot be opened normally.
export function DeleteSpaceEscapeHatchDialog({
  open,
  onOpenChange,
  session,
}: DeleteSpaceEscapeHatchDialogProps) {
  const [step, setStep] = useState<Step>('select')
  const [selectedId, setSelectedId] = useState('')
  const [selectedSnapshot, setSelectedSnapshot] = useState<SpaceChoice | null>(
    null,
  )
  const [acknowledged, setAcknowledged] = useState(false)
  const [typedConfirm, setTypedConfirm] = useState('')

  const { choices, loading } = useSpaceChoices(open, session)
  const liveSelected = choices.find((c) => c.id === selectedId) ?? null
  const selected = selectedSnapshot ?? liveSelected
  const health = useSelectedSpaceHealth(open, session, selectedId)
  const deletion = useSpaceDeletion(session, selected, () =>
    handleOpenChange(false),
  )
  const { submitting, error } = deletion

  // handleOpenChange resets the flow when the dialog closes.
  function handleOpenChange(next: boolean) {
    if (!next) {
      setStep('select')
      setSelectedId('')
      setSelectedSnapshot(null)
      setAcknowledged(false)
      setTypedConfirm('')
      deletion.reset()
    }
    onOpenChange(next)
  }

  function handleSelect(id: string) {
    setSelectedId(id)
    setSelectedSnapshot(choices.find((c) => c.id === id) ?? null)
    deletion.clearError()
  }

  // The typed name must equal the selected space's name; a missing selection
  // never matches.
  const canDelete =
    acknowledged && !submitting && typedConfirm === selected?.name

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle variant="withIcon">
            <LuShieldAlert className="text-destructive size-4" />
            Delete a Space
          </DialogTitle>
          <DeleteSpaceDescription step={step} selected={selected} />
        </DialogHeader>

        {step === 'select' && (
          <SpaceSelectList
            choices={choices}
            loading={loading}
            selectedId={selectedId}
            onSelect={handleSelect}
          />
        )}

        {step === 'warning' && selected && (
          <DeleteWarningStep
            space={selected}
            health={health}
            acknowledged={acknowledged}
            onAcknowledgedChange={setAcknowledged}
          />
        )}

        {step === 'final' && selected && (
          <DeleteFinalStep
            space={selected}
            health={health}
            typedConfirm={typedConfirm}
            onTypedConfirmChange={setTypedConfirm}
            canDelete={canDelete}
            onDelete={() => void deletion.deleteSpace()}
          />
        )}

        {error && <p className="text-destructive text-xs">{error}</p>}

        <DialogFooter>
          {step === 'select' && (
            <SelectStepActions
              canContinue={!!selected}
              onCancel={() => handleOpenChange(false)}
              onContinue={() => setStep('warning')}
            />
          )}

          {step === 'warning' && (
            <WarningStepActions
              acknowledged={acknowledged}
              onBack={() => {
                setStep('select')
                setAcknowledged(false)
              }}
              onContinue={() => setStep('final')}
            />
          )}

          {step === 'final' && (
            <FinalStepActions
              submitting={submitting}
              canDelete={canDelete}
              onBack={() => {
                setStep('warning')
                setTypedConfirm('')
              }}
              onDelete={() => void deletion.deleteSpace()}
            />
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

interface DeleteSpaceDescriptionProps {
  step: Step
  selected: SpaceChoice | null
}

// DeleteSpaceDescription explains the current step under the dialog title.
function DeleteSpaceDescription({
  step,
  selected,
}: DeleteSpaceDescriptionProps) {
  if (step === 'select') {
    return (
      <DialogDescription>
        Pick a space to permanently delete. Use this only when a space will not
        open and cannot be removed from inside the space itself. Deletion is
        final and does not require the space to mount.
      </DialogDescription>
    )
  }
  if (!selected) return null
  if (step === 'warning') {
    return (
      <DialogDescription>
        This will permanently delete{' '}
        <span className="text-foreground font-medium">
          {selected.hasName ? selected.name : selected.id}
        </span>{' '}
        and all of its data. Confirm you understand before continuing.
      </DialogDescription>
    )
  }
  return (
    <DialogDescription>
      Type the {selected.hasName ? 'space name' : 'shared object id'} exactly to
      confirm.
    </DialogDescription>
  )
}

interface DeleteWarningStepProps {
  space: SpaceChoice
  health: SharedObjectHealth | null
  acknowledged: boolean
  onAcknowledgedChange: (acknowledged: boolean) => void
}

// DeleteWarningStep asks the user to acknowledge that deletion is permanent.
function DeleteWarningStep({
  space,
  health,
  acknowledged,
  onAcknowledgedChange,
}: DeleteWarningStepProps) {
  return (
    <div className="space-y-3">
      <SelectedSpaceSummary space={space} health={health} />
      <label className="border-destructive/30 bg-destructive/5 text-destructive flex cursor-pointer items-start gap-2 rounded-md border p-3 text-xs select-none">
        <input
          type="checkbox"
          checked={acknowledged}
          onChange={(e) => onAcknowledgedChange(e.target.checked)}
          className="accent-destructive mt-0.5 size-3.5 shrink-0"
          aria-label="Confirm delete is permanent"
        />
        <span>
          I understand this permanently deletes the space and its data.
        </span>
      </label>
    </div>
  )
}

interface DeleteFinalStepProps {
  space: SpaceChoice
  health: SharedObjectHealth | null
  typedConfirm: string
  onTypedConfirmChange: (value: string) => void
  canDelete: boolean
  onDelete: () => void
}

// DeleteFinalStep asks the user to type the space name to confirm deletion.
function DeleteFinalStep({
  space,
  health,
  typedConfirm,
  onTypedConfirmChange,
  canDelete,
  onDelete,
}: DeleteFinalStepProps) {
  return (
    <div className="space-y-3">
      <SelectedSpaceSummary space={space} health={health} />
      <TypedConfirmField
        label={
          <>
            Type{' '}
            <span className="text-destructive font-medium break-all">
              {space.name}
            </span>{' '}
            to confirm
          </>
        }
        value={typedConfirm}
        onChange={onTypedConfirmChange}
        placeholder={space.name}
        ariaLabel="Confirm space name or id"
        canSubmit={canDelete}
        onSubmit={onDelete}
      />
    </div>
  )
}

interface SelectStepActionsProps {
  canContinue: boolean
  onCancel: () => void
  onContinue: () => void
}

// SelectStepActions renders the footer buttons of the select step.
function SelectStepActions({
  canContinue,
  onCancel,
  onContinue,
}: SelectStepActionsProps) {
  return (
    <>
      <button
        type="button"
        onClick={onCancel}
        className="text-foreground-alt hover:text-foreground rounded-md px-4 py-2 text-sm transition-colors"
      >
        Cancel
      </button>
      <button
        type="button"
        disabled={!canContinue}
        onClick={onContinue}
        className={cn(
          'rounded-md border px-4 py-2 text-sm transition-all',
          'border-destructive/30 bg-destructive/10 text-destructive hover:bg-destructive/20',
          'disabled:cursor-not-allowed disabled:opacity-50',
        )}
      >
        Review space deletion
      </button>
    </>
  )
}

interface WarningStepActionsProps {
  acknowledged: boolean
  onBack: () => void
  onContinue: () => void
}

// WarningStepActions renders the footer buttons of the warning step.
function WarningStepActions({
  acknowledged,
  onBack,
  onContinue,
}: WarningStepActionsProps) {
  return (
    <>
      <button
        type="button"
        onClick={onBack}
        className="text-foreground-alt hover:text-foreground rounded-md px-4 py-2 text-sm transition-colors"
      >
        Back
      </button>
      <button
        type="button"
        disabled={!acknowledged}
        onClick={onContinue}
        className={cn(
          'rounded-md border px-4 py-2 text-sm transition-all',
          'border-destructive/30 bg-destructive/10 text-destructive hover:bg-destructive/20',
          'disabled:cursor-not-allowed disabled:opacity-50',
        )}
      >
        Confirm space deletion
      </button>
    </>
  )
}

interface FinalStepActionsProps {
  submitting: boolean
  canDelete: boolean
  onBack: () => void
  onDelete: () => void
}

// FinalStepActions renders the footer buttons of the final step.
function FinalStepActions({
  submitting,
  canDelete,
  onBack,
  onDelete,
}: FinalStepActionsProps) {
  return (
    <>
      <button
        type="button"
        onClick={onBack}
        disabled={submitting}
        className="text-foreground-alt hover:text-foreground rounded-md px-4 py-2 text-sm transition-colors"
      >
        Back
      </button>
      <button
        type="button"
        onClick={onDelete}
        disabled={!canDelete}
        className={cn(
          'rounded-md border px-4 py-2 text-sm transition-all',
          'border-destructive bg-destructive/20 text-destructive hover:bg-destructive/30',
          'disabled:cursor-not-allowed disabled:opacity-50',
        )}
      >
        {submitting ? 'Deleting…' : 'Delete Space'}
      </button>
    </>
  )
}

interface SpaceSelectListProps {
  choices: SpaceChoice[]
  loading: boolean
  selectedId: string
  onSelect: (id: string) => void
}

// SpaceSelectList renders the chooser used in the first dialog step.
function SpaceSelectList({
  choices,
  loading,
  selectedId,
  onSelect,
}: SpaceSelectListProps) {
  if (loading) {
    return (
      <div className="text-foreground-alt flex items-center gap-2 text-xs select-none">
        <LuBoxes className="text-foreground-alt/40 size-3.5" />
        Loading spaces…
      </div>
    )
  }
  if (choices.length === 0) {
    return (
      <div className="text-foreground-alt flex items-center gap-2 text-xs select-none">
        <LuBoxes className="text-foreground-alt/40 size-3.5" />
        No spaces in this session.
      </div>
    )
  }
  return (
    <div
      role="radiogroup"
      aria-label="Spaces"
      className="max-h-72 space-y-1.5 overflow-y-auto pr-1"
    >
      {choices.map((choice) => {
        const isSelected = choice.id === selectedId
        return (
          <button
            key={choice.id}
            type="button"
            role="radio"
            aria-checked={isSelected}
            onClick={() => onSelect(choice.id)}
            className={cn(
              'flex w-full cursor-pointer items-start gap-2.5 rounded-md border p-2.5 text-left transition-colors',
              isSelected
                ? 'border-destructive/40 bg-destructive/5'
                : 'border-foreground/10 bg-foreground/5 hover:border-destructive/30 hover:bg-destructive/5',
            )}
          >
            <LuBoxes
              className={cn(
                'mt-0.5 size-3.5 shrink-0 transition-colors',
                isSelected ? 'text-destructive' : 'text-foreground-alt',
              )}
            />
            <div className="min-w-0 flex-1">
              <div className="text-foreground truncate text-xs font-medium select-none">
                {choice.hasName ? choice.name : 'Unnamed space'}
              </div>
              <div className="text-foreground-alt/70 micro-label truncate font-mono break-all select-text">
                {choice.id}
              </div>
            </div>
          </button>
        )
      })}
    </div>
  )
}

interface SelectedSpaceSummaryProps {
  space: SpaceChoice
  health: SharedObjectHealth | null
}

// SelectedSpaceSummary shows identity and health for the chosen space.
function SelectedSpaceSummary({ space, health }: SelectedSpaceSummaryProps) {
  return (
    <div className="border-foreground/8 bg-background-card/30 rounded-lg border px-3 py-2.5">
      <div className="text-foreground-alt/50 micro-fine font-medium tracking-widest uppercase select-none">
        Space
      </div>
      <div className="text-foreground mt-0.5 truncate text-sm font-medium">
        {space.hasName ? space.name : 'Unnamed space'}
      </div>
      <div className="text-foreground-alt/70 mt-0.5 font-mono text-xs break-all select-text">
        {space.id}
      </div>
      <HealthBadge health={health} />
    </div>
  )
}

interface HealthBadgeProps {
  health: SharedObjectHealth | null
}

// HealthBadge surfaces broken/degraded status inline on the confirmation step.
function HealthBadge({ health }: HealthBadgeProps) {
  if (!health) return null
  const status = health.status ?? SharedObjectHealthStatus.UNKNOWN
  if (status === SharedObjectHealthStatus.READY) {
    return (
      <div className="text-foreground-alt/70 mt-2 flex items-center gap-1.5 text-xs select-none">
        <LuCircleCheck className="text-foreground-alt/50 size-3" />
        Space is reachable.
      </div>
    )
  }
  if (status === SharedObjectHealthStatus.LOADING) {
    return (
      <div className="text-foreground-alt/70 mt-2 flex items-center gap-1.5 text-xs select-none">
        <LuCircleAlert className="text-foreground-alt/50 size-3" />
        Checking status…
      </div>
    )
  }
  if (status === SharedObjectHealthStatus.DEGRADED) {
    return (
      <div className="text-warning mt-2 flex items-center gap-1.5 text-xs select-none">
        <LuTriangleAlert className="size-3" />
        Degraded. Some data may be partially available.
      </div>
    )
  }
  if (status === SharedObjectHealthStatus.CLOSED) {
    return (
      <div className="text-destructive mt-2 flex items-center gap-1.5 text-xs select-none">
        <LuShieldAlert className="size-3" />
        Cannot mount. This space is broken and must be deleted from here.
      </div>
    )
  }
  return null
}
