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

import { fonts as themeFonts } from '../../../web/style/tokens.stylex.js'
import { groupMarker as groupMarker3 } from '../../../web/style/relations.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
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
  rule2: {
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule3: { marginBottom: `calc(${themeLayout['--spacing']} * 8)` },
  rule4: {
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    columnGap: `calc(${themeLayout['--spacing']} * 3)`,
    rowGap: themeLayout['--spacing'],
  },
  rule5: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    '--tw-numeric-spacing': 'tabular-nums',
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
  },
  rule6: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 20%, transparent)`,
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
    marginTop: `calc(${themeLayout['--spacing']} * 12)`,
    display: 'grid',
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule9: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule10: {
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
  rule11: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule12: {
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    borderRadius: '3.40282e38px',
    objectFit: 'cover',
  },
  rule13: { display: 'flex', flexDirection: 'column' },
  rule14: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule15: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule16: {
    transitionProperty: 'opacity',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    opacity: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 0.8 },
    },
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
    <article sx={stylexStyles3.rule1}>
      {/* Back button */}
      <button type="button" onClick={onBack} sx={stylexStyles3.rule2}>
        <LuArrowLeft sx={stylexStyles.rule1} />
        Back to posts
      </button>

      {/* Post header */}
      <header sx={stylexStyles3.rule3}>
        <div sx={stylexStyles3.rule4}>
          <time sx={stylexStyles3.rule5}>{post.date}</time>
          <span sx={stylexStyles3.rule6}>/</span>
          {post.tags.map((tag) => (
            <TagChip key={tag} tag={tag} onSelectTag={onSelectTag} />
          ))}
        </div>

        <h1 sx={stylexStyles3.rule7}>{post.title}</h1>

        {post.author && (
          <AuthorDisplay slug={post.author} registry={authorRegistry ?? {}} />
        )}
      </header>

      {/* Post body */}
      <div
        className={
          /* TODO(stylex): global or unknown class: blog-prose */ 'blog-prose'
        }
      >
        <Markdown options={markdownCodeOptions}>{post.body}</Markdown>
      </div>

      {/* Post navigation */}
      {(prevPost || nextPost) && (
        <nav sx={stylexStyles3.rule8}>
          {prevPost ? (
            <button
              type="button"
              onClick={navigatePrev}
              sx={[stylexStyles2.rule1, groupMarker2]}
            >
              <span sx={stylexStyles3.rule9}>
                <LuArrowLeft sx={stylexStyles.rule2} />
                Previous
              </span>
              <span sx={stylexStyles3.rule10}>{prevPost.title}</span>
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
              <span sx={stylexStyles3.rule9}>
                Next
                <LuArrowRight sx={stylexStyles.rule3} />
              </span>
              <span sx={stylexStyles3.rule10}>{nextPost.title}</span>
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
    <div sx={stylexStyles3.rule11}>
      {author.avatar && (
        <img src={author.avatar} alt={author.name} sx={stylexStyles3.rule12} />
      )}
      <div sx={stylexStyles3.rule13}>
        <span sx={stylexStyles3.rule14}>{author.name}</span>
        {author.bio && <span sx={stylexStyles3.rule15}>{author.bio}</span>}
      </div>
    </div>
  )

  if (author.url) {
    return (
      <a
        href={author.url}
        target="_blank"
        rel="noopener noreferrer"
        sx={stylexStyles3.rule16}
      >
        {inner}
      </a>
    )
  }

  return inner
}
