import { controlTargetStyles } from '@s4wave/web/style/ControlStylesProvider.js'
import { LuArrowLeft } from '@s4wave/web/images/icons.js'

import type { Release } from '@s4wave/core/changelog/changelog.pb.js'

import { LegalFooter } from './LegalFooter.js'
import { ReleaseCard } from './ReleaseCard.js'
import { ChangelogReleaseDownloads } from './ChangelogReleaseDownloads.js'
import { useLandingBackNavigation } from './useLandingBackNavigation.js'

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
      default: `calc(${themeLayout['--spacing']} * 8)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 10)`,
    },
    paddingTop: {
      default: `calc(${themeLayout['--spacing']} * 14)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 20)`,
    },
    width: '100%',
  },
  rule5: {
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
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule6: {
    color: themeColors['--color-foreground-alt'],
    fontSize: {
      default: 'var(--text-base)',
      '@container (min-width: 32rem)': 'var(--text-lg)',
    },
    lineHeight: {
      default: 'var(--tw-leading, var(--text-base--line-height))',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-lg--line-height))',
    },
  },
  rule7: {
    gap: `calc(${themeLayout['--spacing']} * 12)`,
    marginInline: 'auto',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
    zIndex: 10,
    maxWidth: 'var(--container-4xl)',
    paddingBottom: {
      default: `calc(${themeLayout['--spacing']} * 14)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 16)`,
    },
    width: '100%',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

// ChangelogReleasePage renders a single release's notes plus the desktop-app
// download options for that version.
export function ChangelogReleasePage({ release }: { release: Release }) {
  const goBack = useLandingBackNavigation()

  return (
    <div sx={stylexStyles2.rule1}>
      {/* Back button */}
      <div sx={stylexStyles2.rule2}>
        <button
          type="button"
          onClick={goBack}
          sx={[stylexStyles2.rule3, controlTargetStyles.minimum]}
        >
          <LuArrowLeft sx={stylexStyles.rule1} />
          Back
        </button>
      </div>

      {/* Hero */}
      <header sx={stylexStyles2.rule4}>
        <h1 sx={stylexStyles2.rule5}>Spacewave v{release.version}</h1>
        {release.date && (
          <p sx={stylexStyles2.rule6}>Released {release.date}</p>
        )}
      </header>

      <section sx={stylexStyles2.rule7}>
        <ReleaseCard release={release} />
        <ChangelogReleaseDownloads version={release.version ?? ''} />
      </section>

      <LegalFooter />
    </div>
  )
}
