import { useMemo } from 'react'
import { useParams } from '@s4wave/web/router/router.js'
import { BlogPostPage } from './BlogPost.js'
import { loadPosts } from './load-posts.js'
import { NavigatePath } from '@s4wave/web/router/NavigatePath.js'

// BlogPostRoute renders a single blog post, loading it by route params.
export function BlogPostRoute() {
  const params = useParams()
  const year = params['year']
  const month = params['month']
  const slug = params['slug']
  const url = `/blog/${year}/${month}/${slug}`

  const posts = useMemo(() => loadPosts(), [])
  const postIndex = posts.findIndex((p) => p.url === url)

  if (postIndex === -1) {
    return <NavigatePath to="/blog" replace />
  }

  const post = posts[postIndex]
  const prevPost = posts[postIndex + 1]
  const nextPost = posts[postIndex - 1]

  return <BlogPostPage post={post} prevPost={prevPost} nextPost={nextPost} />
}
