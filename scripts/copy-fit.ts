/**
 * copy-fit renders interface copy in Chrome with the website's real styles
 * and fonts, and reports how each piece sets: how many lines it takes, how
 * full its last line is, and whether boxes that sit side by side carry the
 * same amount of text.
 *
 * It measures either a fixture of component states or a whole page.
 *
 * A fixture (.tsx under scripts/fixtures) renders every state at once. Each
 * state is an element with:
 *
 *   data-state      the state's name in the report
 *   data-copy       a selector for the copy to measure inside the state
 *   data-max-lines  optional; copy that takes more lines overflows
 *   data-balance    optional; a selector for grids whose boxes in one row
 *                   should take the same number of copy lines
 *
 * Wrap each state in the container the page puts the component in, at the
 * width it has there, so the copy wraps where it does on the page.
 *
 * A page (.html file or http URL) is measured at each --width, with
 * --copy, --max-lines and --balance standing in for the attributes.
 *
 *   bun scripts/copy-fit.ts scripts/fixtures/<component>.tsx
 *   bun scripts/copy-fit.ts scripts/fixtures/<component>.tsx \
 *     --try <state> "First wording" "Second wording"
 *   bun scripts/copy-fit.ts http://localhost:5173/ --width 1440,1100 \
 *     --copy 'section p' --balance '.grid'
 */
import { resolve } from 'node:path'
import { parseArgs } from 'node:util'
import { chromium, type Page } from '@playwright/test'

import { serveFixture } from './fixture-server.js'

/** Copy is one measured piece of text in one state. */
export interface Copy {
  state: string
  text: string
  /** lines is the number of line boxes the text sets on; zero when the
   * state renders no copy. */
  lines: number
  /** fill is the last line's width as a share of the copy's box width. */
  fill: number
  /** maxLines is the state's limit; zero when it has none. */
  maxLines: number
}

/** Row is one row of side-by-side boxes in a balanced grid. */
export interface Row {
  state: string
  /** grid is the balanced grid's label, or its tag and first class. */
  grid: string
  /** lines holds each box's copy line count, left to right. */
  lines: number[]
}

/** Report is everything one run measured. */
export interface Report {
  copies: Copy[]
  rows: Row[]
}

/** Options selects what a run measures beyond the fixture's own states. */
export interface Options {
  /** widths are the viewport widths a page is measured at. */
  widths?: number[]
  /** copy, maxLines and balance mark a page's copy like the attributes. */
  copy?: string
  maxLines?: number
  balance?: string
  /** trial measures candidate strings in place of a state's first copy. */
  trial?: { state: string; candidates: string[] }
  /** shot is a PNG path for a full-page picture of the last width. */
  shot?: string
}

/** copyFit measures the fixture or page at entry. */
export async function copyFit(
  entry: string,
  options: Options = {},
): Promise<Report> {
  // Serve a fixture through the website's Vite configuration; open a page
  // directly.
  const fixture = entry.endsWith('.tsx')
  const served = fixture ? await serveFixture(entry) : undefined
  const url =
    served?.url ?? (/^https?:/.test(entry) ? entry : `file://${resolve(entry)}`)
  const browser = await chromium.launch({ channel: 'chrome', headless: true })
  try {
    // Load the page and wait for its fonts, which decide where text wraps.
    const page = await browser.newPage({
      viewport: { width: 1440, height: 900 },
    })
    const errors: string[] = []
    page.on('pageerror', (error) => errors.push(error.message))
    await page.goto(url)
    if (fixture) await page.waitForSelector('[data-state]', { timeout: 20000 })
    await page.evaluate(() => document.fonts.ready)
    if (errors.length) throw new Error(errors.join('\n'))

    // Measure a fixture once, or a page once per width as one state each.
    const report: Report = { copies: [], rows: [] }
    for (const width of fixture ? [0] : (options.widths ?? [1440])) {
      if (width) {
        await page.setViewportSize({ width, height: 900 })
        await markPage(page, `${width}px`, options)
      }
      const measured = await page.evaluate(measure, options.trial)
      report.copies.push(...measured.copies)
      report.rows.push(...measured.rows)
    }

    // Keep a picture of the last layout when asked.
    if (options.shot)
      await page.screenshot({ path: options.shot, fullPage: true })
    return report
  } finally {
    await browser.close()
    await served?.server.close()
  }
}

