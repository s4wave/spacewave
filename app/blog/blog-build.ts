// Blog Build Script
//
// Discovers .md files in app/blog/posts/, parses frontmatter,
// compiles markdown, prerenders blog pages.
//
// Called from build.ts via buildBlog(ctx) with shared PrerenderContext,
// or standalone via: bun run app/blog/blog-build.ts [--dist-dir <path>] [--include-drafts]

import { createElement } from 'react'
import {
  existsSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  writeFileSync,
} from 'fs'
import { join } from 'path'
import matter from 'gray-matter'
import { createHighlighter } from 'shiki'

import { buildPageHtml } from '../prerender/html-template.js'
import { prerenderElement, type PrerenderContext } from '../prerender/build.js'
import { authors } from './authors.js'
import { BlogIndex, metadata as blogIndexMetadata } from './BlogIndex.js'
import { BlogMarkdown, blogMarkdownOptions } from './BlogMarkdown.js'
import { BlogPostPage } from './BlogPost.js'
import { BlogTagPage } from './BlogTagPage.js'
import type { BlogPost, BlogPostFrontmatter } from './types.js'

const postsDir = join(process.cwd(), 'app/blog/posts')

function discoverPosts(includeDrafts: boolean): BlogPost[] {
  if (!existsSync(postsDir)) return []

  const files = readdirSync(postsDir, { recursive: true })
    .map(String)
    .filter((f) => f.endsWith('.md'))
    .sort()

  const posts: BlogPost[] = []
  for (const file of files) {
    const filePath = join(postsDir, file)
    const raw = readFileSync(filePath, 'utf-8')
    const { data, content } = matter(raw)
    const fm = data as BlogPostFrontmatter

    if (!fm.title || !fm.date || !fm.author || !fm.summary) {
      console.warn(`[blog] Skipping ${file}: missing required frontmatter`)
      continue
    }

    const isDraft = fm.draft === true
    if (isDraft && !includeDrafts) continue

    // Derive slug from filename: 2026-03-20-hello-world.md -> hello-world
    const basename = file.replace(/\.md$/, '')
    const slug = basename.replace(/^\d{4}-\d{2}-\d{2}-/, '')

    // Normalize date to YYYY-MM-DD string (gray-matter may parse as Date).
    const rawDate = fm.date as unknown
    const dateStr =
      rawDate instanceof Date ?
        rawDate.toISOString().slice(0, 10)
      : String(rawDate)

    // Derive URL: /blog/YYYY/MM/slug
    const dateParts = dateStr.split('-')
    const year = dateParts[0]
    const month = dateParts[1]
    const url = `/blog/${year}/${month}/${slug}`

    const author = authors[fm.author]
    if (!author) {
      console.warn(
        `[blog] Unknown author "${fm.author}" in ${file}, using fallback`,
      )
    }

    posts.push({
      slug,
      url,
      title: fm.title,
      date: dateStr,
      author: author ?? {
        name: fm.author,
        avatar: '',
        url: '',
        bio: '',
      },
      authorSlug: fm.author,
      summary: fm.summary,
      tags: fm.tags ?? [],
      draft: isDraft,
      ogImage: fm.ogImage,
      body: content,
    })
  }

  // Sort by date descending.
  posts.sort((a, b) => b.date.localeCompare(a.date))
  return posts
}

function collectBlogPathsFromPosts(posts: BlogPost[]): string[] {
  if (posts.length === 0) {
    return []
  }

  const tagSet = new Set<string>()
  for (const post of posts) {
    for (const tag of post.tags) {
      tagSet.add(tag)
    }
  }
  const allTags = Array.from(tagSet).sort()

  const blogPaths = ['/blog']
  for (const post of posts) {
    blogPaths.push(post.url)
  }
  for (const tag of allTags) {
    blogPaths.push(`/blog/tag/${tag}`)
  }
  return blogPaths
}

// collectBlogPaths discovers the blog route inventory without rendering pages.
export function collectBlogPaths(includeDrafts = false): string[] {
  return collectBlogPathsFromPosts(discoverPosts(includeDrafts))
}

const SHIKI_LANGS = [
  'typescript',
  'javascript',
  'go',
  'bash',
  'json',
  'yaml',
  'html',
  'css',
  'markdown',
  'proto',
  'toml',
  'shell',
] as const

function highlightCodeBlocks(
  content: string,
  highlighter: Awaited<ReturnType<typeof createHighlighter>>,
): string {
  const codeBlockRegex = /```(\w+)?\n([\s\S]*?)```/g
  return content.replace(
    codeBlockRegex,
    (_match: string, lang: string | undefined, code: string) => {
      const language = lang || 'text'
      const trimmedCode = code.replace(/\n$/, '')
      try {
        const highlighted = highlighter.codeToHtml(trimmedCode, {
          lang: language,
          theme: 'vitesse-dark',
        })
        return `<div dangerouslySetInnerHTML="true">${highlighted}</div>`
      } catch {
        return `\`\`\`${language}\n${code}\`\`\``
      }
    },
  )
}

// postToHydrationMeta strips the body and normalizes a BlogPost for
// serialization into the blog-data hydration JSON.
function postToHydrationMeta(p: BlogPost) {
  return {
    slug: p.slug,
    url: p.url,
    title: p.title,
    date: p.date,
    author: { name: p.author.name, avatar: p.author.avatar, url: p.author.url },
    authorSlug: p.authorSlug,
    summary: p.summary,
    tags: p.tags,
    draft: p.draft,
    ogImage: p.ogImage,
    body: '',
  }
}

