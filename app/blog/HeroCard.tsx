import { useCallback } from 'react'
import { useNavigate } from '@s4wave/web/router/router.js'
import { TagChip } from './TagChip.js'
import { LuArrowRight } from '@s4wave/web/images/icons.js'
import { safeHref } from './safe-href.js'
import type { BlogPost } from './types.js'

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

import { fonts as themeFonts } from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker3 } from '../../web/style/relations.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    '--tw-duration': '.5s',
    '--tw-gradient-from': `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    '--tw-gradient-position': 'to bottom right in oklab',
    '--tw-gradient-stops':
      'var(--tw-gradient-via-stops, var(--tw-gradient-position), var(--tw-gradient-from) var(--tw-gradient-from-position), var(--tw-gradient-to) var(--tw-gradient-to-position))',
    '--tw-gradient-to': 'transparent',
    '--tw-gradient-via': 'transparent',
    '--tw-gradient-via-stops':
      'var(--tw-gradient-position), var(--tw-gradient-from) var(--tw-gradient-from-position), var(--tw-gradient-via) var(--tw-gradient-via-position), var(--tw-gradient-to) var(--tw-gradient-to-position)',
    inset: 0,
    backgroundImage: 'linear-gradient(var(--tw-gradient-stops))',
    opacity: {
      default: 0,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker3)]: 1,
      },
    },
    pointerEvents: 'none',
    position: 'absolute',
    transitionDuration: '.5s',
    transitionProperty: 'opacity',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule2: {
    padding: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 10)`,
    },
    gap: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 10)`,
    },
    alignItems: {
      default: null,
      '@container (min-width: 32rem)': 'flex-start',
    },
    display: 'flex',
    flexDirection: {
      default: 'column',
      '@container (min-width: 32rem)': 'row',
    },
    position: 'relative',
  },
  rule3: { flex: 1 },
  rule4: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexWrap: 'wrap',
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: {
    '--tw-duration': '.3s',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    color: {
      default: themeColors['--color-foreground'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker3)]:
          themeColors['--color-brand'],
      },
    },
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
    transitionDuration: '.3s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule6: {
    cursor: 'pointer',
    textAlign: 'left',
    '::after': { inset: 0, content: 'var(--tw-content)', position: 'absolute' },
  },
  rule7: {
    '--tw-leading': {
      default: 'var(--leading-relaxed)',
      '@container (min-width: 32rem)': 'var(--leading-relaxed)',
    },
    color: themeColors['--color-foreground-alt'],
    fontSize: {
      default: themeFonts['--text-sm'],
      '@container (min-width: 32rem)': 'var(--text-base)',
    },
    lineHeight: {
      default: 'var(--leading-relaxed)',
      '@container (min-width: 32rem)': 'var(--leading-relaxed)',
    },
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule8: {
    '--tw-duration': '.3s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: themeColors['--color-brand'],
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    opacity: {
      default: 0,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker3)]: 1,
      },
    },
    transitionDuration: '.3s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule9: {
    borderColor: {
      default: null,
      '@container (min-width: 32rem)': `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    },
    gap: {
      default: `calc(${themeLayout['--spacing']} * 3)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 3)`,
    },
    alignItems: {
      default: 'center',
      '@container (min-width: 32rem)': 'flex-end',
    },
    display: 'flex',
    flexDirection: { default: null, '@container (min-width: 32rem)': 'column' },
    flexShrink: 0,
    borderLeftStyle: {
      default: null,
      '@container (min-width: 32rem)': themeUtilityState['--tw-border-style'],
    },
    borderLeftWidth: { default: null, '@container (min-width: 32rem)': '1px' },
    paddingLeft: {
      default: null,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 10)`,
    },
  },
  rule10: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    height: {
      default: `calc(${themeLayout['--spacing']} * 10)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 12)`,
    },
    width: {
      default: `calc(${themeLayout['--spacing']} * 10)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 12)`,
    },
  },
  rule11: {
    textAlign: { default: null, '@container (min-width: 32rem)': 'right' },
  },
  rule12: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    position: 'relative',
    textDecorationLine: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 'underline' },
    },
  },
  rule13: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-duration': '.3s',
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-white']} 12%, transparent)`,
      },
    },
    borderRadius: 'var(--radius-2xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 20%, transparent)`,
    cursor: 'pointer',
    position: 'relative',
    transitionDuration: '.3s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': 'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    '--tw-duration': '.3s',
    '--tw-translate-x': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          themeLayout['--spacing'],
      },
    },
    transitionDuration: '.3s',
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
    height: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

// HeroCardProps defines the props for HeroCard.
interface HeroCardProps {
  post: BlogPost
}

// HeroCard renders the featured hero card for the latest blog post.
export function HeroCard({ post }: HeroCardProps) {
  const navigate = useNavigate()

  const handleHeroCardSelect = useCallback(() => {
    navigate({ path: post.url })
  }, [navigate, post.url])

  return (
    <article sx={[stylexStyles2.rule1, groupMarker2]}>
      {/* Subtle gradient glow on hover */}
      <div sx={stylexStyles3.rule1} />

      <div sx={stylexStyles3.rule2}>
        {/* Content */}
        <div sx={stylexStyles3.rule3}>
          <div sx={stylexStyles3.rule4}>
            {post.tags.map((tag) => (
              <TagChip key={tag} tag={tag} />
            ))}
          </div>

          <h2 sx={stylexStyles3.rule5}>
            <button
              type="button"
              onClick={handleHeroCardSelect}
              sx={stylexStyles3.rule6}
            >
              {post.title}
            </button>
          </h2>

          <p sx={stylexStyles3.rule7}>{post.summary}</p>

          <div sx={stylexStyles3.rule8}>
            Read post
            <LuArrowRight sx={stylexStyles.rule1} />
          </div>
        </div>

        {/* Author + date sidebar */}
        <div sx={stylexStyles3.rule9}>
          <img
            src={post.author.avatar}
            alt={post.author.name}
            sx={stylexStyles3.rule10}
            loading="lazy"
          />
          <div sx={stylexStyles3.rule11}>
            <a
              href={safeHref(post.author.url)}
              target="_blank"
              rel="noopener noreferrer"
              sx={stylexStyles3.rule12}
            >
              {post.author.name}
            </a>
            <time sx={stylexStyles3.rule13}>{post.date}</time>
          </div>
        </div>
      </div>
    </article>
  )
}
