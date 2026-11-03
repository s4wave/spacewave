import { useCallback, useMemo, useRef, useState } from 'react'
import { LuArrowLeft, LuChevronDown } from '@s4wave/web/images/icons.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { LegalFooter } from './LegalFooter.js'

import { useRootResource } from '@s4wave/web/hooks/useRootResource.js'
import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { ReleaseCard } from './ReleaseCard.js'
import { useLandingBackNavigation } from './useLandingBackNavigation.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

import {
  colors as themeColors,
  fonts as themeFonts,
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
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule6: {
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
  rule7: {
    marginInline: 'auto',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    position: 'relative',
    zIndex: 20,
    maxWidth: 'var(--container-4xl)',
    paddingBottom: `calc(${themeLayout['--spacing']} * 6)`,
    width: '100%',
  },
  rule8: { display: 'inline-block', position: 'relative' },
  rule9: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
    color: themeColors['--color-foreground'],
    cursor: 'pointer',
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule10: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 25%, transparent)`,
  },
  rule11: {
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    '--tw-shadow':
      '0 10px 15px -3px var(--tw-shadow-color, #0000001a), 0 4px 6px -4px var(--tw-shadow-color, #0000001a)',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 15%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backgroundColor: themeColors['--color-background-card'],
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    position: 'absolute',
    left: 0,
    marginTop: themeLayout['--spacing'],
    maxHeight: `calc(${themeLayout['--spacing']} * 60)`,
    overflowY: 'auto',
    width: `calc(${themeLayout['--spacing']} * 48)`,
  },
  rule12: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    cursor: 'pointer',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textAlign: 'left',
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    width: '100%',
  },
  rule13: {
    marginInline: 'auto',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    position: 'relative',
    zIndex: 10,
    maxWidth: 'var(--container-4xl)',
    paddingBottom: {
      default: `calc(${themeLayout['--spacing']} * 14)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 16)`,
    },
    width: '100%',
  },
  rule14: {
    gap: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'flex',
    flexDirection: 'column',
  },
  rule15: {
    marginInline: 'auto',
    marginTop: `calc(${themeLayout['--spacing']} * 8)`,
    maxWidth: 'var(--container-sm)',
    width: '100%',
  },
  rule16: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textAlign: 'center',
    marginTop: `calc(${themeLayout['--spacing']} * 8)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: { rotate: '180deg' },
})

export const metadata = {
  title: 'Changelog - Spacewave',
  description: 'See what is new in Spacewave.',
}

// Changelog renders the changelog landing page.
export function Changelog() {
  const goBack = useLandingBackNavigation()

  const rootResource = useRootResource()
  const changelogResource = useResource(
    rootResource,
    async (root, signal) => root.getChangelog(signal),
    [],
  )
  const releases = useMemo(
    () => changelogResource.value?.releases ?? [],
    [changelogResource.value],
  )

  const [dropdownOpen, setDropdownOpen] = useState(false)
  const scrollRef = useRef<HTMLDivElement>(null)

  const toggleDropdown = useCallback(() => {
    setDropdownOpen((prev) => !prev)
  }, [])

  const scrollToVersion = useCallback((version: string) => {
    setDropdownOpen(false)
    // Version ids contain dots (e.g. v0.53.1), so escape them before
    // building the id selector; a raw `#v0.53.1` is an invalid selector.
    const el = scrollRef.current?.querySelector(`#${CSS.escape(`v${version}`)}`)
    if (el) {
      el.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }
  }, [])

  const latestVersion = releases.length > 0 ? releases[0].version : null

  return (
    <div ref={scrollRef} sx={stylexStyles2.rule1}>
      {/* Back button */}
      <div sx={stylexStyles2.rule2}>
        <button type="button" onClick={goBack} sx={stylexStyles2.rule3}>
          <LuArrowLeft sx={stylexStyles.rule1} />
          Back
        </button>
      </div>

      {/* Hero */}
      <header sx={stylexStyles2.rule4}>
        <h1 sx={stylexStyles2.rule5}>Changelog</h1>
        <p sx={stylexStyles2.rule6}>See what is new in Spacewave.</p>
      </header>

      {/* Version dropdown */}
      {releases.length > 0 && (
        <div sx={stylexStyles2.rule7}>
          <div sx={stylexStyles2.rule8}>
            <button
              type="button"
              onClick={toggleDropdown}
              sx={[stylexStyles2.rule9, !!dropdownOpen && stylexStyles2.rule10]}
            >
              {latestVersion ? `v${latestVersion}` : 'Versions'}
              <LuChevronDown
                sx={[stylexStyles.rule2, !!dropdownOpen && stylexStyles.rule3]}
              />
            </button>
            {dropdownOpen && (
              <div sx={stylexStyles2.rule11}>
                {releases.map((release) => (
                  <button
                    type="button"
                    key={release.version}
                    onClick={() => scrollToVersion(release.version ?? '')}
                    sx={stylexStyles2.rule12}
                  >
                    v{release.version}
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>
      )}

      {/* Release cards */}
      <section sx={stylexStyles2.rule13}>
        <div sx={stylexStyles2.rule14}>
          {releases.map((release) => (
            <ReleaseCard key={release.version} release={release} linkToDetail />
          ))}
        </div>
        {changelogResource.loading && (
          <div sx={stylexStyles2.rule15}>
            <LoadingCard
              view={{
                state: 'active',
                title: 'Loading changelog',
                detail: 'Fetching the latest Spacewave releases.',
              }}
            />
          </div>
        )}
        {!changelogResource.loading && releases.length === 0 && (
          <p sx={stylexStyles2.rule16}>No releases yet.</p>
        )}
      </section>

      <LegalFooter />
    </div>
  )
}