// buildBlog prerenders all blog pages using the shared PrerenderContext.
// Returns the list of URL paths for all generated blog pages.
export async function buildBlog(
  ctx: PrerenderContext,
  includeDrafts = false,
): Promise<string[]> {
  ctx.log('[blog] === Blog Build ===')

  // Discover posts.
  const posts = discoverPosts(includeDrafts)
  ctx.log(
    `[blog] Discovered ${posts.length} post(s)${includeDrafts ? ' (including drafts)' : ''}`,
  )

  if (posts.length === 0) {
    ctx.log('[blog] No posts found, skipping build.')
    return []
  }

  // Highlight code blocks in all posts with a shared highlighter.
  const highlighter = await createHighlighter({
    themes: ['vitesse-dark'],
    langs: [...SHIKI_LANGS],
  })
  for (let i = 0; i < posts.length; i++) {
    posts[i].body = highlightCodeBlocks(posts[i].body, highlighter)
  }
  highlighter.dispose()

  // Collect all unique tags.
  const tagSet = new Set<string>()
  for (const post of posts) {
    for (const tag of post.tags) {
      tagSet.add(tag)
    }
  }
  const allTags = Array.from(tagSet).sort()

  const postListForHydration = posts.map(postToHydrationMeta)

  // Prerender blog index.
  ctx.log('[blog] Prerendering /blog...')
  const indexBody = await prerenderElement(
    createElement(BlogIndex, { posts }),
    '/blog',
  )
  const indexBlogData = JSON.stringify({
    type: 'index',
    posts: postListForHydration,
  })
  const indexBlogDataTag = `<script type="application/json" id="blog-data">${indexBlogData}</script>`
  const indexHtml = buildPageHtml({
    body: indexBody + indexBlogDataTag,
    title: blogIndexMetadata.title,
    description: blogIndexMetadata.description,
    bootstrapScript: ctx.bootstrapScript,
    hydrateScript: ctx.hydrateScriptTag,
    criticalCss: '',
    mainCssUrl: ctx.mainCssUrl,
    iconUrl: ctx.iconUrl,
    importMap: ctx.importMap,
  })
  writeFileSync(join(ctx.outputDir, 'blog.html'), indexHtml)

  // Prerender each blog post.
  for (let i = 0; i < posts.length; i++) {
    const post = posts[i]
    const prevPost = posts[i + 1]
    const nextPost = posts[i - 1]

    ctx.log(`[blog] Prerendering ${post.url}...`)

    const postBody = await prerenderElement(
      createElement(BlogPostPage, { post, prevPost, nextPost }),
      post.url,
    )

    const jsonLd = {
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
    }

    // Prerender just the markdown body to HTML so the hydration bundle
    // does not need to bundle markdown-to-jsx or shiki.
    const renderedBody = await prerenderElement(
      createElement(BlogMarkdown, null, post.body),
      post.url,
    )

    const meta = postToHydrationMeta(post)
    const blogData = {
      type: 'post' as const,
      ...meta,
      bodyHtml: renderedBody,
      prev: prevPost ? { title: prevPost.title, url: prevPost.url } : null,
      next: nextPost ? { title: nextPost.title, url: nextPost.url } : null,
    }
    const blogDataTag = `<script type="application/json" id="blog-data">${JSON.stringify(blogData)}</script>`

    const postHtml = buildPageHtml({
      body: postBody + blogDataTag,
      title: `${post.title} - Spacewave Blog`,
      description: post.summary,
      ogImage: post.ogImage,
      jsonLd,
      bootstrapScript: ctx.bootstrapScript,
      hydrateScript: ctx.hydrateScriptTag,
      criticalCss: '',
      mainCssUrl: ctx.mainCssUrl,
      iconUrl: ctx.iconUrl,
      importMap: ctx.importMap,
    })

    // Write to dist/blog/YYYY/MM/slug.html
    const urlParts = post.url.split('/')
    const postDir = join(ctx.outputDir, ...urlParts.slice(1, -1))
    mkdirSync(postDir, { recursive: true })
    writeFileSync(
      join(postDir, `${urlParts[urlParts.length - 1]}.html`),
      postHtml,
    )
  }

  // Prerender tag pages.
  for (const tag of allTags) {
    const tagPosts = posts.filter((p) => p.tags.includes(tag))
    ctx.log(`[blog] Prerendering /blog/tag/${tag}...`)

    const tagBody = await prerenderElement(
      createElement(BlogTagPage, { tag, posts: tagPosts }),
      `/blog/tag/${tag}`,
    )
    const tagPostsForHydration = tagPosts.map(postToHydrationMeta)
    const tagBlogData = JSON.stringify({
      type: 'tag',
      tag,
      posts: tagPostsForHydration,
    })
    const tagBlogDataTag = `<script type="application/json" id="blog-data">${tagBlogData}</script>`
    const tagHtml = buildPageHtml({
      body: tagBody + tagBlogDataTag,
      title: `"${tag}" posts - Spacewave Blog`,
      description: `Blog posts tagged "${tag}".`,
      bootstrapScript: ctx.bootstrapScript,
      hydrateScript: ctx.hydrateScriptTag,
      criticalCss: '',
      mainCssUrl: ctx.mainCssUrl,
      iconUrl: ctx.iconUrl,
      importMap: ctx.importMap,
    })

    const tagDir = join(ctx.outputDir, 'blog', 'tag')
    mkdirSync(tagDir, { recursive: true })
    writeFileSync(join(tagDir, `${tag}.html`), tagHtml)
  }

  // Collect all blog paths for the unified manifest.
  const blogPaths = collectBlogPathsFromPosts(posts)

  ctx.log('[blog] === Blog Build Complete ===')
  return blogPaths
}
