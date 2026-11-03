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
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    maxWidth: 'var(--container-3xl)',
    paddingBottom: `calc(${themeLayout['--spacing']} * 20)`,
    paddingTop: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 10)`,
    },
    width: '100%',
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
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
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule3: { marginBottom: `calc(${themeLayout['--spacing']} * 8)` },
  rule4: {
    alignItems: 'center',
    columnGap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    flexWrap: 'wrap',
    rowGap: themeLayout['--spacing'],
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule5: {
    '--tw-numeric-spacing': 'tabular-nums',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    fontVariantNumeric:
      'var(--tw-ordinal, ) var(--tw-slashed-zero, ) var(--tw-numeric-figure, ) var(--tw-numeric-spacing, ) var(--tw-numeric-fraction, )',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule6: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 20%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
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
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'grid',
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
    marginTop: `calc(${themeLayout['--spacing']} * 12)`,
  },
  rule9: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule10: {
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
  rule11: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
  },
  rule12: {
    borderRadius: '3.40282e38px',
    objectFit: 'cover',
    height: `calc(${themeLayout['--spacing']} * 8)`,
    width: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule13: { display: 'flex', flexDirection: 'column' },
  rule14: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule15: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule16: {
    opacity: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 0.8 },
    },
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty: 'opacity',
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
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
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
          'blog-prose'
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
