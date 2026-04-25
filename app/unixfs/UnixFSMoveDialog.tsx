import { useCallback, useMemo, useState } from 'react'
import { LuCheck, LuFolder } from 'react-icons/lu'
import type { FSHandle } from '@s4wave/sdk/unixfs/handle.js'
import { usePromise } from '@s4wave/web/hooks/usePromise.js'
import { cn } from '@s4wave/web/style/utils.js'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@s4wave/web/ui/command.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import {
  describeUnixFSMoveValidation,
  listUnixFSDirectories,
  type UnixFSMoveItem,
  validateUnixFSMove,
} from './move.js'

export interface UnixFSMoveDialogProps {
  rootHandle: FSHandle
  moveItems: UnixFSMoveItem[]
  initialSelectedPath?: string | null
  onOpenChange: (open: boolean) => void
  onConfirm: (destinationPath: string) => Promise<void>
}

function getMoveDialogTitle(moveItems: UnixFSMoveItem[]): string {
  if (moveItems.length === 1) {
    return `Move ${moveItems[0].name}`
  }
  return `Move ${moveItems.length} items`
}

function getMoveDialogDescription(moveItems: UnixFSMoveItem[]): string {
  if (moveItems.length === 1) {
    return 'Choose the destination folder for this item.'
  }
  return 'Choose the destination folder for the selected items.'
}

// UnixFSMoveDialog renders the destination picker for same-UnixFS move actions.
export function UnixFSMoveDialog({
  rootHandle,
  moveItems,
  initialSelectedPath = null,
  onOpenChange,
  onConfirm,
}: UnixFSMoveDialogProps) {
  const [selectedPath, setSelectedPath] = useState<string | null>(
    initialSelectedPath,
  )
  const [submitting, setSubmitting] = useState(false)
  const [submitError, setSubmitError] = useState<string | null>(null)

  const dirs = usePromise(
    useCallback(
      (signal: AbortSignal) => listUnixFSDirectories(rootHandle, signal),
      [rootHandle],
    ),
  )

  const validation = useMemo(() => {
    if (!selectedPath) return null
    return validateUnixFSMove(moveItems, selectedPath)
  }, [moveItems, selectedPath])
  const validationMessage = useMemo(
    () =>
      validation && !validation.accepted ?
        describeUnixFSMoveValidation(validation)
      : null,
    [validation],
  )

  const handleOpenChange = useCallback(
    (open: boolean) => {
      if (!open) {
        setSubmitError(null)
        setSubmitting(false)
      }
      onOpenChange(open)
    },
    [onOpenChange],
  )

  const handleConfirm = useCallback(async () => {
    if (!selectedPath || (validation && !validation.accepted)) return
    setSubmitting(true)
    setSubmitError(null)
    try {
      await onConfirm(selectedPath)
      handleOpenChange(false)
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'Move failed')
      setSubmitting(false)
    }
  }, [handleOpenChange, onConfirm, selectedPath, validation])

  const confirmDisabled =
    !selectedPath ||
    submitting ||
    dirs.loading ||
    !!dirs.error ||
    (validation ? !validation.accepted : false)

  const errorMessage = validationMessage ?? submitError

  return (
    <Dialog open onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{getMoveDialogTitle(moveItems)}</DialogTitle>
          <DialogDescription>
            {getMoveDialogDescription(moveItems)}
          </DialogDescription>
        </DialogHeader>

        <div className="border-foreground/6 bg-background-card/30 overflow-hidden rounded-lg border">
          <Command className="bg-transparent">
            <CommandInput placeholder="Search folders..." />
            <CommandList className="max-h-72">
              {dirs.loading && (
                <div className="px-3 py-2">
                  <LoadingInline
                    label="Loading folders"
                    tone="muted"
                    size="sm"
                  />
                </div>
              )}
              {dirs.error && (
                <div className="text-destructive px-3 py-2 text-xs">
                  Failed to load folders: {dirs.error.message}
                </div>
              )}
              {!dirs.loading && !dirs.error && (
                <>
                  <CommandEmpty className="text-foreground-alt/40 px-3 py-2 text-left text-xs">
                    No folders found.
                  </CommandEmpty>
                  <CommandGroup>
                    {(dirs.data ?? []).map((dir) => {
                      const isSelected = selectedPath === dir.path
                      return (
                        <CommandItem
                          key={dir.path}
                          value={`${dir.path} ${dir.name}`}
                          onSelect={() => {
                            setSelectedPath(dir.path)
                            setSubmitError(null)
                          }}
                          className="flex items-center gap-2 text-xs"
                        >
                          <div
                            className="flex min-w-0 flex-1 items-center gap-2"
                            style={{ paddingLeft: `${dir.depth * 12}px` }}
                          >
                            <LuFolder className="text-file-folder-icon h-3.5 w-3.5 shrink-0" />
                            <span className="truncate">{dir.name}</span>
                          </div>
                          {isSelected && (
                            <LuCheck className="text-brand h-3.5 w-3.5 shrink-0" />
                          )}
                        </CommandItem>
                      )
                    })}
                  </CommandGroup>
                </>
              )}
            </CommandList>
          </Command>
        </div>

        {selectedPath && (
          <div className="text-foreground-alt/60 flex items-center gap-1.5 text-xs select-none">
            <span>Moving to</span>
            <code className="text-foreground bg-foreground/5 truncate rounded px-1.5 py-0.5 font-mono text-[11px]">
              {selectedPath}
            </code>
          </div>
        )}

        {errorMessage && (
          <p className="text-destructive text-xs">{errorMessage}</p>
        )}

        <DialogFooter>
          <button
            type="button"
            className="text-foreground-alt hover:text-foreground rounded-md px-4 py-2 text-sm transition-colors"
            onClick={() => handleOpenChange(false)}
            disabled={submitting}
          >
            Cancel
          </button>
          <button
            type="button"
            className={cn(
              'rounded-md border px-4 py-2 text-sm transition-all',
              'border-brand/30 bg-brand/10 text-brand',
              'hover:border-brand/40 hover:bg-brand/20',
              'disabled:cursor-not-allowed disabled:opacity-50',
            )}
            onClick={() => void handleConfirm()}
            disabled={confirmDisabled}
          >
            {submitting ? 'Moving...' : 'Move'}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
