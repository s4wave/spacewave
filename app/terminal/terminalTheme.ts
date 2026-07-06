import type { ITheme } from '@xterm/xterm'

// Terminal visuals derive from the Spacewave design tokens so the console reads
// as part of the warm app canvas rather than a stock zinc-cold xterm. xterm
// cannot parse the oklch() token strings, so the surface colors are resolved to
// the browser-computed rgb() form at mount from the live document; the literal
// fallbacks keep a coherent warm palette when the tokens are absent (jsdom or
// happy-dom unit runs that do not load app.css).

export const TERMINAL_FONT_FAMILY =
  "'Pragmasevka', ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace"
export const TERMINAL_FONT_SIZE = 12
export const TERMINAL_LINE_HEIGHT = 1.5

const FALLBACK_BACKGROUND = '#0c0b0b'
const FALLBACK_FOREGROUND = '#e1dddd'
const FALLBACK_BRAND = '#ff9ea5'

// ANSI 16-color palette tuned for the warm near-black canvas. Hue semantics stay
// conventional (red is red, green is green) but lightness and saturation are
// pulled toward the app's warm red-pink and violet accents: reds sit on the
// brand/error hue, magenta borrows the app violet, blue and cyan are cooled only
// enough to stay legible, and the normal set holds mid lightness so it reads on
// #0c0b0b while the bright set lifts lightness for emphasis. These are not design
// tokens, so they live as literals rather than resolved custom properties.
const ANSI_PALETTE = {
  black: '#3b3536',
  red: '#ff8f8f',
  green: '#6cd6a2',
  yellow: '#e8c07a',
  blue: '#7fb0ff',
  magenta: '#b48ee8',
  cyan: '#5fc9d6',
  white: '#d8d2d2',
  brightBlack: '#6f6767',
  brightRed: '#ffb3b3',
  brightGreen: '#8ff0bd',
  brightYellow: '#ffd9a0',
  brightBlue: '#a9c7ff',
  brightMagenta: '#cfacf5',
  brightCyan: '#8fe0ea',
  brightWhite: '#ffffff',
} as const

// normalizeColor resolves any CSS color (including an oklch() token) to the
// concrete rgb() form xterm accepts, by rasterizing one pixel and reading it
// back. getComputedStyle and canvas fillStyle both keep oklch() tokens in oklch
// form, and xterm cannot parse those, so this pixel readback is the load-bearing
// conversion (it is also how xterm normalizes its own theme colors).
function normalizeColor(input: string): string {
  if (typeof document === 'undefined') return input
  const canvas = document.createElement('canvas')
  canvas.width = 1
  canvas.height = 1
  const ctx = canvas.getContext('2d')
  if (!ctx) return input
  ctx.fillStyle = input
  ctx.fillRect(0, 0, 1, 1)
  const [r, g, b, a] = ctx.getImageData(0, 0, 1, 1).data
  return a === 255
    ? `rgb(${r}, ${g}, ${b})`
    : `rgba(${r}, ${g}, ${b}, ${a / 255})`
}

// resolveCssColor reads a design token from the live document by letting the
// browser compute a probe element's color, then normalizes it to hex/rgb. The
// fallback is embedded in the var() so a missing token still yields a valid
// color.
function resolveCssColor(
  host: HTMLElement,
  cssVar: string,
  fallback: string,
): string {
  if (typeof document === 'undefined') return fallback
  const probe = document.createElement('span')
  probe.style.position = 'absolute'
  probe.style.visibility = 'hidden'
  probe.style.pointerEvents = 'none'
  probe.style.color = `var(${cssVar}, ${fallback})`
  host.appendChild(probe)
  const resolved = getComputedStyle(probe).color
  probe.remove()
  return normalizeColor(resolved || fallback)
}

// withAlpha rewrites a resolved hex/rgb color to carry the given alpha, leaving
// any other literal (unresolved fallbacks in unit runs) untouched.
function withAlpha(color: string, alpha: number): string {
  const rgb = color.match(/^rgba?\(([^)]+)\)$/)
  if (rgb) {
    const [r, g, b] = rgb[1].split(',').map((part) => part.trim())
    return `rgba(${r}, ${g}, ${b}, ${alpha})`
  }
  const hex = color.match(/^#([0-9a-fA-F]{6})$/)
  if (hex) {
    const value = parseInt(hex[1], 16)
    return `rgba(${(value >> 16) & 255}, ${(value >> 8) & 255}, ${value & 255}, ${alpha})`
  }
  return color
}

export interface TerminalThemeOptions {
  fontFamily: string
  fontSize: number
  lineHeight: number
  theme: ITheme
}

// resolveTerminalTheme reads the app design tokens from the document that owns
// host and returns the xterm options that paint the console on the Spacewave
// canvas: a warm near-black background, soft off-white text, and brand-tinted
// cursor and selection, over the tuned ANSI palette.
export function resolveTerminalTheme(host: HTMLElement): TerminalThemeOptions {
  const background = resolveCssColor(
    host,
    '--color-background-dark',
    FALLBACK_BACKGROUND,
  )
  const foreground = resolveCssColor(
    host,
    '--color-foreground-alt',
    FALLBACK_FOREGROUND,
  )
  const brand = resolveCssColor(host, '--color-brand', FALLBACK_BRAND)
  return {
    fontFamily: TERMINAL_FONT_FAMILY,
    fontSize: TERMINAL_FONT_SIZE,
    lineHeight: TERMINAL_LINE_HEIGHT,
    theme: {
      background,
      foreground,
      cursor: brand,
      cursorAccent: background,
      selectionBackground: withAlpha(brand, 0.3),
      ...ANSI_PALETTE,
    },
  }
}
