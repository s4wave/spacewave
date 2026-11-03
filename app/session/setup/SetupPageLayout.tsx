import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    position: 'relative',
    zIndex: 10,
    display: 'flex',
    width: '100%',
    maxWidth: 'var(--container-md)',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 6)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    position: 'relative',
    display: 'flex',
    flex: 1,
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    overflowY: 'auto',
    backgroundColor: themeColors['--color-background-landing'],
    padding: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@media (min-width: 48rem)': `calc(${themeLayout['--spacing']} * 10)`,
    },
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
  },
  rule2: {
    position: 'absolute',
    top: `calc(${themeLayout['--spacing']} * 4)`,
    left: `calc(${themeLayout['--spacing']} * 4)`,
    zIndex: 20,
  },
  rule3: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule4: {
    fontSize: 'var(--text-xl)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
  },
  rule5: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
})

// SetupPageLayoutProps configures the shared setup page layout.
export interface SetupPageLayoutProps {
  title: string
  subtitle?: string
  sx?: stylex.StyleXStyles
  contentSx?: stylex.StyleXStyles
  className?: never
  topLeft?: React.ReactNode
  showHeader?: boolean
  children: React.ReactNode
}

// SetupPageLayout renders the shared outer layout for setup pages.
export function SetupPageLayout({
  title,
  subtitle,
  sx,
  contentSx,
  topLeft,
  showHeader = true,
  children,
}: SetupPageLayoutProps) {
  return (
    <div sx={[stylexStyles.rule1, sx]}>
      {topLeft && <div sx={stylexStyles.rule2}>{topLeft}</div>}
      <div sx={[stylexStyles2.rule1, contentSx]}>
        {showHeader && (
          <div sx={stylexStyles.rule3}>
            <AnimatedLogo followMouse={false} />
            <h1 sx={stylexStyles.rule4}>{title}</h1>
            {subtitle && <p sx={stylexStyles.rule5}>{subtitle}</p>}
          </div>
        )}
        {children}
      </div>
    </div>
  )
}
