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

const stylexStyles3 = stylex.create({
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

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 5)`,
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
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
      <header className="mb-10">
        <h1 className="text-foreground mb-3 text-2xl font-semibold tracking-tight @lg:text-3xl">
          Documentation
        </h1>
        <p className="text-foreground-alt text-sm leading-relaxed @lg:text-base">
          Guides and references for building with Spacewave.
        </p>
      </header>

      <div className="grid gap-5 @lg:grid-cols-3">
        {siteDefs.map((site) => {
          const Icon = siteIcons[site.id] ?? LuUser
          return (
            <button
              type="button"
              key={site.id}
              onClick={() => goToSite(site.id)}
              sx={[stylexStyles3.rule1, groupMarker2]}
            >
              <div className="bg-brand/10 text-brand flex size-10 items-center justify-center rounded-lg">
                <Icon sx={stylexStyles2.rule1} />
              </div>
              <h2 className="text-foreground text-lg font-semibold">
                {site.label}
              </h2>
              <p className="text-foreground-alt/70 text-sm leading-relaxed">
                {site.description}
              </p>
              <span className="text-brand group-hover:text-brand-highlight mt-auto flex items-center gap-1.5 text-xs font-medium transition-colors">
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
