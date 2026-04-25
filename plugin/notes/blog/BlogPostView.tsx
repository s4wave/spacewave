import { useCallback, useMemo } from 'react'

import Markdown from 'markdown-to-jsx'
import { LuArrowLeft, LuArrowRight } from 'react-icons/lu'

import { useMarkdownCodeOverrides } from '../CodeBlock.js'
import { TagChip } from './TagChip.js'
import type { BlogPostData } from './types.js'
import { resolveAuthor, type AuthorRegistry } from './authors.js'
import './blog-prose.css'

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

  const markdownOptions = useMarkdownCodeOverrides()

  return (
    <article className="mx-auto w-full max-w-3xl px-4 pt-6 pb-20 @lg:px-8 @lg:pt-10">
      {/* Back button */}
      <button
        onClick={onBack}
        className="text-foreground-alt/60 hover:text-foreground mb-6 flex items-center gap-1.5 text-xs transition-colors"
      >
        <LuArrowLeft className="h-3 w-3" />
        Back to posts
      </button>

      {/* Post header */}
      <header className="mb-8">
        <div className="mb-3 flex flex-wrap items-center gap-x-3 gap-y-1">
          <time className="text-foreground-alt/50 text-xs tabular-nums">
            {post.date}
          </time>
          <span className="text-foreground-alt/20 text-xs">/</span>
          {post.tags.map((tag) => (
            <TagChip key={tag} tag={tag} onSelectTag={onSelectTag} />
          ))}
        </div>

        <h1 className="text-foreground mb-4 text-2xl leading-snug font-bold tracking-tight @lg:text-3xl @lg:leading-snug">
          {post.title}
        </h1>

        {post.author && (
          <AuthorDisplay
            slug={post.author}
            registry={authorRegistry ?? {}}
          />
        )}
      </header>

      {/* Post body */}
      <div className="blog-prose">
        <Markdown options={markdownOptions}>{post.body}</Markdown>
      </div>

      {/* Post navigation */}
      {(prevPost || nextPost) && (
        <nav className="mt-12 grid grid-cols-2 gap-4">
          {prevPost ?
            <button
              onClick={navigatePrev}
              className="border-foreground/6 hover:border-foreground/12 hover:bg-background-card/30 group flex cursor-pointer flex-col items-start gap-1.5 rounded-xl border p-5 text-left transition-all duration-200"
            >
              <span className="text-foreground-alt/50 flex items-center gap-1.5 text-xs">
                <LuArrowLeft className="h-3 w-3 transition-transform duration-200 group-hover:-translate-x-0.5" />
                Previous
              </span>
              <span className="text-foreground group-hover:text-brand text-sm font-medium transition-colors duration-200">
                {prevPost.title}
              </span>
            </button>
          : <div />}

          {nextPost ?
            <button
              onClick={navigateNext}
              className="border-foreground/6 hover:border-foreground/12 hover:bg-background-card/30 group flex cursor-pointer flex-col items-end gap-1.5 rounded-xl border p-5 text-right transition-all duration-200"
            >
              <span className="text-foreground-alt/50 flex items-center gap-1.5 text-xs">
                Next
                <LuArrowRight className="h-3 w-3 transition-transform duration-200 group-hover:translate-x-0.5" />
              </span>
              <span className="text-foreground group-hover:text-brand text-sm font-medium transition-colors duration-200">
                {nextPost.title}
              </span>
            </button>
          : <div />}
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
    <div className="flex items-center gap-3">
      {author.avatar && (
        <img
          src={author.avatar}
          alt={author.name}
          className="h-8 w-8 rounded-full object-cover"
        />
      )}
      <div className="flex flex-col">
        <span className="text-foreground-alt/70 text-xs font-medium">
          {author.name}
        </span>
        {author.bio && (
          <span className="text-foreground-alt/50 text-xs">
            {author.bio}
          </span>
        )}
      </div>
    </div>
  )

  if (author.url) {
    return (
      <a
        href={author.url}
        target="_blank"
        rel="noopener noreferrer"
        className="hover:opacity-80 transition-opacity"
      >
        {inner}
      </a>
    )
  }

  return inner
}
