import { useCallback } from 'react'
import {
  LuArrowLeft,
  LuShield,
  LuCreditCard,
  LuBuilding2,
  LuKey,
  LuFingerprint,
} from '@s4wave/web/images/icons.js'

import { useNavigate } from '@s4wave/web/router/router.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { useStateNamespace, useStateAtom } from '@s4wave/web/state/persist.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  layout as themeLayout,
  utilityState as themeUtilityState,
  motion as themeMotion,
  fonts as themeFonts,
  radii as themeRadii,
} from '../style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: { marginInline: 'auto', maxWidth: 'var(--container-lg)' },
})

const childSpacing = stylex.create({
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
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    height: '100%',
    width: '100%',
    flexDirection: 'column',
    overflow: 'auto',
    backgroundColor: themeColors['--color-background'],
  },
  rule2: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    flexShrink: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule6: {
    flex: 1,
    overflow: 'auto',
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule7: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule8: {
    width: '100%',
    textAlign: 'left',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule9: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingBlock: themeLayout['--spacing'],
  },
  rule10: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule11: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule12: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
  },
  rule13: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
  },
  rule14: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 20%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule15: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  rule16: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule17: {
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
  },
})

// useSectionOpen returns persistent open/close state for a settings section.
function useSectionOpen(
  key: string,
  defaultOpen: boolean,
): [boolean, (open: boolean) => void] {
  const ns = useStateNamespace(['session-settings-debug'])
  return useStateAtom(ns, key, defaultOpen)
}

// SessionSettingsDebug renders CollapsibleSection variants for visual
// iteration on the settings page redesign.
export function SessionSettingsDebug() {
  const navigate = useNavigate()
  const goBack = useCallback(() => {
    navigate({ path: '/' })
  }, [navigate])

  const [securityOpen, setSecurityOpen] = useSectionOpen('security', true)
  const [billingOpen, setBillingOpen] = useSectionOpen('billing', true)
  const [orgsOpen, setOrgsOpen] = useSectionOpen('orgs', false)
  const [cryptoOpen, setCryptoOpen] = useSectionOpen('crypto', false)
  const [identifiersOpen, setIdentifiersOpen] = useSectionOpen(
    'identifiers',
    false,
  )

  return (
    <div sx={stylexStyles.rule1}>
      <div sx={stylexStyles.rule2}>
        <button
          type="button"
          onClick={goBack}
          aria-label="Go back"
          sx={stylexStyles.rule3}
        >
          <LuArrowLeft sx={stylexStyles.rule4} />
        </button>
        <span sx={stylexStyles.rule5}>Session Settings Variants</span>
      </div>

      <div sx={stylexStyles.rule6}>
        <div
          sx={
            /* TODO(stylex): a descendant, group, peer or compound selector: space-y-3 */ stylexStyles2.rule1
          }
        >
          <CollapsibleSection
            sx={childSpacing.s3}
            title="Security"
            icon={<LuShield sx={stylexStyles.rule7} />}
            open={securityOpen}
            onOpenChange={setSecurityOpen}
          >
            <div>
              <PlaceholderRow
                sx={childSpacing.s2}
                label="Session Lock"
                detail="PIN lock enabled"
              />
              <PlaceholderRow
                sx={childSpacing.s2}
                label="Security Level"
                detail="Standard (any one method)"
              />
            </div>
          </CollapsibleSection>

          <CollapsibleSection
            sx={childSpacing.s3}
            title="Billing"
            icon={<LuCreditCard sx={stylexStyles.rule7} />}
            open={billingOpen}
            onOpenChange={setBillingOpen}
            badge={<StatusBadge label="Active" />}
          >
            <div>
              <PlaceholderCard
                sx={childSpacing.s2}
                title="Personal"
                detail="Active Monthly | Next: May 12"
              />
              <PlaceholderCard
                sx={childSpacing.s2}
                title="Acme Corp"
                detail="Active Annual | Next: Jan 1"
              />
            </div>
          </CollapsibleSection>

          <CollapsibleSection
            sx={childSpacing.s3}
            title="Organizations"
            icon={<LuBuilding2 sx={stylexStyles.rule7} />}
            open={orgsOpen}
            onOpenChange={setOrgsOpen}
            badge={<CountBadge count={2} />}
          >
            <div>
              <PlaceholderRow
                sx={childSpacing.s2}
                label="Acme Corp"
                detail="3 members"
                action
              />
              <PlaceholderRow
                sx={childSpacing.s2}
                label="Side Project"
                detail="1 member"
                action
              />
              <button type="button" sx={[stylexStyles.rule8, childSpacing.s2]}>
                + Create Organization
              </button>
            </div>
          </CollapsibleSection>

          <CollapsibleSection
            sx={childSpacing.s3}
            title="Crypto & Keys"
            icon={<LuKey sx={stylexStyles.rule7} />}
            open={cryptoOpen}
            onOpenChange={setCryptoOpen}
          >
            <div>
              <PlaceholderRow
                sx={childSpacing.s2}
                label="Key Type"
                detail="Ed25519"
              />
              <PlaceholderRow
                sx={childSpacing.s2}
                label="Export Public Key (PEM)"
                action
              />
            </div>
          </CollapsibleSection>

          <CollapsibleSection
            sx={childSpacing.s3}
            title="Identifiers"
            icon={<LuFingerprint sx={stylexStyles.rule7} />}
            open={identifiersOpen}
            onOpenChange={setIdentifiersOpen}
          >
            <div>
              <PlaceholderRow
                sx={childSpacing.s2}
                label="Session ID"
                detail="abc123...def456"
              />
              <PlaceholderRow
                sx={childSpacing.s2}
                label="Peer ID"
                detail="12D3KooW...xyz"
              />
              <PlaceholderRow
                sx={childSpacing.s2}
                label="Account ID"
                detail="01HXYZ...789"
              />
            </div>
          </CollapsibleSection>
        </div>
      </div>
    </div>
  )
}

function PlaceholderRow({
  label,
  detail,
  action,
  sx,
}: {
  label: string
  detail?: string
  action?: boolean
  sx?: stylex.StyleXStyles
}) {
  return (
    <div sx={[stylexStyles.rule9, sx]}>
      <span sx={stylexStyles.rule10}>{label}</span>
      {detail && <span sx={stylexStyles.rule11}>{detail}</span>}
      {action && !detail && <span sx={stylexStyles.rule12}>Export</span>}
      {action && detail && <span sx={stylexStyles.rule13}>Open &gt;</span>}
    </div>
  )
}

function PlaceholderCard({
  title,
  detail,
  sx,
}: {
  title: string
  detail: string
  sx?: stylex.StyleXStyles
}) {
  return (
    <div sx={[stylexStyles.rule14, sx]}>
      <div sx={stylexStyles.rule15}>
        <span sx={stylexStyles.rule16}>{title}</span>
        <span sx={stylexStyles.rule12}>Manage</span>
      </div>
      <span sx={stylexStyles.rule11}>{detail}</span>
    </div>
  )
}

function StatusBadge({ label }: { label: string }) {
  return <span sx={stylexStyles.rule17}>{label}</span>
}

function CountBadge({ count }: { count: number }) {
  return <span sx={stylexStyles.rule11}>{count}</span>
}
