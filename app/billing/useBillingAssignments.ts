import { useCallback, useMemo, useState } from 'react'

import { SpacewaveOrgListContext } from '@s4wave/web/contexts/SpacewaveOrgListContext.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'

import { ORG_ROLE_OWNER } from '../org/org-constants.js'
import type { DetachAssignmentTarget } from './DetachAssignmentDialog.js'
import { useManagedBillingAccounts } from './useManagedBillingAccounts.js'

export interface BillingAssignmentTarget {
  ownerType: 'account' | 'organization'
  ownerId: string
  label: string
}

// useBillingAssignments owns billing assignment targets and mutation lifecycle
// for the account list and individual billing account views.
export function useBillingAssignments() {
  const { session, store } = useManagedBillingAccounts()
  const orgList = SpacewaveOrgListContext.useContext()
  const { accountId: callerAccountId } = useSessionInfo(session)
  const [assigningBillingAccountId, setAssigningBillingAccountId] = useState<
    string | null
  >(null)
  const [assignError, setAssignError] = useState<string | null>(null)
  const [detachTarget, setDetachTarget] =
    useState<DetachAssignmentTarget | null>(null)
  const [detaching, setDetaching] = useState(false)
  const [detachError, setDetachError] = useState<string | null>(null)

  const assignTargets = useMemo<BillingAssignmentTarget[]>(() => {
    const targets: BillingAssignmentTarget[] = []
    if (callerAccountId) {
      targets.push({
        ownerType: 'account',
        ownerId: callerAccountId,
        label: 'Personal account',
      })
    }
    for (const organization of orgList.organizations) {
      if (organization.role !== ORG_ROLE_OWNER || !organization.id) continue
      targets.push({
        ownerType: 'organization',
        ownerId: organization.id,
        label: organization.displayName || organization.id,
      })
    }
    return targets
  }, [callerAccountId, orgList.organizations])

  const assign = useCallback(
    async (billingAccountId: string, target: BillingAssignmentTarget) => {
      if (!store || assigningBillingAccountId) return
      setAssigningBillingAccountId(billingAccountId)
      setAssignError(null)
      try {
        await store.assign(billingAccountId, target.ownerType, target.ownerId)
      } catch (cause) {
        setAssignError(cause instanceof Error ? cause.message : String(cause))
      } finally {
        setAssigningBillingAccountId(null)
      }
    },
    [assigningBillingAccountId, store],
  )

  const confirmDetach = useCallback(async () => {
    if (!store || !detachTarget || detaching) return
    setDetaching(true)
    setDetachError(null)
    try {
      await store.detach(detachTarget.ownerType, detachTarget.ownerId)
      setDetachTarget(null)
    } catch (cause) {
      setDetachError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setDetaching(false)
    }
  }, [detachTarget, detaching, store])

  const cancelDetach = useCallback(() => {
    if (detaching) return
    setDetachTarget(null)
    setDetachError(null)
  }, [detaching])

  return {
    actionsDisabled: !store,
    assign,
    assignError,
    assigningBillingAccountId,
    assignTargets,
    callerAccountId,
    cancelDetach,
    confirmDetach,
    detachError,
    detachTarget,
    detaching,
    requestDetach: setDetachTarget,
  }
}
