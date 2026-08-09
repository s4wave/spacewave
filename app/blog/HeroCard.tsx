import { useCallback, type MouseEvent } from 'react'
import { LuArrowRight } from 'react-icons/lu'

import { useNavigate } from '@s4wave/web/router/router.js'

import { shouldNavigateInApp } from './link-click.js'
import { safeHref } from './safe-href.js'
import { TagChip } from './TagChip.js'
import type { BlogPost } from './types.js'

interface HeroCardProps {
  post: BlogPost
}

export function HeroCard({ post }: HeroCardProps) {
  const navigate = useNavigate()

  const handlePostSelect = useCallback(
    (event: MouseEvent<HTMLAnchorElement>) => {
      if (!shouldNavigateInApp(event)) return
      event.preventDefault()
      navigate({ path: post.url })
    },
    [navigate, post.url],
  )

  return (
    <article className="border-foreground/8 bg-background-card/20 group relative overflow-hidden rounded-2xl border backdrop-blur-sm transition duration-300 hover:-translate-y-0.5 hover:border-white/12">
      <a
        href={post.url}
        aria-label={post.title}
        onClick={handlePostSelect}
        className="focus-visible:ring-brand/50 absolute inset-0 z-0 cursor-pointer rounded-2xl focus-visible:ring-2 focus-visible:outline-none"
      />
      <div className="bg-brand/5 pointer-events-none absolute inset-0 opacity-0 transition-opacity duration-500 group-hover:opacity-100" />

      <div className="pointer-events-none relative z-10 flex flex-col gap-6 p-6 @lg:flex-row @lg:items-start @lg:gap-10 @lg:p-10">
        <div className="flex-1">
          <div className="pointer-events-none mb-4 flex flex-wrap gap-2">
            {post.tags.map((tag) => (
              <TagChip key={tag} tag={tag} />
            ))}
          </div>

          <h2 className="text-foreground group-hover:text-brand mb-3 text-2xl font-semibold tracking-tight transition-colors duration-300 @lg:text-3xl">
            {post.title}
          </h2>

          <p className="text-foreground-alt mb-6 text-sm leading-relaxed @lg:text-base @lg:leading-relaxed">
            {post.summary}
          </p>

          <div className="text-brand flex items-center gap-2 text-sm font-medium opacity-0 transition duration-300 group-hover:opacity-100">
            Read post
            <LuArrowRight className="size-4 transition-transform duration-300 group-hover:translate-x-1" />
          </div>
        </div>

        <div className="@lg:border-foreground/6 flex shrink-0 items-center gap-3 @lg:flex-col @lg:items-end @lg:gap-3 @lg:border-l @lg:pl-10">
          <img
            src={post.author.avatar}
            alt={post.author.name}
            className="border-foreground/10 size-10 rounded-full border @lg:h-12 @lg:w-12"
            loading="lazy"
          />
          <div className="@lg:text-right">
            <a
              href={safeHref(post.author.url)}
              target="_blank"
              rel="noopener noreferrer"
              className="text-foreground pointer-events-auto text-sm font-medium hover:underline"
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
