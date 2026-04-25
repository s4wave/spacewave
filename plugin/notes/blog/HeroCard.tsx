import { useCallback, useMemo } from 'react'

import { LuArrowRight } from 'react-icons/lu'

import { TagChip } from './TagChip.js'
import type { BlogPostData } from './types.js'
import { resolveAuthor, type AuthorRegistry } from './authors.js'

// HeroCardProps defines the props for HeroCard.
interface HeroCardProps {
  post: BlogPostData
  onSelectPost: (post: BlogPostData) => void
  onSelectTag?: (tag: string) => void
  authorRegistry?: AuthorRegistry
}

// HeroCard renders the featured hero card for the latest blog post.
export function HeroCard({ post, onSelectPost, onSelectTag, authorRegistry }: HeroCardProps) {
  const handleClick = useCallback(() => {
    onSelectPost(post)
  }, [onSelectPost, post])

  const author = useMemo(
    () => resolveAuthor(authorRegistry ?? {}, post.author),
    [authorRegistry, post.author],
  )

  return (
    <article
      onClick={handleClick}
      className="border-foreground/8 bg-background-card/20 group relative cursor-pointer overflow-hidden rounded-2xl border backdrop-blur-sm transition-all duration-300 hover:-translate-y-0.5 hover:border-white/12"
    >
      <div className="relative flex flex-col gap-6 p-6 @lg:flex-row @lg:items-start @lg:gap-10 @lg:p-10">
        <div className="flex-1">
          <div className="mb-4 flex flex-wrap gap-2">
            {post.tags.map((tag) => (
              <TagChip key={tag} tag={tag} onSelectTag={onSelectTag} />
            ))}
          </div>

          <h2 className="text-foreground group-hover:text-brand mb-3 text-2xl font-bold tracking-tight transition-colors duration-300 @lg:text-3xl">
            {post.title}
          </h2>

          {post.summary && (
            <p className="text-foreground-alt mb-6 text-sm leading-relaxed @lg:text-base @lg:leading-relaxed">
              {post.summary}
            </p>
          )}

          <div className="text-brand flex items-center gap-2 text-sm font-medium opacity-0 transition-all duration-300 group-hover:opacity-100">
            Read post
            <LuArrowRight className="h-4 w-4 transition-transform duration-300 group-hover:translate-x-1" />
          </div>
        </div>

        {/* Author and date */}
        <div className="@lg:border-foreground/6 flex shrink-0 items-center gap-3 @lg:flex-col @lg:items-end @lg:gap-3 @lg:border-l @lg:pl-10">
          {author?.avatar && (
            <img
              src={author.avatar}
              alt={author.name}
              className="h-8 w-8 rounded-full object-cover"
            />
          )}
          <div className="@lg:text-right">
            {author && (
              <span className="text-foreground text-sm font-medium">
                {author.name}
              </span>
            )}
            <time className="text-foreground-alt/60 block text-xs">
              {post.date}
            </time>
          </div>
        </div>
      </div>
    </article>
  )
}
