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
      <header className="mb-10">
        <h1 className="text-foreground mb-3 text-2xl font-semibold tracking-tight @lg:text-3xl">
          Documentation
        </h1>
        <p className="text-foreground-alt text-sm leading-relaxed @lg:text-base">
          Learn how to build and manage your spaces with Spacewave.
        </p>
      </header>

      {/* Section cards */}
      <div className="grid gap-5 @lg:grid-cols-2 @5xl:grid-cols-3">
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
              <div className="bg-brand/10 text-brand flex size-10 items-center justify-center rounded-lg">
                <LuBookOpen sx={stylexStyles.rule1} />
              </div>
              <h2 className="text-foreground text-lg font-semibold">
                {section.label}
              </h2>
              <p className="text-foreground-alt/70 text-sm leading-relaxed">
                {section.pages.length}{' '}
                {section.pages.length === 1 ? 'page' : 'pages'}
              </p>
              {firstPage && (
                <span className="text-brand group-hover:text-brand-highlight mt-auto flex items-center gap-1.5 text-xs font-medium transition-colors">
                  Get started
                  <LuArrowRight sx={stylexStyles.rule2} />
                </span>
              )}
            </button>
          )
        })}
      </div>

      {sections.every((s) => s.pages.length === 0) && (
        <div className="border-foreground/6 rounded-xl border border-dashed px-8 py-20 text-center">
          <p className="text-foreground-alt text-sm">
            No documentation pages yet. Check back soon.
          </p>
        </div>
      )}
    </div>
  )
}
