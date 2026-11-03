import { useCallback } from 'react'
import { useNavigate } from '@s4wave/web/router/router.js'
import { TagChip } from './TagChip.js'
import { LuArrowRight } from '@s4wave/web/images/icons.js'
import type { BlogPost } from './types.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'
import { groupMarker } from '../../web/style/relations.stylex.js'

import { utilityState as themeUtilityState } from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker2 } from '../../web/style/relations.stylex.js'

import { fonts as themeFonts } from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker3 } from '../../web/style/relations.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: { flex: 1, minWidth: 0 },
  rule2: {
    '--tw-duration': '.2s',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: {
      default: themeColors['--color-foreground'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker3)]:
          themeColors['--color-brand'],
      },
    },
    fontSize: {
      default: themeFonts['--text-sm'],
      '@container (min-width: 32rem)': 'var(--text-base)',
    },
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: {
      default: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-base--line-height))',
    },
    transitionDuration: '.2s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    marginBottom: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule3: {
    cursor: 'pointer',
    textAlign: 'left',
    '::after': { inset: 0, content: 'var(--tw-content)', position: 'absolute' },
  },
  rule4: {
    overflow: 'hidden',
    WebkitBoxOrient: 'vertical',
    WebkitLineClamp: 1,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    display: { default: 'none', '@container (min-width: 28rem)': 'block' },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule5: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    display: { default: 'none', '@container (min-width: 28rem)': 'flex' },
    flexShrink: 0,
  },
  rule6: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
  },
  rule7: {
    '--tw-numeric-spacing': 'tabular-nums',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule8: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 10%, transparent)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-duration': '.2s',
    padding: `calc(${themeLayout['--spacing']} * 5)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 5)`,
    alignItems: {
      default: 'flex-start',
      '@container (min-width: 32rem)': 'center',
    },
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      },
    },
    cursor: 'pointer',
    display: 'flex',
    position: 'relative',
    transitionDuration: '.2s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    borderBottomStyle: {
      default: themeUtilityState['--tw-border-style'],
      ':last-child': themeUtilityState['--tw-border-style'],
    },
    borderBottomWidth: { default: '1px', ':last-child': 0 },
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
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          themeColors['--color-brand'],
      },
    },
    transitionDuration: '.2s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    translate: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          'var(--tw-translate-x) var(--tw-translate-y)',
      },
    },
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

// PostListItemProps defines the props for PostListItem.
interface PostListItemProps {
  post: BlogPost
}

// PostListItem renders a compact post row with title, date, and tags.
function PostListItem({ post }: PostListItemProps) {
  const navigate = useNavigate()

  const handlePostSelect = useCallback(() => {
    navigate({ path: post.url })
  }, [navigate, post.url])

  return (
    <article sx={[stylexStyles2.rule1, groupMarker2]}>
      <div sx={stylexStyles3.rule1}>
        <h3 sx={stylexStyles3.rule2}>
          <button
            type="button"
            onClick={handlePostSelect}
            sx={stylexStyles3.rule3}
          >
            {post.title}
          </button>
        </h3>
        <p sx={stylexStyles3.rule4}>{post.summary}</p>
      </div>
      <div sx={stylexStyles3.rule5}>
        {post.tags.map((tag) => (
          <TagChip key={tag} tag={tag} />
        ))}
      </div>
      <div sx={stylexStyles3.rule6}>
        <time sx={stylexStyles3.rule7}>{post.date}</time>
        <LuArrowRight sx={stylexStyles.rule1} />
      </div>
    </article>
  )
}

// PostListProps defines the props for PostList.
interface PostListProps {
  posts: BlogPost[]
}

// PostList renders a compact list of blog posts.
export function PostList({ posts }: PostListProps) {
  return (
    <div sx={stylexStyles3.rule8}>
      {posts.map((post) => (
        <PostListItem key={post.slug} post={post} />
      ))}
    </div>
  )
}
