import { createHash } from 'node:crypto'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { PNG } from 'pngjs'
import { chromium, webkit, type BrowserContext, type Page } from 'playwright'

import type { ElementStyle, Snapshot, Viewport } from './model.js'
import type {} from './browser.js'

/** ParityState supplies the real route and interaction that establish a screen. */
export interface ParityState {
  id: string
  path: string
  hasTouch?: boolean
  forcedColors?: 'active' | 'none'
  ready: string | ((page: Page) => Promise<void>)
  setup?: (page: Page) => Promise<void>
  // observe collects diagnostics after the image and accessible tree agree, before publication.
  observe?: (page: Page, snapshot: Readonly<Snapshot>) => Promise<void>
}

/** ParityManifest declares the states compared between revisions. */
export interface ParityManifest {
  states: ParityState[]
}

/** viewports is the accepted mobile, laptop and desktop capture matrix. */
export const viewports: Viewport[] = [
  { width: 390, height: 844 },
  { width: 1280, height: 800 },
  { width: 1920, height: 1080 },
]

/** capture writes isolated snapshots and closes its browser when the caller cancels. */
export async function capture(
  manifest: ParityManifest,
  base: string,
  out: string,
  signal?: AbortSignal,
): Promise<string[]> {
  // Cancel before validating or allocating browser resources.
  signal?.throwIfAborted()

  // Validate state identifiers before using them as artifact paths.
  const ids = new Set<string>()
  for (const state of manifest.states) {
    if (!/^[a-z0-9][a-z0-9-]*$/.test(state.id) || ids.has(state.id)) {
      throw new Error(`Invalid or duplicate state id: ${state.id}`)
    }
    ids.add(state.id)
  }
  if (!ids.size) throw new Error('The parity manifest is empty')

  // Keep a single browser alive at a time to bound the capture's resource use.
  const paths: string[] = []
  const failures: string[] = []
  for (const [engine, launcher] of Object.entries({ chromium, webkit })) {
    signal?.throwIfAborted()
    const proxy = process.env.STYLE_PARITY_PROXY
    const browser = await launcher.launch({
      headless: true,
      proxy: proxy
        ? { server: proxy, bypass: '127.0.0.1,localhost' }
        : undefined,
    })
    let closing: Promise<void> | undefined
    const closeBrowser = () => (closing ??= browser.close())
    const closeOnAbort = () => {
      void closeBrowser().catch(() => {})
    }
    signal?.addEventListener('abort', closeOnAbort, { once: true })
    try {
      for (const viewport of viewports) {
        for (const state of manifest.states) {
          signal?.throwIfAborted()
          const relative = `${engine}/${viewport.width}x${viewport.height}/${state.id}`
          await mkdir(
            join(out, engine, `${viewport.width}x${viewport.height}`),
            { recursive: true },
          )
          for (let attempt = 1; attempt <= 2; attempt++) {
            let context: BrowserContext | undefined
            let page: Page | undefined
            const errors: string[] = []
            const networkFailures: string[] = []
            const consoleFailures: string[] = []
            let stage = 'create-context'
            let expired = false
            // Page evaluations can wait forever on a resource; close their context at the state deadline.
            const deadline = setTimeout(() => {
              expired = true
              void context?.close().catch(() => {})
            }, 180_000)
            try {
              context = await browser.newContext({
                viewport,
                deviceScaleFactor: 1,
                hasTouch: state.hasTouch,
                forcedColors: state.forcedColors,
                colorScheme: 'dark',
                locale: 'en-US',
                timezoneId: 'UTC',
              })
              page = await context.newPage()
              page.setDefaultTimeout(120_000)
              page.on('pageerror', (error) => errors.push(error.message))
              page.on('console', (message) => {
                if (message.type() !== 'warning' && message.type() !== 'error')
                  return
                consoleFailures.push(message.text().slice(0, 4000))
                if (consoleFailures.length > 20) consoleFailures.shift()
              })
              page.on('requestfailed', (request) => {
                if (networkFailures.length < 20)
                  networkFailures.push(
                    `${request.url()}: ${request.failure()?.errorText}`,
                  )
              })
              await page.clock.setFixedTime(new Date('2026-10-31T12:00:00Z'))
              stage = 'navigate'
              const response = await page.goto(new URL(state.path, base).href, {
                waitUntil: 'domcontentloaded',
              })
              if (response && !response.ok())
                throw new Error(`${state.path}: HTTP ${response.status()}`)
              stage = 'setup'
              await state.setup?.(page)
              stage = 'ready'
              if (typeof state.ready === 'string') {
                await page
                  .locator(state.ready)
                  .first()
                  .waitFor({ state: 'visible' })
              } else {
                await state.ready(page)
              }
              // Publish only a frame whose accessible content stayed stable around its pixels.
              for (;;) {
                stage = 'settle'
                await settle(page)
                if (errors.length)
                  throw new Error(`${state.id}: ${errors.join('\n')}`)
                const beforeAria = await page.locator('body').ariaSnapshot()
                stage = 'screenshot'
                const png = await page.screenshot({
                  path: join(out, `${relative}.png`),
                  fullPage: true,
                  caret: 'hide',
                  animations: 'disabled',
                })
                // Screenshot cleanup can restart CSS timelines; observe the same static sample as its pixels.
                stage = 'resample'
                await settle(page)
                stage = 'collect'
                const elements = await readElements(page)
                const aria = await page.locator('body').ariaSnapshot()
                if (aria !== beforeAria) {
                  console.log(
                    `content changed during ${relative}; sampling the loaded state again`,
                  )
                  continue
                }

                // Compare rendered pixels, independently of PNG metadata and compression.
                const pixels = PNG.sync.read(png)
                const snapshot: Snapshot = {
                  format: 2,
                  id: state.id,
                  engine,
                  viewport,
                  url: state.path,
                  elements,
                  aria,
                  screenshot: createHash('sha256')
                    .update(`${pixels.width}x${pixels.height}:`)
                    .update(pixels.data)
                    .digest('hex'),
                }
                stage = 'observe'
                await state.observe?.(page, snapshot)
                await writeFile(
                  join(out, `${relative}.json`),
                  JSON.stringify(snapshot),
                )
                paths.push(`${relative}.json`)
                console.log(`captured ${relative}: ${elements.length} elements`)
                break
              }
              break
            } catch (error) {
              signal?.throwIfAborted()

              // Retain failed states and continue so one broken screen cannot hide others.
              const message = expired
                ? `State exceeded its 180s deadline at ${stage}`
                : error instanceof Error
                  ? error.message
                  : String(error)
              // Retry a transient browser operation once, retaining its failure before rebuilding the state.
              const networkChanged = networkFailures.some((failure) =>
                failure.includes('net::ERR_NETWORK_CHANGED'),
              )
              const screenshotProtocolFailure =
                stage === 'screenshot' &&
                message.startsWith(
                  'page.screenshot: Protocol error (Page.captureScreenshot): Unable to capture screenshot',
                ) &&
                errors.length === 0 &&
                networkFailures.length === 0 &&
                consoleFailures.length === 0
              const retry =
                attempt === 1 && (networkChanged || screenshotProtocolFailure)
              const diagnostic = retry
                ? `${relative}.attempt-${attempt}`
                : relative
              const probes = page
                ? await Promise.allSettled([
                    page.screenshot({
                      path: join(out, `${diagnostic}.failed.png`),
                      timeout: 10_000,
                    }),
                    page
                      .locator('body')
                      .ariaSnapshot({ timeout: 10_000 })
                      .then((aria) =>
                        writeFile(
                          join(out, `${diagnostic}.failed.aria.txt`),
                          aria,
                        ),
                      ),
                  ])
                : []
              await writeFile(
                join(out, `${diagnostic}.failure.json`),
                JSON.stringify(
                  {
                    id: state.id,
                    engine,
                    viewport,
                    error: message,
                    stage,
                    attempt,
                    retry,
                    pageErrors: errors,
                    networkFailures,
                    consoleFailures,
                    probes: probes.map((result) =>
                      result.status === 'fulfilled'
                        ? 'saved'
                        : String(result.reason),
                    ),
                  },
                  null,
                  2,
                ),
              )
              if (retry) {
                console.error(
                  `retrying ${relative} after a browser ${networkChanged ? 'network change' : 'screenshot protocol failure'}; first attempt saved`,
                )
              } else {
                failures.push(relative)
                console.error(`failed ${relative}: ${message}`)
                break
              }
            } finally {
              clearTimeout(deadline)
              if (!signal?.aborted) await context?.close()
            }
          }
        }
      }
    } finally {
      signal?.removeEventListener('abort', closeOnAbort)
      try {
        await closeBrowser()
      } finally {
        signal?.throwIfAborted()
      }
    }
  }
  if (failures.length)
    throw new Error(
      `${failures.length} captures failed: ${failures.join(', ')}; see *.failure.json`,
    )
  return paths
}

