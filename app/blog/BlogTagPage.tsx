import { useCallback } from 'react'
import { useNavigate } from '@s4wave/web/router/router.js'
import { LuArrowLeft } from '@s4wave/web/images/icons.js'
import { LegalFooter } from '@s4wave/app/landing/LegalFooter.js'
import { PostList } from './PostList.js'
import type { BlogPost } from './types.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  colors as themeColors,
  fonts as themeFonts,
  motion as themeMotion,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    flex: 1,
    backgroundColor: themeColors['--color-background-landing'],
    containerType: 'inline-size',
    display: 'flex',
    flexDirection: 'column',
    overflowY: 'auto',
    width: '100%',
  },
  rule2: {
    marginInline: 'auto',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    position: 'relative',
    zIndex: 10,
    maxWidth: 'var(--container-5xl)',
    paddingTop: `calc(${themeLayout['--spacing']} * 6)`,
    width: '100%',
  },
  rule3: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    cursor: 'pointer',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule4: {
    marginInline: 'auto',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    position: 'relative',
    textAlign: 'center',
    zIndex: 10,
    maxWidth: 'var(--container-5xl)',
    paddingBottom: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingTop: {
      default: `calc(${themeLayout['--spacing']} * 16)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 20)`,
    },
    width: '100%',
  },
  rule5: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    color: `color-mix(in oklab, ${themeColors['--color-brand']} 80%, transparent)`,
    display: 'inline-block',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginBottom: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule6: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
    fontSize: {
      default: 'var(--text-3xl)',
      '@container (min-width: 32rem)': 'var(--text-4xl)',
    },
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: {
      default: 'var(--tw-leading, var(--text-3xl--line-height))',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-4xl--line-height))',
    },
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule7: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule8: {
    marginInline: 'auto',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 7%, transparent)`,
    position: 'relative',
    zIndex: 10,
    height: '1px',
    maxWidth: 'var(--container-5xl)',
    width: '100%',
  },
  rule9: {
    marginInline: 'auto',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    position: 'relative',
    zIndex: 10,
    maxWidth: 'var(--container-5xl)',
    paddingBottom: `calc(${themeLayout['--spacing']} * 20)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 10)`,
    width: '100%',
  },
  rule10: {
    '--tw-border-style': 'dashed',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: 'dashed',
    borderWidth: '1px',
    paddingBlock: `calc(${themeLayout['--spacing']} * 20)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 8)`,
    textAlign: 'center',
  },
  rule11: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

// BlogTagPageProps defines the props for BlogTagPage.
export interface BlogTagPageProps {
  tag: string
  posts: BlogPost[]
}

// BlogTagPage renders a filtered listing of posts for a specific tag.
export function BlogTagPage({ tag, posts }: BlogTagPageProps) {
  const navigate = useNavigate()

  const navigateBlog = useCallback(() => {
    navigate({ path: '/blog' })
  }, [navigate])

  return (
    <div sx={stylexStyles2.rule1}>
      <div sx={stylexStyles2.rule2}>
        <button type="button" onClick={navigateBlog} sx={stylexStyles2.rule3}>
          <LuArrowLeft sx={stylexStyles.rule1} />
          All posts
        </button>
      </div>

      <header sx={stylexStyles2.rule4}>
        <div sx={stylexStyles2.rule5}>{tag}</div>
        <h1 sx={stylexStyles2.rule6}>Posts tagged "{tag}"</h1>
        <p sx={stylexStyles2.rule7}>
          {posts.length} {posts.length === 1 ? 'post' : 'posts'}
        </p>
      </header>

      <div sx={stylexStyles2.rule8} />

      <main sx={stylexStyles2.rule9}>
        {posts.length > 0 ? (
          <PostList posts={posts} />
        ) : (
          <div sx={stylexStyles2.rule10}>
            <p sx={stylexStyles2.rule11}>No posts with this tag.</p>
          </div>
        )}
      </main>

      <LegalFooter />
    </div>
  )
}
