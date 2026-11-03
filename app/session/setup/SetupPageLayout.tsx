import AnimatedLogo from '@s4wave/app/landing/AnimatedLogo.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
    zIndex: 10,
    maxWidth: 'var(--container-md)',
    width: '100%',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    '--tw-outline-style': 'none',
    padding: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@media (min-width: 48rem)': `calc(${themeLayout['--spacing']} * 10)`,
    },
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    backgroundColor: themeColors['--color-background-landing'],
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'center',
    outlineStyle: 'none',
    position: 'relative',
    overflowY: 'auto',
  },
  rule2: {
    position: 'absolute',
    zIndex: 20,
    left: `calc(${themeLayout['--spacing']} * 4)`,
    top: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
  },
  rule4: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    fontSize: 'var(--text-xl)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-wide)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
  },
  rule5: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
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
