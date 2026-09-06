// Prerender Build Script
//
// Post-bldr injection step. Reads bldr dist manifest.json for asset URLs,
// prerenders each static page via React 19, injects bootstrap scripts,
// writes per-route HTML files.
//
// Usage: bun run app/prerender/build.ts [--dist-dir <path>] [--quiet]

import { createElement } from 'react'
import { prerender } from 'react-dom/static'
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'fs'
import { dirname, join, resolve } from 'path'

import { RouterProvider } from '@s4wave/web/router/router.js'
import {
  Landing,
  metadata as landingMetadata,
} from '@s4wave/app/landing/Landing.js'
import { SPACEWAVE_PUBLIC_BASE_URL } from '@s4wave/app/urls.js'

import { buildBlog, collectBlogPaths } from '../blog/blog-build.js'
import { buildBrowserReleaseDescriptor } from './browser-release.js'
import { buildBootstrapScript } from './bootstrap.js'
import { buildPageHtml } from './html-template.js'
import {
  collectRequiredStaticAssetUrls,
  preparePrerenderStaticAssets,
  selectViteEntryAssets,
  type ViteManifest,
} from './static-assets.js'
import { StaticProvider } from './StaticContext.js'
import { STATIC_ROUTES, type StaticRoute } from './static-pages.js'
import type { PageMetadata } from './metadata.js'
import {
  ROOT_BOOT_VISIBILITY_CSS,
  ROOT_BOOT_VISIBILITY_SCRIPT,
} from './root-loading-shell.js'
import { ROOT_LANDING_SHELL_CLASS } from './root-landing-shell.js'
import { buildStartupShell } from './startup-shell.js'

const SITE_ORIGIN = process.env.SITE_ORIGIN ?? SPACEWAVE_PUBLIC_BASE_URL

// When built as an SSR bundle, import.meta.url points to the output
// file. Use process.cwd() which is always the spacewave project root.
const projectRoot = process.cwd()
const prerenderDir = resolve(projectRoot, 'app/prerender')

function getDistDir(): string {
  const idx = process.argv.indexOf('--dist-dir')
  if (idx !== -1 && process.argv[idx + 1]) {
    return resolve(projectRoot, process.argv[idx + 1])
  }
  return join(projectRoot, '.bldr-dist/build/js/spacewave-browser/dist')
}

const DIST_DIR = getDistDir()
const OUTPUT_DIR = join(prerenderDir, 'dist')

interface BldrManifest {
  entrypoint: string
  entrypointDecompressedSize?: number
  serviceWorker: string
  sharedWorker: string
  opfsWorker?: string
  requiredStaticAssets?: string[]
  wasm?: string
  css: string[]
}

// PrerenderContext holds all shared infrastructure needed by both the
// static page pipeline and the blog pipeline.
export interface PrerenderContext {
  bldrManifest: BldrManifest
  browserGenerationId: string
  mainCssUrl: string
  hydrateCssUrls: string[]
  iconUrl: string
  importMap: string
  bootstrapScript: string
  hydrateScriptTag: string
  siteOrigin: string
  outputDir: string
  log: (msg: string) => void
}

async function streamToString(
  stream: ReadableStream<Uint8Array>,
): Promise<string> {
  const reader = stream.getReader()
  const chunks: Uint8Array[] = []
  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    if (value) chunks.push(value)
  }
  return Buffer.concat(chunks).toString('utf-8')
}

function noop() {}

// prerenderElement renders a React element wrapped in static/router
// providers to an HTML string.
export async function prerenderElement(
  element: React.ReactElement,
  path: string,
): Promise<string> {
  const child = createElement(StaticProvider, null, element)
  const props = { path, onNavigate: noop, children: child }
  const wrapped = createElement(RouterProvider, props)
  const { prelude } = await prerender(wrapped)
  return streamToString(prelude)
}

function validateMetadata(path: string, meta: PageMetadata) {
  if (!meta.title) {
    throw new Error(`[prerender] SEO error: missing title for ${path}`)
  }
  if (!meta.description) {
    throw new Error(`[prerender] SEO error: missing description for ${path}`)
  }
  if (!meta.canonicalPath && path !== '/') {
    throw new Error(`[prerender] SEO error: missing canonicalPath for ${path}`)
  }
  if (!meta.ogImage) {
    console.warn(`[prerender] SEO warning: missing ogImage for ${path}`)
  }
  if (meta.description.length < 120 || meta.description.length > 160) {
    console.warn(
      `[prerender] SEO warning: description length ${meta.description.length} outside 120-160 chars for ${path}`,
    )
  }
}

