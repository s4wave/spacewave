import { useCallback } from 'react'
import { LuArrowLeft } from '@s4wave/web/images/icons.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { LegalFooter } from '@s4wave/app/landing/LegalFooter.js'
import { HeroCard } from './HeroCard.js'
import { PostList } from './PostList.js'
import { TagChip } from './TagChip.js'
import type { BlogPost } from './types.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  colors as themeColors,
  fonts as themeFonts,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    containerType: 'inline-size',
    display: 'flex',
    width: '100%',
    flex: 1,
    flexDirection: 'column',
    overflowY: 'auto',
    backgroundColor: themeColors['--color-background-landing'],
  },
  rule2: {
    position: 'absolute',
    top: `calc(${themeLayout['--spacing']} * 4)`,
    left: `calc(${themeLayout['--spacing']} * 4)`,
    zIndex: 20,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule3: { WebkitUserSelect: 'none', userSelect: 'none' },
  rule4: {
    position: 'relative',
    zIndex: 10,
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-5xl)',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingTop: {
      default: `calc(${themeLayout['--spacing']} * 24)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 32)`,
    },
    paddingBottom: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
  },
  rule5: {
    marginBottom: `calc(${themeLayout['--spacing']} * 5)`,
    textAlign: 'center',
    fontSize: {
      default: 'var(--text-4xl)',
      '@container (min-width: 32rem)': 'var(--text-5xl)',
    },
    lineHeight: {
      default: 'var(--tw-leading, var(--text-4xl--line-height))',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-5xl--line-height))',
    },
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  rule6: {
    marginInline: 'auto',
    maxWidth: 'var(--container-xl)',
    textAlign: 'center',
    fontSize: {
      default: themeFonts['--text-sm'],
      '@container (min-width: 32rem)': 'var(--text-base)',
    },
    lineHeight: {
      default: 'var(--leading-relaxed)',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-base--line-height))',
    },
    '--tw-leading': 'var(--leading-relaxed)',
    '--tw-font-weight': 'var(--font-weight-light)',
    fontWeight: 'var(--font-weight-light)',
    color: themeColors['--color-foreground-alt'],
  },
  rule7: {
    marginTop: `calc(${themeLayout['--spacing']} * 8)`,
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule8: {
    position: 'relative',
    zIndex: 10,
    marginInline: 'auto',
    height: '1px',
    width: '100%',
    maxWidth: 'var(--container-5xl)',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 7%, transparent)`,
  },
  rule9: {
    position: 'relative',
    zIndex: 10,
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-5xl)',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingTop: `calc(${themeLayout['--spacing']} * 10)`,
    paddingBottom: `calc(${themeLayout['--spacing']} * 20)`,
  },
  rule10: { marginBottom: `calc(${themeLayout['--spacing']} * 14)` },
  rule11: {
    marginBottom: `calc(${themeLayout['--spacing']} * 6)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule12: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-wide)',
    letterSpacing: 'var(--tracking-wide)',
    color: themeColors['--color-foreground'],
    textTransform: 'uppercase',
  },
  rule13: {
    height: '1px',
    flex: 1,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 7%, transparent)`,
  },
  rule14: {
    borderRadius: 'var(--radius-xl)',
    borderStyle: 'dashed',
    borderWidth: '1px',
    '--tw-border-style': 'dashed',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 8)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 20)`,
    textAlign: 'center',
  },
  rule15: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

// metadata exports SEO metadata for prerender.
export const metadata = {
  title: 'Blog - Spacewave',
  description:
    'Release announcements, development updates, and technical deep dives on local-first sync, encryption, and peer-to-peer networking from the Spacewave team.',
}

// BlogIndexProps defines the props for BlogIndex.
export interface BlogIndexProps {
  posts: BlogPost[]
}

// BlogIndex renders the /blog listing page.
export function BlogIndex({ posts }: BlogIndexProps) {
  const navigate = useNavigate()
  const goHome = useCallback(() => navigate({ path: '/' }), [navigate])
  const latest = posts[0]
  const rest = posts.slice(1)

  const allTags = Array.from(new Set(posts.flatMap((p) => p.tags))).toSorted()

  return (
    <div sx={stylexStyles2.rule1}>
      {/* Back to home */}
      <button type="button" onClick={goHome} sx={stylexStyles2.rule2}>
        <LuArrowLeft sx={stylexStyles.rule1} />
        <span sx={stylexStyles2.rule3}>Back to home</span>
      </button>

      {/* Hero header */}
      <header sx={stylexStyles2.rule4}>
        <h1 sx={stylexStyles2.rule5}>Spacewave Blog</h1>
        <p sx={stylexStyles2.rule6}>
          Development updates, release announcements, and technical deep dives.
        </p>

        {/* Tag bar */}
        <div sx={stylexStyles2.rule7}>
          {allTags.map((tag) => (
            <TagChip key={tag} tag={tag} />
          ))}
        </div>
      </header>

      {/* Gradient separator */}
      <div sx={stylexStyles2.rule8} />

      <main sx={stylexStyles2.rule9}>
        {/* Featured post */}
        {latest && (
          <section sx={stylexStyles2.rule10}>
            <HeroCard post={latest} />
          </section>
        )}

        {/* Post list */}
        {rest.length > 0 && (
          <section>
            <div sx={stylexStyles2.rule11}>
              <h2 sx={stylexStyles2.rule12}>More posts</h2>
              <div sx={stylexStyles2.rule13} />
            </div>
            <PostList posts={rest} />
          </section>
        )}

        {posts.length === 0 && (
          <div sx={stylexStyles2.rule14}>
            <p sx={stylexStyles2.rule15}>No posts yet. Check back soon.</p>
          </div>
        )}
      </main>

      <LegalFooter />
    </div>
  )
}
