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
    containerType: 'inline-size',
    display: 'flex',
    width: '100%',
    flex: 1,
    flexDirection: 'column',
    overflowY: 'auto',
    backgroundColor: themeColors['--color-background-landing'],
  },
  rule2: {
    position: 'relative',
    zIndex: 10,
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-5xl)',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingTop: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule3: {
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
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
      default: `calc(${themeLayout['--spacing']} * 16)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 20)`,
    },
    paddingBottom: {
      default: `calc(${themeLayout['--spacing']} * 6)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    textAlign: 'center',
  },
  rule5: {
    marginBottom: `calc(${themeLayout['--spacing']} * 5)`,
    display: 'inline-block',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-brand']} 80%, transparent)`,
  },
  rule6: {
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
    fontSize: {
      default: 'var(--text-3xl)',
      '@container (min-width: 32rem)': 'var(--text-4xl)',
    },
    lineHeight: {
      default: 'var(--tw-leading, var(--text-3xl--line-height))',
      '@container (min-width: 32rem)':
        'var(--tw-leading, var(--text-4xl--line-height))',
    },
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  rule7: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
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
  rule10: {
    borderRadius: 'var(--radius-xl)',
    borderStyle: 'dashed',
    borderWidth: '1px',
    '--tw-border-style': 'dashed',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 8)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 20)`,
    textAlign: 'center',
  },
  rule11: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
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
