import { useSyncExternalStore } from 'react'
import Markdown from 'markdown-to-jsx'
import { LuExternalLink } from '@s4wave/web/images/icons.js'

import { MarkdownLink } from '@s4wave/app/docs/MarkdownLink.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    height: '100%',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    textAlign: 'center',
  },
  rule2: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule3: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule4: { marginBlock: `calc(${themeLayout['--spacing']} * 8)` },
  rule5: {
    overflow: 'hidden',
    borderRadius: 'var(--radius-2xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: themeColors['--color-background-card'],
    '--tw-shadow':
      '0 1px 3px 0 var(--tw-shadow-color, #0000001a), 0 1px 2px -1px var(--tw-shadow-color, #0000001a)',
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
  },
  rule6: { aspectRatio: 'var(--aspect-video)' },
  rule7: { height: '100%', width: '100%' },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
})

interface YouTubeEmbedProps {
  videoid?: string
  title?: string
}

interface YouTubeEmbedSupport {
  canInline: boolean
  useCredentialless: boolean
}

const fallbackEmbedSupport: YouTubeEmbedSupport = {
  canInline: false,
  useCredentialless: false,
}

const inlineEmbedSupport: YouTubeEmbedSupport = {
  canInline: true,
  useCredentialless: false,
}

const credentiallessEmbedSupport: YouTubeEmbedSupport = {
  canInline: true,
  useCredentialless: true,
}

function subscribeEmbedSupport() {
  return () => {}
}

function getYouTubeEmbedSupport(): YouTubeEmbedSupport {
  if (typeof window === 'undefined') {
    return fallbackEmbedSupport
  }

  if (!window.crossOriginIsolated) {
    return inlineEmbedSupport
  }

  const iframeCredentiallessSupported =
    'credentialless' in window.HTMLIFrameElement.prototype

  return iframeCredentiallessSupported
    ? credentiallessEmbedSupport
    : fallbackEmbedSupport
}

function getServerYouTubeEmbedSupport(): YouTubeEmbedSupport {
  return fallbackEmbedSupport
}

function useYouTubeEmbedSupport(): YouTubeEmbedSupport {
  return useSyncExternalStore(
    subscribeEmbedSupport,
    getYouTubeEmbedSupport,
    getServerYouTubeEmbedSupport,
  )
}

function YouTubeFallback({
  frameTitle,
  watchUrl,
}: {
  frameTitle: string
  watchUrl: string
}) {
  return (
    <div sx={stylexStyles2.rule1}>
      <p sx={stylexStyles2.rule2}>{frameTitle}</p>
      <a
        href={watchUrl}
        target="_blank"
        rel="noreferrer"
        sx={stylexStyles2.rule3}
      >
        Watch on YouTube
        <LuExternalLink sx={stylexStyles.rule1} aria-hidden="true" />
      </a>
    </div>
  )
}

function YouTubeEmbed({ videoid, title }: YouTubeEmbedProps) {
  const support = useYouTubeEmbedSupport()

  if (!videoid) return null

  const src =
    'https://www.youtube-nocookie.com/embed/' +
    encodeURIComponent(videoid) +
    '?rel=0'
  const watchUrl =
    'https://www.youtube.com/watch?v=' + encodeURIComponent(videoid)
  const frameTitle = title ?? `YouTube video ${videoid}`
  const credentiallessProps = support.useCredentialless
    ? { credentialless: '' }
    : {}

  return (
    <div sx={stylexStyles2.rule4}>
      <div sx={stylexStyles2.rule5}>
        <div sx={stylexStyles2.rule6}>
          {support.canInline ? (
            <iframe
              {...credentiallessProps}
              src={src}
              title={frameTitle}
              sx={stylexStyles2.rule7}
              loading="lazy"
              allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share"
              referrerPolicy="strict-origin-when-cross-origin"
              allowFullScreen
            />
          ) : (
            <YouTubeFallback frameTitle={frameTitle} watchUrl={watchUrl} />
          )}
        </div>
      </div>
    </div>
  )
}

export const blogMarkdownOptions = {
  overrides: {
    a: { component: MarkdownLink },
    'yt-embed': {
      component: YouTubeEmbed,
    },
  },
} as const

export function BlogMarkdown({ children }: { children: string }) {
  return <Markdown options={blogMarkdownOptions}>{children}</Markdown>
}
