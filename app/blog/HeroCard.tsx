import { useCallback } from 'react'
import { useNavigate } from '@s4wave/web/router/router.js'
import { TagChip } from './TagChip.js'
import { LuArrowRight } from 'react-icons/lu'
import type { BlogPost } from './types.js'

// HeroCardProps defines the props for HeroCard.
interface HeroCardProps {
  post: BlogPost
}

// HeroCard renders the featured hero card for the latest blog post.
export function HeroCard({ post }: HeroCardProps) {
  const navigate = useNavigate()

  const handleClick = useCallback(() => {
    navigate({ path: post.url })
  }, [navigate, post.url])

  return (
    <article
      onClick={handleClick}
      className="border-foreground/8 bg-background-card/20 group relative cursor-pointer overflow-hidden rounded-2xl border backdrop-blur-sm transition-all duration-300 hover:-translate-y-0.5 hover:border-white/12"
    >
      {/* Subtle gradient glow on hover */}
      <div className="from-brand/5 pointer-events-none absolute inset-0 bg-gradient-to-br via-transparent to-transparent opacity-0 transition-opacity duration-500 group-hover:opacity-100" />

      <div className="relative flex flex-col gap-6 p-6 @lg:flex-row @lg:items-start @lg:gap-10 @lg:p-10">
        {/* Content */}
        <div className="flex-1">
          <div className="mb-4 flex flex-wrap gap-2">
            {post.tags.map((tag) => (
              <TagChip key={tag} tag={tag} />
            ))}
          </div>

          <h2 className="text-foreground group-hover:text-brand mb-3 text-2xl font-bold tracking-tight transition-colors duration-300 @lg:text-3xl">
            {post.title}
          </h2>

          <p className="text-foreground-alt mb-6 text-sm leading-relaxed @lg:text-base @lg:leading-relaxed">
            {post.summary}
          </p>

          <div className="text-brand flex items-center gap-2 text-sm font-medium opacity-0 transition-all duration-300 group-hover:opacity-100">
            Read post
            <LuArrowRight className="h-4 w-4 transition-transform duration-300 group-hover:translate-x-1" />
          </div>
        </div>

        {/* Author + date sidebar */}
        <div className="@lg:border-foreground/6 flex shrink-0 items-center gap-3 @lg:flex-col @lg:items-end @lg:gap-3 @lg:border-l @lg:pl-10">
          <img
            src={post.author.avatar}
            alt={post.author.name}
            className="border-foreground/10 h-10 w-10 rounded-full border @lg:h-12 @lg:w-12"
            loading="lazy"
          />
          <div className="@lg:text-right">
            <a
              href={post.author.url}
              target="_blank"
              rel="noopener noreferrer"
              className="text-foreground text-sm font-medium hover:underline"
              onClick={(e) => e.stopPropagation()}
            >
              {post.author.name}
            </a>
            <time className="text-foreground-alt/60 block text-xs">
              {post.date}
            </time>
          </div>
        </div>
      </div>
    </article>
  )
}
