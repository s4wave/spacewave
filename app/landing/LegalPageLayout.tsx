import { LuArrowLeft } from '@s4wave/web/images/icons.js'
import { LegalFooter } from './LegalFooter.js'
import { useLandingBackNavigation } from './useLandingBackNavigation.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  colors as themeColors,
  fonts as themeFonts,
  motion as themeMotion,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    containerType: 'inline-size',
    display: 'flex',
    width: '100%',
    flex: 1,
    flexDirection: 'column',
    overflowY: 'auto',
    backgroundColor: themeColors['--color-background-landing'],
  },
  rule2: {
    position: 'relative',
    zIndex: 10,
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingTop: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule3: {
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
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
    position: 'relative',
    zIndex: 10,
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-4xl)',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingTop: {
      default: `calc(${themeLayout['--spacing']} * 14)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 20)`,
    },
    paddingBottom: {
      default: `calc(${themeLayout['--spacing']} * 14)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 16)`,
    },
    textAlign: 'center',
  },
  rule5: {
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    color: themeColors['--color-brand'],
  },
  rule6: {
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
    fontSize: {
      default: 'var(--text-4xl)',
      '@container (min-width: 32rem)': 'var(--text-5xl)',
    },
    lineHeight: {
      default: 'var(--tw-leading, var(--text-4xl--line-height))',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-5xl--line-height))',
    },
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  rule7: {
    marginInline: 'auto',
    maxWidth: 'var(--container-xl)',
    fontSize: {
      default: 'var(--text-base)',
      '@container (min-width: 32rem)': 'var(--text-lg)',
    },
    lineHeight: {
      default: 'var(--leading-relaxed)',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-lg--line-height))',
    },
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule8: {
    position: 'relative',
    zIndex: 10,
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-4xl)',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
  },
  rule9: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule10: {
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
    textAlign: 'center',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule11: {
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-yellow-500']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-yellow-500']} 10%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    textAlign: 'center',
  },
  rule12: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-yellow-400'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

// LegalPageLayoutProps defines the props for LegalPageLayout.
interface LegalPageLayoutProps {
  icon?: React.ReactElement
  title: string
  subtitle?: string
  children: React.ReactNode
  lastUpdated?: string
  draftBanner?: boolean
}

// LegalPageLayout renders the shared layout for legal and informational pages.
export function LegalPageLayout({
  icon,
  title,
  subtitle,
  children,
  lastUpdated,
  draftBanner,
}: LegalPageLayoutProps) {
  const goBack = useLandingBackNavigation()

  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <button type="button" onClick={goBack} sx={stylexStyles2.rule3}>
          <LuArrowLeft sx={stylexStyles.rule1} />
          Back
        </button>
      </div>

      <header sx={stylexStyles2.rule4}>
        {icon && <div sx={stylexStyles2.rule5}>{icon}</div>}
        <h1 sx={stylexStyles2.rule6}>{title}</h1>
        {subtitle && <p sx={stylexStyles2.rule7}>{subtitle}</p>}
      </header>

      {(lastUpdated || draftBanner) && (
        <div sx={stylexStyles2.rule8}>
          {lastUpdated && <p sx={stylexStyles2.rule9}>{lastUpdated}</p>}
          {draftBanner && (
            <>
              <p sx={stylexStyles2.rule10}>Effective Date: TBD</p>
              <div sx={stylexStyles2.rule11}>
                <p sx={stylexStyles2.rule12}>DRAFT</p>
              </div>
            </>
          )}
        </div>
      )}

      {children}

      <LegalFooter />
    </div>
  )
}
