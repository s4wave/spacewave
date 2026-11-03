import type { IconType } from '@s4wave/web/images/icons.js'
import { useCallback } from 'react'
import {
  LuArrowRight,
  LuUser,
  LuServer,
  LuCode,
} from '@s4wave/web/images/icons.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { siteDefs } from './sections.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'
import { groupMarker } from '../../web/style/relations.stylex.js'

import {
  utilityState as themeUtilityState,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker2 } from '../../web/style/relations.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
} from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker3 } from '../../web/style/relations.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: { marginBottom: `calc(${themeLayout['--spacing']} * 10)` },
  rule2: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    fontSize: {
      default: 'var(--text-2xl)',
      '@container (min-width: 32rem)': 'var(--text-3xl)',
    },
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: {
      default: 'var(--tw-leading, var(--text-2xl--line-height))',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-3xl--line-height))',
    },
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
    fontSize: {
      default: themeFonts['--text-sm'],
      '@container (min-width: 32rem)': 'var(--text-base)',
    },
    lineHeight: {
      default: 'var(--leading-relaxed)',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-base--line-height))',
    },
  },
  rule4: {
    gap: `calc(${themeLayout['--spacing']} * 5)`,
    display: 'grid',
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(3, minmax(0, 1fr))',
    },
  },
  rule5: {
    borderRadius: themeRadii['--radius-lg'],
    alignItems: 'center',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
    display: 'flex',
    justifyContent: 'center',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
  rule6: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: 'var(--text-lg)',
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
  },
  rule7: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
  },
  rule8: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker3)]:
          themeColors['--color-brand-highlight'],
      },
    },
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    marginTop: 'auto',
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    '--tw-duration': '.2s',
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'flex-start',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      },
    },
    cursor: 'pointer',
    display: 'flex',
    flexDirection: 'column',
    textAlign: 'left',
    transitionDuration: '.2s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 5)`,
    width: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    '--tw-duration': '.2s',
    '--tw-translate-x': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          `calc(${themeLayout['--spacing']} * .5)`,
      },
    },
    transitionDuration: '.2s',
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

const siteIcons: Record<string, IconType> = {
  users: LuUser,
  'self-hosters': LuServer,
  developers: LuCode,
}

// DocsHub renders the documentation hub with audience surface cards.
export function DocsHub() {
  const navigate = useNavigate()

  const goToSite = useCallback(
    (siteId: string) => {
      navigate({ path: `/docs/${siteId}` })
    },
    [navigate],
  )

  return (
    <div>
      <header sx={stylexStyles4.rule1}>
        <h1 sx={stylexStyles4.rule2}>Documentation</h1>
        <p sx={stylexStyles4.rule3}>
          Guides and references for building with Spacewave.
        </p>
      </header>

      <div sx={stylexStyles4.rule4}>
        {siteDefs.map((site) => {
          const Icon = siteIcons[site.id] ?? LuUser
          return (
            <button
              type="button"
              key={site.id}
              onClick={() => goToSite(site.id)}
              sx={[stylexStyles3.rule1, groupMarker2]}
            >
              <div sx={stylexStyles4.rule5}>
                <Icon sx={stylexStyles2.rule1} />
              </div>
              <h2 sx={stylexStyles4.rule6}>{site.label}</h2>
              <p sx={stylexStyles4.rule7}>{site.description}</p>
              <span sx={stylexStyles4.rule8}>
                Browse
                <LuArrowRight sx={stylexStyles.rule1} />
              </span>
            </button>
          )
        })}
      </div>
    </div>
  )
}
