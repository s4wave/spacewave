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
    pointerEvents: 'none',
    position: 'absolute',
    inset: 0,
    '--tw-gradient-position': 'to bottom right in oklab',
    backgroundImage: 'linear-gradient(var(--tw-gradient-stops))',
    '--tw-gradient-from': `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    '--tw-gradient-stops':
      'var(--tw-gradient-via-stops, var(--tw-gradient-position), var(--tw-gradient-from) var(--tw-gradient-from-position), var(--tw-gradient-to) var(--tw-gradient-to-position))',
    '--tw-gradient-via': 'transparent',
    '--tw-gradient-via-stops':
      'var(--tw-gradient-position), var(--tw-gradient-from) var(--tw-gradient-from-position), var(--tw-gradient-via) var(--tw-gradient-via-position), var(--tw-gradient-to) var(--tw-gradient-to-position)',
    '--tw-gradient-to': 'transparent',
    opacity: {
      default: 0,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker3)]: 1,
      },
    },
    transitionProperty: 'opacity',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.5s',
    '--tw-duration': '.5s',
  },
  rule2: {
    position: 'relative',
    display: 'flex',
    flexDirection: {
      default: 'column',
      '@container (min-width: 32rem)': 'row',
    },
    gap: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 10)`,
    },
    padding: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 10)`,
    },
    alignItems: {
      default: null,
      '@container (min-width: 32rem)': 'flex-start',
    },
  },
  rule3: { flex: 1 },
  rule4: {
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'flex',
    flexWrap: 'wrap',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule5: {
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
    color: {
      default: themeColors['--color-foreground'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker3)]:
          themeColors['--color-brand'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
  },
  rule6: {
    cursor: 'pointer',
    textAlign: 'left',
    '::after': { content: 'var(--tw-content)', position: 'absolute', inset: 0 },
  },
  rule7: {
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
    fontSize: {
      default: themeFonts['--text-sm'],
      '@container (min-width: 32rem)': 'var(--text-base)',
    },
    lineHeight: {
      default: 'var(--leading-relaxed)',
      '@container (min-width: 32rem)': 'var(--leading-relaxed)',
    },
    '--tw-leading': {
      default: 'var(--leading-relaxed)',
      '@container (min-width: 32rem)': 'var(--leading-relaxed)',
    },
    color: themeColors['--color-foreground-alt'],
  },
  rule8: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-brand'],
    opacity: {
      default: 0,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker3)]: 1,
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
  },
  rule9: {
    display: 'flex',
    flexShrink: 0,
    alignItems: {
      default: 'center',
      '@container (min-width: 32rem)': 'flex-end',
    },
    gap: {
      default: `calc(${themeLayout['--spacing']} * 3)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 3)`,
    },
    flexDirection: { default: null, '@container (min-width: 32rem)': 'column' },
    borderLeftStyle: {
      default: null,
      '@container (min-width: 32rem)': themeUtilityState['--tw-border-style'],
    },
    borderLeftWidth: { default: null, '@container (min-width: 32rem)': '1px' },
    borderColor: {
      default: null,
      '@container (min-width: 32rem)': `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    },
    paddingLeft: {
      default: null,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 10)`,
    },
  },
  rule10: {
    width: {
      default: `calc(${themeLayout['--spacing']} * 10)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 12)`,
    },
    height: {
      default: `calc(${themeLayout['--spacing']} * 10)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 12)`,
    },
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
  },
  rule11: {
    textAlign: { default: null, '@container (min-width: 32rem)': 'right' },
  },
  rule12: {
    position: 'relative',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    textDecorationLine: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 'underline' },
    },
  },
  rule13: {
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    position: 'relative',
    cursor: 'pointer',
    overflow: 'hidden',
    borderRadius: 'var(--radius-2xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-white']} 12%, transparent)`,
      },
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 20%, transparent)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
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
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    transitionProperty: 'transform, translate, scale, rotate',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.3s',
    '--tw-duration': '.3s',
    '--tw-translate-x': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          themeLayout['--spacing'],
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
