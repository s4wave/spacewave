import { useEffect, useCallback } from 'react'
import { useNavigate } from '@s4wave/web/router/router.js'
import { LuArrowLeft, LuArrowRight } from '@s4wave/web/images/icons.js'
import { LuGithub } from '@s4wave/web/images/icons.js'
import { BlogLayout } from './BlogLayout.js'
import { BlogCta } from './BlogCta.js'
import { BlogMarkdown } from './BlogMarkdown.js'
import { TagChip } from './TagChip.js'
import { safeHref } from './safe-href.js'
import { GITHUB_REPO_URL } from '@s4wave/app/github.js'
import type { BlogPost as BlogPostType } from './types.js'
import './blog-prose.css'

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
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
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

export interface BlogPostNavLink {
  title: string
  url: string
}

export interface BlogPostPageProps {
  post: BlogPostType
  prevPost?: BlogPostNavLink
  nextPost?: BlogPostNavLink
}

// BlogPostPage renders a single blog post in clean reading mode.
export function BlogPostPage({ post, prevPost, nextPost }: BlogPostPageProps) {
  const navigate = useNavigate()

  const navigatePrev = useCallback(() => {
    if (prevPost) navigate({ path: prevPost.url })
  }, [navigate, prevPost])

  const navigateNext = useCallback(() => {
    if (nextPost) navigate({ path: nextPost.url })
  }, [navigate, nextPost])

  // Inject JSON-LD structured data into document head.
  useEffect(() => {
    const script = document.createElement('script')
    script.type = 'application/ld+json'
    script.textContent = JSON.stringify({
      '@context': 'https://schema.org',
      '@type': 'Article',
      headline: post.title,
      datePublished: post.date,
      author: {
        '@type': 'Person',
        name: post.author.name,
        url: post.author.url,
      },
      description: post.summary,
    })
    document.head.appendChild(script)
    return () => {
      document.head.removeChild(script)
    }
  }, [post])

  return (
    <BlogLayout>
      <article className="relative z-10 mx-auto w-full max-w-3xl px-4 pt-6 pb-20 @lg:px-8 @lg:pt-10">
        {/* Post header */}
        <header className="mb-8">
          <div className="mb-3 flex flex-wrap items-center gap-x-3 gap-y-1">
            <time className="text-foreground-alt/50 text-xs tabular-nums">
              {post.date}
            </time>
            <span className="text-foreground-alt/20 text-xs">/</span>
            {post.tags.map((tag) => (
              <TagChip key={tag} tag={tag} />
            ))}
            <a
              href={`${GITHUB_REPO_URL}/blob/master/app/blog/posts/${post.date.slice(0, 4)}-${post.date.slice(5, 7)}-${post.date.slice(8, 10)}-${post.slug}.md`}
              target="_blank"
              rel="noopener noreferrer"
              className="text-foreground-alt/40 hover:text-foreground-alt ml-auto flex items-center gap-1.5 text-xs transition-colors"
              title="View source on GitHub"
            >
              <LuGithub sx={stylexStyles.rule1} />
            </a>
          </div>

          <h1 className="text-foreground mb-4 text-2xl leading-snug font-semibold tracking-tight @lg:text-3xl @lg:leading-snug">
            {post.title}
          </h1>

          <div className="flex items-center gap-3">
            <img
              src={post.author.avatar}
              alt={post.author.name}
              className="border-foreground/10 size-7 rounded-full border"
              loading="lazy"
            />
            <a
              href={safeHref(post.author.url)}
              target="_blank"
              rel="noopener noreferrer"
              className="text-foreground-alt/70 text-xs font-medium hover:underline"
            >
              {post.author.name}
            </a>
          </div>
        </header>

        {/* Post body */}
        <div className="blog-prose">
          <BlogMarkdown>{post.body}</BlogMarkdown>
        </div>

        <BlogCta />

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
    </BlogLayout>
  )
}
