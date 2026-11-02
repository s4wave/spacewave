import { useCallback, useMemo } from 'react'

import Markdown from 'markdown-to-jsx'
import { LuArrowLeft, LuArrowRight } from '@s4wave/web/images/icons.js'

import { markdownCodeOptions } from '@s4wave/code/markdown.js'

import { TagChip } from './TagChip.js'
import type { BlogPostData } from './types.js'
import { resolveAuthor, type AuthorRegistry } from './authors.js'
import './blog-prose.css'

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
    display: 'flex',
    cursor: 'pointer',
    flexDirection: 'column',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
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
    padding: `calc(${themeLayout['--spacing']} * 5)`,
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
  rule2: {
    display: 'flex',
    cursor: 'pointer',
    flexDirection: 'column',
    alignItems: 'flex-end',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
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
    padding: `calc(${themeLayout['--spacing']} * 5)`,
    textAlign: 'right',
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
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
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
          `calc(${themeLayout['--spacing']} * -.5)`,
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
  rule3: {
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

// BlogPostViewProps defines the props for BlogPostView.
interface BlogPostViewProps {
  post: BlogPostData
  prevPost?: BlogPostData
  nextPost?: BlogPostData
  onSelectPost: (post: BlogPostData) => void
  onSelectTag?: (tag: string) => void
  onBack: () => void
  authorRegistry?: AuthorRegistry
}

// BlogPostView renders a single blog post with prose styling.
export function BlogPostView({
  post,
  prevPost,
  nextPost,
  onSelectPost,
  onSelectTag,
  onBack,
  authorRegistry,
}: BlogPostViewProps) {
  const navigatePrev = useCallback(() => {
    if (prevPost) onSelectPost(prevPost)
  }, [onSelectPost, prevPost])

  const navigateNext = useCallback(() => {
    if (nextPost) onSelectPost(nextPost)
  }, [onSelectPost, nextPost])

  return (
    <article className="mx-auto w-full max-w-3xl px-4 pt-6 pb-20 @lg:px-8 @lg:pt-10">
      {/* Back button */}
      <button
        type="button"
        onClick={onBack}
        className="text-foreground-alt/60 hover:text-foreground mb-6 flex items-center gap-1.5 text-xs transition-colors"
      >
        <LuArrowLeft sx={stylexStyles.rule1} />
        Back to posts
      </button>

      {/* Post header */}
      <header className="mb-8">
        <div className="mb-3 flex flex-wrap items-center gap-x-3 gap-y-1">
          <time className="text-foreground-alt/50 text-xs tabular-nums">
            {post.date}
          </time>
          <span className="text-foreground-alt/20 text-xs">/</span>
          {post.tags.map((tag) => (
            <TagChip key={tag} tag={tag} onSelectTag={onSelectTag} />
          ))}
        </div>

        <h1 className="text-foreground mb-4 text-2xl leading-snug font-semibold tracking-tight @lg:text-3xl @lg:leading-snug">
          {post.title}
        </h1>

        {post.author && (
          <AuthorDisplay slug={post.author} registry={authorRegistry ?? {}} />
        )}
      </header>

      {/* Post body */}
      <div className="blog-prose">
        <Markdown options={markdownCodeOptions}>{post.body}</Markdown>
      </div>

      {/* Post navigation */}
      {(prevPost || nextPost) && (
        <nav className="mt-12 grid grid-cols-2 gap-4">
          {prevPost ? (
            <button
              type="button"
              onClick={navigatePrev}
              sx={[stylexStyles2.rule1, groupMarker2]}
            >
              <span className="text-foreground-alt/50 flex items-center gap-1.5 text-xs">
                <LuArrowLeft sx={stylexStyles.rule2} />
                Previous
              </span>
              <span className="text-foreground group-hover:text-brand text-sm font-medium transition-colors duration-200">
                {prevPost.title}
              </span>
            </button>
          ) : (
            <div />
          )}

          {nextPost ? (
            <button
              type="button"
              onClick={navigateNext}
              sx={[stylexStyles2.rule2, groupMarker2]}
            >
              <span className="text-foreground-alt/50 flex items-center gap-1.5 text-xs">
                Next
                <LuArrowRight sx={stylexStyles.rule3} />
              </span>
              <span className="text-foreground group-hover:text-brand text-sm font-medium transition-colors duration-200">
                {nextPost.title}
              </span>
            </button>
          ) : (
            <div />
          )}
        </nav>
      )}
    </article>
  )
}

// AuthorDisplayProps defines the props for AuthorDisplay.
interface AuthorDisplayProps {
  slug: string
  registry: AuthorRegistry
}

// AuthorDisplay renders the author info with avatar and bio when available.
function AuthorDisplay({ slug, registry }: AuthorDisplayProps) {
  const author = useMemo(() => resolveAuthor(registry, slug), [registry, slug])
  if (!author) return null

  const inner = (
    <div className="flex items-center gap-3">
      {author.avatar && (
        <img
          src={author.avatar}
          alt={author.name}
          className="size-8 rounded-full object-cover"
        />
      )}
      <div className="flex flex-col">
        <span className="text-foreground-alt/70 text-xs font-medium">
          {author.name}
        </span>
        {author.bio && (
          <span className="text-foreground-alt/50 text-xs">{author.bio}</span>
        )}
      </div>
    </div>
  )

  if (author.url) {
    return (
      <a
        href={author.url}
        target="_blank"
        rel="noopener noreferrer"
        className="transition-opacity hover:opacity-80"
      >
        {inner}
      </a>
    )
  }

  return inner
}
