import { useCallback, useState } from 'react'
import { LuArrowRight, LuChevronDown } from '@s4wave/web/images/icons.js'

import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { SpacewaveOrgListContext } from '@s4wave/web/contexts/SpacewaveOrgListContext.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
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
  },
  rule2: { position: 'relative' },
  rule3: {
    width: '100%',
    appearance: 'none',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingRight: `calc(${themeLayout['--spacing']} * 8)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
  rule4: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule5: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
})

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

  return (
    <div>
      <div sx={[nativeChildSpacing.s3, stylexStyles2.rule1]}>Transfer to</div>
      <div sx={[nativeChildSpacing.s3, stylexStyles2.rule2]}>
        <select
          aria-label="Transfer to"
          value={targetOrgId}
          onChange={(e) => setTargetOrgId(e.target.value)}
          sx={[stylexStyles2.rule3]}
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
        <div sx={[nativeChildSpacing.s3, stylexStyles2.rule4]}>
          Already owned by this{' '}
          {targetOwnerType === 'organization' ? 'organization' : 'account'}.
        </div>
      )}
      <DashboardButton
        sx={nativeChildSpacing.s3}
        icon={<LuArrowRight sx={stylexStyles.rule2} />}
        onClick={() => void handleTransfer()}
        disabled={transferring || sameAsCurrent}
      >
        {transferring ? 'Transferring…' : 'Transfer'}
      </DashboardButton>
      {error && (
        <div sx={[nativeChildSpacing.s3, stylexStyles2.rule5]}>{error}</div>
      )}
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
