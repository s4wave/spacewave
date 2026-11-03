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

import { fonts as themeFonts } from '../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker3 } from '../../web/style/relations.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    position: 'relative',
    zIndex: 10,
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-3xl)',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingTop: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 10)`,
    },
    paddingBottom: `calc(${themeLayout['--spacing']} * 20)`,
  },
  rule2: { marginBottom: `calc(${themeLayout['--spacing']} * 8)` },
  rule3: {
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    columnGap: `calc(${themeLayout['--spacing']} * 3)`,
    rowGap: themeLayout['--spacing'],
  },
  rule4: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
  rule5: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 20%, transparent)`,
  },
  rule6: {
    marginLeft: 'auto',
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground-alt'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule7: {
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
    fontSize: {
      default: 'var(--text-2xl)',
      '@container (min-width: 32rem)': 'var(--text-3xl)',
    },
    lineHeight: {
      default: 'var(--leading-snug)',
      '@container (min-width: 32rem)': 'var(--leading-snug)',
    },
    '--tw-leading': {
      default: 'var(--leading-snug)',
      '@container (min-width: 32rem)': 'var(--leading-snug)',
    },
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  rule8: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule9: {
    width: `calc(${themeLayout['--spacing']} * 7)`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
  },
  rule10: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    textDecorationLine: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 'underline' },
    },
  },
  rule11: {
    marginTop: `calc(${themeLayout['--spacing']} * 12)`,
    display: 'grid',
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule12: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule13: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
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
    transitionDuration: '.2s',
    '--tw-duration': '.2s',
  },
})

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
      <article sx={stylexStyles3.rule1}>
        {/* Post header */}
        <header sx={stylexStyles3.rule2}>
          <div sx={stylexStyles3.rule3}>
            <time sx={stylexStyles3.rule4}>{post.date}</time>
            <span sx={stylexStyles3.rule5}>/</span>
            {post.tags.map((tag) => (
              <TagChip key={tag} tag={tag} />
            ))}
            <a
              href={`${GITHUB_REPO_URL}/blob/master/app/blog/posts/${post.date.slice(0, 4)}-${post.date.slice(5, 7)}-${post.date.slice(8, 10)}-${post.slug}.md`}
              target="_blank"
              rel="noopener noreferrer"
              sx={stylexStyles3.rule6}
              title="View source on GitHub"
            >
              <LuGithub sx={stylexStyles.rule1} />
            </a>
          </div>

          <h1 sx={stylexStyles3.rule7}>{post.title}</h1>

          <div sx={stylexStyles3.rule8}>
            <img
              src={post.author.avatar}
              alt={post.author.name}
              sx={stylexStyles3.rule9}
              loading="lazy"
            />
            <a
              href={safeHref(post.author.url)}
              target="_blank"
              rel="noopener noreferrer"
              sx={stylexStyles3.rule10}
            >
              {post.author.name}
            </a>
          </div>
        </header>

        {/* Post body */}
        <div
          className={
            /* TODO(stylex): global or unknown class: blog-prose */ 'blog-prose'
          }
        >
          <BlogMarkdown>{post.body}</BlogMarkdown>
        </div>

        <BlogCta />

        {/* Post navigation */}
        {(prevPost || nextPost) && (
          <nav sx={stylexStyles3.rule11}>
            {prevPost ? (
              <button
                type="button"
                onClick={navigatePrev}
                sx={[stylexStyles2.rule1, groupMarker2]}
              >
                <span sx={stylexStyles3.rule12}>
                  <LuArrowLeft sx={stylexStyles.rule2} />
                  Previous
                </span>
                <span sx={stylexStyles3.rule13}>{prevPost.title}</span>
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
                <span sx={stylexStyles3.rule12}>
                  Next
                  <LuArrowRight sx={stylexStyles.rule3} />
                </span>
                <span sx={stylexStyles3.rule13}>{nextPost.title}</span>
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