// buildPrerenderContext reads bldr manifest, extracts CSS, importmap,
// bootstrap script, and hydration script. Returns a PrerenderContext
// shared by both static page and blog pipelines.
export function buildPrerenderContext(
  verbose: boolean,
  blogPaths: string[],
): PrerenderContext {
  function log(message: string) {
    if (verbose) console.log(`[prerender] ${message}`)
  }

  // Read bldr manifest.
  const manifestPath = join(DIST_DIR, 'manifest.json')
  if (!existsSync(manifestPath)) {
    console.error(
      `manifest.json not found at ${manifestPath}. Run a bldr release build first.`,
    )
    process.exit(1)
  }
  const parsedManifest: unknown = JSON.parse(
    readFileSync(manifestPath, 'utf-8'),
  )
  const manifest = parsedManifest as BldrManifest
  log(
    `Read manifest: entrypoint=${manifest.entrypoint}, wasm=${manifest.wasm ?? ''}`,
  )

  // Extract importmap from bldr dist index.html.
  const distIndexPath = join(DIST_DIR, 'index.html')
  let importMap = ''
  if (existsSync(distIndexPath)) {
    const distHtml = readFileSync(distIndexPath, 'utf-8')
    const match = distHtml.match(
      /<script type="importmap">\s*([\s\S]*?)\s*<\/script>/,
    )
    if (match) {
      importMap = match[1].replaceAll('"./entrypoint/', '"/entrypoint/')
      log('Extracted importmap from dist index.html')
    }
  }
  if (!importMap) {
    console.error('importmap not found in dist index.html')
    process.exit(1)
  }

  // Resolve the hydration entry and all static page styles from the manifest
  // emitted by vite.hydrate.config.ts.
  const hydrateManifestPath = join(OUTPUT_DIR, '.vite/manifest.json')
  if (!existsSync(hydrateManifestPath)) {
    console.error(
      `Hydration manifest not found at ${hydrateManifestPath}. ` +
        'Run vite build --config app/prerender/vite.hydrate.config.ts first.',
    )
    process.exit(1)
  }
  const hydrateManifest = JSON.parse(
    readFileSync(hydrateManifestPath, 'utf-8'),
  ) as ViteManifest
  const hydrateAssets = selectViteEntryAssets(
    hydrateManifest,
    'app/prerender/hydrate.tsx',
  )
  if (!hydrateAssets) {
    console.error(
      `Hydration entry app/prerender/hydrate.tsx not found in ${hydrateManifestPath}`,
    )
    process.exit(1)
  }
  const staticAssets = preparePrerenderStaticAssets(
    OUTPUT_DIR,
    join(projectRoot, 'web/images'),
    hydrateAssets,
  )
  const hydrateScriptTag = `<script type="module" src="/static/${hydrateAssets.file}"></script>`
  log(`Hydration script: ${hydrateAssets.file}`)
  log(`Hydration styles: ${hydrateAssets.css.join(', ')}`)

  const browserRelease = buildBrowserReleaseDescriptor(
    {
      entrypoint: manifest.entrypoint,
      entrypointDecompressedSize: manifest.entrypointDecompressedSize,
      serviceWorker: manifest.serviceWorker,
      sharedWorker: manifest.sharedWorker,
      opfsWorker: manifest.opfsWorker,
      wasm: manifest.wasm,
      css: manifest.css,
    },
    ['/', ...STATIC_ROUTES.map((page) => page.path), ...blogPaths],
    [
      ...(manifest.requiredStaticAssets ?? []),
      ...collectRequiredStaticAssetUrls(OUTPUT_DIR),
    ],
  )
  const browserReleasePath = join(DIST_DIR, 'browser-release.json')
  writeFileSync(
    browserReleasePath,
    JSON.stringify(browserRelease, null, 2) + '\n',
  )
  log(`Generated ${browserReleasePath} (${browserRelease.generationId})`)

  // Use the stable boot asset as the only boot entry script.
  const bootstrapScript = buildBootstrapScript()

  return {
    bldrManifest: manifest,
    browserGenerationId: browserRelease.generationId,
    mainCssUrl: staticAssets.mainCssUrl,
    hydrateCssUrls: staticAssets.additionalCssUrls,
    iconUrl: staticAssets.iconUrl,
    importMap,
    bootstrapScript,
    hydrateScriptTag,
    siteOrigin: SITE_ORIGIN,
    outputDir: OUTPUT_DIR,
    log,
  }
}

