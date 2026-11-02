import { useCallback, useMemo } from 'react'

import { LuArrowRight } from '@s4wave/web/images/icons.js'

import { TagChip } from './TagChip.js'
import type { BlogPostData } from './types.js'
import { resolveAuthor, type AuthorRegistry } from './authors.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  motion as themeMotion,
} from '../../../web/style/tokens.stylex.js'
import { groupMarker } from '../../../web/style/relations.stylex.js'

import {
  utilityState as themeUtilityState,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker2 } from '../../../web/style/relations.stylex.js'

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
    '--tw-backdrop-blur': 'blur(var(--blur-sm))',
    WebkitBackdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
    backdropFilter:
      'var(--tw-backdrop-blur, ) var(--tw-backdrop-brightness, ) var(--tw-backdrop-contrast, ) var(--tw-backdrop-grayscale, ) var(--tw-backdrop-hue-rotate, ) var(--tw-backdrop-invert, ) var(--tw-backdrop-opacity, ) var(--tw-backdrop-saturate, ) var(--tw-backdrop-sepia, )',
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
      <div className="relative flex flex-col gap-6 p-6 @lg:flex-row @lg:items-start @lg:gap-10 @lg:p-10">
        <div className="flex-1">
          <div className="mb-4 flex flex-wrap gap-2">
            {post.tags.map((tag) => (
              <TagChip key={tag} tag={tag} onSelectTag={onSelectTag} />
            ))}
          </div>

          <h2 className="text-foreground group-hover:text-brand mb-3 text-2xl font-semibold tracking-tight transition-colors duration-300 @lg:text-3xl">
            <button
              type="button"
              onClick={handleHeroCardSelect}
              className="cursor-pointer text-left after:absolute after:inset-0"
            >
              {post.title}
            </button>
          </h2>

          {post.summary && (
            <p className="text-foreground-alt mb-6 text-sm leading-relaxed @lg:text-base @lg:leading-relaxed">
              {post.summary}
            </p>
          )}

          <div className="text-brand flex items-center gap-2 text-sm font-medium opacity-0 transition duration-300 group-hover:opacity-100">
            Read post
            <LuArrowRight sx={stylexStyles.rule1} />
          </div>
        </div>

        {/* Author and date */}
        <div className="@lg:border-foreground/6 flex shrink-0 items-center gap-3 @lg:flex-col @lg:items-end @lg:gap-3 @lg:border-l @lg:pl-10">
          {author?.avatar && (
            <img
              src={author.avatar}
              alt={author.name}
              className="size-8 rounded-full object-cover"
            />
          )}
          <div className="@lg:text-right">
            {author && (
              <span className="text-foreground text-sm font-medium">
                {author.name}
              </span>
            )}
            <time className="text-foreground-alt/60 block text-xs">
              {post.date}
            </time>
          </div>
        </div>
      </div>
    </article>
  )
}
