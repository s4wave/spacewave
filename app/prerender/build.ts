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
import { STATIC_PAGES } from './static-pages.js'
import { getMetadata, type PageMetadata } from './metadata.js'
import {
  ROOT_BOOT_VISIBILITY_CSS,
  ROOT_BOOT_VISIBILITY_SCRIPT,
  ROOT_LOADING_STYLE,
} from './root-loading-shell.js'
import { ROOT_LANDING_SHELL_CLASS } from './root-landing-shell.js'

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

  async function buildStaticPage(
    page: (typeof STATIC_PAGES)[number],
  ): Promise<void> {
    const Component = page.component
    const meta = { ...getMetadata(page.path) }

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
  for (const page of STATIC_PAGES) {
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

// buildRootTemplate builds the root document with the landing and loading
// shells needed before the application starts.
async function buildRootTemplate(ctx: PrerenderContext) {
  const landingHtml = await prerenderElement(createElement(Landing), '/')

  const canonicalUrl = ctx.siteOrigin + '/'

  const body = `<div id="sw-landing" class="${ROOT_LANDING_SHELL_CLASS}">${landingHtml}</div>
      <style>@keyframes sw-boot-shimmer{0%{transform:translateX(-100%)}100%{transform:translateX(300%)}}[data-sw-boot-progress][data-sw-boot-progress-stalled]{position:relative;overflow:hidden}[data-sw-boot-progress][data-sw-boot-progress-stalled]::after{content:"";position:absolute;top:0;bottom:0;left:0;width:35%;border-radius:inherit;background:color-mix(in srgb,var(--color-foreground,#fafafa) 45%,transparent);animation:sw-boot-shimmer 1.1s ease-in-out infinite}@media (prefers-reduced-motion:reduce){[data-sw-boot-progress][data-sw-boot-progress-stalled]::after{display:none;animation:none}}</style>
      <div id="sw-loading" data-sw-boot-state="loading" style="${ROOT_LOADING_STYLE}">
        <div style="display:flex;align-items:center;justify-content:center;flex:1 1 0%;height:100%;min-height:0;width:100%;background:var(--color-background,#0a0a0a);color:var(--color-foreground,#fafafa);overflow:hidden">
          <div style="display:flex;width:min(30rem,calc(100vw - 2rem));flex-direction:column;align-items:center;gap:1.25rem;text-align:center">
            <div style="display:flex;height:5rem;width:5rem;align-items:center;justify-content:center;border-radius:1rem;background:color-mix(in srgb,var(--color-brand,var(--color-logo-blue,#4f8cff)) 10%,transparent);box-shadow:0 0 42px color-mix(in srgb,var(--color-brand,var(--color-logo-blue,#4f8cff)) 22%,transparent)">
              <img src="${ctx.iconUrl}" alt="" width="44" height="44" style="display:block;height:2.75rem;width:2.75rem;border-radius:0.75rem"/>
            </div>
            <div style="display:flex;flex-direction:column;align-items:center;gap:0.5rem">
              <h1 style="margin:0;font-size:1.5rem;font-weight:600;letter-spacing:0;color:var(--color-foreground,#fafafa)">Spacewave</h1>
              <p data-sw-boot-status style="margin:0;font-size:0.875rem;color:color-mix(in srgb,var(--color-foreground-alt,#a1a1aa) 70%,transparent)">Prepare: Loading the app shell.</p>
              <div style="margin-top:0.5rem;display:flex;width:min(16rem,calc(100vw - 4rem));align-items:center;gap:0.75rem">
                <div style="height:0.375rem;flex:1;overflow:hidden;border-radius:9999px;background:color-mix(in srgb,var(--color-foreground,#fafafa) 8%,transparent)">
                  <div data-sw-boot-progress role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow="2" style="height:100%;width:2%;border-radius:9999px;background:var(--color-brand,var(--color-logo-blue,#4f8cff));transition:width 200ms"></div>
                </div>
                <span data-sw-boot-progress-label style="width:2rem;text-align:right;font-family:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,monospace;font-size:0.65rem;color:color-mix(in srgb,var(--color-foreground-alt,#a1a1aa) 70%,transparent);font-variant-numeric:tabular-nums">2%</span>
              </div>
              <p data-sw-boot-error style="display:none;margin:0.5rem 0 0;max-width:20rem;border:1px solid color-mix(in srgb,var(--color-destructive,#ef4444) 15%,transparent);border-radius:0.375rem;background:color-mix(in srgb,var(--color-destructive,#ef4444) 5%,transparent);padding:0.5rem 0.75rem;font-size:0.75rem;line-height:1.5;color:var(--color-destructive,#ef4444)"></p>
              <div data-sw-boot-error-actions style="display:none;align-items:center;justify-content:center;gap:0.5rem;margin-top:0.25rem">
                <button data-sw-boot-retry type="button" style="height:2.25rem;border:1px solid color-mix(in srgb,var(--color-foreground,#fafafa) 10%,transparent);border-radius:0.375rem;background:color-mix(in srgb,var(--color-foreground,#fafafa) 5%,transparent);padding:0 0.75rem;color:var(--color-foreground,#fafafa);font-size:0.875rem;font-weight:500">Retry</button>
                <button data-sw-boot-back type="button" style="height:2.25rem;border:0;border-radius:0.375rem;background:transparent;padding:0 0.75rem;color:color-mix(in srgb,var(--color-foreground-alt,#a1a1aa) 80%,transparent);font-size:0.875rem;font-weight:500">Back</button>
              </div>
            </div>
            <ol aria-label="Startup phases" style="display:grid;width:100%;grid-template-columns:repeat(5,minmax(0,1fr));gap:0.5rem;margin:0;padding:0;list-style:none">
              <li data-sw-boot-phase="prepare" data-sw-boot-phase-state="current" style="min-width:0"><div style="display:flex;height:1.5rem;align-items:center;justify-content:center"><span data-sw-boot-phase-dot style="display:block;height:0.625rem;width:0.625rem;border-radius:9999px;background:var(--color-brand,var(--color-logo-blue,#4f8cff))"></span></div><div data-sw-boot-phase-label style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;text-align:center;font-size:0.65rem;font-weight:500;color:var(--color-foreground,#fafafa)">Prepare</div></li>
              <li data-sw-boot-phase="connect" data-sw-boot-phase-state="pending" style="min-width:0"><div style="display:flex;height:1.5rem;align-items:center;justify-content:center"><span data-sw-boot-phase-dot style="display:block;height:0.625rem;width:0.625rem;border-radius:9999px;background:color-mix(in srgb,var(--color-foreground,#fafafa) 15%,transparent)"></span></div><div data-sw-boot-phase-label style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;text-align:center;font-size:0.65rem;font-weight:500;color:color-mix(in srgb,var(--color-foreground-alt,#a1a1aa) 40%,transparent)">Connect</div></li>
              <li data-sw-boot-phase="runtime" data-sw-boot-phase-state="pending" style="min-width:0"><div style="display:flex;height:1.5rem;align-items:center;justify-content:center"><span data-sw-boot-phase-dot style="display:block;height:0.625rem;width:0.625rem;border-radius:9999px;background:color-mix(in srgb,var(--color-foreground,#fafafa) 15%,transparent)"></span></div><div data-sw-boot-phase-label style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;text-align:center;font-size:0.65rem;font-weight:500;color:color-mix(in srgb,var(--color-foreground-alt,#a1a1aa) 40%,transparent)">Runtime</div></li>
              <li data-sw-boot-phase="frame" data-sw-boot-phase-state="pending" style="min-width:0"><div style="display:flex;height:1.5rem;align-items:center;justify-content:center"><span data-sw-boot-phase-dot style="display:block;height:0.625rem;width:0.625rem;border-radius:9999px;background:color-mix(in srgb,var(--color-foreground,#fafafa) 15%,transparent)"></span></div><div data-sw-boot-phase-label style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;text-align:center;font-size:0.65rem;font-weight:500;color:color-mix(in srgb,var(--color-foreground-alt,#a1a1aa) 40%,transparent)">App</div></li>
              <li data-sw-boot-phase="done" data-sw-boot-phase-state="pending" style="min-width:0"><div style="display:flex;height:1.5rem;align-items:center;justify-content:center"><span data-sw-boot-phase-dot style="display:block;height:0.625rem;width:0.625rem;border-radius:9999px;background:color-mix(in srgb,var(--color-foreground,#fafafa) 15%,transparent)"></span></div><div data-sw-boot-phase-label style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;text-align:center;font-size:0.65rem;font-weight:500;color:color-mix(in srgb,var(--color-foreground-alt,#a1a1aa) 40%,transparent)">Done</div></li>
            </ol>
            <div data-sw-boot-downloads style="display:none;flex-direction:column;gap:0.75rem;width:100%"></div>
            <div aria-hidden="true" style="position:relative;height:7.5rem;width:100%;overflow:hidden;border-radius:0.5rem;border:1px solid color-mix(in srgb,var(--color-foreground,#fafafa) 8%,transparent);background:color-mix(in srgb,var(--color-background-card,#171717) 35%,transparent);box-shadow:0 1.5rem 4rem color-mix(in srgb,var(--color-background-dark,#000) 45%,transparent)">
              <div style="display:flex;height:1.75rem;align-items:center;gap:0.375rem;border-bottom:1px solid color-mix(in srgb,var(--color-foreground,#fafafa) 8%,transparent);background:color-mix(in srgb,var(--color-background-card,#171717) 70%,transparent);padding:0 0.75rem">
                <span style="height:0.5rem;width:0.5rem;border-radius:9999px;background:color-mix(in srgb,var(--color-destructive,#ef4444) 75%,transparent)"></span>
                <span style="height:0.5rem;width:0.5rem;border-radius:9999px;background:color-mix(in srgb,var(--color-warning,#f59e0b) 75%,transparent)"></span>
                <span style="height:0.5rem;width:0.5rem;border-radius:9999px;background:color-mix(in srgb,var(--color-success,#22c55e) 75%,transparent)"></span>
                <span style="margin-left:0.5rem;height:0.5rem;width:4.5rem;border-radius:9999px;background:color-mix(in srgb,var(--color-foreground,#fafafa) 8%,transparent)"></span>
              </div>
              <div style="display:grid;height:calc(100% - 1.75rem);grid-template-columns:4.25rem 1fr">
                <div style="display:flex;flex-direction:column;gap:0.5rem;border-right:1px solid color-mix(in srgb,var(--color-foreground,#fafafa) 6%,transparent);background:color-mix(in srgb,var(--color-background-dark,#000) 35%,transparent);padding:0.5rem">
                  <span data-sw-boot-preview="0" style="height:0.5rem;border-radius:9999px;background:color-mix(in srgb,var(--color-brand,var(--color-logo-blue,#4f8cff)) 55%,transparent)"></span>
                  <span data-sw-boot-preview="1" style="height:0.5rem;border-radius:9999px;background:color-mix(in srgb,var(--color-foreground,#fafafa) 10%,transparent)"></span>
                  <span data-sw-boot-preview="2" style="height:0.5rem;border-radius:9999px;background:color-mix(in srgb,var(--color-foreground,#fafafa) 10%,transparent)"></span>
                  <span data-sw-boot-preview="3" style="height:0.5rem;border-radius:9999px;background:color-mix(in srgb,var(--color-foreground,#fafafa) 10%,transparent)"></span>
                </div>
                <div style="display:grid;grid-template-columns:1fr 5.5rem;gap:0.75rem;padding:0.75rem">
                  <div style="display:flex;flex-direction:column;gap:0.5rem">
                    <span style="height:0.625rem;width:6rem;border-radius:9999px;background:color-mix(in srgb,var(--color-foreground,#fafafa) 20%,transparent)"></span>
                    <div style="display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:0.5rem">
                      <span data-sw-boot-preview="0" style="height:3rem;border-radius:0.375rem;border:1px solid color-mix(in srgb,var(--color-brand,var(--color-logo-blue,#4f8cff)) 20%,transparent);background:color-mix(in srgb,var(--color-brand,var(--color-logo-blue,#4f8cff)) 15%,transparent)"></span>
                      <span data-sw-boot-preview="1" style="height:3rem;border-radius:0.375rem;border:1px solid color-mix(in srgb,var(--color-foreground,#fafafa) 6%,transparent);background:color-mix(in srgb,var(--color-foreground,#fafafa) 4%,transparent)"></span>
                      <span data-sw-boot-preview="2" style="height:3rem;border-radius:0.375rem;border:1px solid color-mix(in srgb,var(--color-foreground,#fafafa) 6%,transparent);background:color-mix(in srgb,var(--color-foreground,#fafafa) 4%,transparent)"></span>
                    </div>
                    <span style="height:0.5rem;width:10.5rem;border-radius:9999px;background:color-mix(in srgb,var(--color-foreground,#fafafa) 10%,transparent)"></span>
                  </div>
                  <div style="display:flex;flex-direction:column;justify-content:flex-end;gap:0.375rem;border-radius:0.375rem;border:1px solid color-mix(in srgb,var(--color-foreground,#fafafa) 6%,transparent);background:color-mix(in srgb,var(--color-background,#0a0a0a) 35%,transparent);padding:0.5rem">
                    <span data-sw-boot-preview="0" style="height:0.375rem;width:72%;border-radius:9999px;background:color-mix(in srgb,var(--color-brand,var(--color-logo-blue,#4f8cff)) 65%,transparent)"></span>
                    <span data-sw-boot-preview="1" style="height:0.375rem;width:55%;border-radius:9999px;background:color-mix(in srgb,var(--color-foreground,#fafafa) 10%,transparent)"></span>
                    <span data-sw-boot-preview="2" style="height:0.375rem;width:88%;border-radius:9999px;background:color-mix(in srgb,var(--color-foreground,#fafafa) 10%,transparent)"></span>
                  </div>
                </div>
              </div>
              <span style="position:absolute;right:1rem;bottom:1rem;height:0.5rem;width:0.5rem;border-radius:9999px;background:color-mix(in srgb,var(--color-brand,var(--color-logo-blue,#4f8cff)) 60%,transparent);box-shadow:0 0 18px var(--color-brand,var(--color-logo-blue,#4f8cff))"></span>
            </div>
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
