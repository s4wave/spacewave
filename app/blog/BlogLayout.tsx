import { useCallback } from 'react'
import { LuArrowLeft } from '@s4wave/web/images/icons.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { LegalFooter } from '@s4wave/app/landing/LegalFooter.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  colors as themeColors,
  fonts as themeFonts,
  motion as themeMotion,
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
    marginInline: 'auto',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    position: 'relative',
    zIndex: 10,
    maxWidth: 'var(--container-3xl)',
    paddingTop: `calc(${themeLayout['--spacing']} * 4)`,
    width: '100%',
  },
  rule3: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    cursor: 'pointer',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

// BlogLayoutProps defines the props for BlogLayout.
interface BlogLayoutProps {
  children: React.ReactNode
  showBack?: boolean
}

// BlogLayout renders the premium layout for blog reading pages.
export function BlogLayout({ children, showBack = true }: BlogLayoutProps) {
  const navigate = useNavigate()

  const goBack = useCallback(() => {
    navigate({ path: '/blog' })
  }, [navigate])

  return (
    <div sx={stylexStyles2.rule1}>
      {showBack && (
        <div sx={stylexStyles2.rule2}>
          <button type="button" onClick={goBack} sx={stylexStyles2.rule3}>
            <LuArrowLeft sx={stylexStyles.rule1} />
            All posts
          </button>
        </div>
      )}

      {children}

      <LegalFooter />
    </div>
  )
}
