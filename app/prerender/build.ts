// Prerender Build Script
//
// Post-bldr injection step. Reads bldr dist manifest.json for asset URLs,
// prerenders each static page via React 19, injects bootstrap scripts,
// writes per-route HTML files.
//
// Usage: bun run app/prerender/build.ts [--dist-dir <path>] [--quiet]

import { createElement } from 'react'
import { prerender } from 'react-dom/static'
import {
  copyFileSync,
  existsSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  writeFileSync,
} from 'fs'
import { dirname, join, resolve } from 'path'

import { RouterProvider } from '@s4wave/web/router/router.js'
import {
  Landing,
  metadata as landingMetadata,
} from '@s4wave/app/landing/Landing.js'

import { buildBlog, collectBlogPaths } from '../blog/blog-build.js'
import { buildBrowserReleaseDescriptor } from './browser-release.js'
import { buildBootstrapScript } from './bootstrap.js'
import { buildPageHtml } from './html-template.js'
import { collectRequiredStaticAssetUrls } from './static-assets.js'
import { StaticProvider } from './StaticContext.js'
import { STATIC_PAGES } from './static-pages.js'
import { getMetadata, type PageMetadata } from './metadata.js'

const SITE_ORIGIN = process.env.SITE_ORIGIN ?? 'https://spacewave.app'

// When built as an SSR bundle, import.meta.url points to the output
// file. Use process.cwd() which is always the spacewave project root.
const projectRoot = process.cwd()
const prerenderDir = resolve(projectRoot, 'app/prerender')

function getDistDir(): string {
  const idx = process.argv.indexOf('--dist-dir')
  if (idx !== -1 && process.argv[idx + 1]) {
    return resolve(projectRoot, process.argv[idx + 1])
  }
  return join(
    projectRoot,
    '.bldr-dist/build/desktop/js/wasm/spacewave-dist/dist',
  )
}

const DIST_DIR = getDistDir()
const OUTPUT_DIR = join(prerenderDir, 'dist')

interface BldrManifest {
  entrypoint: string
  serviceWorker: string
  sharedWorker: string
  wasm: string
  css: string[]
}

// PrerenderContext holds all shared infrastructure needed by both the
// static page pipeline and the blog pipeline.
export interface PrerenderContext {
  bldrManifest: BldrManifest
  browserGenerationId: string
  mainCssUrl: string
  iconUrl: string
  importMap: string
  bootstrapScript: string
  hydrateScriptTag: string
  siteOrigin: string
  outputDir: string
  log: (msg: string) => void
}

