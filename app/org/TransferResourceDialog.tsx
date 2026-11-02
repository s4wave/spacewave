import { useCallback, useState } from 'react'
import { LuArrowRight, LuChevronDown } from '@s4wave/web/images/icons.js'
import { cn } from '@s4wave/web/style/utils.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { SpacewaveOrgListContext } from '@s4wave/web/contexts/SpacewaveOrgListContext.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    pointerEvents: 'none',
    position: 'absolute',
    top: '50%',
    right: `calc(${themeLayout['--spacing']} * 2.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    '--tw-translate-y': 'calc(calc(1 / 2 * 100%) * -1)',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

// TransferResourceDialog lets the user transfer a space between personal
// (their own account) and an organization. Personal target is encoded as the
// caller's own accountId under the typed principal model.
export function TransferResourceDialog(props: {
  resourceId: string
  currentOwnerType: string
  currentOwnerId: string
  selfAccountId: string
  onDone?: () => void
}) {
  const {
    resourceId,
    currentOwnerType,
    currentOwnerId,
    selfAccountId,
    onDone,
  } = props
  const session = SessionContext.useContext().value
  const orgListCtx = SpacewaveOrgListContext.useContextSafe()
  const organizations = orgListCtx?.organizations ?? []

  // Empty string in the select means "Personal" -> account principal targeting
  // the caller's own account.
  const [targetOrgId, setTargetOrgId] = useState('')
  const [transferring, setTransferring] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const targetOwnerType = targetOrgId ? 'organization' : 'account'
  const targetOwnerId = targetOrgId ? targetOrgId : selfAccountId
  const sameAsCurrent =
    targetOwnerType === currentOwnerType && targetOwnerId === currentOwnerId

  const handleTransfer = useCallback(async () => {
    if (!session || transferring) return
    if (sameAsCurrent) return
    setTransferring(true)
    setError(null)
    try {
      await session.spacewave.transferResource(
        resourceId,
        targetOwnerType,
        targetOwnerId,
      )
      onDone?.()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Transfer failed')
    } finally {
      setTransferring(false)
    }
  }, [
    session,
    resourceId,
    sameAsCurrent,
    targetOwnerType,
    targetOwnerId,
    transferring,
    onDone,
  ])

  const inputClass = cn(
    'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-2 text-sm transition-colors outline-none appearance-none pr-8',
    'focus:border-brand/50',
  )

  return (
    <div className="space-y-3">
      <div className="text-foreground-alt/60 text-xs font-medium tracking-wider uppercase">
        Transfer to
      </div>
      <div className="relative">
        <select
          aria-label="Transfer to"
          value={targetOrgId}
          onChange={(e) => setTargetOrgId(e.target.value)}
          className={inputClass}
        >
          <option value="">Personal</option>
          {organizations.map((org) => (
            <option key={org.id} value={org.id}>
              {org.displayName || org.id}
            </option>
          ))}
        </select>
        <LuChevronDown sx={stylexStyles.rule1} />
      </div>
      {sameAsCurrent && (
        <div className="text-foreground-alt/50 text-xs">
          Already owned by this{' '}
          {targetOwnerType === 'organization' ? 'organization' : 'account'}.
        </div>
      )}
      <DashboardButton
        icon={<LuArrowRight sx={stylexStyles.rule2} />}
        onClick={() => void handleTransfer()}
        disabled={transferring || sameAsCurrent}
      >
        {transferring ? 'Transferring…' : 'Transfer'}
      </DashboardButton>
      {error && <div className="text-destructive text-xs">{error}</div>}
    </div>
  )
}
