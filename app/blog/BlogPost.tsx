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
    marginInline: 'auto',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    position: 'relative',
    zIndex: 10,
    maxWidth: 'var(--container-3xl)',
    paddingBottom: `calc(${themeLayout['--spacing']} * 20)`,
    paddingTop: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 10)`,
    },
    width: '100%',
  },
  rule2: { marginBottom: `calc(${themeLayout['--spacing']} * 8)` },
  rule3: {
    alignItems: 'center',
    columnGap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    flexWrap: 'wrap',
    rowGap: themeLayout['--spacing'],
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule4: {
    '--tw-numeric-spacing': 'tabular-nums',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule5: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 20%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule6: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground-alt'],
      },
    },
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    marginLeft: 'auto',
  },
  rule7: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-leading': {
      default: 'var(--leading-snug)',
      '@container (min-width: 32rem)': 'var(--leading-snug)',
    },
    '--tw-tracking': 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    fontSize: {
      default: 'var(--text-2xl)',
      '@container (min-width: 32rem)': 'var(--text-3xl)',
    },
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: {
      default: 'var(--leading-snug)',
      '@container (min-width: 32rem)': 'var(--leading-snug)',
    },
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule8: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule9: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    height: `calc(${themeLayout['--spacing']} * 7)`,
    width: `calc(${themeLayout['--spacing']} * 7)`,
  },
  rule10: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textDecorationLine: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 'underline' },
    },
  },
  rule11: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'grid',
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
    marginTop: `calc(${themeLayout['--spacing']} * 12)`,
  },
  rule12: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule13: {
    '--tw-duration': '.2s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: {
      default: themeColors['--color-foreground'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker3)]:
          themeColors['--color-brand'],
      },
    },
    fontSize: themeFonts['--text-sm'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    transitionDuration: '.2s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    '--tw-duration': '.2s',
    padding: `calc(${themeLayout['--spacing']} * 5)`,
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'flex-start',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      },
    },
    cursor: 'pointer',
    display: 'flex',
    flexDirection: 'column',
    textAlign: 'left',
    transitionDuration: '.2s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule2: {
    '--tw-duration': '.2s',
    padding: `calc(${themeLayout['--spacing']} * 5)`,
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'flex-end',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      },
    },
    cursor: 'pointer',
    display: 'flex',
    flexDirection: 'column',
    textAlign: 'right',
    transitionDuration: '.2s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    '--tw-duration': '.2s',
    '--tw-translate-x': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          `calc(${themeLayout['--spacing']} * -.5)`,
      },
    },
    transitionDuration: '.2s',
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
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    '--tw-duration': '.2s',
    '--tw-translate-x': {
      default: null,
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          `calc(${themeLayout['--spacing']} * .5)`,
      },
    },
    transitionDuration: '.2s',
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
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
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
        <div className={'blog-prose'}>
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
