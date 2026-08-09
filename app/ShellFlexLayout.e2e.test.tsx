/**
 * E2E tests for ShellFlexLayout OptimizedLayout tab dimensions.
 *
 * These tests verify that OptimizedLayout renders tab content with
 * proper non-zero dimensions, both for the outer shell tabs and
 * nested inner layouts.
 *
 * Issue: Tab panels render with height: 0px while width is computed correctly.
 * The OptimizedLayout tab container falls back to 100% sizing when rect.height is 0.
 */
import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { render, cleanup } from 'vitest-browser-react'
import {
  Model,
  OptimizedLayout,
  TabNode,
  IJsonModel,
  Actions,
  DockLocation,
} from '@aptre/flex-layout'

import '@s4wave/web/style/app.css'

const simpleModel: IJsonModel = {
  global: {
    tabEnableClose: false,
    tabSetEnableMaximize: false,
  },
  layout: {
    type: 'row',
    weight: 100,
    children: [
      {
        type: 'tabset',
        id: 'main-tabset',
        weight: 100,
        children: [
          {
            type: 'tab',
            id: 'tab1',
            name: 'Tab 1',
            component: 'test',
          },
        ],
      },
    ],
  },
}

const twoTabModel: IJsonModel = {
  global: {
    tabEnableClose: false,
    tabSetEnableMaximize: false,
  },
  layout: {
    type: 'row',
    weight: 100,
    children: [
      {
        type: 'tabset',
        id: 'main-tabset',
        weight: 100,
        selected: 0,
        children: [
          {
            type: 'tab',
            id: 'tab1',
            name: 'Tab 1',
            component: 'test',
          },
          {
            type: 'tab',
            id: 'tab2',
            name: 'Tab 2',
            component: 'test',
          },
        ],
      },
    ],
  },
}

async function waitForTabPanelWithDimensions(
  selector = '[role="tabpanel"]',
  timeout = 2000,
) {
  await expect
    .poll(
      () => {
        const panel = document.querySelector<HTMLElement>(selector)
        if (!panel) return null
        const height = panel.style.height
        const width = panel.style.width
        if (height && height !== '100%' && width && width !== '100%') {
          return { width, height }
        }
        return null
      },
      { timeout },
    )
    .not.toBeNull()
}

async function waitForElement(
  selector: string,
  timeout = 2000,
): Promise<HTMLElement> {
  await expect
    .poll(() => document.querySelector(selector), { timeout })
    .not.toBeNull()
  return document.querySelector(selector) as HTMLElement
}

async function waitForElements(
  selector: string,
  count: number,
  timeout = 2000,
) {
  await expect
    .poll(() => document.querySelectorAll(selector).length, { timeout })
    .toBe(count)
}