// markPage turns a whole page into one state with the options' selectors.
async function markPage(page: Page, state: string, options: Options) {
  await page.evaluate(
    ([name, copy, maxLines, balance]) =>
      Object.assign(document.body.dataset, {
        state: name,
        copy,
        maxLines,
        balance,
      }),
    [
      state,
      options.copy ?? 'p',
      String(options.maxLines ?? ''),
      options.balance ?? '',
    ],
  )
}

// measure runs in the page. It counts an element's line boxes by grouping
// its text's client rectangles by vertical position, so inline children on
// one line count once. With a trial, it measures each candidate string in
// place of the named state's first copy instead.
function measure(trial?: { state: string; candidates: string[] }): Report {
  // lineSpans merges an element's text rectangles into one span per line.
  function lineSpans(element: Element) {
    // Read the rectangles of every text run in the element.
    const range = document.createRange()
    range.selectNodeContents(element)

    // Join rectangles whose vertical middles share a line.
    const lines: { middle: number; left: number; right: number }[] = []
    for (const rect of range.getClientRects()) {
      if (!rect.width) continue
      const middle = rect.top + rect.height / 2
      const line = lines.find(
        (span) => Math.abs(span.middle - middle) < rect.height / 2,
      )
      if (line) {
        line.left = Math.min(line.left, rect.left)
        line.right = Math.max(line.right, rect.right)
      } else {
        lines.push({ middle, left: rect.left, right: rect.right })
      }
    }
    return lines
  }

  // boxWidth returns the content width the element's text can fill. An
  // inline element or a flex item that does not grow shrinks to its text, so
  // its own width is always full; the box is then the nearest ancestor that
  // sets the width.
  function boxWidth(element: Element) {
    // Climb past every box that only wraps its text.
    const shrinks = (box: Element) => {
      const style = getComputedStyle(box)
      return (
        style.display.startsWith('inline') ||
        (getComputedStyle(box.parentElement!).display.endsWith('flex') &&
          style.flexGrow === '0')
      )
    }
    let box = element
    while (box.parentElement && shrinks(box)) box = box.parentElement

    // Measure that box inside its padding.
    const style = getComputedStyle(box)
    return (
      box.clientWidth -
      parseFloat(style.paddingLeft) -
      parseFloat(style.paddingRight)
    )
  }

  // copyOf measures one element as it reads now.
  function copyOf(state: HTMLElement, element: HTMLElement): Copy {
    const lines = lineSpans(element)
    const last = lines.at(-1)
    return {
      state: state.dataset.state ?? '',
      text: element.innerText.replace(/\s+/g, ' ').trim(),
      lines: lines.length,
      fill: last ? (last.right - last.left) / boxWidth(element) : 0,
      maxLines: Number(state.dataset.maxLines) || 0,
    }
  }

  // Measure each state's copy and balanced grids.
  const report: Report = { copies: [], rows: [] }
  for (const state of document.querySelectorAll<HTMLElement>('[data-state]')) {
    const selector = state.dataset.copy || '*'
    // Copy is HTML text the page renders; a closed details body or an SVG
    // shape is outside the view being measured.
    const elements = [...state.querySelectorAll(selector)].filter(
      (element) => element instanceof HTMLElement && element.checkVisibility(),
    ) as HTMLElement[]

    // Try candidates in the named state's first copy, then restore it.
    if (trial) {
      const element = elements[0]
      if (state.dataset.state !== trial.state || !element) continue
      const original = element.textContent
      for (const candidate of trial.candidates) {
        element.textContent = candidate
        report.copies.push(copyOf(state, element))
      }
      element.textContent = original
      continue
    }

    // Measure every copy; a state with none still gets a row.
    for (const element of elements) report.copies.push(copyOf(state, element))
    if (!elements.length) {
      report.copies.push({
        state: state.dataset.state ?? '',
        text: '',
        lines: 0,
        fill: 0,
        maxLines: Number(state.dataset.maxLines) || 0,
      })
    }

    // Sum each balanced grid's box copy lines by row of boxes.
    const grids = state.dataset.balance
      ? [...state.querySelectorAll(state.dataset.balance)]
      : []
    for (const grid of grids) {
      const rows = new Map<number, number[]>()
      for (const box of grid.children) {
        const top = Math.round(box.getBoundingClientRect().top)
        const lines = [...box.querySelectorAll(selector)]
          .map((element) => lineSpans(element).length)
          .reduce((sum, count) => sum + count, 0)
        if (!lines) continue
        rows.set(top, [...(rows.get(top) ?? []), lines])
      }
      for (const lines of rows.values()) {
        if (lines.length < 2) continue
        report.rows.push({
          state: state.dataset.state ?? '',
          grid:
            grid.getAttribute('aria-label') ??
            `${grid.localName}.${grid.classList[0] ?? ''}`,
          lines,
        })
      }
    }
  }
  return report
}

