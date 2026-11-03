import { useNavigate } from '@s4wave/web/router/router.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  layout as themeLayout,
  fonts as themeFonts,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    flex: 1,
    backgroundColor: themeColors['--color-background'],
    display: 'flex',
    height: '100%',
    overflowY: 'auto',
    width: '100%',
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    marginInline: 'auto',
    display: 'flex',
    flexDirection: 'column',
    maxWidth: 'var(--container-3xl)',
    width: '100%',
  },
  rule3: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-brand'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: '.22em',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textTransform: 'uppercase',
  },
  rule4: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-2xl)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: 'var(--tw-leading, var(--text-2xl--line-height))',
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule5: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule6: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'grid',
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 40rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule7: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
      },
    },
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: themeLayout['--spacing'],
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-background-card'],
      },
    },
    display: 'flex',
    flexDirection: 'column',
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule8: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule9: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 65%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
  },
  rule10: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    color: `color-mix(in oklab, ${themeColors['--color-brand']} 80%, transparent)`,
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    marginTop: themeLayout['--spacing'],
  },
})

interface DebugIndexLink {
  path: string
  title: string
  description: string
}

interface DebugIndexProps {
  links: DebugIndexLink[]
}

// DebugIndex renders the development route index.
export function DebugIndex({ links }: DebugIndexProps) {
  const navigate = useNavigate()

  return (
    <div sx={stylexStyles.rule1}>
      <div sx={stylexStyles.rule2}>
        <div>
          <p sx={stylexStyles.rule3}>Developer</p>
          <h1 sx={stylexStyles.rule4}>Debug tools</h1>
          <p sx={stylexStyles.rule5}>
            Inspect rendering fixtures, storage benchmarks, and debug-only UI
            surfaces.
          </p>
        </div>

        <div sx={stylexStyles.rule6}>
          {links.map((link) => (
            <a
              key={link.path}
              href={`#${link.path}`}
              onClick={(event) => {
                event.preventDefault()
                navigate({ path: link.path })
              }}
              sx={stylexStyles.rule7}
            >
              <span sx={stylexStyles.rule8}>{link.title}</span>
              <span sx={stylexStyles.rule9}>{link.description}</span>
              <span sx={stylexStyles.rule10}>{link.path}</span>
            </a>
          ))}
        </div>
      </div>
    </div>
  )
}
