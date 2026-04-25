import Markdown from 'markdown-to-jsx'

interface YouTubeEmbedProps {
  videoid?: string
  title?: string
}

function YouTubeEmbed({ videoid, title }: YouTubeEmbedProps) {
  if (!videoid) return null

  const src =
    'https://www.youtube-nocookie.com/embed/' +
    encodeURIComponent(videoid) +
    '?rel=0'

  return (
    <div className="my-8">
      <div className="border-foreground/10 bg-background-card overflow-hidden rounded-2xl border shadow-sm">
        <div className="aspect-video">
          <iframe
            src={src}
            title={title || 'YouTube video'}
            className="h-full w-full"
            loading="lazy"
            allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share"
            referrerPolicy="strict-origin-when-cross-origin"
            allowFullScreen
          />
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