// settle finishes finite transitions and stops repeating effects before sampling static styles.
async function settle(page: Page) {
  await page.evaluate(async () => {
    // Complete resource-driven layout before choosing the animation sample.
    const fonts = new Set<string>()
    for (const element of document.querySelectorAll('*')) {
      const style = getComputedStyle(element)
      fonts.add(
        `${style.fontStyle} ${style.fontWeight} ${style.fontSize} ${style.fontFamily}`,
      )
    }
    await Promise.all([...fonts].map((font) => document.fonts.load(font)))
    await document.fonts.ready
    for (const image of document.images) {
      if (image.currentSrc) await image.decode()
    }

    // Keep CSS declarations intact while sampling static styles and SVG’s separate timeline.
    const deadline = performance.now() + 10_000
    let quietFrames = 0
    while (quietFrames < 3) {
      for (const svg of document.querySelectorAll('svg')) {
        svg.pauseAnimations()
        svg.setCurrentTime(0)
      }
      const animations = document.getAnimations()
      let transitioning = false
      for (const animation of animations) {
        const end = animation.effect?.getComputedTiming().endTime ?? Infinity
        if (Number.isFinite(end)) {
          if (animation.playState !== 'finished' || animation.pending) {
            animation.finish()
            transitioning = true
          }
        } else {
          animation.cancel()
        }
      }
      quietFrames = transitioning ? 0 : quietFrames + 1
      if (performance.now() > deadline)
        throw new Error(
          `Animations kept restarting during the static sample: ${JSON.stringify(document.getAnimations().map((animation) => ({ state: animation.playState, pending: animation.pending, timing: animation.effect?.getComputedTiming() })))}`,
        )
      await new Promise<void>((resolve) =>
        requestAnimationFrame(() => resolve()),
      )
    }
  })
}

// readElements loads the browser collector compiled beside the Node entrypoint.
async function readElements(page: Page): Promise<ElementStyle[]> {
  await page.evaluate(
    await readFile(
      fileURLToPath(new URL('./browser.js', import.meta.url)),
      'utf8',
    ),
  )
  return page.evaluate(() => globalThis.readStyleParity())
}