export async function streamToString(
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

export function noop() {}

// prerenderElement renders a React element wrapped in static/router
// providers to an HTML string.
export async function prerenderElement(
  element: React.ReactElement,
  path: string,
): Promise<string> {
  const child = createElement(StaticProvider, null, element)
  const wrapped = createElement(RouterProvider, {
    path,
    onNavigate: noop,
    children: child,
  })
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

// VITE_ASSET_PREFIX is the bldr plugin asset URL prefix used in Vite CSS output.
const VITE_ASSET_PREFIX = '/b/pa/spacewave-app/v/b/fe/'
// STATIC_ASSET_PREFIX is where we serve these assets from R2.
const STATIC_ASSET_PREFIX = '/static/assets/'

// extractViteCss finds the processed CSS from the spacewave-app Vite build,
// rewrites asset URLs to the static serving path, copies CSS and referenced
// assets (fonts, images) to the prerender output dir, and returns the URL
// path to reference in prerendered HTML.
function extractViteCss(log: (msg: string) => void): {
  cssUrl: string
  iconUrl: string
} {
  // Walk up from DIST_DIR (spacewave-dist/dist) to the js build root
  const jsBuildRoot = resolve(DIST_DIR, '../..')
  const viteAssetsDir = join(jsBuildRoot, 'spacewave-app/sub/vite/assets/b/fe')
  const viteManifestPath = join(viteAssetsDir, '.vite/manifest.json')
  if (!existsSync(viteManifestPath)) {
    console.error(`Vite manifest not found at ${viteManifestPath}`)
    process.exit(1)
  }

  const viteManifest = JSON.parse(
    readFileSync(viteManifestPath, 'utf-8'),
  ) as Record<string, { css?: string[] }>
  let cssFile: string | undefined
  for (const entry of Object.values(viteManifest)) {
    if (entry.css?.length) {
      cssFile = entry.css[0]
      break
    }
  }
  if (!cssFile) {
    console.error('No CSS entry found in Vite manifest')
    process.exit(1)
  }

  const viteCssPath = join(viteAssetsDir, cssFile)
  if (!existsSync(viteCssPath)) {
    console.error(`Vite CSS not found at ${viteCssPath}`)
    process.exit(1)
  }

  // Read CSS and rewrite asset URLs from bldr plugin paths to static paths.
  let cssContent = readFileSync(viteCssPath, 'utf-8')
  cssContent = cssContent.replaceAll(VITE_ASSET_PREFIX, STATIC_ASSET_PREFIX)

  // Copy referenced assets (fonts, images) to the output dir.
  const assetsOutDir = join(OUTPUT_DIR, 'assets')
  mkdirSync(assetsOutDir, { recursive: true })
  let assetCount = 0
  let iconFile: string | undefined
  for (const file of readdirSync(viteAssetsDir)) {
    if (file.endsWith('.woff2') || file.endsWith('.png')) {
      copyFileSync(join(viteAssetsDir, file), join(assetsOutDir, file))
      assetCount++
      if (file.startsWith('spacewave-icon-')) {
        iconFile = file
      }
    }
  }
  // Also copy source images referenced by components (unhashed names).
  // The bun preload plugin resolves these to /static/assets/<basename>.
  const srcImagesDir = join(projectRoot, 'web/images')
  for (const file of readdirSync(srcImagesDir)) {
    if (
      file.endsWith('.png') ||
      file.endsWith('.svg') ||
      file.endsWith('.ico')
    ) {
      if (!existsSync(join(assetsOutDir, file))) {
        copyFileSync(join(srcImagesDir, file), join(assetsOutDir, file))
        assetCount++
      }
    }
  }
  log(`Copied ${assetCount} assets to ${assetsOutDir}`)

  if (!iconFile) {
    console.error('spacewave-icon PNG not found in Vite assets')
    process.exit(1)
  }

  // Write rewritten CSS to output dir.
  mkdirSync(OUTPUT_DIR, { recursive: true })
  const outputCssPath = join(OUTPUT_DIR, cssFile)
  writeFileSync(outputCssPath, cssContent)
  log(`Wrote CSS to ${outputCssPath} (${cssContent.length} bytes)`)

  return {
    cssUrl: '/static/' + cssFile,
    iconUrl: STATIC_ASSET_PREFIX + iconFile,
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
  log(`Read manifest: entrypoint=${manifest.entrypoint}, wasm=${manifest.wasm}`)

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

  // Extract processed CSS from the Vite build (spacewave-app plugin).
  const { cssUrl: mainCssUrl, iconUrl } = extractViteCss(log)

  // Find the hydration script built by vite.hydrate.config.ts.
  mkdirSync(OUTPUT_DIR, { recursive: true })
  const hydrateFile = readdirSync(OUTPUT_DIR).find(
    (f) => f.startsWith('hydrate-') && f.endsWith('.js'),
  )
  if (!hydrateFile) {
    console.error(
      'hydrate-*.js not found in ' +
        OUTPUT_DIR +
        '. Run vite build --config app/prerender/vite.hydrate.config.ts first.',
    )
    process.exit(1)
  }
  const hydrateScriptTag = `<script type="module" src="/static/${hydrateFile}"></script>`
  log(`Hydration script: ${hydrateFile}`)

  const browserRelease = buildBrowserReleaseDescriptor(
    {
      entrypoint: manifest.entrypoint,
      serviceWorker: manifest.serviceWorker,
      sharedWorker: manifest.sharedWorker,
      wasm: manifest.wasm,
      css: manifest.css,
    },
    ['/', ...STATIC_PAGES.map((page) => page.path), ...blogPaths],
    collectRequiredStaticAssetUrls(OUTPUT_DIR),
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
    mainCssUrl,
    iconUrl,
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

  // Prerender each static page.
  for (const page of STATIC_PAGES) {
    const Component = page.component
    const meta = { ...getMetadata(page.path) }

    // For /landing, override canonicalPath to '/'
    if (page.path === '/landing') {
      meta.canonicalPath = '/'
    }

    validateMetadata(page.path, meta)

    ctx.log(`Prerendering ${page.path}...`)
    const body = await prerenderElement(createElement(Component), page.path)

    const canonicalUrl =
      meta.canonicalPath ? ctx.siteOrigin + meta.canonicalPath : undefined

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
      iconUrl: ctx.iconUrl,
      importMap: ctx.importMap,
    })

    const filename = page.path.slice(1) + '.html'
    const outputPath = join(ctx.outputDir, filename)
    mkdirSync(dirname(outputPath), { recursive: true })
    writeFileSync(outputPath, pageHtml)
    ctx.log(`Wrote ${outputPath} (${pageHtml.length} bytes)`)
  }

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
  for (const page of STATIC_PAGES) {
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

// buildRootTemplate builds the special / route HTML.
// Contains both sw-landing (prerendered landing) and sw-loading (loading screen).
// Bootstrap script handles three-tier visibility:
//   hash in URL -> show loading, boot immediately
//   hasSession -> show loading, boot immediately
//   new visitor -> show landing, deferred boot
// hydrate.tsx handles auto-boot for return visitors.
async function buildRootTemplate(ctx: PrerenderContext) {
  const landingHtml = await prerenderElement(createElement(Landing), '/')

  const canonicalUrl = ctx.siteOrigin + '/'

  // Root template includes both landing and loading screen.
  // Bootstrap inline script (in bootstrapScript) handles visibility
  // based on hasSession/hash. No separate inline script needed.
  const body = `<div id="sw-landing" style="display:flex;flex-direction:column;flex:1;min-height:0">${landingHtml}</div>
      <div id="sw-loading" style="display:none">
        <div style="display:flex;align-items:center;justify-content:center;min-height:100vh;width:100%;background:var(--color-background,#0a0a0a)">
          <div style="text-align:center">
            <h1 style="font-size:1.5rem;font-weight:600;color:var(--color-foreground,#fafafa)">Spacewave</h1>
            <p style="font-size:0.875rem;color:var(--color-foreground-alt,#a1a1aa);margin-top:0.75rem">Loading application...</p>
          </div>
        </div>
      </div>`

  const rootHtml = buildPageHtml({
    body,
    title: landingMetadata.title,
    description: landingMetadata.description,
    canonicalUrl,
    ogImage: landingMetadata.ogImage,
    jsonLd: landingMetadata.jsonLd,
    bootstrapScript: ctx.bootstrapScript,
    hydrateScript: ctx.hydrateScriptTag,
    criticalCss: '',
    mainCssUrl: ctx.mainCssUrl,
    iconUrl: ctx.iconUrl,
    importMap: ctx.importMap,
  })

  const outputPath = join(ctx.outputDir, 'index.html')
  writeFileSync(outputPath, rootHtml)
  console.log(`[prerender] Wrote ${outputPath} (${rootHtml.length} bytes)`)
}

void main()
