// landing-journey-capture records the use-case landing journeys for the
// journey-review report. It drives a running web program through real links
// in fresh Chromium contexts and writes the capture JSON described by the
// journey-review collector contract.
//
// Usage: bun scripts/landing-journey-capture.ts --base http://127.0.0.1:5595
//   --variant before --out .tmp/landing-journeys/before.json [--live]
//
// --live also starts each page's live demo and waits for the nested app.

import { execFileSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { chromium, type Browser, type Page } from 'playwright'

interface Source {
  file: string
  line: number
  kind: string
  text: string
}

interface Screen {
  id: string
  title: string
  url: string
  scenario: string
  scope: string
  blocks: Array<{
    kind: string
    text: string
    region: string
    surface: string
    sources: Source[]
  }>
  controls: Array<{
    kind: string
    label: string
    text: string
    surface: string
    href: string | null
    disabled: boolean
    value: string | null
    state: Record<string, string>
  }>
  aria: string
  pageText: string
  scroll: number[]
  viewport: { width: number; height: number }
}

interface Transition {
  from: string
  to: string
  label: string
  originalHref: string | null
  kind: string
  expectation: string
}

// USE_CASES lists each use-case page with the goal a visitor brings to it.
const USE_CASES = [
  { slug: 'drive', goal: 'Try private files and keep them on my devices.' },
  { slug: 'devices', goal: 'Reach and manage my own devices.' },
  { slug: 'plugins', goal: 'Build and run my own app on Spacewave.' },
  { slug: 'notes', goal: 'Write notes that stay on my devices.' },
  { slug: 'chat', goal: 'Message my people privately.' },
  { slug: 'cli', goal: 'Script my Spaces from a terminal.' },
]

// SOURCE_FILES are the writing sources of the landing journeys.
const SOURCE_FILES = [
  'app/landing/LandingContent.tsx',
  'app/landing/LegalPageLayout.tsx',
  ...USE_CASES.map(({ slug }) => `app/landing/${pageFile(slug)}`),
  'app/landing/use-case/UseCasePage.tsx',
  'app/landing/use-case/UseCaseDemo.tsx',
]

const VIEWPORT = { width: 1280, height: 900 }

// PAGE_HEADING matches a rendered page heading, skipping the boot
// placeholder and the app loading screen titles.
const PAGE_HEADING = 'h1:not(.sw-initial-title):not(.swl-title)'

const args = parseArgs(process.argv.slice(2))
const root = process.cwd()
const inventory = readInventory()
const screens = new Map<string, Screen>()
const transitions: Transition[] = []
const limits: string[] = [
  'Chromium only. Screens are captured at 1280x900 without screenshots.',
  'Each journey starts in a fresh browser context with empty storage.',
  'Full-page text includes content below the fold; it does not establish what a visitor read.',
]
const journeys: Array<{ title: string; goal: string; steps: string[] }> = []

// Remove the previous success artifact before capturing a replacement.
rmSync(args.out, { force: true })

const browser = await chromium.launch({ headless: true })
try {
  for (const useCase of USE_CASES) {
    await captureFromHome(browser, useCase)
    await captureDirect(browser, useCase)
  }
} finally {
  await browser.close()
}

// Publish the capture only after every journey settled.
mkdirSync(dirname(args.out), { recursive: true })
writeFileSync(
  args.out,
  JSON.stringify(
    {
      revision: git(['rev-parse', 'HEAD']),
      variant: args.variant,
      engine: 'chromium',
      viewport: VIEWPORT,
      limits,
      sourceFingerprint: sourceFingerprint(),
      inventory,
      screens: [...screens.values()],
      transitions,
      journeys,
    },
    null,
    2,
  ),
)
console.log(`wrote ${args.out}: ${screens.size} screens`)

// captureFromHome follows a new visitor from the main landing page through
// the use-case card, the live demo when present, and the primary action.
async function captureFromHome(
  browser: Browser,
  useCase: (typeof USE_CASES)[number],
) {
  const scenario = `from-home-${useCase.slug}`
  const steps: string[] = []
  const context = await browser.newContext({ viewport: VIEWPORT })
  try {
    const page = await openFresh(context, '/')

    // Read the main landing page as the journey's first state.
    await waitForPage(page)
    await record(page, 'home', 'Main landing page', scenario, steps)

    // Follow the use-case card to its page.
    const card = page.locator(`a[href$="/landing/${useCase.slug}"]`).first()
    const cardLabel = await controlLabel(card)
    const homeHeading = await page.locator(PAGE_HEADING).first().innerText()
    await card.click()
    await waitForHeadingChange(page, homeHeading)
    const pageId = `${useCase.slug}`
    await record(page, pageId, `${useCase.slug} use-case page`, scenario, steps)
    transitions.push({
      from: 'home',
      to: pageId,
      label: cardLabel,
      originalHref: `/landing/${useCase.slug}`,
      kind: 'click',
      expectation: `Visitor reaches the ${useCase.slug} page.`,
    })

    // Start the live demo and read the running workspace.
    let lastId = pageId
    const start = page.locator('[data-landing-live-start]').first()
    if (args.live && (await start.count()) > 0) {
      const liveId = `${useCase.slug}-live`
      const startLabel = await controlLabel(start)
      await start.click()
      await page
        .locator('[data-spacewave-app][data-app-status="ready"]')
        .first()
        .waitFor({ timeout: 110_000 })
      await record(page, liveId, `${useCase.slug} live demo`, scenario, steps)
      transitions.push({
        from: pageId,
        to: liveId,
        label: startLabel,
        originalHref: null,
        kind: 'click',
        expectation: 'A real in-memory workspace opens in the page.',
      })
      lastId = liveId
    }

    // Follow the primary action and read where it lands.
    const cta = page.locator('[data-landing-primary-cta]').first()
    const fallback = page
      .locator('a.use-case-primary, a[class*="border-brand/40"]')
      .first()
    const primary = (await cta.count()) ? cta : fallback
    const primaryLabel = await controlLabel(primary)
    const primaryHref = await primary.getAttribute('href')
    const before = page.url()
    await primary.click()
    await page.waitForFunction((href) => location.href !== href, before, {
      timeout: 30_000,
    })
    const settled = await waitForAppState(page)
    if (!settled) {
      limits.push(
        `${scenario}: primary action destination did not reach a declared app state within 90s.`,
      )
    }
    const ctaId = `${useCase.slug}-primary`
    await record(
      page,
      ctaId,
      `${useCase.slug} primary action result`,
      scenario,
      steps,
    )
    transitions.push({
      from: lastId,
      to: ctaId,
      label: primaryLabel,
      originalHref: primaryHref,
      kind: 'click',
      expectation:
        'Visitor keeps the workspace they just tried, or starts it for real.',
    })

    journeys.push({
      title: `New visitor explores ${useCase.slug} from the main page`,
      goal: useCase.goal,
      steps,
    })
  } finally {
    await context.close()
  }
}

// captureDirect records a search arrival straight onto the use-case page.
async function captureDirect(
  browser: Browser,
  useCase: (typeof USE_CASES)[number],
) {
  const scenario = `direct-${useCase.slug}`
  const steps: string[] = []
  const context = await browser.newContext({ viewport: VIEWPORT })
  try {
    const page = await openFresh(context, `/landing/${useCase.slug}`)
    await waitForPage(page)
    await record(
      page,
      `${useCase.slug}-direct`,
      `${useCase.slug} direct arrival`,
      scenario,
      steps,
    )
    journeys.push({
      title: `Search visitor lands on ${useCase.slug}`,
      goal: useCase.goal,
      steps,
    })
  } finally {
    await context.close()
  }
}

// openFresh opens a path in a context and asserts new-visitor storage.
async function openFresh(
  context: Awaited<ReturnType<Browser['newContext']>>,
  path: string,
): Promise<Page> {
  const page = await context.newPage()
  page.setDefaultTimeout(60_000)
  page.on('pageerror', (error) => {
    limits.push(`page error at ${page.url()}: ${error.message}`)
  })
  await page.goto(new URL(path, args.base).href, {
    waitUntil: 'domcontentloaded',
  })
  const hasSession = await page.evaluate(() =>
    localStorage.getItem('spacewave-has-session'),
  )
  if (hasSession) throw new Error(`${path} did not start as a new visitor`)
  return page
}

// waitForPage waits for the rendered page heading.
async function waitForPage(page: Page) {
  await page.locator(PAGE_HEADING).first().waitFor()
}

// waitForHeadingChange waits until the first heading leaves a previous page.
// The dev startup render keeps the previous page until the app takes over.
async function waitForHeadingChange(page: Page, previous: string) {
  await page.waitForFunction(
    ([selector, heading]) => {
      const current = document.querySelector(selector)?.textContent?.trim()
      return !!current && current !== heading.trim()
    },
    [PAGE_HEADING, previous] as const,
    { timeout: 120_000 },
  )
}

// waitForAppState waits for a running app destination after a primary action.
async function waitForAppState(page: Page): Promise<boolean> {
  try {
    await page
      .locator(
        '.shell-flexlayout, [data-app-status="ready"], article h1, main h1',
      )
      .first()
      .waitFor({ timeout: 90_000 })
    return true
  } catch {
    return false
  }
}

// controlLabel returns the accessible label of a control locator.
async function controlLabel(
  locator: ReturnType<Page['locator']>,
): Promise<string> {
  return (
    (await locator.getAttribute('aria-label')) ??
    (await locator.innerText()).replace(/\s+/g, ' ').trim()
  )
}

// record captures the current page once per state id and appends the step.
async function record(
  page: Page,
  id: string,
  title: string,
  scenario: string,
  steps: string[],
) {
  steps.push(id)
  if (screens.has(id)) return

  // Read blocks and controls in DOM order with their surface and region.
  const observed = await page.evaluate(() => {
    const normalize = (text: string) => text.replace(/\s+/g, ' ').trim()
    const surfaceOf = (node: Element) => {
      if (node.closest('[role="dialog"], dialog')) return 'dialog'
      if (node.closest('header, nav, footer')) return 'chrome'
      return 'main'
    }
    const regionOf = (node: Element) => {
      const region = node.closest(
        'section, header, nav, footer, [data-spacewave-app]',
      )
      if (!region) return 'page'
      if (region.hasAttribute('data-spacewave-app')) return 'live-app'
      const heading = region.querySelector('h1, h2, h3')
      return normalize(
        heading?.textContent ?? region.tagName.toLowerCase(),
      ).slice(0, 80)
    }
    const visible = (node: Element) => {
      const style = getComputedStyle(node)
      return style.display !== 'none' && style.visibility !== 'hidden'
    }

    const blocks = [
      ...document.querySelectorAll(
        'h1, h2, h3, h4, p, li, figcaption, blockquote, td, th, dt, dd',
      ),
    ]
      .filter((node) => visible(node) && !node.closest('[data-spacewave-app]'))
      .filter((node) => !node.querySelector('p, li, h1, h2, h3, h4'))
      .map((node) => ({
        kind: node.tagName.toLowerCase(),
        text: normalize((node as HTMLElement).innerText ?? ''),
        region: regionOf(node),
        surface: surfaceOf(node),
      }))
      .filter((block) => block.text)

    const controls = [
      ...document.querySelectorAll(
        'a[href], button, input, select, textarea, summary, [role="button"], [role="tab"]',
      ),
    ]
      .filter((node) => visible(node) && !node.closest('[data-spacewave-app]'))
      .map((node) => {
        const element = node as HTMLElement
        const text = normalize(element.innerText ?? '')
        const state: Record<string, string> = {}
        for (const attribute of [
          'aria-pressed',
          'aria-expanded',
          'aria-checked',
          'aria-current',
          'aria-selected',
        ]) {
          const value = element.getAttribute(attribute)
          if (value !== null) state[attribute] = value
        }
        return {
          kind: element.getAttribute('role') ?? element.tagName.toLowerCase(),
          label: normalize(
            element.getAttribute('aria-label') ??
              element.getAttribute('title') ??
              text,
          ),
          text,
          surface: surfaceOf(element),
          href: element.getAttribute('href'),
          disabled:
            element.hasAttribute('disabled') ||
            element.getAttribute('aria-disabled') === 'true',
          value:
            'value' in element
              ? String((element as HTMLInputElement).value)
              : null,
          state,
        }
      })

    const app = document.querySelector(
      '[data-spacewave-app]',
    ) as HTMLElement | null
    if (app) {
      blocks.push({
        kind: 'live-app',
        text: normalize(app.innerText ?? '').slice(0, 2000),
        region: 'live-app',
        surface: 'main',
      })
    }

    return {
      title: document.title,
      blocks,
      controls,
      pageText: normalize(document.body.innerText ?? ''),
      scroll: [window.scrollX, window.scrollY],
    }
  })

  screens.set(id, {
    id,
    title: `${title}: ${observed.title}`,
    url: page.url(),
    scenario,
    scope: 'page',
    blocks: observed.blocks.map((block) => ({
      ...block,
      sources: inventory.filter(
        (source) =>
          source.text.length >= 12 && block.text.includes(source.text),
      ),
    })),
    controls: observed.controls,
    aria: await page.locator('body').ariaSnapshot(),
    pageText: observed.pageText,
    scroll: observed.scroll,
    viewport: VIEWPORT,
  })
}

// readInventory lists literal copy and links in the landing writing sources.
function readInventory(): Source[] {
  const sources: Source[] = []
  for (const file of SOURCE_FILES) {
    let text: string
    try {
      text = readFileSync(resolve(root, file), 'utf8')
    } catch {
      continue
    }
    text.split('\n').forEach((line, index) => {
      for (const match of line.matchAll(/(['"`])((?:(?!\1).){8,}?)\1/g)) {
        const value = match[2]
        if (!/[a-z] [a-z]/i.test(value)) continue
        sources.push({ file, line: index + 1, kind: 'copy', text: value })
      }
      for (const match of line.matchAll(/href[=:]\s*\{?['"]([^'"]+)['"]/g)) {
        sources.push({ file, line: index + 1, kind: 'link', text: match[1] })
      }
      const jsxText = line.trim()
      if (/^[A-Z][^<>{}=;]*[a-z.,]$/.test(jsxText) && jsxText.includes(' ')) {
        sources.push({
          file,
          line: index + 1,
          kind: 'copy-candidate',
          text: jsxText,
        })
      }
    })
  }
  return sources
}

// sourceFingerprint hashes the writing sources captured by this run.
function sourceFingerprint(): string {
  const hash = createHash('sha256')
  for (const file of SOURCE_FILES) {
    try {
      hash.update(file)
      hash.update(readFileSync(resolve(root, file)))
    } catch {
      hash.update('missing')
    }
  }
  return hash.digest('hex').slice(0, 16)
}

function pageFile(slug: string): string {
  return `Landing${slug[0].toUpperCase()}${slug.slice(1)}.tsx`
}

function git(argv: string[]): string {
  return execFileSync('git', argv, { cwd: root, encoding: 'utf8' }).trim()
}

function parseArgs(argv: string[]) {
  const value = (name: string) => {
    const index = argv.indexOf(name)
    return index === -1 ? undefined : argv[index + 1]
  }
  const out = value('--out')
  const variant = value('--variant')
  if (!out || !variant) throw new Error('--out and --variant are required')
  return {
    base: value('--base') ?? 'http://127.0.0.1:5595',
    out: resolve(out),
    variant,
    live: argv.includes('--live'),
  }
}
