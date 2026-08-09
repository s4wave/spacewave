import { useMemo } from 'react'
import { BlogIndex } from './BlogIndex.js'
import { loadPosts } from './load-posts.js'

export function BlogIndexRoute() {
  const posts = useMemo(() => loadPosts(), [])
  return <BlogIndex posts={posts} />
}
