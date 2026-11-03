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
    flex: 1,
    backgroundColor: themeColors['--color-background-landing'],
    containerType: 'inline-size',
    display: 'flex',
    flexDirection: 'column',
    overflowY: 'auto',
    width: '100%',
  },
  rule2: {
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    position: 'relative',
    zIndex: 10,
    paddingTop: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    cursor: 'pointer',
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule4: {
    marginInline: 'auto',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    position: 'relative',
    textAlign: 'center',
    zIndex: 10,
    maxWidth: 'var(--container-4xl)',
    paddingBottom: {
      default: `calc(${themeLayout['--spacing']} * 14)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 16)`,
    },
    paddingTop: {
      default: `calc(${themeLayout['--spacing']} * 14)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 20)`,
    },
    width: '100%',
  },
  rule5: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: themeColors['--color-brand'],
    display: 'flex',
    justifyContent: 'center',
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule6: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    fontSize: {
      default: 'var(--text-4xl)',
      '@container (min-width: 32rem)': 'var(--text-5xl)',
    },
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: {
      default: 'var(--tw-leading, var(--text-4xl--line-height))',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-5xl--line-height))',
    },
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule7: {
    '--tw-leading': 'var(--leading-relaxed)',
    marginInline: 'auto',
    color: themeColors['--color-foreground-alt'],
    fontSize: {
      default: 'var(--text-base)',
      '@container (min-width: 32rem)': 'var(--text-lg)',
    },
    lineHeight: {
      default: 'var(--leading-relaxed)',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-lg--line-height))',
    },
    maxWidth: 'var(--container-xl)',
  },
  rule8: {
    marginInline: 'auto',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    position: 'relative',
    zIndex: 10,
    maxWidth: 'var(--container-4xl)',
    width: '100%',
  },
  rule9: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule10: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textAlign: 'center',
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule11: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-yellow-500']} 30%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-yellow-500']} 10%, transparent)`,
    textAlign: 'center',
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule12: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-yellow-400'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
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
