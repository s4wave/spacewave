import { LuCheck, LuLink, LuX } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import type {
  ManagedBillingAccount,
  PrincipalAssignment,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@s4wave/web/ui/DropdownMenu.js'
import { DropdownTriggerButton } from '@s4wave/web/ui/DropdownTriggerButton.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'

import { useBillingAssignments } from './useBillingAssignments.js'
import { DetachAssignmentDialog } from './DetachAssignmentDialog.js'

export interface BillingAssignmentsSectionProps {
  baId: string
  managedBillingAccount: ManagedBillingAccount | null
  loading?: boolean
}

// BillingAssignmentsSection lets the viewer assign the given billing account
// to their personal account or an owned organization, and detach existing
// assignments. Mirrors the controls rendered on the billing accounts list.
export function BillingAssignmentsSection({
  baId,
  managedBillingAccount,
  loading,
}: BillingAssignmentsSectionProps) {
  const assignments = useBillingAssignments()

  const assignees: PrincipalAssignment[] =
    managedBillingAccount?.assignees ?? []

  const noTargets = assignments.assignTargets.length === 0
  const menuDisabled =
    assignments.actionsDisabled ||
    assignments.assigningBillingAccountId === baId ||
    noTargets

  return (
    <div className="space-y-3">
      <div className="text-foreground-alt/60 text-xs font-medium tracking-wider uppercase select-none">
        Assigned to
      </div>
      {loading && !managedBillingAccount && (
        <LoadingInline label="Loading assignments" tone="muted" size="sm" />
      )}
      {!loading && assignees.length === 0 && (
        <div className="text-foreground-alt/60 text-xs">
          Unassigned. Pick a principal below to link this billing account.
        </div>
      )}
      {assignees.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          {assignees.map((a) => {
            const isPersonal =
              a.ownerType === 'account' &&
              a.ownerId === assignments.callerAccountId
            const label = isPersonal
              ? 'Personal'
              : a.displayName || a.ownerId || ''
            return (
              <span
                key={`${a.ownerType}:${a.ownerId}`}
                className="border-foreground/10 bg-foreground/5 text-foreground-alt flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs"
              >
                <span>{label}</span>
                <button
                  onClick={() =>
                    assignments.requestDetach({
                      ownerType: a.ownerType as 'account' | 'organization',
                      ownerId: a.ownerId ?? '',
                      label,
                    })
                  }
                  aria-label={`Detach ${label}`}
                  className="hover:text-destructive cursor-pointer transition-colors"
                >
                  <LuX className="size-3" />
                </button>
              </span>
            )
          })}
        </div>
      )}
      <div className="flex items-center gap-2">
        <DropdownMenu>
          <DropdownMenuTrigger asChild disabled={menuDisabled}>
            <DropdownTriggerButton icon={<LuLink className="size-3" />}>
              {assignments.assigningBillingAccountId === baId
                ? 'Assigning…'
                : 'Assign to…'}
            </DropdownTriggerButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            <DropdownMenuLabel>Assign this BA to</DropdownMenuLabel>
            <DropdownMenuSeparator />
            {assignments.assignTargets.map((t) => {
              const isSelected = assignees.some(
                (a) => a.ownerType === t.ownerType && a.ownerId === t.ownerId,
              )
              return (
                <DropdownMenuItem
                  key={`${t.ownerType}:${t.ownerId}`}
                  onSelect={() => {
                    if (baId) void assignments.assign(baId, t)
                  }}
                >
                  <LuCheck
                    className={cn(
                      'size-3',
                      isSelected ? 'text-brand' : 'text-transparent',
                    )}
                  />
                  <span>{t.label}</span>
                </DropdownMenuItem>
              )
            })}
          </DropdownMenuContent>
        </DropdownMenu>
        {noTargets && (
          <span className="text-foreground-alt/50 text-xs">
            No owned organizations. Create one to assign.
          </span>
        )}
      </div>
      {assignments.assignError && (
        <div className="border-destructive/20 bg-destructive/5 text-destructive rounded-md border px-3 py-2 text-xs">
          {assignments.assignError}
        </div>
      )}
      {assignments.detachError && (
        <div className="border-destructive/20 bg-destructive/5 text-destructive rounded-md border px-3 py-2 text-xs">
          {assignments.detachError}
        </div>
      )}
      <DetachAssignmentDialog
        target={assignments.detachTarget}
        busy={assignments.detaching}
        onCancel={assignments.cancelDetach}
        onConfirm={() => void assignments.confirmDetach()}
      />
    </div>
  )
}
