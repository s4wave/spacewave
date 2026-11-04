import { controlTargetStyles } from '@s4wave/web/style/ControlStylesProvider.js'
import { useCallback, useMemo } from 'react'

import { LuArrowRight } from '@s4wave/web/images/icons.js'

import { TagChip } from './TagChip.js'
import type { BlogPostData } from './types.js'
import { resolveAuthor, type AuthorRegistry } from './authors.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  motion as themeMotion,
} from '@s4wave/web/style/tokens.stylex.js'
import { groupMarker } from '../../../web/style/relations.stylex.js'

import {
  utilityState as themeUtilityState,
  colors as themeColors,
} from '@s4wave/web/style/tokens.stylex.js'
import { groupMarker as groupMarker2 } from '../../../web/style/relations.stylex.js'

import { fonts as themeFonts } from '@s4wave/web/style/tokens.stylex.js'
import { groupMarker as groupMarker3 } from '../../../web/style/relations.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
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
  rule2: { flex: 1 },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    flexWrap: 'wrap',
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule4: {
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
  rule5: {
    cursor: 'pointer',
    textAlign: 'left',
    '::after': { inset: 0, content: 'var(--tw-content)', position: 'absolute' },
  },
  rule6: {
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
  rule7: {
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
  rule8: {
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
  rule9: {
    borderRadius: '3.40282e38px',
    objectFit: 'cover',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule10: {
    textAlign: { default: null, '@container (min-width: 32rem)': 'right' },
  },
  rule11: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule12: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    '--tw-duration': '.3s',
    '--tw-translate-y': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
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
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
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
  post: BlogPostData
  onSelectPost: (post: BlogPostData) => void
  onSelectTag?: (tag: string) => void
  authorRegistry?: AuthorRegistry
}

// HeroCard renders the featured hero card for the latest blog post.
export function HeroCard({
  post,
  onSelectPost,
  onSelectTag,
  authorRegistry,
}: HeroCardProps) {
  const handleHeroCardSelect = useCallback(() => {
    onSelectPost(post)
  }, [onSelectPost, post])

  const author = useMemo(
    () => resolveAuthor(authorRegistry ?? {}, post.author),
    [authorRegistry, post.author],
  )

  return (
    <article sx={[stylexStyles2.rule1, groupMarker2]}>
      <div sx={stylexStyles3.rule1}>
        <div sx={stylexStyles3.rule2}>
          <div sx={stylexStyles3.rule3}>
            {post.tags.map((tag) => (
              <TagChip key={tag} tag={tag} onSelectTag={onSelectTag} />
            ))}
          </div>

          <h2 sx={stylexStyles3.rule4}>
            <button
              type="button"
              onClick={handleHeroCardSelect}
              sx={[stylexStyles3.rule5, controlTargetStyles.minimum]}
            >
              {post.title}
            </button>
          </h2>

          {post.summary && <p sx={stylexStyles3.rule6}>{post.summary}</p>}

          <div sx={stylexStyles3.rule7}>
            Read post
            <LuArrowRight sx={stylexStyles.rule1} />
          </div>
        </div>

        {/* Author and date */}
        <div sx={stylexStyles3.rule8}>
          {author?.avatar && (
            <img
              src={author.avatar}
              alt={author.name}
              sx={stylexStyles3.rule9}
            />
          )}
          <div sx={stylexStyles3.rule10}>
            {author && <span sx={stylexStyles3.rule11}>{author.name}</span>}
            <time sx={stylexStyles3.rule12}>{post.date}</time>
          </div>
        </div>
      </div>
    </article>
  )
}
