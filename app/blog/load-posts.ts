import { parseBlogPost } from './parse-blog-post.js'
import type { BlogPost } from './types.js'

const postModules = import.meta.glob('./posts/*.md', {
  query: '?raw',
  eager: true,
  import: 'default',
})

let cachedPosts: BlogPost[] | null = null

export function loadPosts(): BlogPost[] {
  if (cachedPosts) return cachedPosts

  const posts = Object.entries(postModules).flatMap(([path, raw]) => {
    const post = parseBlogPost(raw as string, path)
    return post && !post.draft ? [post] : []
  })

  posts.sort((a, b) => b.date.localeCompare(a.date))
  cachedPosts = posts
  return posts
}
