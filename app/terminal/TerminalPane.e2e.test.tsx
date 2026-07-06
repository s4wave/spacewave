import { afterEach, describe, expect, it, vi } from 'vitest'
import { page } from 'vitest/browser'
import { cleanup, render } from 'vitest-browser-react'

import '@s4wave/web/style/app.css'

import {
  TerminalFrameKind,
  type TerminalFrame,
} from '@s4wave/sdk/terminal/terminal.pb.js'
import { TerminalPane, type TerminalPaneConnector } from './TerminalPane.js'

const encoder = new TextEncoder()

// SAMPLE exercises the full 16-color ANSI palette: SGR 30-37 drives the normal
// set, 90-97 the bright set, and 40-47 the background swatches, so the screenshot
// captures every themed color over sample foreground text.
const ESC = '\x1b['
const SAMPLE =
  [
    `${ESC}1;97mspacewave${ESC}0m:~$ echo design-system`,
    'design-system',
    '',
    Array.from({ length: 8 }, (_, i) => `${ESC}3${i}m██${ESC}0m`).join('') +
      '  normal',
    Array.from({ length: 8 }, (_, i) => `${ESC}9${i}m██${ESC}0m`).join('') +
      '  bright',
    Array.from({ length: 8 }, (_, i) => `${ESC}4${i}m  ${ESC}0m`).join('') +
      '  background',
    '',
    `${ESC}32m✓${ESC}0m ok   ${ESC}31m✗${ESC}0m fail   ${ESC}33m!${ESC}0m warn   ${ESC}34m›${ESC}0m info`,
  ].join('\r\n') + '\r\n'

async function drainClientFrames(
  frames: AsyncIterable<TerminalFrame>,
  signal: AbortSignal,
) {
  try {
    for await (const _frame of frames) {
      if (signal.aborted) return
    }
  } catch {
    return
  }
}

function sampleConnector(): TerminalPaneConnector {
  return (frames, signal) => {
    void drainClientFrames(frames, signal)
    return (async function* renderSample() {
      await Promise.resolve()
      yield {
        kind: TerminalFrameKind.OUTPUT,
        data: encoder.encode(SAMPLE),
      }
      yield { kind: TerminalFrameKind.READY }
      // Hold the stream open so the pane stays mounted for the screenshot,
      // releasing only when the pane aborts the RPC on unmount.
      await new Promise<void>((resolve) => {
        if (signal.aborted) {
          resolve()
          return
        }
        signal.addEventListener('abort', () => resolve(), { once: true })
      })
    })()
  }
}

// normalizeColor mirrors the terminal theme resolver: canvas parses any CSS
// color (including an oklch() token) into the concrete form the browser also
// reports for the rendered viewport background.
function normalizeColor(input: string): string {
  const canvas = document.createElement('canvas')
  canvas.width = 1
  canvas.height = 1
  const ctx = canvas.getContext('2d')
  if (!ctx) throw new Error('no canvas 2d context')
  ctx.fillStyle = input
  ctx.fillRect(0, 0, 1, 1)
  const [r, g, b] = ctx.getImageData(0, 0, 1, 1).data
  return `rgb(${r}, ${g}, ${b})`
}

// resolveToken reads a design token the same way TerminalPane does, so the test
// compares the rendered terminal against the live token, not a duplicated
// literal.
function resolveToken(cssVar: string): string {
  const probe = document.createElement('span')
  probe.style.color = `var(${cssVar})`
  document.body.appendChild(probe)
  const value = getComputedStyle(probe).color
  probe.remove()
  return normalizeColor(value)
}

function toRgb(color: string): [number, number, number] {
  const rgb = color.match(/rgba?\(([^)]+)\)/)
  if (rgb) {
    const [r, g, b] = rgb[1].split(',').map((part) => parseInt(part.trim(), 10))
    return [r, g, b]
  }
  const hex = color.match(/^#([0-9a-fA-F]{6})$/)
  if (hex) {
    const value = parseInt(hex[1], 16)
    return [(value >> 16) & 255, (value >> 8) & 255, value & 255]
  }
  throw new Error(`not an rgb/hex color: ${color}`)
}

describe('TerminalPane design-system visuals', () => {
  afterEach(async () => {
    await cleanup()
  })

  it('paints the console on the warm design tokens and mono font', async () => {
    await render(
      <div className="bg-background-dark flex h-[560px] w-[900px] flex-col">
        <TerminalPane connectTerminal={sampleConnector()} />
      </div>,
    )

    // xterm 6 paints its theme background on the scrollable element; wait for
    // it to carry a resolved color, then screenshot the fully rendered console.
    const surface = await vi.waitFor(() => {
      const el = document.querySelector<HTMLElement>(
        '.xterm-scrollable-element',
      )
      const bg = el ? getComputedStyle(el).backgroundColor : ''
      if (!el || !/^rgb/.test(bg)) throw new Error('xterm surface not painted')
      return el
    })

    await page.screenshot({
      path: '__screenshots__/terminal/terminal-ansi.png',
    })

    // The rendered terminal background is the resolved warm near-black
    // --color-background-dark token, not the cold zinc #09090b the pane hardcoded
    // before this change.
    const rendered = toRgb(getComputedStyle(surface).backgroundColor)
    const token = toRgb(resolveToken('--color-background-dark'))
    for (let channel = 0; channel < 3; channel++) {
      expect(Math.abs(rendered[channel] - token[channel])).toBeLessThanOrEqual(
        2,
      )
    }
    const coldZinc = [9, 9, 11]
    expect(rendered.some((c, i) => Math.abs(c - coldZinc[i]) > 2)).toBe(true)

    // The console uses the Spacewave mono stack (Pragmasevka first).
    const rows = document.querySelector<HTMLElement>('.xterm-rows')
    expect(rows).not.toBeNull()
    const rowsFont = getComputedStyle(rows as HTMLElement).fontFamily
    expect(rowsFont).toMatch(/^pragmasevka/i)
  })
})
