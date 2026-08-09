import { useCallback, type MouseEvent } from 'react'
import { LuArrowRight } from 'react-icons/lu'

import { useNavigate } from '@s4wave/web/router/router.js'

import { shouldNavigateInApp } from './link-click.js'
import { TagChip } from './TagChip.js'
import type { BlogPost } from './types.js'

interface PostListItemProps {
  post: BlogPost
}

function PostListItem({ post }: PostListItemProps) {
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
    <article className="border-foreground/6 hover:bg-background-card/30 group relative flex items-start gap-5 border-b p-5 transition duration-200 last:border-b-0 @lg:items-center">
      <a
        href={post.url}
        aria-label={post.title}
        onClick={handlePostSelect}
        className="focus-visible:ring-brand/50 absolute inset-0 z-0 cursor-pointer focus-visible:ring-2 focus-visible:outline-none"
      />
      <div className="pointer-events-none relative z-10 min-w-0 flex-1">
        <h3 className="text-foreground group-hover:text-brand mb-1.5 text-sm font-semibold transition-colors duration-200 @lg:text-base">
          {post.title}
        </h3>
        <p className="text-foreground-alt/60 line-clamp-1 hidden text-xs @md:block">
          {post.summary}
        </p>
      </div>
      <div className="pointer-events-none relative z-10 hidden shrink-0 items-center gap-1.5 @md:flex">
        {post.tags.map((tag) => (
          <TagChip key={tag} tag={tag} />
        ))}
      </div>
      <div className="pointer-events-none relative z-10 flex shrink-0 items-center gap-3">
        <time className="text-foreground-alt/50 text-xs tabular-nums">
          {post.date}
        </time>
        <LuArrowRight className="text-foreground-alt/30 group-hover:text-brand size-3.5 transition duration-200 group-hover:translate-x-0.5" />
      </div>
    </article>
  )
}

interface PostListProps {
  posts: BlogPost[]
}

export function PostList({ posts }: PostListProps) {
  return (
    <div className="border-foreground/6 bg-background-card/10 overflow-hidden rounded-xl border backdrop-blur-sm">
      {posts.map((post) => (
        <PostListItem key={post.slug} post={post} />
      ))}
    </div>
  )
}