/** overflows lists the copies that take more lines than their state allows. */
export function overflows(report: Report): Copy[] {
  return report.copies.filter(
    (copy) => copy.maxLines && copy.lines > copy.maxLines,
  )
}

/** uneven lists the rows whose boxes take different numbers of lines. */
export function uneven(report: Report): Row[] {
  return report.rows.filter(
    (row) => Math.max(...row.lines) !== Math.min(...row.lines),
  )
}

// tight is the last-line fill above which a line at its limit has no room
// for a longer name or a different font.
const tight = 0.9

// Print the report and fail when copy overflows or a row is uneven.
if (import.meta.main) {
  // Read the entry, the trial candidates and the page options.
  const { values, positionals } = parseArgs({
    allowPositionals: true,
    options: {
      width: { type: 'string' },
      copy: { type: 'string' },
      'max-lines': { type: 'string' },
      balance: { type: 'string' },
      try: { type: 'string' },
      shot: { type: 'string' },
    },
  })
  const [entry, ...candidates] = positionals
  if (!entry) {
    console.error(
      'usage: bun scripts/copy-fit.ts <fixture.tsx|page.html|url> [--try <state> <text>...]\n' +
        '         [--width 1440,900] [--copy <selector>] [--max-lines <n>] [--balance <selector>] [--shot <png>]',
    )
    process.exit(2)
  }
  const report = await copyFit(entry, {
    widths: values.width?.split(',').map(Number),
    copy: values.copy,
    maxLines: Number(values['max-lines']) || undefined,
    balance: values.balance,
    trial: values.try ? { state: values.try, candidates } : undefined,
    shot: values.shot,
  })

  // Print one line per copy: verdict, lines against the limit, last-line
  // fill, state and text.
  for (const copy of report.copies) {
    const verdict = !copy.lines
      ? 'none'
      : copy.maxLines && copy.lines > copy.maxLines
        ? 'OVER'
        : copy.lines === copy.maxLines && copy.fill > tight
          ? 'tight'
          : 'ok'
    console.log(
      [
        verdict.padEnd(5),
        `${copy.lines}${copy.maxLines ? `/${copy.maxLines}` : ''} lines`.padEnd(
          9,
        ),
        `last ${Math.round(copy.fill * 100)}%`.padEnd(9),
        copy.state.padEnd(20),
        copy.text,
      ].join('  '),
    )
  }

  // Print each balanced row's line counts and exit nonzero on a violation.
  for (const row of report.rows) {
    const even = Math.max(...row.lines) === Math.min(...row.lines)
    console.log(
      [
        (even ? 'even' : 'UNEVEN').padEnd(6),
        row.state.padEnd(8),
        row.grid.padEnd(32),
        row.lines.join(' '),
      ].join('  '),
    )
  }
  process.exit(overflows(report).length || uneven(report).length ? 1 : 0)
}
