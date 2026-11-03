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
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-wider)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-wider)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textTransform: 'uppercase',
  },
  rule2: { position: 'relative' },
  rule3: {
    '--tw-outline-style': 'none',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    appearance: 'none',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    outlineStyle: 'none',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    paddingRight: `calc(${themeLayout['--spacing']} * 8)`,
    width: '100%',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
  rule4: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule5: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    '--tw-translate-y': 'calc(calc(1 / 2 * 100%) * -1)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    pointerEvents: 'none',
    position: 'absolute',
    translate: 'var(--tw-translate-x) var(--tw-translate-y)',
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    right: `calc(${themeLayout['--spacing']} * 2.5)`,
    top: '50%',
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
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
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
  },
})
