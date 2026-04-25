import { useCallback } from 'react'
import { LuArrowLeft } from 'react-icons/lu'
import { ShootingStars } from '@s4wave/web/ui/shooting-stars.js'
import { useIsStaticMode } from '@s4wave/app/prerender/StaticContext.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { LegalFooter } from '@s4wave/app/landing/LegalFooter.js'
import { HeroCard } from './HeroCard.js'
import { PostList } from './PostList.js'
import { TagChip } from './TagChip.js'
import type { BlogPost } from './types.js'

// metadata exports SEO metadata for prerender.
export const metadata = {
  title: 'Blog - Spacewave',
  description:
    'Development updates, release announcements, and technical deep dives from the Spacewave team.',
}

// BlogIndexProps defines the props for BlogIndex.
export interface BlogIndexProps {
  posts: BlogPost[]
}

// BlogIndex renders the /blog listing page with ShootingStars background.
export function BlogIndex({ posts }: BlogIndexProps) {
  const navigate = useNavigate()
  const isStatic = useIsStaticMode()
  const goHome = useCallback(() => navigate({ path: '/' }), [navigate])
  const latest = posts[0]
  const rest = posts.slice(1)

  const allTags = [...new Set(posts.flatMap((p) => p.tags))].sort()

  return (
    <div className="bg-background-landing @container flex w-full flex-1 flex-col overflow-y-auto">
      {!isStatic && (
        <ShootingStars className="pointer-events-none fixed inset-0 opacity-40" />
      )}

      {/* Back to home */}
      <button
        onClick={goHome}
        className="text-foreground-alt hover:text-brand absolute top-4 left-4 z-20 flex items-center gap-2 text-sm transition-colors"
      >
        <LuArrowLeft className="h-4 w-4" />
        <span className="select-none">Back to home</span>
      </button>

      {/* Hero header */}
      <header className="relative z-10 mx-auto w-full max-w-5xl px-4 pt-24 pb-6 @lg:px-8 @lg:pt-32 @lg:pb-8">
        <h1 className="text-foreground mb-5 text-center text-4xl font-bold tracking-tight @lg:text-5xl">
          Spacewave Blog
        </h1>
        <p className="text-foreground-alt mx-auto max-w-xl text-center text-sm leading-relaxed font-light @lg:text-base">
          Development updates, release announcements, and technical deep dives.
        </p>

        {/* Tag bar */}
        <div className="mt-8 flex flex-wrap items-center justify-center gap-2">
          {allTags.map((tag) => (
            <TagChip key={tag} tag={tag} />
          ))}
        </div>
      </header>

      {/* Gradient separator */}
      <div className="via-foreground/8 relative z-10 mx-auto h-px w-full max-w-5xl bg-gradient-to-r from-transparent to-transparent" />

      <main className="relative z-10 mx-auto w-full max-w-5xl px-4 pt-10 pb-20 @lg:px-8">
        {/* Featured post */}
        {latest && (
          <section className="mb-14">
            <HeroCard post={latest} />
          </section>
        )}

        {/* Post list */}
        {rest.length > 0 && (
          <section>
            <div className="mb-6 flex items-center gap-3">
              <h2 className="text-foreground text-sm font-semibold tracking-wide uppercase">
                More posts
              </h2>
              <div className="via-foreground/8 h-px flex-1 bg-gradient-to-r from-transparent to-transparent" />
            </div>
            <PostList posts={rest} />
          </section>
        )}

        {posts.length === 0 && (
          <div className="border-foreground/6 rounded-xl border border-dashed px-8 py-20 text-center">
            <p className="text-foreground-alt text-sm">
              No posts yet. Check back soon.
            </p>
          </div>
        )}
      </main>

      <LegalFooter />
    </div>
  )
}
