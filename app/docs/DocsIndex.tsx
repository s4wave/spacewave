import { useCallback } from 'react'
import { LuArrowRight, LuBookOpen } from '@s4wave/web/images/icons.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import type { DocSection } from './types.js'

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

const stylexStyles3 = stylex.create({
  rule1: { marginBottom: `calc(${themeLayout['--spacing']} * 10)` },
  rule2: {
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
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
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  rule3: {
    fontSize: {
      default: themeFonts['--text-sm'],
      '@container (min-width: 32rem)': 'var(--text-base)',
    },
    lineHeight: {
      default: 'var(--leading-relaxed)',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-base--line-height))',
    },
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule4: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 5)`,
    gridTemplateColumns: {
      default: null,
      '@container (min-width: 32rem)': 'repeat(2, minmax(0, 1fr))',
      '@container (min-width: 64rem)': 'repeat(3, minmax(0, 1fr))',
    },
  },
  rule5: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-lg'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule6: {
    fontSize: 'var(--text-lg)',
    lineHeight: 'var(--tw-leading, var(--text-lg--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule7: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule8: {
    marginTop: 'auto',
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker3)]:
          themeColors['--color-brand-highlight'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule9: {
    borderRadius: 'var(--radius-xl)',
    borderStyle: 'dashed',
    borderWidth: '1px',
    '--tw-border-style': 'dashed',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 8)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 20)`,
    textAlign: 'center',
  },
  rule10: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    cursor: 'pointer',
    flexDirection: 'column',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.2s',
    '--tw-duration': '.2s',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      },
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.2s',
    '--tw-duration': '.2s',
    '--tw-translate-x': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          `calc(${themeLayout['--spacing']} * .5)`,
      },
    },
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
  },
})

// DocsIndexProps defines the props for DocsIndex.
export interface DocsIndexProps {
  sections: DocSection[]
}

// DocsIndex renders the documentation landing page with section cards.
export function DocsIndex({ sections }: DocsIndexProps) {
  const navigate = useNavigate()

  const goToSection = useCallback(
    (url: string) => {
      navigate({ path: url })
    },
    [navigate],
  )

  return (
    <div>
      {/* Hero header */}
      <header sx={stylexStyles3.rule1}>
        <h1 sx={stylexStyles3.rule2}>Documentation</h1>
        <p sx={stylexStyles3.rule3}>
          Learn how to build and manage your spaces with Spacewave.
        </p>
      </header>

      {/* Section cards */}
      <div sx={stylexStyles3.rule4}>
        {sections.map((section) => {
          const firstPage = section.pages[0]
          return (
            <button
              type="button"
              key={section.id}
              onClick={() => firstPage && goToSection(firstPage.url)}
              disabled={!firstPage}
              sx={[stylexStyles2.rule1, groupMarker2]}
            >
              <div sx={stylexStyles3.rule5}>
                <LuBookOpen sx={stylexStyles.rule1} />
              </div>
              <h2 sx={stylexStyles3.rule6}>{section.label}</h2>
              <p sx={stylexStyles3.rule7}>
                {section.pages.length}{' '}
                {section.pages.length === 1 ? 'page' : 'pages'}
              </p>
              {firstPage && (
                <span sx={stylexStyles3.rule8}>
                  Get started
                  <LuArrowRight sx={stylexStyles.rule2} />
                </span>
              )}
            </button>
          )
        })}
      </div>

      {sections.every((s) => s.pages.length === 0) && (
        <div sx={stylexStyles3.rule9}>
          <p sx={stylexStyles3.rule10}>
            No documentation pages yet. Check back soon.
          </p>
        </div>
      )}
    </div>
  )
}
