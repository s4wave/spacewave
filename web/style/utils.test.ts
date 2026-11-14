import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

import { cn, textSizes } from './utils.js'

describe('cn', () => {
  it('keeps every theme text size beside a text color', () => {
    // Every --text-<name> size in app.css must be known to cn.
    const css = readFileSync(join(import.meta.dirname, 'app.css'), 'utf8')
    const tokens = new Set(
      Array.from(css.matchAll(/^\s*--text-([a-z-]+):/gm), (m) => m[1]).filter(
        (name) => !/--|^shadow|^color/.test(name),
      ),
    )
    expect([...tokens].sort()).toEqual([...textSizes].sort())
    for (const size of textSizes) {
      expect(cn(`text-${size}`, 'text-foreground')).toBe(
        `text-${size} text-foreground`,
      )
      expect(cn(`text-${size}`, 'text-xs')).toBe('text-xs')
    }
  })
})
