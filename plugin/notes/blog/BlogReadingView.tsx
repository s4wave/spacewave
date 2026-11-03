import { useCallback, useMemo, useState } from 'react'

import { LuX } from '@s4wave/web/images/icons.js'

import { HeroCard } from './HeroCard.js'
import { PostList } from './PostList.js'
import { BlogPostView } from './BlogPostView.js'
import type { BlogPostData } from './types.js'
import type { AuthorRegistry } from './authors.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '@s4wave/web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
  motion as themeMotion,
  colors as themeColors,
} from '@s4wave/web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: { height: '100%', overflowY: 'auto' },
  rule2: {
    marginInline: 'auto',
    paddingBlock: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 10)`,
    },
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    maxWidth: 'var(--container-3xl)',
    width: '100%',
  },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    flexWrap: 'wrap',
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule4: {
    '--tw-duration': '.2s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    cursor: 'pointer',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration: '.2s',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule5: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-brand'],
  },
  rule6: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-white']} 8%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      },
    },
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
      },
    },
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand'],
      },
    },
  },
  rule7: {
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
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
  },
  rule8: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 20)`,
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    justifyContent: 'center',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule9: { marginBottom: `calc(${themeLayout['--spacing']} * 8)` },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

// BlogReadingViewProps defines the props for BlogReadingView.
interface BlogReadingViewProps {
  posts: BlogPostData[]
  selectedPost: BlogPostData | null
  onSelectPost: (post: BlogPostData | null) => void
  authorRegistry: AuthorRegistry
}

// BlogReadingView renders the blog reading experience with index and post modes.
export function BlogReadingView({
  posts,
  selectedPost,
  onSelectPost,
  authorRegistry,
}: BlogReadingViewProps) {
  const [tagFilter, setTagFilter] = useState<string | null>(null)

  // Filter out drafts and posts without dates, sort by date descending.
  const publishedPosts = useMemo(() => {
    let filtered = posts.filter((p) => p.date && !p.draft)
    if (tagFilter) {
      filtered = filtered.filter((p) =>
        p.tags.some((t) => t.toLowerCase() === tagFilter.toLowerCase()),
      )
    }
    return filtered.sort((a, b) => b.date.localeCompare(a.date))
  }, [posts, tagFilter])

  // Collect all unique tags.
  const allTags = useMemo(() => {
    const tags = new Set<string>()
    for (const post of posts) {
      if (post.date && !post.draft) {
        for (const tag of post.tags) {
          tags.add(tag)
        }
      }
    }
    return Array.from(tags).sort()
  }, [posts])

  const handleSelectTag = useCallback((tag: string) => {
    setTagFilter((prev) => (prev === tag ? null : tag))
  }, [])

  const handleClearFilter = useCallback(() => {
    setTagFilter(null)
  }, [])

  const handleBack = useCallback(() => {
    onSelectPost(null)
  }, [onSelectPost])

  const selectedPublishedPost =
    selectedPost &&
    publishedPosts.some((post) => post.name === selectedPost.name)
      ? selectedPost
      : null

  // Post view mode: show selected published post.
  if (selectedPublishedPost) {
    const idx = publishedPosts.findIndex(
      (p) => p.name === selectedPublishedPost.name,
    )
    const prevPost =
      idx < publishedPosts.length - 1 ? publishedPosts[idx + 1] : undefined
    const nextPost = idx > 0 ? publishedPosts[idx - 1] : undefined

    return (
      <div sx={stylexStyles2.rule1}>
        <BlogPostView
          post={selectedPublishedPost}
          prevPost={prevPost}
          nextPost={nextPost}
          onSelectPost={onSelectPost}
          onSelectTag={handleSelectTag}
          onBack={handleBack}
          authorRegistry={authorRegistry}
        />
      </div>
    )
  }

  // Index mode: hero card + post list.
  const heroPost = publishedPosts[0]
  const remainingPosts = publishedPosts.slice(1)

  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        {/* Tag filter bar */}
        {allTags.length > 0 && (
          <div sx={stylexStyles2.rule3}>
            {allTags.map((tag) => (
              <button
                type="button"
                key={tag}
                onClick={() => handleSelectTag(tag)}
                sx={[
                  stylexStyles2.rule4,
                  tagFilter === tag ? stylexStyles2.rule5 : stylexStyles2.rule6,
                ]}
              >
                {tag}
              </button>
            ))}
            {tagFilter && (
              <button
                type="button"
                onClick={handleClearFilter}
                sx={stylexStyles2.rule7}
              >
                <LuX sx={stylexStyles.rule1} />
                Clear
              </button>
            )}
          </div>
        )}

        {publishedPosts.length === 0 ? (
          <div sx={stylexStyles2.rule8}>
            {tagFilter ? 'No posts match this tag' : 'No published posts yet'}
          </div>
        ) : (
          <>
            {/* Hero card for latest post */}
            {heroPost && (
              <div sx={stylexStyles2.rule9}>
                <HeroCard
                  post={heroPost}
                  onSelectPost={onSelectPost}
                  onSelectTag={handleSelectTag}
                  authorRegistry={authorRegistry}
                />
              </div>
            )}

            {/* Remaining posts */}
            {remainingPosts.length > 0 && (
              <PostList posts={remainingPosts} onSelectPost={onSelectPost} />
            )}
          </>
        )}
      </div>
    </div>
  )
}
