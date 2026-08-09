import { LuCheck, LuX } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import type { ManagedBillingAccount } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@s4wave/web/ui/DropdownMenu.js'
import { DropdownTriggerButton } from '@s4wave/web/ui/DropdownTriggerButton.js'

import type { DetachAssignmentTarget } from './DetachAssignmentDialog.js'
import type { BillingAssignmentTarget } from './useBillingAssignments.js'
import {
  lifecycleStateLabel,
  subscriptionStatusBadgeColor,
  subscriptionStatusLabel,
} from './billing-utils.js'

export function BillingAccountsList(props: {
  accounts: ManagedBillingAccount[]
  callerAccountId: string
  assignTargets: BillingAssignmentTarget[]
  assigningBillingAccountId: string | null
  actionsDisabled?: boolean
  onOpen: (billingAccountId: string) => void
  onAssign: (billingAccountId: string, target: BillingAssignmentTarget) => void
  onDetach: (target: DetachAssignmentTarget) => void
}) {
  return (
    <ul className="space-y-2">
      {props.accounts.map((account) => (
        <BillingAccountRow
          key={account.id}
          account={account}
          callerAccountId={props.callerAccountId}
          assignTargets={props.assignTargets}
          assigning={props.assigningBillingAccountId === account.id}
          actionsDisabled={props.actionsDisabled}
          onOpen={props.onOpen}
          onAssign={props.onAssign}
          onDetach={props.onDetach}
        />
      ))}
    </ul>
  )
}

function BillingAccountRow(props: {
  account: ManagedBillingAccount
  callerAccountId: string
  assignTargets: BillingAssignmentTarget[]
  assigning: boolean
  actionsDisabled?: boolean
  onOpen: (billingAccountId: string) => void
  onAssign: (billingAccountId: string, target: BillingAssignmentTarget) => void
  onDetach: (target: DetachAssignmentTarget) => void
}) {
  const { account } = props
  const billingAccountId = account.id ?? ''
  const assignees = account.assignees ?? []
  return (
    <li className="border-foreground/10 bg-foreground/5 hover:border-brand/30 hover:bg-brand/5 overflow-hidden rounded-md border transition-colors">
      <button
        onClick={() => props.onOpen(billingAccountId)}
        className="flex w-full cursor-pointer flex-col gap-1 p-3 text-left"
      >
        <div className="flex items-center gap-2">
          <span className="text-foreground text-sm font-medium">
            {account.displayName || billingAccountId}
          </span>
          <span
            className={cn(
              'rounded-full px-2 py-0.5 text-[10px] font-semibold tracking-wider uppercase',
              subscriptionStatusBadgeColor(account.subscriptionStatus),
            )}
          >
            {subscriptionStatusLabel(account.subscriptionStatus)}
          </span>
        </div>
        <div
          suppressHydrationWarning
          className="text-foreground-alt/60 text-xs"
        >
          {[
            lifecycleStateLabel(account.lifecycleState),
            account.createdAt &&
              `Created ${new Date(Number(account.createdAt)).toLocaleDateString()}`,
          ]
            .filter(Boolean)
            .join(' · ')}
        </div>
      </button>
      <div className="border-foreground/5 flex items-center justify-between gap-2 border-t px-3 py-2">
        <div className="flex min-w-0 flex-1 flex-wrap items-center gap-1">
          {assignees.length === 0 && (
            <span className="text-foreground-alt/60 text-xs">Unassigned</span>
          )}
          {assignees.map((assignee) => {
            const personal =
              assignee.ownerType === 'account' &&
              assignee.ownerId === props.callerAccountId
            const label = personal
              ? 'Personal'
              : assignee.displayName || assignee.ownerId || ''
            return (
              <span
                key={`${assignee.ownerType}:${assignee.ownerId}`}
                className="border-foreground/10 bg-foreground/5 text-foreground-alt flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs"
              >
                <span>{label}</span>
                <button
                  onClick={() =>
                    props.onDetach({
                      ownerType: assignee.ownerType as
                        | 'account'
                        | 'organization',
                      ownerId: assignee.ownerId ?? '',
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
        <DropdownMenu>
          <DropdownMenuTrigger
            asChild
            disabled={
              props.actionsDisabled ||
              props.assigning ||
              props.assignTargets.length === 0
            }
          >
            <DropdownTriggerButton triggerStyle="ghost">
              {props.assigning ? 'Assigning…' : 'Assign to…'}
            </DropdownTriggerButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuLabel>Assign this BA to</DropdownMenuLabel>
            <DropdownMenuSeparator />
            {props.assignTargets.map((target) => {
              const selected = assignees.some(
                (assignee) =>
                  assignee.ownerType === target.ownerType &&
                  assignee.ownerId === target.ownerId,
              )
              return (
                <DropdownMenuItem
                  key={`${target.ownerType}:${target.ownerId}`}
                  onSelect={() => props.onAssign(billingAccountId, target)}
                >
                  <LuCheck
                    className={cn(
                      'size-3',
                      selected ? 'text-brand' : 'text-transparent',
                    )}
                  />
                  <span>{target.label}</span>
                </DropdownMenuItem>
              )
            })}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </li>
  )
}
