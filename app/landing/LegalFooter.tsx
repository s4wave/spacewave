import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { ExternalLink } from './ExternalLink.js'

import * as stylex from '@stylexjs/stylex'
import {
  fonts as themeFonts,
  colors as themeColors,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  layout as themeLayout,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-white']} 10%, transparent)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 6)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    position: 'relative',
    zIndex: 10,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    marginTop: 'auto',
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    marginInline: 'auto',
    alignItems: 'center',
    display: 'flex',
    flexDirection: { default: 'column', '@media (min-width: 40rem)': 'row' },
    maxWidth: 'var(--container-4xl)',
  },
  rule3: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule4: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'flex',
    marginLeft: { default: null, '@media (min-width: 40rem)': 'auto' },
  },
  rule5: {
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground-alt'],
      },
    },
    fontSize: themeFonts['--text-xs'],
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
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-white']} 80%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-white'],
      },
    },
    fontWeight: themeFonts['--font-weight-medium'],
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

// LegalFooter renders the footer for legal pages (ToS, Privacy, Pricing, DMCA).
export function LegalFooter() {
  const tosHref = useStaticHref('/tos')
  const privacyHref = useStaticHref('/privacy')
  const dmcaHref = useStaticHref('/dmca')
  const pricingHref = useStaticHref('/pricing')
  const downloadHref = useStaticHref('/download')
  const licensesHref = useStaticHref('/licenses')
  const blogHref = useStaticHref('/blog')

  return (
    <footer sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <p sx={stylexStyles2.rule3}>
          &copy; 2018&ndash;2026{' '}
          <ExternalLink
            href="https://github.com/aperturerobotics"
            sx={stylexStyles.rule1}
          >
            Aperture Robotics
          </ExternalLink>
          , LLC. and contributors
        </p>
        <nav sx={stylexStyles2.rule4}>
          <a href={tosHref} sx={stylexStyles2.rule5}>
            Terms
          </a>
          <a href={privacyHref} sx={stylexStyles2.rule5}>
            Privacy
          </a>
          <a href={dmcaHref} sx={stylexStyles2.rule5}>
            DMCA
          </a>
          <a href={pricingHref} sx={stylexStyles2.rule5}>
            Pricing
          </a>
          <a href={downloadHref} sx={stylexStyles2.rule5}>
            Download
          </a>
          <a href={licensesHref} sx={stylexStyles2.rule5}>
            Licenses
          </a>
          <a href={blogHref} sx={stylexStyles2.rule5}>
            Blog
          </a>
        </nav>
      </div>
    </footer>
  )
}
