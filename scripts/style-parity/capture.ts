import { createHash } from 'node:crypto'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { chromium, webkit, type Page } from 'playwright'

import type { ElementStyle, Snapshot, Viewport } from './model.js'

/** ParityState supplies the real route and interaction that establish a screen. */
export interface ParityState {
  id: string
  path: string
  ready: string | ((page: Page) => Promise<void>)
  setup?: (page: Page) => Promise<void>
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

/** capture writes one snapshot at a time and closes each isolated browser context. */
export async function capture(
  manifest: ParityManifest,
  base: string,
  out: string,
): Promise<string[]> {
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
    const proxy = process.env.STYLE_PARITY_PROXY
    const browser = await launcher.launch({
      headless: true,
      proxy: proxy
        ? { server: proxy, bypass: '127.0.0.1,localhost' }
        : undefined,
    })
    try {
      for (const viewport of viewports) {
        for (const state of manifest.states) {
          const relative = `${engine}/${viewport.width}x${viewport.height}/${state.id}`
          await mkdir(
            join(out, engine, `${viewport.width}x${viewport.height}`),
            { recursive: true },
          )
          const context = await browser.newContext({
            viewport,
            deviceScaleFactor: 1,
            colorScheme: 'dark',
            locale: 'en-US',
            timezoneId: 'UTC',
          })
          const page = await context.newPage()
          try {
            page.setDefaultTimeout(120_000)
            const errors: string[] = []
            page.on('pageerror', (error) => errors.push(error.message))
            await page.clock.setFixedTime(new Date('2026-10-31T12:00:00Z'))
            const response = await page.goto(new URL(state.path, base).href, {
              waitUntil: 'domcontentloaded',
            })
            if (response && !response.ok())
              throw new Error(`${state.path}: HTTP ${response.status()}`)
            await state.setup?.(page)
            if (typeof state.ready === 'string') {
              await page
                .locator(state.ready)
                .first()
                .waitFor({ state: 'visible' })
            } else {
              await state.ready(page)
            }
            await settle(page)
            if (errors.length)
              throw new Error(`${state.id}: ${errors.join('\n')}`)
            const elements = await readElements(page)
            const png = await page.screenshot({
              path: join(out, `${relative}.png`),
              fullPage: true,
              caret: 'hide',
              animations: 'disabled',
            })
            const snapshot: Snapshot = {
              id: state.id,
              engine,
              viewport,
              url: state.path,
              elements,
              aria: await page.locator('body').ariaSnapshot(),
              screenshot: createHash('sha256').update(png).digest('hex'),
            }
            await writeFile(
              join(out, `${relative}.json`),
              JSON.stringify(snapshot),
            )
            paths.push(`${relative}.json`)
            console.log(`captured ${relative}: ${elements.length} elements`)
          } catch (error) {
            // Retain failed states and continue so one broken screen cannot hide others.
            const message =
              error instanceof Error ? error.message : String(error)
            const probes = await Promise.allSettled([
              page.screenshot({
                path: join(out, `${relative}.failed.png`),
                timeout: 10_000,
              }),
              page
                .locator('body')
                .ariaSnapshot({ timeout: 10_000 })
                .then((aria) =>
                  writeFile(join(out, `${relative}.failed.aria.txt`), aria),
                ),
            ])
            await writeFile(
              join(out, `${relative}.failure.json`),
              JSON.stringify(
                {
                  id: state.id,
                  engine,
                  viewport,
                  error: message,
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
            failures.push(relative)
            console.error(`failed ${relative}: ${message}`)
          } finally {
            await context.close()
          }
        }
      }
    } finally {
      await browser.close()
    }
  }
  if (failures.length)
    throw new Error(
      `${failures.length} captures failed: ${failures.join(', ')}; see *.failure.json`,
    )
  return paths
}

// settle waits for fonts, images and layout, then freezes animation progress.
async function settle(page: Page) {
  await page.evaluate(async () => {
    // Finish rendering assets before freezing animation progress.
    await document.fonts.ready
    for (const image of document.images) {
      if (image.currentSrc) await image.decode()
    }
    const animations = document.getAnimations()
    for (const animation of animations) {
      animation.pause()
      const end = animation.effect?.getComputedTiming().endTime ?? Infinity
      animation.currentTime = Number.isFinite(end) ? Number(end) : 0
    }
    await Promise.all(animations.map((animation) => animation.ready))
    await new Promise<void>((resolve) =>
      requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
    )
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
