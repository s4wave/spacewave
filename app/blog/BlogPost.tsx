import { useEffect, useCallback } from 'react'
import { useNavigate } from '@s4wave/web/router/router.js'
import { LuArrowLeft, LuArrowRight } from 'react-icons/lu'
import { LuGithub } from 'react-icons/lu'
import { BlogLayout } from './BlogLayout.js'
import { BlogCta } from './BlogCta.js'
import { BlogMarkdown } from './BlogMarkdown.js'
import { TagChip } from './TagChip.js'
import { GITHUB_REPO_URL } from '@s4wave/app/github.js'
import type { BlogPost as BlogPostType } from './types.js'
import './blog-prose.css'

export interface BlogPostNavLink {
  title: string
  url: string
}

export interface BlogPostPageProps {
  post: BlogPostType
  prevPost?: BlogPostNavLink
  nextPost?: BlogPostNavLink
  // When set, renders pre-rendered HTML via dangerouslySetInnerHTML instead
  // of running markdown-to-jsx. Used during hydration from blog-data JSON.
  bodyHtml?: string
}

// BlogPostPage renders a single blog post in clean reading mode.
export function BlogPostPage({
  post,
  prevPost,
  nextPost,
  bodyHtml,
}: BlogPostPageProps) {
  const navigate = useNavigate()

  const navigatePrev = useCallback(() => {
    if (prevPost) navigate({ path: prevPost.url })
  }, [navigate, prevPost])

  const navigateNext = useCallback(() => {
    if (nextPost) navigate({ path: nextPost.url })
  }, [navigate, nextPost])

  // Inject JSON-LD structured data into document head.
  useEffect(() => {
    const script = document.createElement('script')
    script.type = 'application/ld+json'
    script.textContent = JSON.stringify({
      '@context': 'https://schema.org',
      '@type': 'Article',
      headline: post.title,
      datePublished: post.date,
      author: {
        '@type': 'Person',
        name: post.author.name,
        url: post.author.url,
      },
      description: post.summary,
    })
    document.head.appendChild(script)
    return () => {
      document.head.removeChild(script)
    }
  }, [post])

  return (
    <BlogLayout>
      <article className="relative z-10 mx-auto w-full max-w-3xl px-4 pt-6 pb-20 @lg:px-8 @lg:pt-10">
        {/* Post header */}
        <header className="mb-8">
          <div className="mb-3 flex flex-wrap items-center gap-x-3 gap-y-1">
            <time className="text-foreground-alt/50 text-xs tabular-nums">
              {post.date}
            </time>
            <span className="text-foreground-alt/20 text-xs">/</span>
            {post.tags.map((tag) => (
              <TagChip key={tag} tag={tag} />
            ))}
            <a
              href={`${GITHUB_REPO_URL}/blob/master/app/blog/posts/${post.date.slice(0, 4)}-${post.date.slice(5, 7)}-${post.date.slice(8, 10)}-${post.slug}.md`}
              target="_blank"
              rel="noopener noreferrer"
              className="text-foreground-alt/40 hover:text-foreground-alt ml-auto flex items-center gap-1.5 text-xs transition-colors"
              title="View source on GitHub"
            >
              <LuGithub className="h-3.5 w-3.5" />
            </a>
          </div>

          <h1 className="text-foreground mb-4 text-2xl leading-snug font-bold tracking-tight @lg:text-3xl @lg:leading-snug">
            {post.title}
          </h1>

          <div className="flex items-center gap-3">
            <img
              src={post.author.avatar}
              alt={post.author.name}
              className="border-foreground/10 h-7 w-7 rounded-full border"
              loading="lazy"
            />
            <a
              href={post.author.url}
              target="_blank"
              rel="noopener noreferrer"
              className="text-foreground-alt/70 text-xs font-medium hover:underline"
            >
              {post.author.name}
            </a>
          </div>
        </header>

        {/* Post body */}
        {bodyHtml ?
          <div
            className="blog-prose"
            dangerouslySetInnerHTML={{ __html: bodyHtml }}
          />
        : <div className="blog-prose">
            <BlogMarkdown>{post.body}</BlogMarkdown>
          </div>
        }

        <BlogCta />

        {/* Post navigation */}
        {(prevPost || nextPost) && (
          <nav className="mt-12 grid grid-cols-2 gap-4">
            {prevPost ?
              <button
                onClick={navigatePrev}
                className="border-foreground/6 hover:border-foreground/12 hover:bg-background-card/30 group flex cursor-pointer flex-col items-start gap-1.5 rounded-xl border p-5 text-left transition-all duration-200"
              >
                <span className="text-foreground-alt/50 flex items-center gap-1.5 text-xs">
                  <LuArrowLeft className="h-3 w-3 transition-transform duration-200 group-hover:-translate-x-0.5" />
                  Previous
                </span>
                <span className="text-foreground group-hover:text-brand text-sm font-medium transition-colors duration-200">
                  {prevPost.title}
                </span>
              </button>
            : <div />}

            {nextPost ?
              <button
                onClick={navigateNext}
                className="border-foreground/6 hover:border-foreground/12 hover:bg-background-card/30 group flex cursor-pointer flex-col items-end gap-1.5 rounded-xl border p-5 text-right transition-all duration-200"
              >
                <span className="text-foreground-alt/50 flex items-center gap-1.5 text-xs">
                  Next
                  <LuArrowRight className="h-3 w-3 transition-transform duration-200 group-hover:translate-x-0.5" />
                </span>
                <span className="text-foreground group-hover:text-brand text-sm font-medium transition-colors duration-200">
                  {nextPost.title}
                </span>
              </button>
            : <div />}
          </nav>
        )}
      </article>
    </BlogLayout>
  )
}