async function main() {
  const args = process.argv.slice(2)
  const verbose = !args.includes('--quiet')
  const includeDrafts = args.includes('--include-drafts')

  console.log('[prerender] === Prerender Build ===')

  const blogPaths = collectBlogPaths(includeDrafts)
  const ctx = buildPrerenderContext(verbose, blogPaths)

  async function buildStaticPage(page: StaticRoute): Promise<void> {
    const Component = page.component
    const meta = { ...page.metadata }

    // For /landing, override canonicalPath to '/'
    if (page.path === '/landing') {
      meta.canonicalPath = '/'
    }

    validateMetadata(page.path, meta)

    ctx.log(`Prerendering ${page.path}...`)
    const body = await prerenderElement(createElement(Component), page.path)

    const canonicalUrl = meta.canonicalPath
      ? ctx.siteOrigin + meta.canonicalPath
      : undefined

    const pageHtml = buildPageHtml({
      body,
      title: meta.title,
      description: meta.description,
      canonicalUrl,
      ogImage: meta.ogImage,
      ogType: meta.ogType,
      twitterCard: meta.twitterCard,
      jsonLd: meta.jsonLd,
      bootstrapScript: ctx.bootstrapScript,
      hydrateScript: ctx.hydrateScriptTag,
      criticalCss: '',
      mainCssUrl: ctx.mainCssUrl,
      additionalCssUrls: ctx.hydrateCssUrls,
      iconUrl: ctx.iconUrl,
      importMap: ctx.importMap,
    })

    const filename = page.path.slice(1) + '.html'
    const outputPath = join(ctx.outputDir, filename)
    mkdirSync(dirname(outputPath), { recursive: true })
    writeFileSync(outputPath, pageHtml)
    ctx.log(`Wrote ${outputPath} (${pageHtml.length} bytes)`)
  }

  // Prerender each static page in route order so logs and failure ordering stay stable.
  let staticPageSequence = Promise.resolve()
  for (const page of STATIC_ROUTES) {
    staticPageSequence = staticPageSequence.then(() => buildStaticPage(page))
  }
  await staticPageSequence

  // Build root path special template.
  ctx.log('Building root template (/)...')
  await buildRootTemplate(ctx)

  // Build blog pages using the same prerender context.
  await buildBlog(ctx, includeDrafts)

  // Generate unified static-manifest.ts with all paths.
  // Maps URL paths to R2 keys for the CF Worker.
  const manifestEntries: Record<string, string> = {
    '/': 'static/index.html',
  }
  for (const page of STATIC_ROUTES) {
    manifestEntries[page.path] = `static/${page.path.slice(1)}.html`
  }
  for (const blogPath of blogPaths) {
    if (blogPath === '/blog') {
      manifestEntries['/blog'] = 'static/blog.html'
    } else {
      manifestEntries[blogPath] = `static${blogPath}.html`
    }
  }

  const manifestLines = Object.entries(manifestEntries)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([path, key]) => `  '${path}': '${key}',`)
    .join('\n')
  const manifestContent = `// Generated by app/prerender/build.ts. Do not edit.\n// Maps URL paths to R2 keys for pre-rendered HTML pages.\nexport const STATIC_MANIFEST: Record<string, string> = {\n${manifestLines}\n}\n`
  const manifestPath = join(ctx.outputDir, 'static-manifest.ts')
  writeFileSync(manifestPath, manifestContent)
  ctx.log(
    `Generated static-manifest.ts (${Object.keys(manifestEntries).length} paths)`,
  )

  // Generate sitemap.xml from all static paths.
  const sitemapUrls = Object.keys(manifestEntries)
    .sort()
    .map((path) => {
      let priority = '0.5'
      if (path === '/') priority = '1.0'
      else if (path === '/landing' || path === '/pricing') priority = '0.8'
      else if (path === '/blog' || path.startsWith('/blog/tag/'))
        priority = '0.7'
      else if (path.startsWith('/blog/')) priority = '0.6'
      else if (path.startsWith('/landing/')) priority = '0.5'
      else if (path === '/tos' || path === '/privacy' || path === '/dmca')
        priority = '0.3'
      return `  <url>\n    <loc>${ctx.siteOrigin}${path}</loc>\n    <priority>${priority}</priority>\n  </url>`
    })
    .join('\n')
  const sitemapXml = `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n${sitemapUrls}\n</urlset>\n`
  const sitemapPath = join(ctx.outputDir, 'sitemap.xml')
  writeFileSync(sitemapPath, sitemapXml)
  ctx.log(`Generated sitemap.xml (${Object.keys(manifestEntries).length} URLs)`)

  console.log('[prerender] === Prerender Build Complete ===')
}

// buildRootTemplate builds the root document with the landing and loading
// shells needed before the application starts.
async function buildRootTemplate(ctx: PrerenderContext) {
  const landingHtml = await prerenderElement(createElement(Landing), '/')

  const canonicalUrl = ctx.siteOrigin + '/'

  const body = `<div id="sw-landing" class="${ROOT_LANDING_SHELL_CLASS}">${landingHtml}</div>${buildStartupShell(ctx.iconUrl)}`

  const rootHtml = buildPageHtml({
    body,
    title: landingMetadata.title,
    description: landingMetadata.description,
    canonicalUrl,
    ogImage: landingMetadata.ogImage,
    jsonLd: landingMetadata.jsonLd,
    bootstrapScript: ctx.bootstrapScript,
    headScript: ROOT_BOOT_VISIBILITY_SCRIPT,
    hydrateScript: ctx.hydrateScriptTag,
    criticalCss: ROOT_BOOT_VISIBILITY_CSS,
    mainCssUrl: ctx.mainCssUrl,
    additionalCssUrls: ctx.hydrateCssUrls,
    iconUrl: ctx.iconUrl,
    importMap: ctx.importMap,
  })

  const outputPath = join(ctx.outputDir, 'index.html')
  writeFileSync(outputPath, rootHtml)
  console.log(`[prerender] Wrote ${outputPath} (${rootHtml.length} bytes)`)
}

void main()
