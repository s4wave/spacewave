/**
 * E2E tests for LayoutDebug component.
 *
 * Verifies all 6 variants render real FlexLayout instances correctly
 * with proper tab buttons, tabsets, splitters, styling differences,
 * and variant selection UI.
 */
import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { render, cleanup } from 'vitest-browser-react'
import '@s4wave/web/style/app.css'

import { LayoutDebug } from '@s4wave/web/debug/LayoutDebug.js'
import { RouterProvider } from '@s4wave/web/router/router.js'

function TestWrapper({ children }: { children: React.ReactNode }) {
  return (
    <RouterProvider path="/debug/ui/layout" onNavigate={() => {}}>
      {children}
    </RouterProvider>
  )
}

const VARIANT_IDS = [
  'frosted-glass',
  'wire-outline',
  'brand-strip',
  'soft-blend',
  'elevated',
  'flat-segment',
] as const

function getVariant(id: string): HTMLElement | null {
  return document.querySelector(`[data-testid="variant-${id}"]`)
}

describe('LayoutDebug UI Variants', () => {
  beforeEach(() => void cleanup())
  afterEach(() => void cleanup())

  it('renders all 6 variant containers with FlexLayout inside', async () => {
    await render(
      <TestWrapper>
        <LayoutDebug />
      </TestWrapper>,
    )

    for (const id of VARIANT_IDS) {
      await expect
        .poll(() => {
          const v = getVariant(id)
          if (!v) return null
          // Should contain a FlexLayout
          return v.querySelector('.flexlayout__layout')
        })
        .not.toBeNull()
    }
  })

  it('each variant has 6 tab buttons (4 left tabset + 2 right tabset)', async () => {
    await render(
      <TestWrapper>
        <LayoutDebug />
      </TestWrapper>,
    )

    for (const id of VARIANT_IDS) {
      await expect
        .poll(() => {
          const v = getVariant(id)
          if (!v) return 0
          return v.querySelectorAll('.flexlayout__tab_button').length
        })
        .toBe(6)
    }
  })

  it('each variant has two tabsets with a splitter between them', async () => {
    await render(
      <TestWrapper>
        <LayoutDebug />
      </TestWrapper>,
    )

    for (const id of VARIANT_IDS) {
      await expect
        .poll(() => {
          const v = getVariant(id)
          if (!v) return null
          const tabsets = v.querySelectorAll('.flexlayout__tabset')
          const splitters = v.querySelectorAll('.flexlayout__splitter')
          if (tabsets.length < 2) return null
          return { tabsets: tabsets.length, splitters: splitters.length }
        })
        .not.toBeNull()

      const v = getVariant(id)!
      expect(v.querySelectorAll('.flexlayout__tabset').length).toBe(2)
      expect(
        v.querySelectorAll('.flexlayout__splitter').length,
      ).toBeGreaterThanOrEqual(1)
    }
  })

  it('selected tab color differs from unselected tab color', async () => {
    await render(
      <TestWrapper>
        <LayoutDebug />
      </TestWrapper>,
    )

    for (const id of VARIANT_IDS) {
      await expect
        .poll(() => {
          const v = getVariant(id)
          if (!v) return null
          const selected = v.querySelector('.flexlayout__tab_button--selected')
          const unselected = v.querySelector(
            '.flexlayout__tab_button--unselected',
          )
          if (!selected || !unselected) return null
          return {
            sel: window.getComputedStyle(selected).color,
            unsel: window.getComputedStyle(unselected).color,
          }
        })
        .not.toBeNull()

      const v = getVariant(id)!
      const selected = v.querySelector(
        '.flexlayout__tab_button--selected',
      ) as HTMLElement
      const unselected = v.querySelector(
        '.flexlayout__tab_button--unselected',
      ) as HTMLElement
      const selColor = window.getComputedStyle(selected).color
      const unselColor = window.getComputedStyle(unselected).color
      expect(selColor).not.toBe(unselColor)
    }
  })

  it('tab buttons show expected tab names', async () => {
    await render(
      <TestWrapper>
        <LayoutDebug />
      </TestWrapper>,
    )

    const expectedNames = [
      'Home',
      'main.tsx',
      'Terminal',
      'Settings',
      'Output',
      'Console',
    ]

    // Check first variant — all variants use the same model
    await expect
      .poll(() => {
        const v = getVariant('frosted-glass')
        if (!v) return null
        const buttons = v.querySelectorAll('.flexlayout__tab_button_content')
        if (buttons.length < 6) return null
        return Array.from(buttons).map((b) => b.textContent?.trim())
      })
      .not.toBeNull()

    const v = getVariant('frosted-glass')!
    const buttons = v.querySelectorAll('.flexlayout__tab_button_content')
    const names = Array.from(buttons).map((b) => b.textContent?.trim())
    for (const name of expectedNames) {
      expect(names).toContain(name)
    }
  })

  it('variant selection works when clicking cards', async () => {
    await render(
      <TestWrapper>
        <LayoutDebug />
      </TestWrapper>,
    )

    // Default selection is 'frosted-glass'
    await expect
      .poll(() => {
        const card = document.querySelector(
          '[data-testid="variant-card-frosted-glass"]',
        )
        return card?.textContent?.includes('Selected') ?? false
      })
      .toBe(true)

    // Click Wire Outline variant
    const cardWire = document.querySelector(
      '[data-testid="variant-card-wire-outline"]',
    ) as HTMLElement
    cardWire.click()

    await expect
      .poll(() => {
        const card = document.querySelector(
          '[data-testid="variant-card-wire-outline"]',
        )
        return card?.textContent?.includes('Selected') ?? false
      })
      .toBe(true)

    // Frosted Glass should no longer be selected
    await expect
      .poll(() => {
        const card = document.querySelector(
          '[data-testid="variant-card-frosted-glass"]',
        )
        return card?.textContent?.includes('Selected') ?? false
      })
      .toBe(false)
  })

  it('variant overrides produce visually distinct computed styles', async () => {
    await render(
      <TestWrapper>
        <LayoutDebug />
      </TestWrapper>,
    )

    // Wait for all variants to render
    for (const id of VARIANT_IDS) {
      await expect
        .poll(() => {
          const v = getVariant(id)
          if (!v) return null
          return v.querySelector('.flexlayout__tab_button')
        })
        .not.toBeNull()
    }

    // Collect computed border-radius of a tab_button from each variant
    const radii: Record<string, string> = {}
    for (const id of VARIANT_IDS) {
      const v = getVariant(id)!
      const btn = v.querySelector('.flexlayout__tab_button') as HTMLElement
      radii[id] = window.getComputedStyle(btn).borderRadius
    }

    // Brand Strip and Flat Segment should have 0 border-radius (rectangular)
    expect(radii['brand-strip']).toBe('0px')
    expect(radii['flat-segment']).toBe('0px')

    // Frosted Glass should have top-only rounding (5px 5px 0 0)
    expect(radii['frosted-glass']).toBe('5px 5px 0px 0px')

    // Wire Outline should have top-only rounding (4px 4px 0 0)
    expect(radii['wire-outline']).toBe('4px 4px 0px 0px')

    // Soft Blend should have top-only rounding (5px 5px 0 0)
    expect(radii['soft-blend']).toBe('5px 5px 0px 0px')

    // Elevated should have top-only rounding (6px 6px 0 0)
    expect(radii['elevated']).toBe('6px 6px 0px 0px')

    // Flat Segment must differ from Elevated
    expect(radii['flat-segment']).not.toBe(radii['elevated'])
  })
})
