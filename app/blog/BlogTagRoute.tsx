import { useMemo } from 'react'
import { useParams } from '@s4wave/web/router/router.js'
import { BlogTagPage } from './BlogTagPage.js'
import { loadPosts } from './load-posts.js'

// BlogTagRoute renders a tag page, loading posts filtered by the route tag param.
export function BlogTagRoute() {
  const params = useParams()
  const tag = params['tag'] ?? ''

  const posts = useMemo(() => loadPosts(), [])
  const tagPosts = useMemo(
    () => posts.filter((p) => p.tags.includes(tag)),
    [posts, tag],
  )

  return <BlogTagPage tag={tag} posts={tagPosts} />
}
