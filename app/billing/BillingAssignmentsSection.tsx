import { useCallback, useMemo, useState } from 'react'
import { LuCheck, LuLink, LuX } from '@s4wave/web/images/icons.js'

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

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  utilityState as themeUtilityState,
  motion as themeMotion,
  radii as themeRadii,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wider)',
    letterSpacing: 'var(--tracking-wider)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    textTransform: 'uppercase',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule2: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule3: {
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule4: {
    display: 'flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule5: {
    cursor: 'pointer',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    color: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-destructive'],
      },
    },
  },
  rule6: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule7: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule8: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: { color: themeColors['--color-brand'] },
  rule3: { color: '#0000' },
})

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
    <div>
      <div sx={[nativeChildSpacing.s3, stylexStyles2.rule1]}>Assigned to</div>
      {loading && !managedBillingAccount && (
        <LoadingInline
          sx={nativeChildSpacing.s3}
          label="Loading assignments"
          tone="muted"
          size="sm"
        />
      )}
      {!loading && assignees.length === 0 && (
        <div sx={[nativeChildSpacing.s3, stylexStyles2.rule2]}>
          Unassigned. Pick a principal below to link this billing account.
        </div>
      )}
      {assignees.length > 0 && (
        <div sx={[nativeChildSpacing.s3, stylexStyles2.rule3]}>
          {assignees.map((a) => {
            const isPersonal =
              a.ownerType === 'account' && a.ownerId === callerAccountId
            const label = isPersonal
              ? 'Personal'
              : a.displayName || a.ownerId || ''
            return (
              <span
                key={`${a.ownerType}:${a.ownerId}`}
                sx={stylexStyles2.rule4}
              >
                <span>{label}</span>
                <button
                  type="button"
                  onClick={() =>
                    setDetachTarget({
                      ownerType: a.ownerType as 'account' | 'organization',
                      ownerId: a.ownerId ?? '',
                      label,
                    })
                  }
                  aria-label={`Detach ${label}`}
                  sx={stylexStyles2.rule5}
                >
                  <LuX sx={stylexStyles.rule1} />
                </button>
              </span>
            )
          })}
        </div>
      )}
      <div sx={[nativeChildSpacing.s3, stylexStyles2.rule6]}>
        <DropdownMenu>
          <DropdownMenuTrigger asChild disabled={menuDisabled}>
            <DropdownTriggerButton icon={<LuLink sx={stylexStyles.rule1} />}>
              {assigning ? 'Assigning…' : 'Assign to…'}
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
                    sx={[
                      stylexStyles.rule1,
                      isSelected ? stylexStyles.rule2 : stylexStyles.rule3,
                    ]}
                  />
                  <span>{t.label}</span>
                </DropdownMenuItem>
              )
            })}
          </DropdownMenuContent>
        </DropdownMenu>
        {noTargets && (
          <span sx={stylexStyles2.rule7}>
            No owned organizations. Create one to assign.
          </span>
        )}
      </div>
      {assignError && (
        <div sx={[nativeChildSpacing.s3, stylexStyles2.rule8]}>
          {assignError}
        </div>
      )}
      {detachError && (
        <div sx={[nativeChildSpacing.s3, stylexStyles2.rule8]}>
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

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
