import { useMemo } from 'react'
import { BlogIndex } from './BlogIndex.js'
import { loadPosts } from './load-posts.js'

// BlogIndexRoute renders the blog index page, loading posts at runtime.
export function BlogIndexRoute() {
  const posts = useMemo(() => loadPosts(), [])
  return <BlogIndex posts={posts} />
}
