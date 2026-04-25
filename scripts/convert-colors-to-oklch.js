/**
 * Convert all color tokens in app.css @theme block to oklch color space.
 *
 * Reads web/style/app.css, finds the @theme { ... } block, converts each
 * recognized color value (hex, rgb, rgba, hsl, hsla, hwb) to oklch() using
 * the culori library, then writes the result back.
 *
 * Usage: bun scripts/convert-colors-to-oklch.js
 */

import { readFileSync, writeFileSync } from 'node:fs'
import { resolve, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { parse, converter } from 'culori'

const __dirname = dirname(fileURLToPath(import.meta.url))
const cssPath = resolve(__dirname, '../web/style/app.css')

const toOklch = converter('oklch')

/**
 * Format a number to N decimal places, stripping trailing zeros.
 */
function fmt(n, decimals) {
  return parseFloat(n.toFixed(decimals)).toString()
}

/**
 * Convert a parsed culori color object to an oklch() CSS string.
 */
function formatOklch(oklch) {
  const L = fmt(oklch.l, 4)
  const C = fmt(oklch.c, 4)
  // Achromatic colors (grays, black, white) have undefined hue — use 0.
  const H = fmt(oklch.h != null ? oklch.h : 0, 2)
  const hasAlpha = oklch.alpha != null && oklch.alpha !== 1
  if (hasAlpha) {
    return `oklch(${L} ${C} ${H} / ${fmt(oklch.alpha, 4)})`
  }
  return `oklch(${L} ${C} ${H})`
}

/**
 * Try to convert a single CSS color string to oklch.
 * Returns the oklch() string, or null if the input is not a convertible color.
 */
function convertColor(colorStr) {
  const trimmed = colorStr.trim()
  const parsed = parse(trimmed)
  if (!parsed) return null
  const oklch = toOklch(parsed)
  return formatOklch(oklch)
}

/**
 * Regex matching CSS color values we want to convert.
 * Matches: #hex (3, 4, 6, 8 digit), rgb(), rgba(), hsl(), hsla(), hwb()
 *
 * This is used to find and replace color values within a CSS property value,
 * preserving any surrounding text (e.g. "1px solid" prefix, shadow coordinates).
 */
const COLOR_RE =
  /(?:#(?:[0-9a-fA-F]{3,4}){1,2}\b|(?:rgba?|hsla?|hwb)\([^)]+\))/g

/**
 * Process a single line from within the @theme block.
 * Returns the converted line, or the original line if no conversion is needed.
 */
function processLine(line) {
  const trimmed = line.trim()

  // Skip empty lines and comments
  if (!trimmed || trimmed.startsWith('/*') || trimmed.startsWith('*') || trimmed.startsWith('//')) {
    return line
  }

  // Must be a CSS custom property line
  if (!trimmed.startsWith('--')) {
    return line
  }

  const colonIdx = line.indexOf(':')
  if (colonIdx === -1) return line

  const propName = line.slice(0, colonIdx)
  let value = line.slice(colonIdx + 1).trimEnd()

  // Remove trailing semicolon for processing, add back later
  let hasSemicolon = false
  if (value.endsWith(';')) {
    hasSemicolon = true
    value = value.slice(0, -1)
  }

  const valueTrimmed = value.trim()

  // Skip lines that should not be converted
  if (
    valueTrimmed.includes('var(') ||
    valueTrimmed.startsWith('linear-gradient') ||
    valueTrimmed.startsWith('oklch(') ||
    valueTrimmed === 'transparent' ||
    valueTrimmed === 'none' ||
    valueTrimmed === 'white' ||
    valueTrimmed === 'black'
  ) {
    return line
  }

  // Skip non-color properties (font families, sizes, spacing, etc.)
  // Color properties should contain at least one color-like token
  if (!COLOR_RE.test(valueTrimmed)) {
    return line
  }
  // Reset lastIndex since we used .test()
  COLOR_RE.lastIndex = 0

  // Replace all color values in the line, preserving surrounding text
  const converted = value.replace(COLOR_RE, (match) => {
    const oklch = convertColor(match)
    return oklch || match
  })

  return propName + ':' + converted + (hasSemicolon ? ';' : '')
}

// ─── Main ────────────────────────────────────────────────────────────────────

const css = readFileSync(cssPath, 'utf-8')
const lines = css.split('\n')

// Find the @theme block
let themeStart = -1
let themeEnd = -1
let braceDepth = 0

for (let i = 0; i < lines.length; i++) {
  const trimmed = lines[i].trim()
  if (themeStart === -1 && trimmed.startsWith('@theme')) {
    themeStart = i
    // Count braces on the @theme line itself
    for (const ch of trimmed) {
      if (ch === '{') braceDepth++
      if (ch === '}') braceDepth--
    }
    if (braceDepth === 0) {
      themeEnd = i
      break
    }
    continue
  }
  if (themeStart !== -1) {
    for (const ch of trimmed) {
      if (ch === '{') braceDepth++
      if (ch === '}') braceDepth--
    }
    if (braceDepth === 0) {
      themeEnd = i
      break
    }
  }
}

if (themeStart === -1 || themeEnd === -1) {
  console.error('Could not find @theme block in', cssPath)
  process.exit(1)
}

console.log(`Found @theme block: lines ${themeStart + 1}–${themeEnd + 1}`)

let convertedCount = 0
for (let i = themeStart; i <= themeEnd; i++) {
  const original = lines[i]
  const processed = processLine(original)
  if (processed !== original) {
    convertedCount++
    lines[i] = processed
  }
}

console.log(`Converted ${convertedCount} lines`)

writeFileSync(cssPath, lines.join('\n'), 'utf-8')
console.log(`Wrote updated CSS to ${cssPath}`)
