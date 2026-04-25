import { useCallback, useState } from 'react'
import { LuTrash2 } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import {
  SessionContext,
  useSessionNavigate,
} from '@s4wave/web/contexts/contexts.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'

export interface OrgActionsSectionProps {
  orgId: string
  displayName: string
  spaceCount: number
}

// OrgActionsSection renders owner-only actions (currently Delete) using the
// same button-card layout as SessionDetails Actions.
export function OrgActionsSection({
  orgId,
  displayName,
  spaceCount,
}: OrgActionsSectionProps) {
  const [dialogOpen, setDialogOpen] = useState(false)
  const blocked = spaceCount > 0

  return (
    <section>
      <div className="mb-2 flex items-center justify-between">
        <h2 className="text-foreground-alt text-xs font-medium select-none">
          Actions
        </h2>
      </div>

      <div className="space-y-2">
        <button
          onClick={() => setDialogOpen(true)}
          disabled={blocked}
          className={cn(
            'border-destructive/30 bg-destructive/5 hover:border-destructive hover:bg-destructive hover:text-destructive-foreground group flex w-full cursor-pointer items-center gap-3 rounded-md border p-2.5 text-left transition-colors',
            blocked && 'hover:bg-destructive/5 cursor-not-allowed opacity-50',
          )}
        >
          <div className="bg-destructive/20 group-hover:bg-destructive-foreground/20 flex h-8 w-8 shrink-0 items-center justify-center rounded-md transition-colors">
            <LuTrash2 className="text-destructive group-hover:text-destructive-foreground h-3.5 w-3.5 transition-colors" />
          </div>
          <div className="flex min-w-0 flex-1 flex-col">
            <h4 className="text-destructive group-hover:text-destructive-foreground text-xs font-medium transition-colors select-none">
              Delete Organization
            </h4>
            <p className="text-destructive/80 group-hover:text-destructive-foreground/80 text-xs transition-colors select-none">
              {blocked ?
                `Remove or transfer all ${spaceCount} space${spaceCount !== 1 ? 's' : ''} before deleting`
              : 'Permanently delete this organization'}
            </p>
          </div>
        </button>
      </div>

      <DeleteOrgDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        orgId={orgId}
        displayName={displayName}
      />
    </section>
  )
}

function DeleteOrgDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  orgId: string
  displayName: string
}) {
  const session = SessionContext.useContext().value
  const navigateSession = useSessionNavigate()
  const [confirmText, setConfirmText] = useState('')
  const [deleting, setDeleting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const confirmed =
    confirmText.trim().toLowerCase() === props.displayName.trim().toLowerCase()

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (deleting) return
      if (!next) {
        setConfirmText('')
        setError(null)
      }
      props.onOpenChange(next)
    },
    [deleting, props],
  )

  const handleDelete = useCallback(async () => {
    if (!session || !confirmed || deleting) return
    setDeleting(true)
    setError(null)
    try {
      await session.spacewave.deleteOrganization(props.orgId)
      navigateSession({ path: '' })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete')
      setDeleting(false)
    }
  }, [session, props.orgId, confirmed, deleting, navigateSession])

  return (
    <Dialog open={props.open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete Organization</DialogTitle>
          <DialogDescription>
            This will permanently delete the organization and cannot be undone.
          </DialogDescription>
        </DialogHeader>
        <div>
          <label className="text-foreground-alt mb-1.5 block text-xs select-none">
            Type{' '}
            <span className="text-foreground font-medium">
              {props.displayName}
            </span>{' '}
            to confirm.
          </label>
          <input
            type="text"
            value={confirmText}
            onChange={(e) => setConfirmText(e.target.value)}
            placeholder={props.displayName}
            autoFocus
            className={cn(
              'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-2 text-sm transition-colors outline-none',
              'focus:border-destructive/50',
            )}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && confirmed && !deleting) {
                void handleDelete()
              }
            }}
          />
        </div>
        {error && <p className="text-destructive text-xs">{error}</p>}
        <DialogFooter>
          <button
            onClick={() => handleOpenChange(false)}
            disabled={deleting}
            className="text-foreground-alt hover:text-foreground rounded-md px-4 py-2 text-sm transition-colors disabled:cursor-not-allowed disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            onClick={() => void handleDelete()}
            disabled={!confirmed || deleting}
            className={cn(
              'rounded-md border px-4 py-2 text-sm transition-all',
              'border-destructive bg-destructive/20 text-destructive hover:bg-destructive/30',
              'disabled:cursor-not-allowed disabled:opacity-50',
            )}
          >
            {deleting ? 'Deleting...' : 'Delete Organization'}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