describe('ShellFlexLayout OptimizedLayout E2E', () => {
  beforeEach(() => {
    void cleanup()
    localStorage.clear()
    sessionStorage.clear()
    window.location.hash = ''
  })

  afterEach(() => {
    void cleanup()
  })

  describe('Basic OptimizedLayout Tab Dimensions', () => {
    it('renders tab panel with non-zero height in simple layout', async () => {
      const model = Model.fromJson(simpleModel)

      await render(
        <OptimizedLayout
          model={model}
          renderTab={(node: TabNode) => (
            <div data-testid={`content-${node.getId()}`}>
              Content for {node.getName()}
            </div>
          )}
        />,
      )

      await waitForTabPanelWithDimensions()

      const tabPanel = document.querySelector(
        '[role="tabpanel"]',
      ) as HTMLElement
      expect(tabPanel).not.toBeNull()

      const width = tabPanel.style.width
      const height = tabPanel.style.height

      expect(height).not.toBe('100%')
      expect(width).not.toBe('100%')

      if (height.includes('px')) {
        const heightValue = parseFloat(height)
        expect(heightValue).toBeGreaterThan(0)
      }
      if (width.includes('px')) {
        const widthValue = parseFloat(width)
        expect(widthValue).toBeGreaterThan(0)
      }
    })

    it('TabSetNode.contentRect has valid dimensions after render', async () => {
      const model = Model.fromJson(simpleModel)

      await render(
        <OptimizedLayout
          model={model}
          renderTab={(node: TabNode) => (
            <div data-testid={`content-${node.getId()}`}>
              Content for {node.getName()}
            </div>
          )}
        />,
      )

      interface ContentRect {
        width: number
        height: number
        x: number
        y: number
      }

      await expect
        .poll(() => {
          let rect: ContentRect | null = null
          model.visitNodes((node) => {
            if (node.getType() === 'tabset') {
              const maybe = node as { getContentRect?: unknown }
              if (typeof maybe.getContentRect === 'function') {
                rect = (maybe.getContentRect as () => ContentRect)()
              }
            }
          })
          if (rect === null) return null
          const r = rect as ContentRect
          return r.width > 0 && r.height > 0 ? r : null
        })
        .not.toBeNull()

      let tabsetContentRect: ContentRect | null = null
      model.visitNodes((node) => {
        if (node.getType() === 'tabset') {
          const maybe = node as { getContentRect?: unknown }
          if (typeof maybe.getContentRect === 'function') {
            tabsetContentRect = (maybe.getContentRect as () => ContentRect)()
          }
        }
      })

      expect(tabsetContentRect).not.toBeNull()
      expect(tabsetContentRect!.width).toBeGreaterThan(0)
      expect(tabsetContentRect!.height).toBeGreaterThan(0)
    })

    it('TabNode.rect is updated via resize events', async () => {
      const model = Model.fromJson(simpleModel)

      await render(
        <OptimizedLayout
          model={model}
          renderTab={(node: TabNode) => (
            <div data-testid={`content-${node.getId()}`}>
              Content for {node.getName()}
            </div>
          )}
        />,
      )

      await expect
        .poll(() => {
          let rect: { width: number; height: number } | null = null
          model.visitNodes((node) => {
            if (node.getId() === 'tab1') {
              const maybe = node as {
                rect?: { width?: number; height?: number }
              }
              if (
                maybe.rect &&
                typeof maybe.rect.width === 'number' &&
                typeof maybe.rect.height === 'number'
              ) {
                rect = { width: maybe.rect.width, height: maybe.rect.height }
              }
            }
          })
          if (rect === null) return null
          const r = rect as { width: number; height: number }
          return r.width > 0 && r.height > 0 ? r : null
        })
        .not.toBeNull()

      let tabRect: { width: number; height: number } | null = null
      model.visitNodes((node) => {
        if (node.getId() === 'tab1') {
          const maybe = node as { rect?: { width?: number; height?: number } }
          if (
            maybe.rect &&
            typeof maybe.rect.width === 'number' &&
            typeof maybe.rect.height === 'number'
          ) {
            tabRect = { width: maybe.rect.width, height: maybe.rect.height }
          }
        }
      })

      expect(tabRect).not.toBeNull()
      expect(tabRect!.width).toBeGreaterThan(0)
      expect(tabRect!.height).toBeGreaterThan(0)
    })
  })

  describe('Two-tab Layout', () => {
    it('both tabs receive valid dimensions', async () => {
      const model = Model.fromJson(twoTabModel)

      await render(
        <OptimizedLayout
          model={model}
          renderTab={(node: TabNode) => (
            <div data-testid={`content-${node.getId()}`}>
              Content for {node.getName()}
            </div>
          )}
        />,
      )

      await waitForElements('[data-tab-id]', 2)

      const tab1 = document.querySelector('[data-tab-id="tab1"]') as HTMLElement
      const tab2 = document.querySelector('[data-tab-id="tab2"]') as HTMLElement

      expect(tab1).not.toBeNull()
      expect(tab2).not.toBeNull()

      expect(tab1.style.visibility).not.toBe('hidden')
      expect(tab1.style.height).not.toBe('100%')
      expect(tab1.style.height).not.toBe('0px')

      expect(tab2.style.visibility).toBe('hidden')
    })
  })

  describe('Dynamically Added Tabs', () => {
    it('creates content div for dynamically added tab', async () => {
      const model = Model.fromJson(twoTabModel)

      await render(
        <OptimizedLayout
          model={model}
          renderTab={(node: TabNode) => (
            <div data-testid={`content-${node.getId()}`}>
              Content for {node.getName()}
            </div>
          )}
        />,
      )

      await waitForElements('[role="tabpanel"]', 2)

      model.doAction(
        Actions.addNode(
          {
            type: 'tab',
            name: 'Dynamic Tab',
            component: 'test',
            id: 'dynamic-tab',
          },
          'main-tabset',
          DockLocation.CENTER,
          -1,
        ),
      )

      await waitForElement('[data-tab-id="dynamic-tab"]')

      const newTabPanel = document.querySelector('[data-tab-id="dynamic-tab"]')
      expect(newTabPanel).not.toBeNull()

      await waitForElements('[role="tabpanel"]', 3)
    })

    it('dynamically added and selected tab is visible', async () => {
      const model = Model.fromJson(simpleModel)

      await render(
        <OptimizedLayout
          model={model}
          renderTab={(node: TabNode) => (
            <div data-testid={`content-${node.getId()}`}>
              Content for {node.getName()}
            </div>
          )}
        />,
      )

      await waitForElement('[role="tabpanel"]')

      model.doAction(
        Actions.addNode(
          {
            type: 'tab',
            name: 'Selected Dynamic Tab',
            component: 'test',
            id: 'selected-dynamic',
          },
          'main-tabset',
          DockLocation.CENTER,
          -1,
          true, // select the new tab
        ),
      )

      await expect
        .poll(() => {
          const panel = document.querySelector(
            '[data-tab-id="selected-dynamic"]',
          )
          return panel !== null
        })
        .toBe(true)

      const newTabPanel = document.querySelector(
        '[data-tab-id="selected-dynamic"]',
      ) as HTMLElement
      expect(newTabPanel).not.toBeNull()
      expect(newTabPanel.style.visibility).not.toBe('hidden')
    })

    it('clicking on dynamically added tab shows its content', async () => {
      const model = Model.fromJson(simpleModel)

      await render(
        <OptimizedLayout
          model={model}
          renderTab={(node: TabNode) => (
            <div data-testid={`content-${node.getId()}`}>
              Content for {node.getName()}
            </div>
          )}
        />,
      )

      await waitForElement('[role="tabpanel"]')

      model.doAction(
        Actions.addNode(
          {
            type: 'tab',
            name: 'Click Me Tab',
            component: 'test',
            id: 'click-me-tab',
          },
          'main-tabset',
          DockLocation.CENTER,
          -1,
          false, // don't select
        ),
      )

      await expect
        .poll(() => {
          const buttons = Array.from(
            document.querySelectorAll('.flexlayout__tab_button'),
          )
          for (const btn of buttons) {
            if (btn.textContent?.trim() === 'Click Me Tab') return btn
          }
          return null
        })
        .not.toBeNull()

      const tabButtons = document.querySelectorAll('.flexlayout__tab_button')
      let newTabButton: HTMLElement | null = null
      tabButtons.forEach((btn) => {
        const btnEl = btn as HTMLElement
        if (btnEl.textContent?.includes('Click Me Tab')) {
          newTabButton = btnEl
        }
      })

      expect(newTabButton).not.toBeNull()
      newTabButton!.click()

      await expect
        .poll(() => {
          const panel = document.querySelector('[data-tab-id="click-me-tab"]')
          return (panel as HTMLElement | null)?.style.visibility
        })
        .not.toBe('hidden')

      const tabPanel = document.querySelector(
        '[data-tab-id="click-me-tab"]',
      ) as HTMLElement
      expect(tabPanel).not.toBeNull()
      expect(tabPanel.style.visibility).not.toBe('hidden')

      const content = tabPanel.querySelector(
        '[data-testid="content-click-me-tab"]',
      )
      expect(content).not.toBeNull()
    })

    it('multiple dynamically added tabs all have content divs', async () => {
      const model = Model.fromJson(simpleModel)

      await render(
        <OptimizedLayout
          model={model}
          renderTab={(node: TabNode) => (
            <div data-testid={`content-${node.getId()}`}>
              Content for {node.getName()}
            </div>
          )}
        />,
      )

      await waitForElement('[role="tabpanel"]')

      for (let i = 0; i < 5; i++) {
        model.doAction(
          Actions.addNode(
            {
              type: 'tab',
              name: `Dynamic ${i}`,
              component: 'test',
              id: `dynamic-${i}`,
            },
            'main-tabset',
            DockLocation.CENTER,
            -1,
          ),
        )
      }

      await waitForElements('[role="tabpanel"]', 6)

      for (let i = 0; i < 5; i++) {
        const panel = document.querySelector(`[data-tab-id="dynamic-${i}"]`)
        expect(panel).not.toBeNull()
      }
    })
  })
})
