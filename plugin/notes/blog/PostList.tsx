import { useCallback } from 'react'

import { LuArrowRight } from 'react-icons/lu'

import { TagChip } from './TagChip.js'
import type { BlogPostData } from './types.js'

// PostListItemProps defines the props for PostListItem.
interface PostListItemProps {
  post: BlogPostData
  onSelectPost: (post: BlogPostData) => void
}

// PostListItem renders a compact post row with title, date, and tags.
function PostListItem({ post, onSelectPost }: PostListItemProps) {
  const handleClick = useCallback(() => {
    onSelectPost(post)
  }, [onSelectPost, post])

  return (
    <article
      onClick={handleClick}
      className="border-foreground/6 hover:bg-background-card/30 group flex cursor-pointer items-start gap-5 border-b px-5 py-5 transition-all duration-200 last:border-b-0 @lg:items-center"
    >
      <div className="min-w-0 flex-1">
        <h3 className="text-foreground group-hover:text-brand mb-1.5 text-sm font-semibold transition-colors duration-200 @lg:text-base">
          {post.title}
        </h3>
        {post.summary && (
          <p className="text-foreground-alt/60 line-clamp-1 hidden text-xs @md:block">
            {post.summary}
          </p>
        )}
      </div>
      <div className="hidden shrink-0 items-center gap-1.5 @md:flex">
        {post.tags.map((tag) => (
          <TagChip key={tag} tag={tag} />
        ))}
      </div>
      <div className="flex shrink-0 items-center gap-3">
        <time className="text-foreground-alt/50 text-xs tabular-nums">
          {post.date}
        </time>
        <LuArrowRight className="text-foreground-alt/30 group-hover:text-brand h-3.5 w-3.5 transition-all duration-200 group-hover:translate-x-0.5" />
      </div>
    </article>
  )
}

// PostListProps defines the props for PostList.
interface PostListProps {
  posts: BlogPostData[]
  onSelectPost: (post: BlogPostData) => void
}

// PostList renders a compact list of blog posts.
export function PostList({ posts, onSelectPost }: PostListProps) {
  return (
    <div className="border-foreground/6 bg-background-card/10 overflow-hidden rounded-xl border backdrop-blur-sm">
      {posts.map((post) => (
        <PostListItem
          key={post.name}
          post={post}
          onSelectPost={onSelectPost}
        />
      ))}
    </div>
  )
}
