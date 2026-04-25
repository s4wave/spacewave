import { useCallback, useMemo, useState } from 'react'

import { cn } from '@s4wave/web/style/utils.js'
import { LuX } from 'react-icons/lu'

import { HeroCard } from './HeroCard.js'
import { PostList } from './PostList.js'
import { BlogPostView } from './BlogPostView.js'
import type { BlogPostData } from './types.js'
import type { AuthorRegistry } from './authors.js'

// BlogReadingViewProps defines the props for BlogReadingView.
interface BlogReadingViewProps {
  posts: BlogPostData[]
  selectedPost: BlogPostData | null
  onSelectPost: (post: BlogPostData | null) => void
  authorRegistry: AuthorRegistry
}

// BlogReadingView renders the blog reading experience with index and post modes.
export function BlogReadingView({
  posts,
  selectedPost,
  onSelectPost,
  authorRegistry,
}: BlogReadingViewProps) {
  const [tagFilter, setTagFilter] = useState<string | null>(null)

  // Filter out drafts and posts without dates, sort by date descending.
  const publishedPosts = useMemo(() => {
    let filtered = posts.filter((p) => p.date && !p.draft)
    if (tagFilter) {
      filtered = filtered.filter((p) =>
        p.tags.some((t) => t.toLowerCase() === tagFilter.toLowerCase()),
      )
    }
    return filtered.sort((a, b) => b.date.localeCompare(a.date))
  }, [posts, tagFilter])

  // Collect all unique tags.
  const allTags = useMemo(() => {
    const tags = new Set<string>()
    for (const post of posts) {
      if (post.date && !post.draft) {
        for (const tag of post.tags) {
          tags.add(tag)
        }
      }
    }
    return Array.from(tags).sort()
  }, [posts])

  const handleSelectTag = useCallback(
    (tag: string) => {
      setTagFilter((prev) => (prev === tag ? null : tag))
    },
    [],
  )

  const handleClearFilter = useCallback(() => {
    setTagFilter(null)
  }, [])

  const handleBack = useCallback(() => {
    onSelectPost(null)
  }, [onSelectPost])

  // Post view mode: show selected post.
  if (selectedPost) {
    const idx = publishedPosts.findIndex((p) => p.name === selectedPost.name)
    const prevPost = idx < publishedPosts.length - 1
      ? publishedPosts[idx + 1]
      : undefined
    const nextPost = idx > 0 ? publishedPosts[idx - 1] : undefined

    return (
      <div className="h-full overflow-y-auto">
        <BlogPostView
          post={selectedPost}
          prevPost={prevPost}
          nextPost={nextPost}
          onSelectPost={onSelectPost}
          onSelectTag={handleSelectTag}
          onBack={handleBack}
          authorRegistry={authorRegistry}
        />
      </div>
    )
  }

  // Index mode: hero card + post list.
  const heroPost = publishedPosts[0]
  const remainingPosts = publishedPosts.slice(1)

  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto w-full max-w-3xl px-4 py-6 @lg:px-8 @lg:py-10">
        {/* Tag filter bar */}
        {allTags.length > 0 && (
          <div className="mb-6 flex flex-wrap items-center gap-2">
            {allTags.map((tag) => (
              <button
                key={tag}
                onClick={() => handleSelectTag(tag)}
                className={cn(
                  'cursor-pointer rounded-md border px-2 py-0.5 text-xs font-medium transition-all duration-200',
                  tagFilter === tag
                    ? 'border-brand/40 bg-brand/10 text-brand'
                    : 'text-foreground-alt/70 hover:text-brand hover:border-brand/30 hover:bg-brand/5 border-white/8',
                )}
              >
                {tag}
              </button>
            ))}
            {tagFilter && (
              <button
                onClick={handleClearFilter}
                className="text-foreground-alt/50 hover:text-foreground flex items-center gap-1 text-xs transition-colors"
              >
                <LuX className="h-3 w-3" />
                Clear
              </button>
            )}
          </div>
        )}

        {publishedPosts.length === 0 ?
          <div className="text-foreground-alt/50 flex items-center justify-center py-20 text-sm">
            {tagFilter ? 'No posts match this tag' : 'No published posts yet'}
          </div>
        : <>
            {/* Hero card for latest post */}
            {heroPost && (
              <div className="mb-8">
                <HeroCard
                  post={heroPost}
                  onSelectPost={onSelectPost}
                  onSelectTag={handleSelectTag}
                  authorRegistry={authorRegistry}
                />
              </div>
            )}

            {/* Remaining posts */}
            {remainingPosts.length > 0 && (
              <PostList
                posts={remainingPosts}
                onSelectPost={onSelectPost}
              />
            )}
          </>
        }
      </div>
    </div>
  )
}
