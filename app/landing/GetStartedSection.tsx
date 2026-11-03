import React, { useCallback, useSyncExternalStore } from 'react'
import * as stylex from '@stylexjs/stylex'

import type { SessionListEntry } from '@s4wave/core/session/session.pb.js'
import { useNavigate } from '@s4wave/web/router/router.js'

import AnimatedLogo from './AnimatedLogo.js'
import { ExternalLink } from './ExternalLink.js'
import GetStarted from './GetStarted.js'
import { NavigationLinks } from './NavigationLinks.js'

import {
  motion as themeMotion,
  colors as themeColors,
  layout as themeLayout,
} from '../../web/style/tokens.stylex.js'

import { fonts as themeFonts } from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    cursor: 'pointer',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * .5)`,
    transitionProperty: 'opacity',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    minWidth: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    justifyContent: { default: null, '@media (pointer: coarse)': 'center' },
  },
  rule2: { opacity: 1 },
  rule3: {
    pointerEvents: { default: 'none', '@media (max-height: 580px)': 'auto' },
    opacity: { default: 0, '@media (max-height: 580px)': 1 },
  },
  rule4: {
    animationName: 'pulse',
    animationDuration: '8s',
    animationTimingFunction: 'ease-in-out',
    animationIterationCount: 'infinite',
    animationDirection: 'normal',
    animationPlayState: 'running',
    animationDelay: '0s',
    animationFillMode: 'none',
    animationTimeline: 'auto',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    position: 'relative',
    display: 'flex',
    minHeight: '100%',
    width: '100%',
    flexDirection: 'column',
    paddingTop: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
      '@container (min-width: 42rem)': '2.84rem',
    },
    flexShrink: { default: null, '@media (max-height: 580px)': 0 },
  },
  rule2: {
    display: { default: 'none', '@media (min-height: 800px)': 'block' },
    flex: { default: null, '@media (min-height: 800px)': 1 },
  },
  rule3: {
    marginBottom: themeLayout['--spacing'],
    display: 'flex',
    minHeight: 0,
    flex: {
      default: 1,
      '@media (min-height: 800px)': '0 auto',
      '@media (max-height: 580px)': 'none',
    },
    flexDirection: 'column',
    alignItems: 'center',
    gap: {
      default: `calc(${themeLayout['--spacing']} * 2)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 3)`,
      '@container (min-width: 42rem)': `calc(${themeLayout['--spacing']} * 4)`,
    },
  },
  rule4: {
    fontSize: {
      default: 'var(--text-2xl)',
      '@container (min-width: 32rem)': 'var(--text-3xl)',
    },
    lineHeight: {
      default: 'var(--tw-leading, var(--text-2xl--line-height))',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-3xl--line-height))',
    },
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: {
      default: '.1rem',
      '@container (min-width: 32rem)': '.142rem',
    },
    whiteSpace: 'nowrap',
    display: { default: null, '@media (max-height: 390px)': 'none' },
  },
  rule5: {
    display: 'flex',
    minHeight: 0,
    width: '100%',
    maxWidth: 'var(--container-2xl)',
    flex: {
      default: 1,
      '@media (min-height: 800px)': '0 auto',
      '@media (max-height: 580px)': 'none',
    },
    flexDirection: 'column',
    gap: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 6)`,
    },
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule6: { textAlign: 'center', color: themeColors['--color-foreground-alt'] },
  rule7: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule8: { color: themeColors['--color-white'] },
  rule9: {
    marginTop: 'auto',
    display: 'flex',
    flexShrink: 0,
    flexDirection: 'column',
    alignItems: 'center',
    paddingTop: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 6)`,
    },
    textAlign: 'center',
  },
  rule10: {
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    justifyContent: 'center',
    columnGap: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule11: {
    cursor: 'pointer',
    textDecorationLine: 'underline',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    color: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand'],
      },
    },
    display: { default: null, '@media (pointer: coarse)': 'inline-flex' },
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    minWidth: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    alignItems: { default: null, '@media (pointer: coarse)': 'center' },
    justifyContent: { default: null, '@media (pointer: coarse)': 'center' },
  },
  rule12: {
    fontSize: '10px',
    lineHeight: 1.5,
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    textTransform: 'uppercase',
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule13: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-bold)',
    fontWeight: 'var(--font-weight-bold)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    textDecorationLine: 'underline',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    color: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand'],
      },
    },
    display: { default: null, '@media (pointer: coarse)': 'inline-flex' },
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    minWidth: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    alignItems: { default: null, '@media (pointer: coarse)': 'center' },
    justifyContent: { default: null, '@media (pointer: coarse)': 'center' },
  },
})

interface GetStartedSectionProps {
  homeRef: React.RefObject<HTMLDivElement | null>
  showScrollIndicator: boolean
  animateScrollIndicator: boolean
  scrollDown: () => void
  sessions?: SessionListEntry[]
}

const shortHeightQuery = '(max-height: 580px)'
const styles = stylex.create({ launcher: { position: 'relative', zIndex: 10 } })

function subscribeShortHeight(onChange: () => void) {
  const query = window.matchMedia(shortHeightQuery)
  query.addEventListener('change', onChange)
  return () => query.removeEventListener('change', onChange)
}

function getShortHeight() {
  return window.matchMedia(shortHeightQuery).matches
}

/** GetStartedSection renders the landing entry and its reachable attribution controls. */
export function GetStartedSection({
  homeRef,
  showScrollIndicator,
  animateScrollIndicator,
  scrollDown,
  sessions,
}: GetStartedSectionProps) {
  const navigate = useNavigate()
  const isShortHeight = useSyncExternalStore(
    subscribeShortHeight,
    getShortHeight,
    () => false,
  )

  const goToCommunity = useCallback(() => {
    navigate({ path: '/community' })
  }, [navigate])
  return (
    <div ref={homeRef} sx={stylexStyles2.rule1}>
      {/* Spacer to center content on tall screens */}
      <div sx={stylexStyles2.rule2} />

      {/* Logo and Navigation Section */}
      <div sx={stylexStyles2.rule3}>
        <AnimatedLogo
          followMouse={true}
          containerSx={logoContainerStyles.root}
        />
        <h1 sx={stylexStyles2.rule4}>[SPACEWAVE]</h1>

        <NavigationLinks />

        <div sx={stylexStyles2.rule5}>
          <GetStarted sx={styles.launcher} sessions={sessions} />

          {/* Description Section */}
          <div sx={stylexStyles2.rule6}>
            <p sx={stylexStyles2.rule7}>
              local-first, end-to-end encrypted,{' '}
              <span sx={stylexStyles2.rule8}>no account required</span>
            </p>
          </div>
        </div>
      </div>

      {/* Bottom spacer to center content on tall screens */}
      <div sx={stylexStyles2.rule2} />

      {/* Footer content follows the entry actions on short screens. */}
      <div sx={stylexStyles2.rule9}>
        <div sx={stylexStyles2.rule10}>
          <span>
            Made by{' '}
            <button
              type="button"
              onClick={goToCommunity}
              sx={stylexStyles2.rule11}
            >
              the community
            </button>
          </span>
          <span aria-hidden>·</span>
          <span>Free software</span>
          <span aria-hidden>·</span>
          <ExternalLink href="https://spacemacs.org" sx={stylexStyles.rule1}>
            Inspired by Spacemacs
          </ExternalLink>
        </div>

        {/* Scroll indicator */}
        <button
          type="button"
          tabIndex={showScrollIndicator || isShortHeight ? 0 : -1}
          aria-label="Scroll down to learn more"
          sx={[
            [
              stylexStyles3.rule1,
              showScrollIndicator ? stylexStyles3.rule2 : stylexStyles3.rule3,
              !!(showScrollIndicator && animateScrollIndicator) &&
                stylexStyles3.rule4,
            ],
          ]}
          onClick={scrollDown}
        >
          <span sx={stylexStyles2.rule12}>Learn more</span>
          <span sx={stylexStyles2.rule13}>▼</span>
        </button>
      </div>
    </div>
  )
}

// logoContainerStyles preserves the caller's logo spacing and height threshold.
const logoContainerStyles = stylex.create({
  root: { display: { default: null, '@media (max-height: 470px)': 'none' } },
})
