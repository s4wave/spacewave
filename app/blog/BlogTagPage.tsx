import { useCallback } from 'react'
import { useNavigate } from '@s4wave/web/router/router.js'
import { LuArrowLeft } from 'react-icons/lu'
import { ShootingStars } from '@s4wave/web/ui/shooting-stars.js'
import { useIsStaticMode } from '@s4wave/app/prerender/StaticContext.js'
import { LegalFooter } from '@s4wave/app/landing/LegalFooter.js'
import { PostList } from './PostList.js'
import type { BlogPost } from './types.js'

// BlogTagPageProps defines the props for BlogTagPage.
export interface BlogTagPageProps {
  tag: string
  posts: BlogPost[]
}

// BlogTagPage renders a filtered listing of posts for a specific tag.
export function BlogTagPage({ tag, posts }: BlogTagPageProps) {
  const isStatic = useIsStaticMode()
  const navigate = useNavigate()

  const navigateBlog = useCallback(() => {
    navigate({ path: '/blog' })
  }, [navigate])

  return (
    <div className="bg-background-landing @container flex w-full flex-1 flex-col overflow-y-auto">
      {!isStatic && (
        <ShootingStars className="pointer-events-none fixed inset-0 opacity-40" />
      )}

      <div className="relative z-10 mx-auto w-full max-w-5xl px-4 pt-6 @lg:px-8">
        <button
          onClick={navigateBlog}
          className="text-foreground-alt/60 hover:text-foreground flex cursor-pointer items-center gap-2 text-xs font-medium transition-colors"
        >
          <LuArrowLeft className="h-3.5 w-3.5" />
          All posts
        </button>
      </div>

      <header className="relative z-10 mx-auto w-full max-w-5xl px-4 pt-16 pb-6 text-center @lg:px-8 @lg:pt-20 @lg:pb-8">
        <div className="text-brand/80 border-brand/20 bg-brand/5 mb-5 inline-block rounded-md border px-3 py-1 text-xs font-medium">
          {tag}
        </div>
        <h1 className="text-foreground mb-4 text-3xl font-bold tracking-tight @lg:text-4xl">
          Posts tagged "{tag}"
        </h1>
        <p className="text-foreground-alt/60 text-sm">
          {posts.length} {posts.length === 1 ? 'post' : 'posts'}
        </p>
      </header>

      <div className="via-foreground/8 relative z-10 mx-auto h-px w-full max-w-5xl bg-gradient-to-r from-transparent to-transparent" />

      <main className="relative z-10 mx-auto w-full max-w-5xl px-4 pt-10 pb-20 @lg:px-8">
        {posts.length > 0 ?
          <PostList posts={posts} />
        : <div className="border-foreground/6 rounded-xl border border-dashed px-8 py-20 text-center">
            <p className="text-foreground-alt text-sm">
              No posts with this tag.
            </p>
          </div>
        }
      </main>

      <LegalFooter />
    </div>
  )
}
