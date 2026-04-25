import { useCallback, useMemo, useState } from 'react'
import { LuCheck, LuLink, LuX } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { SpacewaveOrgListContext } from '@s4wave/web/contexts/SpacewaveOrgListContext.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import type {
  ManagedBillingAccount,
  OrganizationInfo,
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
import { ORG_ROLE_OWNER } from '../org/org-constants.js'
import {
  DetachAssignmentDialog,
  type DetachAssignmentTarget,
} from './DetachAssignmentDialog.js'

interface AssignTarget {
  ownerType: 'account' | 'organization'
  ownerId: string
  label: string
}

export interface BillingAssignmentsSectionProps {
  baId: string
  managedBillingAccount: ManagedBillingAccount | null
  loading?: boolean
  onChanged?: () => void
}

// BillingAssignmentsSection lets the viewer assign the given billing account
// to their personal account or an owned organization, and detach existing
// assignments. Mirrors the controls rendered on the billing accounts list.
export function BillingAssignmentsSection({
  baId,
  managedBillingAccount,
  loading,
  onChanged,
}: BillingAssignmentsSectionProps) {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const orgList = SpacewaveOrgListContext.useContext()
  const { accountId: callerAccountId } = useSessionInfo(session)

  const [assigning, setAssigning] = useState(false)
  const [assignError, setAssignError] = useState<string | null>(null)
  const [detachTarget, setDetachTarget] =
    useState<DetachAssignmentTarget | null>(null)
  const [detaching, setDetaching] = useState(false)
  const [detachError, setDetachError] = useState<string | null>(null)

  const assignees: PrincipalAssignment[] =
    managedBillingAccount?.assignees ?? []

  const ownedOrgs: OrganizationInfo[] = useMemo(
    () => orgList.organizations.filter((o) => o.role === ORG_ROLE_OWNER),
    [orgList.organizations],
  )

  const assignTargets: AssignTarget[] = useMemo(() => {
    const list: AssignTarget[] = []
    if (callerAccountId) {
      list.push({
        ownerType: 'account',
        ownerId: callerAccountId,
        label: 'Personal account',
      })
    }
    for (const o of ownedOrgs) {
      if (!o.id) continue
      list.push({
        ownerType: 'organization',
        ownerId: o.id,
        label: o.displayName || o.id,
      })
    }
    return list
  }, [callerAccountId, ownedOrgs])

  const handleAssign = useCallback(
    async (target: AssignTarget) => {
      if (!session || !baId || assigning) return
      setAssigning(true)
      setAssignError(null)
      try {
        await session.spacewave.assignBillingAccount(
          baId,
          target.ownerType,
          target.ownerId,
        )
        onChanged?.()
      } catch (e) {
        setAssignError(e instanceof Error ? e.message : String(e))
      } finally {
        setAssigning(false)
      }
    },
    [session, baId, assigning, onChanged],
  )

  const handleDetachConfirm = useCallback(async () => {
    if (!session || !detachTarget || detaching) return
    setDetaching(true)
    setDetachError(null)
    try {
      await session.spacewave.detachBillingAccount(
        detachTarget.ownerType,
        detachTarget.ownerId,
      )
      setDetachTarget(null)
      onChanged?.()
    } catch (e) {
      setDetachError(e instanceof Error ? e.message : String(e))
    } finally {
      setDetaching(false)
    }
  }, [session, detachTarget, detaching, onChanged])

  const handleDetachCancel = useCallback(() => {
    if (detaching) return
    setDetachTarget(null)
    setDetachError(null)
  }, [detaching])

  const noTargets = assignTargets.length === 0
  const menuDisabled = !session || assigning || noTargets

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
              a.ownerType === 'account' && a.ownerId === callerAccountId
            const label =
              isPersonal ? 'Personal' : a.displayName || a.ownerId || ''
            return (
              <span
                key={`${a.ownerType}:${a.ownerId}`}
                className="border-foreground/10 bg-foreground/5 text-foreground-alt flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px]"
              >
                <span>{label}</span>
                <button
                  onClick={() =>
                    setDetachTarget({
                      ownerType: a.ownerType as 'account' | 'organization',
                      ownerId: a.ownerId ?? '',
                      label,
                    })
                  }
                  aria-label={`Detach ${label}`}
                  className="hover:text-destructive cursor-pointer transition-colors"
                >
                  <LuX className="h-3 w-3" />
                </button>
              </span>
            )
          })}
        </div>
      )}
      <div className="flex items-center gap-2">
        <DropdownMenu>
          <DropdownMenuTrigger asChild disabled={menuDisabled}>
            <DropdownTriggerButton icon={<LuLink className="h-3 w-3" />}>
              {assigning ? 'Assigning...' : 'Assign to...'}
            </DropdownTriggerButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            <DropdownMenuLabel>Assign this BA to</DropdownMenuLabel>
            <DropdownMenuSeparator />
            {assignTargets.map((t) => {
              const isSelected = assignees.some(
                (a) => a.ownerType === t.ownerType && a.ownerId === t.ownerId,
              )
              return (
                <DropdownMenuItem
                  key={`${t.ownerType}:${t.ownerId}`}
                  onSelect={() => void handleAssign(t)}
                >
                  <LuCheck
                    className={cn(
                      'h-3 w-3',
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
          <span className="text-foreground-alt/50 text-[11px]">
            No owned organizations. Create one to assign.
          </span>
        )}
      </div>
      {assignError && (
        <div className="border-destructive/20 bg-destructive/5 text-destructive rounded-md border px-3 py-2 text-xs">
          {assignError}
        </div>
      )}
      {detachError && (
        <div className="border-destructive/20 bg-destructive/5 text-destructive rounded-md border px-3 py-2 text-xs">
          {detachError}
        </div>
      )}
      <DetachAssignmentDialog
        target={detachTarget}
        busy={detaching}
        onCancel={handleDetachCancel}
        onConfirm={() => void handleDetachConfirm()}
      />
    </div>
  )
}
