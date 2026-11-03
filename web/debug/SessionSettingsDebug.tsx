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
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
  },
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

const stylexStyles = stylex.create({
  rule1: {
    overflow: 'auto',
    backgroundColor: themeColors['--color-background'],
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    width: '100%',
  },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    height: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule3: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule4: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    WebkitUserSelect: 'none',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    userSelect: 'none',
  },
  rule6: {
    flex: 1,
    overflow: 'auto',
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule7: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule8: {
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand'],
      },
    },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule9: {
    paddingBlock: themeLayout['--spacing'],
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule10: {
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule11: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule12: {
    color: `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule13: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule14: {
    padding: `calc(${themeLayout['--spacing']} * 2.5)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 20%, transparent)`,
  },
  rule15: {
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule16: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule17: {
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
export function SessionSettingsDebug({
  sx,
}: { sx?: stylex.StyleXStyles; className?: never } = {}) {
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
    <div sx={[stylexStyles.rule1, sx]}>
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
        <div sx={stylexStyles2.rule1}>
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
