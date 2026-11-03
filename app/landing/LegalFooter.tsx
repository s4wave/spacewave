import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'
import { ExternalLink } from './ExternalLink.js'

import * as stylex from '@stylexjs/stylex'
import {
  fonts as themeFonts,
  colors as themeColors,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-white']} 80%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-white'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
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
    <footer className="relative z-10 mt-auto border-t border-white/10 px-4 py-6">
      <div className="mx-auto flex max-w-4xl flex-col items-center gap-3 sm:flex-row">
        <p className="text-foreground-alt/50 text-xs">
          &copy; 2018&ndash;2026{' '}
          <ExternalLink
            href="https://github.com/aperturerobotics"
            sx={stylexStyles.rule1}
          >
            Aperture Robotics
          </ExternalLink>
          , LLC. and contributors
        </p>
        <nav className="flex gap-4 sm:ml-auto">
          <a
            href={tosHref}
            className="text-foreground-alt/50 hover:text-foreground-alt text-xs transition-colors"
          >
            Terms
          </a>
          <a
            href={privacyHref}
            className="text-foreground-alt/50 hover:text-foreground-alt text-xs transition-colors"
          >
            Privacy
          </a>
          <a
            href={dmcaHref}
            className="text-foreground-alt/50 hover:text-foreground-alt text-xs transition-colors"
          >
            DMCA
          </a>
          <a
            href={pricingHref}
            className="text-foreground-alt/50 hover:text-foreground-alt text-xs transition-colors"
          >
            Pricing
          </a>
          <a
            href={downloadHref}
            className="text-foreground-alt/50 hover:text-foreground-alt text-xs transition-colors"
          >
            Download
          </a>
          <a
            href={licensesHref}
            className="text-foreground-alt/50 hover:text-foreground-alt text-xs transition-colors"
          >
            Licenses
          </a>
          <a
            href={blogHref}
            className="text-foreground-alt/50 hover:text-foreground-alt text-xs transition-colors"
          >
            Blog
          </a>
        </nav>
      </div>
    </footer>
  )
}
