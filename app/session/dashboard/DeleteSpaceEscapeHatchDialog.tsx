import { useCallback, useMemo, useState } from 'react'
import {
  LuBoxes,
  LuCircleAlert,
  LuCircleCheck,
  LuShieldAlert,
  LuTrash2,
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

// DeleteSpaceEscapeHatchDialog is a stepwise destructive flow for deleting a
// space without mounting it, used when a space cannot be opened normally.
export function DeleteSpaceEscapeHatchDialog({
  open,
  onOpenChange,
  session,
}: DeleteSpaceEscapeHatchDialogProps) {
  const [step, setStep] = useState<Step>('select')
  const [selectedId, setSelectedId] = useState('')
  const [acknowledged, setAcknowledged] = useState(false)
  const [typedConfirm, setTypedConfirm] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string>()

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
    return entries
      .map((entry): SpaceChoice | null => {
        const id = entry.entry?.ref?.providerResourceRef?.id ?? ''
        if (!id) return null
        const name = entry.spaceMeta?.name?.trim() ?? ''
        return {
          id,
          name: name || id,
          hasName: name.length > 0,
        }
      })
      .filter((choice): choice is SpaceChoice => choice !== null)
  }, [resourcesList])

  const selected = useMemo(
    () => choices.find((c) => c.id === selectedId) ?? null,
    [choices, selectedId],
  )

  // Only watch health once the user has picked a space. This avoids fanning
  // out one Watch stream per space in the list.
  const healthResp = useWatchStateRpc(
    useCallback(
      (req: WatchSharedObjectHealthRequest, signal: AbortSignal) =>
        open && session && selectedId ?
          session.watchSharedObjectHealth(req, signal)
        : null,
      [open, session, selectedId],
    ),
    selectedId ? { sharedObjectId: selectedId } : null,
    WatchSharedObjectHealthRequest.equals,
    WatchSharedObjectHealthResponse.equals,
  )
  const health = healthResp?.health ?? null

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (!next) {
        setStep('select')
        setSelectedId('')
        setAcknowledged(false)
        setTypedConfirm('')
        setError(undefined)
        setSubmitting(false)
      }
      onOpenChange(next)
    },
    [onOpenChange],
  )

  const handleSelect = useCallback((id: string) => {
    setSelectedId(id)
    setError(undefined)
  }, [])

  const handleContinueFromSelect = useCallback(() => {
    if (!selected) return
    setStep('warning')
  }, [selected])

  const handleContinueFromWarning = useCallback(() => {
    if (!acknowledged) return
    setStep('final')
  }, [acknowledged])

  const handleDelete = useCallback(async () => {
    if (!session || !selected) return
    setSubmitting(true)
    setError(undefined)
    try {
      await session.deleteSpace(selected.id)
      handleOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Delete failed')
      setSubmitting(false)
    }
  }, [handleOpenChange, selected, session])

  const confirmMatches = selected ? typedConfirm === selected.name : false
  const canDelete = !!selected && acknowledged && confirmMatches && !submitting

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <LuShieldAlert className="text-destructive h-4 w-4" />
            Delete a Space
          </DialogTitle>
          {step === 'select' && (
            <DialogDescription>
              Pick a space to permanently delete. Use this only when a space
              will not open and cannot be removed from inside the space itself.
              Deletion is final and does not require the space to mount.
            </DialogDescription>
          )}
          {step === 'warning' && selected && (
            <DialogDescription>
              This will permanently delete{' '}
              <span className="text-foreground font-medium">
                {selected.hasName ? selected.name : selected.id}
              </span>{' '}
              and all of its data. Confirm you understand before continuing.
            </DialogDescription>
          )}
          {step === 'final' && selected && (
            <DialogDescription>
              Type the {selected.hasName ? 'space name' : 'shared object id'}{' '}
              exactly to confirm.
            </DialogDescription>
          )}
        </DialogHeader>

        {step === 'select' && (
          <SpaceSelectList
            choices={choices}
            loading={!resourcesList}
            selectedId={selectedId}
            onSelect={handleSelect}
          />
        )}

        {step === 'warning' && selected && (
          <div className="space-y-3">
            <SelectedSpaceSummary space={selected} health={health} />
            <label className="border-destructive/30 bg-destructive/5 text-destructive flex cursor-pointer items-start gap-2 rounded-md border p-3 text-xs select-none">
              <input
                type="checkbox"
                checked={acknowledged}
                onChange={(e) => setAcknowledged(e.target.checked)}
                className="accent-destructive mt-0.5 h-3.5 w-3.5 shrink-0"
                aria-label="Confirm delete is permanent"
              />
              <span>
                I understand this permanently deletes the space and its data.
              </span>
            </label>
          </div>
        )}

        {step === 'final' && selected && (
          <div className="space-y-3">
            <SelectedSpaceSummary space={selected} health={health} />
            <div>
              <label className="text-foreground-alt mb-1.5 block text-xs select-none">
                Type{' '}
                <span className="text-destructive font-medium break-all">
                  {selected.name}
                </span>{' '}
                to confirm
              </label>
              <input
                value={typedConfirm}
                onChange={(e) => setTypedConfirm(e.target.value)}
                placeholder={selected.name}
                aria-label="Confirm space name or id"
                className={cn(
                  'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-2 text-sm transition-colors outline-none',
                  'focus:border-destructive/50',
                )}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && canDelete) {
                    void handleDelete()
                  }
                }}
              />
            </div>
          </div>
        )}

        {error && <p className="text-destructive text-xs">{error}</p>}

        <DialogFooter>
          {step === 'select' && (
            <>
              <button
                onClick={() => handleOpenChange(false)}
                className="text-foreground-alt hover:text-foreground rounded-md px-4 py-2 text-sm transition-colors"
              >
                Cancel
              </button>
              <button
                disabled={!selected}
                onClick={handleContinueFromSelect}
                className={cn(
                  'rounded-md border px-4 py-2 text-sm transition-all',
                  'border-destructive/30 bg-destructive/10 text-destructive hover:bg-destructive/20',
                  'disabled:cursor-not-allowed disabled:opacity-50',
                )}
              >
                Continue
              </button>
            </>
          )}

          {step === 'warning' && (
            <>
              <button
                onClick={() => {
                  setStep('select')
                  setAcknowledged(false)
                }}
                className="text-foreground-alt hover:text-foreground rounded-md px-4 py-2 text-sm transition-colors"
              >
                Back
              </button>
              <button
                disabled={!acknowledged}
                onClick={handleContinueFromWarning}
                className={cn(
                  'rounded-md border px-4 py-2 text-sm transition-all',
                  'border-destructive/30 bg-destructive/10 text-destructive hover:bg-destructive/20',
                  'disabled:cursor-not-allowed disabled:opacity-50',
                )}
              >
                Continue
              </button>
            </>
          )}

          {step === 'final' && (
            <>
              <button
                onClick={() => {
                  setStep('warning')
                  setTypedConfirm('')
                }}
                disabled={submitting}
                className="text-foreground-alt hover:text-foreground rounded-md px-4 py-2 text-sm transition-colors"
              >
                Back
              </button>
              <button
                onClick={() => void handleDelete()}
                disabled={!canDelete}
                className={cn(
                  'rounded-md border px-4 py-2 text-sm transition-all',
                  'border-destructive bg-destructive/20 text-destructive hover:bg-destructive/30',
                  'disabled:cursor-not-allowed disabled:opacity-50',
                )}
              >
                {submitting ? 'Deleting...' : 'Delete Space'}
              </button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
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
        <LuBoxes className="text-foreground-alt/40 h-3.5 w-3.5" />
        Loading spaces...
      </div>
    )
  }
  if (choices.length === 0) {
    return (
      <div className="text-foreground-alt flex items-center gap-2 text-xs select-none">
        <LuBoxes className="text-foreground-alt/40 h-3.5 w-3.5" />
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
              isSelected ?
                'border-destructive/40 bg-destructive/5'
              : 'border-foreground/10 bg-foreground/5 hover:border-destructive/30 hover:bg-destructive/5',
            )}
          >
            <LuBoxes
              className={cn(
                'mt-0.5 h-3.5 w-3.5 shrink-0 transition-colors',
                isSelected ? 'text-destructive' : 'text-foreground-alt',
              )}
            />
            <div className="min-w-0 flex-1">
              <div className="text-foreground truncate text-xs font-medium select-none">
                {choice.hasName ? choice.name : 'Unnamed space'}
              </div>
              <div className="text-foreground-alt/70 truncate font-mono text-[0.65rem] break-all select-text">
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
      <div className="text-foreground-alt/50 text-[0.55rem] font-medium tracking-widest uppercase select-none">
        Space
      </div>
      <div className="text-foreground mt-0.5 truncate text-sm font-medium">
        {space.hasName ? space.name : 'Unnamed space'}
      </div>
      <div className="text-foreground-alt/70 mt-0.5 font-mono text-[0.65rem] break-all select-text">
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
      <div className="text-foreground-alt/70 mt-2 flex items-center gap-1.5 text-[0.65rem] select-none">
        <LuCircleCheck className="text-foreground-alt/50 h-3 w-3" />
        Space is reachable.
      </div>
    )
  }
  if (status === SharedObjectHealthStatus.LOADING) {
    return (
      <div className="text-foreground-alt/70 mt-2 flex items-center gap-1.5 text-[0.65rem] select-none">
        <LuCircleAlert className="text-foreground-alt/50 h-3 w-3" />
        Checking status...
      </div>
    )
  }
  if (status === SharedObjectHealthStatus.DEGRADED) {
    return (
      <div className="text-warning mt-2 flex items-center gap-1.5 text-[0.65rem] select-none">
        <LuTriangleAlert className="h-3 w-3" />
        Degraded. Some data may be partially available.
      </div>
    )
  }
  if (status === SharedObjectHealthStatus.CLOSED) {
    return (
      <div className="text-destructive mt-2 flex items-center gap-1.5 text-[0.65rem] select-none">
        <LuShieldAlert className="h-3 w-3" />
        Cannot mount. This space is broken and must be deleted from here.
      </div>
    )
  }
  return null
}
