/**
 * E2E tests for the shell overlay (shell-flexlayout--with-menu) split styling:
 *
 * 1. Menu-bar overlay padding clears only the top-left tab strip (the one under
 *    the absolute logo/menu overlay), not every tabset's strip.
 * 2. The menu items collapse when the overlay's container is marked narrow,
 *    independent of the viewport width.
 *
 * These render the real web/style/app.css, so they exercise the production
 * selectors, not a stand-in.
 */
import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { render, cleanup } from 'vitest-browser-react'
import type { IJsonRowNode, IJsonTabSetNode } from '@aptre/flex-layout'

import '@s4wave/web/style/app.css'

async function renderWithMenuShell(layout: IJsonRowNode) {
  const { OptimizedLayout, Model } = await import('@aptre/flex-layout')
  const model = Model.fromJson({ global: { tabEnableClose: false }, layout })
  await render(
    <div
      style={{
        width: '1024px',
        height: '600px',
        position: 'relative',
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
      }}
    >
      <div
        className="shell-flexlayout shell-flexlayout--with-menu flex flex-1 flex-col overflow-hidden"
        style={{ '--menu-bar-width': '233px' }}
        data-testid="shell-container"
      >
        <OptimizedLayout
          model={model}
          renderTab={(node) => <div>Content {node.getName()}</div>}
        />
      </div>
    </div>,
  )
}

function stripPad(path: string): number {
  const el = document.querySelector(`[data-layout-path="${path}/tabstrip"]`)
  return el ? Math.round(parseFloat(getComputedStyle(el).paddingLeft)) : -1
}

function singleTabset(id: string): IJsonTabSetNode {
  return {
    type: 'tabset',
    id,
    weight: 50,
    children: [{ type: 'tab', id: id + '-t', name: id, component: 't' }],
  }
}

describe('Shell overlay top-left padding', () => {
  beforeEach(() => void cleanup())
  afterEach(() => void cleanup())

  it('single tabset keeps the menu-bar clearance', async () => {
    await renderWithMenuShell({
      type: 'row',
      children: [singleTabset('only')],
    })
    await expect
      .poll(() => document.querySelectorAll('.flexlayout__tabset').length)
      .toBe(1)
    expect(stripPad('/ts0')).toBeGreaterThan(200)
  })

  it('horizontal split clears only the top-left tab strip', async () => {
    await renderWithMenuShell({
      type: 'row',
      children: [singleTabset('left'), singleTabset('right')],
    })
    await expect
      .poll(() => document.querySelectorAll('.flexlayout__tabset').length)
      .toBe(2)
    expect(stripPad('/ts0')).toBeGreaterThan(200) // top-left under overlay
    expect(stripPad('/ts1')).toBeLessThan(20) // right strip: normal padding
  })

  it('nested split clears only the true top-left tab strip', async () => {
    // row[ left, row[ topRight, bottomRight ] ]
    await renderWithMenuShell({
      type: 'row',
      children: [
        singleTabset('left'),
        {
          type: 'row',
          weight: 50,
          children: [singleTabset('tr'), singleTabset('br')],
        },
      ],
    })
    await expect
      .poll(() => document.querySelectorAll('.flexlayout__tabset').length)
      .toBe(3)
    const wide = [
      ...document.querySelectorAll('.flexlayout__tabset_tabbar_outer_top'),
    ].filter(
      (el) => Math.round(parseFloat(getComputedStyle(el).paddingLeft)) > 200,
    )
    expect(wide.length).toBe(1)
    expect(wide[0].getAttribute('data-layout-path')).toBe('/ts0/tabstrip')
  })
})

describe('Shell menu container collapse', () => {
  beforeEach(() => void cleanup())
  afterEach(() => void cleanup())

  it('collapses the menu items when the overlay is marked narrow', async () => {
    await render(
      <div className="shell-flexlayout">
        <div
          className="shell-menu-bar-overlay"
          data-menu-collapsed="false"
          data-testid="overlay"
        >
          <button aria-label="logo">L</button>
          <div className="shell-menu-collapsible" data-testid="menu-items">
            <span style={{ display: 'inline-block', width: '180px' }}>
              menus
            </span>
          </div>
        </div>
      </div>,
    )

    const items = document.querySelector(
      '[data-testid="menu-items"]',
    ) as HTMLElement
    const overlay = document.querySelector(
      '[data-testid="overlay"]',
    ) as HTMLElement

    expect(Math.round(items.getBoundingClientRect().width)).toBeGreaterThan(100)
    expect(getComputedStyle(items).opacity).toBe('1')

    overlay.dataset.menuCollapsed = 'true'

    await expect
      .poll(() => Math.round(items.getBoundingClientRect().width))
      .toBe(0)
    expect(getComputedStyle(items).opacity).toBe('0')
  })
})
