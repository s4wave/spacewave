import { Route } from '@s4wave/web/router/router.js'

import { BlogIndexRoute } from '@s4wave/app/blog/BlogIndexRoute.js'
import { BlogPostRoute } from '@s4wave/app/blog/BlogPostRoute.js'
import { BlogTagRoute } from '@s4wave/app/blog/BlogTagRoute.js'

// BlogRoutes contains routes for blog pages.
export const BlogRoutes = (
  <>
    <Route path="/blog">
      <BlogIndexRoute />
    </Route>
    <Route path="/blog/:year/:month/:slug">
      <BlogPostRoute />
    </Route>
    <Route path="/blog/tag/:tag">
      <BlogTagRoute />
    </Route>
  </>
)
