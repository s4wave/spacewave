import { createElement } from 'react'
import {
  existsSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  writeFileSync,
} from 'fs'
import { join } from 'path'
import { createHighlighter } from 'shiki'

import { buildPageHtml } from '../prerender/html-template.js'
import { serializeJsonScriptData } from '../prerender/json-script.js'
import { prerenderElement, type PrerenderContext } from '../prerender/build.js'
import { BlogIndex, metadata as blogIndexMetadata } from './BlogIndex.js'
import { BlogPostPage } from './BlogPost.js'
import { BlogTagPage } from './BlogTagPage.js'
import { parseBlogPost } from './parse-blog-post.js'
import type { BlogPost } from './types.js'

const postsDir = join(process.cwd(), 'app/blog/posts')

function discoverPosts(includeDrafts: boolean): BlogPost[] {
  if (!existsSync(postsDir)) return []

  const files = readdirSync(postsDir, { recursive: true })
    .flatMap((f) => {
      const file = String(f)
      return file.endsWith('.md') ? [file] : []
    })
    .sort()

  const posts: BlogPost[] = []
  for (const file of files) {
    const filePath = join(postsDir, file)
    const raw = readFileSync(filePath, 'utf-8')
    const post = parseBlogPost(raw, file, console.warn)
    if (!post || (post.draft && !includeDrafts)) continue
    posts.push(post)
  }

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
    body: p.body,
  }
}

export async function buildBlog(
  ctx: PrerenderContext,
  includeDrafts = false,
): Promise<string[]> {
  ctx.log('[blog] === Blog Build ===')

  const posts = discoverPosts(includeDrafts)
  ctx.log(
    `[blog] Discovered ${posts.length} post(s)${includeDrafts ? ' (including drafts)' : ''}`,
  )

  if (posts.length === 0) {
    ctx.log('[blog] No posts found, skipping build.')
    return []
  }

  const highlighter = await createHighlighter({
    themes: ['vitesse-dark'],
    langs: [...SHIKI_LANGS],
  })
  for (let i = 0; i < posts.length; i++) {
    posts[i].body = highlightCodeBlocks(posts[i].body, highlighter)
  }
  highlighter.dispose()

  const tagSet = new Set<string>()
  const tagPostsByTag = new Map<string, BlogPost[]>()
  for (const post of posts) {
    for (const tag of post.tags) {
      tagSet.add(tag)
      const tagPosts = tagPostsByTag.get(tag)
      if (tagPosts) tagPosts.push(post)
      else tagPostsByTag.set(tag, [post])
    }
  }
  const allTags = Array.from(tagSet).sort()

  const postListForHydration = posts.map(postToHydrationMeta)

  ctx.log('[blog] Prerendering /blog...')
  const indexBody = await prerenderElement(
    createElement(BlogIndex, { posts }),
    '/blog',
  )
  const indexBlogData = serializeJsonScriptData({
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
    additionalCssUrls: ctx.hydrateCssUrls,
    iconUrl: ctx.iconUrl,
    importMap: ctx.importMap,
  })
  writeFileSync(join(ctx.outputDir, 'blog.html'), indexHtml)

  for (let i = 0; i < posts.length; i++) {
    const post = posts[i]
    const prevPost = posts[i + 1]
    const nextPost = posts[i - 1]

    ctx.log(`[blog] Prerendering ${post.url}...`)

    // Render serially so logs and writes follow the stable post order.
    // eslint-disable-next-line react-doctor/async-await-in-loop
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

    const meta = postToHydrationMeta(post)
    const blogData = {
      type: 'post' as const,
      ...meta,
      prev: prevPost ? { title: prevPost.title, url: prevPost.url } : null,
      next: nextPost ? { title: nextPost.title, url: nextPost.url } : null,
    }
    const blogDataTag = `<script type="application/json" id="blog-data">${serializeJsonScriptData(blogData)}</script>`

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
      additionalCssUrls: ctx.hydrateCssUrls,
      iconUrl: ctx.iconUrl,
      importMap: ctx.importMap,
    })

    const urlParts = post.url.split('/')
    const postDir = join(ctx.outputDir, ...urlParts.slice(1, -1))
    mkdirSync(postDir, { recursive: true })
    writeFileSync(
      join(postDir, `${urlParts[urlParts.length - 1]}.html`),
      postHtml,
    )
  }

  for (const tag of allTags) {
    const tagPosts = tagPostsByTag.get(tag) ?? []
    ctx.log(`[blog] Prerendering /blog/tag/${tag}...`)

    // Render serially so logs and writes follow the sorted tag order.
    // eslint-disable-next-line react-doctor/async-await-in-loop
    const tagBody = await prerenderElement(
      createElement(BlogTagPage, { tag, posts: tagPosts }),
      `/blog/tag/${tag}`,
    )
    const tagPostsForHydration = tagPosts.map(postToHydrationMeta)
    const tagBlogData = serializeJsonScriptData({
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
      additionalCssUrls: ctx.hydrateCssUrls,
      iconUrl: ctx.iconUrl,
      importMap: ctx.importMap,
    })

    const tagDir = join(ctx.outputDir, 'blog', 'tag')
    mkdirSync(tagDir, { recursive: true })
    writeFileSync(join(tagDir, `${tag}.html`), tagHtml)
  }

  const blogPaths = collectBlogPathsFromPosts(posts)

  ctx.log('[blog] === Blog Build Complete ===')
  return blogPaths
}
