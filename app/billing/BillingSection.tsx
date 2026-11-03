import { LuCreditCard } from '@s4wave/web/images/icons.js'

import { useSessionNavigate } from '@s4wave/web/contexts/contexts.js'
import { SpacewaveOrgListContext } from '@s4wave/web/contexts/SpacewaveOrgListContext.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import { useStateNamespace, useStateAtom } from '@s4wave/web/state/persist.js'
import { BillingStatus } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import { useBillingStateContextSafe } from './BillingStateProvider.js'
import { isStatusActive } from './billing-utils.js'
import { BillingAccountCard } from './BillingAccountCard.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
  radii as themeRadii,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    color: `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule2: {
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      },
    },
    cursor: 'pointer',
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule3: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule4: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
  },
  rule5: {
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 20%, transparent)`,
    cursor: 'pointer',
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule6: {
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule7: {
    color: `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

export interface BillingSectionProps {
  isLocal: boolean
  open?: boolean
  onOpenChange?: (open: boolean) => void
  onNavigateToPath?: (path: string) => void
}

// BillingSection shows a collapsible billing section with personal and org
// billing account cards. Cloud-only.
export function BillingSection({
  sx,
  isLocal,
  open,
  onOpenChange,
  onNavigateToPath,
}: BillingSectionProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const ns = useStateNamespace(['session-settings'])
  const [storedOpen, setStoredOpen] = useStateAtom(ns, 'billing', true)
  const sectionOpen = open ?? storedOpen
  const handleOpenChange = onOpenChange ?? setStoredOpen

  const navigateSession = useSessionNavigate()
  const billingState = useBillingStateContextSafe()

  const orgList = SpacewaveOrgListContext.useContextSafe()
  const orgs = orgList?.organizations ?? []

  if (isLocal) return null
  if (!billingState) {
    throw new Error(
      'Billing state context not found. Wrap component in BillingStateProvider.',
    )
  }

  const billing = billingState.response?.billingAccount

  const hasActive = billing && isStatusActive(billing.status)

  const handleNavigate = (path: string) => {
    if (onNavigateToPath) {
      onNavigateToPath(path)
      return
    }
    navigateSession({ path })
  }

  return (
    <CollapsibleSection
      sx={sx}
      title="Billing"
      icon={<LuCreditCard sx={stylexStyles.rule1} />}
      open={sectionOpen}
      onOpenChange={handleOpenChange}
      badge={
        hasActive ? <span sx={stylexStyles2.rule1}>Active</span> : undefined
      }
    >
      <div>
        {billing && billing.status !== BillingStatus.BillingStatus_NONE && (
          <BillingAccountCard
            sx={nativeChildSpacing.s2}
            label="Personal"
            billing={billing}
            onManage={() => {
              if (billing.id) {
                handleNavigate(`billing/${billing.id}`)
                return
              }
              handleNavigate('billing')
            }}
          />
        )}
        {billing && billing.status === BillingStatus.BillingStatus_NONE && (
          <button
            type="button"
            onClick={() => {
              handleNavigate('plan')
            }}
            sx={[nativeChildSpacing.s2, stylexStyles2.rule2]}
          >
            <span sx={stylexStyles2.rule3}>Upgrade to Cloud</span>
            <p sx={stylexStyles2.rule4}>
              Subscribe to enable cloud sync and backup
            </p>
          </button>
        )}
        {!billing && (
          <LoadingInline
            sx={nativeChildSpacing.s2}
            label="Loading billing info"
            tone="muted"
            size="sm"
          />
        )}
        {orgs.flatMap((org) =>
          org.billingAccountId
            ? [
                <button
                  type="button"
                  key={org.id}
                  onClick={() => {
                    handleNavigate(`org/${org.id}/billing`)
                  }}
                  sx={[nativeChildSpacing.s2, stylexStyles2.rule5]}
                >
                  <div sx={stylexStyles2.rule6}>
                    <span sx={stylexStyles2.rule3}>
                      {org.displayName || org.id}
                    </span>
                    <span sx={stylexStyles2.rule7}>Manage</span>
                  </div>
                </button>,
              ]
            : [],
        )}
      </div>
    </CollapsibleSection>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
  },
})
