import { useCallback } from 'react'

import { LuArrowRight } from '@s4wave/web/images/icons.js'

import { TagChip } from './TagChip.js'
import type { BlogPostData } from './types.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'
import { groupMarker } from '../../../web/style/relations.stylex.js'

import { utilityState as themeUtilityState } from '../../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker2 } from '../../../web/style/relations.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    position: 'relative',
    display: 'flex',
    cursor: 'pointer',
    alignItems: {
      default: 'flex-start',
      '@container (min-width: 32rem)': 'center',
    },
    gap: `calc(${themeLayout['--spacing']} * 5)`,
    borderBottomStyle: {
      default: themeUtilityState['--tw-border-style'],
      ':last-child': themeUtilityState['--tw-border-style'],
    },
    borderBottomWidth: { default: '1px', ':last-child': 0 },
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 5)`,
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
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          themeColors['--color-brand'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
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

// PostListItemProps defines the props for PostListItem.
interface PostListItemProps {
  post: BlogPostData
  onSelectPost: (post: BlogPostData) => void
}

// PostListItem renders a compact post row with title, date, and tags.
function PostListItem({ post, onSelectPost }: PostListItemProps) {
  const handlePostSelect = useCallback(() => {
    onSelectPost(post)
  }, [onSelectPost, post])

  return (
    <article sx={[stylexStyles2.rule1, groupMarker2]}>
      <div className="min-w-0 flex-1">
        <h3 className="text-foreground group-hover:text-brand mb-1.5 text-sm font-semibold transition-colors duration-200 @lg:text-base">
          <button
            type="button"
            onClick={handlePostSelect}
            className="cursor-pointer text-left after:absolute after:inset-0"
          >
            {post.title}
          </button>
        </h3>
        {post.summary && (
          <p className="text-foreground-alt/60 line-clamp-1 hidden text-xs @md:block">
            {post.summary}
          </p>
        )}
      </div>
      <div className="hidden shrink-0 items-center gap-1.5 @md:flex">
        {post.tags.map((tag) => (
          <TagChip key={tag} tag={tag} />
        ))}
      </div>
      <div className="flex shrink-0 items-center gap-3">
        <time className="text-foreground-alt/50 text-xs tabular-nums">
          {post.date}
        </time>
        <LuArrowRight sx={stylexStyles.rule1} />
      </div>
    </article>
  )
}

// PostListProps defines the props for PostList.
interface PostListProps {
  posts: BlogPostData[]
  onSelectPost: (post: BlogPostData) => void
}

// PostList renders a compact list of blog posts.
export function PostList({ posts, onSelectPost }: PostListProps) {
  return (
    <div className="border-foreground/6 bg-background-card/10 overflow-hidden rounded-xl border backdrop-blur-sm">
      {posts.map((post) => (
        <PostListItem key={post.name} post={post} onSelectPost={onSelectPost} />
      ))}
    </div>
  )
}
