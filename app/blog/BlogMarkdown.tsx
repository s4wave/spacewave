import { useSyncExternalStore } from 'react'
import Markdown from 'markdown-to-jsx'
import { LuExternalLink } from 'react-icons/lu'

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
    <div className="flex h-full flex-col items-center justify-center gap-4 px-6 text-center">
      <p className="text-foreground text-sm font-medium">{frameTitle}</p>
      <a
        href={watchUrl}
        target="_blank"
        rel="noreferrer"
        className="border-brand/30 bg-brand/10 hover:bg-brand/20 text-foreground inline-flex items-center gap-2 rounded-md border px-4 py-2 text-sm font-medium transition-colors"
      >
        Watch on YouTube
        <LuExternalLink className="size-4" aria-hidden="true" />
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
    <div className="my-8">
      <div className="border-foreground/10 bg-background-card overflow-hidden rounded-2xl border shadow-sm">
        <div className="aspect-video">
          {support.canInline ? (
            <iframe
              {...credentiallessProps}
              src={src}
              title={frameTitle}
              className="h-full w-full"
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
    'yt-embed': {
      component: YouTubeEmbed,
    },
  },
} as const

export function BlogMarkdown({ children }: { children: string }) {
  return <Markdown options={blogMarkdownOptions}>{children}</Markdown>
}
