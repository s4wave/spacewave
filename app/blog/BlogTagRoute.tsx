import { useMemo } from 'react'

import { useParams } from '@s4wave/web/router/router.js'

import { BlogTagPage } from './BlogTagPage.js'
import { loadPosts } from './load-posts.js'

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
