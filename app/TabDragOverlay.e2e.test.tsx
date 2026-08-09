// The drop outline remains visible when a tab drag crosses from the tab bar into content.
import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { render, cleanup } from 'vitest-browser-react'

import '@s4wave/web/style/app.css'

import { AppShell } from './AppShell.js'
import { EditorShell } from './EditorShell.js'

describe('Tab Drag Overlay', () => {
  beforeEach(() => {
    void cleanup()
    localStorage.clear()
    window.location.hash = ''
  })

  afterEach(() => {
    void cleanup()
  })

  it('drag overlay remains visible when dragging tab downward into content area', async () => {
    await render(
      <AppShell>
        <EditorShell />
      </AppShell>,
    )

    await expect
      .poll(
        () => {
          const homeTab = document.querySelector('.flexlayout__tab_button')
          return homeTab !== null
        },
        { timeout: 5000 },
      )
      .toBe(true)

    await expect
      .poll(
        () => {
          const btn = document.querySelector('button[title="New tab"]')
          return btn !== null
        },
        { timeout: 5000 },
      )
      .toBe(true)

    const addButton = document.querySelector(
      'button[title="New tab"]',
    ) as HTMLElement
    addButton.click()

    await expect
      .poll(
        () => {
          const tabButtons = document.querySelectorAll(
            '.flexlayout__tab_button',
          )
          return tabButtons.length
        },
        { timeout: 5000 },
      )
      .toBeGreaterThanOrEqual(2)

    const tabButtons = document.querySelectorAll('.flexlayout__tab_button')
    const secondTab = tabButtons[1] as HTMLElement

    if (!secondTab) {
      throw new Error('Second tab not found')
    }

    const layoutElement = document.querySelector(
      '.flexlayout__layout',
    ) as HTMLElement
    if (!layoutElement) {
      throw new Error('Layout element not found')
    }

    const tabRect = secondTab.getBoundingClientRect()
    const startX = tabRect.left + tabRect.width / 2
    const startY = tabRect.top + tabRect.height / 2

    const createDataTransfer = () => new DataTransfer()

    const dragStartEvent = new DragEvent('dragstart', {
      bubbles: true,
      cancelable: true,
      clientX: startX,
      clientY: startY,
      dataTransfer: createDataTransfer(),
    })
    secondTab.dispatchEvent(dragStartEvent)

    const dragEnterEvent = new DragEvent('dragenter', {
      bubbles: true,
      cancelable: true,
      clientX: startX,
      clientY: startY,
      dataTransfer: createDataTransfer(),
    })
    layoutElement.dispatchEvent(dragEnterEvent)

    const dragOverTabBar = new DragEvent('dragover', {
      bubbles: true,
      cancelable: true,
      clientX: startX,
      clientY: startY + 10,
      dataTransfer: createDataTransfer(),
    })
    layoutElement.dispatchEvent(dragOverTabBar)

    await expect
      .poll(() => document.querySelector('.flexlayout__outline_rect'))
      .not.toBeNull()

    const outlineRectAtTabBar = document.querySelector(
      '.flexlayout__outline_rect',
    ) as HTMLElement

    expect(outlineRectAtTabBar).not.toBeNull()
    expect(outlineRectAtTabBar.style.visibility).toBe('visible')

    const contentY = startY + 100 // 100px below the tab button

    const dragOverContent = new DragEvent('dragover', {
      bubbles: true,
      cancelable: true,
      clientX: startX,
      clientY: contentY,
      dataTransfer: createDataTransfer(),
    })
    layoutElement.dispatchEvent(dragOverContent)

    const outlineRectAtContent = document.querySelector(
      '.flexlayout__outline_rect',
    ) as HTMLElement

    expect(outlineRectAtContent).not.toBeNull()

    if (outlineRectAtContent) {
      const visibility = outlineRectAtContent.style.visibility
      const computedVisibility =
        window.getComputedStyle(outlineRectAtContent).visibility

      expect(visibility).toBe('visible')
      expect(computedVisibility).toBe('visible')
    }

    const dragEndEvent = new DragEvent('dragend', {
      bubbles: true,
      cancelable: true,
      clientX: startX,
      clientY: contentY,
      dataTransfer: createDataTransfer(),
    })
    secondTab.dispatchEvent(dragEndEvent)

    await expect
      .poll(
        () => {
          const buttons = document.querySelectorAll('.flexlayout__tab_button')
          return buttons.length
        },
        { timeout: 5000 },
      )
      .toBeGreaterThanOrEqual(2)
  })

  it('drag overlay remains visible when dragging from quickstart Space tab', async () => {
    window.location.hash = '#/quickstart/drive'

    await render(
      <AppShell>
        <EditorShell />
      </AppShell>,
    )

    await expect
      .poll(
        () => {
          const layout = document.querySelector('.flexlayout__layout')
          return layout !== null
        },
        { timeout: 5000 },
      )
      .toBe(true)

    await expect
      .poll(
        () => {
          const tabButtons = document.querySelectorAll(
            '.flexlayout__tab_button',
          )
          return tabButtons.length
        },
        { timeout: 5000 },
      )
      .toBeGreaterThanOrEqual(1)

    const addButton = document.querySelector(
      'button[title="New tab"]',
    ) as HTMLElement
    if (addButton) {
      addButton.click()
    }

    await expect
      .poll(
        () => {
          const tabButtons = document.querySelectorAll(
            '.flexlayout__tab_button',
          )
          return tabButtons.length
        },
        { timeout: 5000 },
      )
      .toBeGreaterThanOrEqual(2)

    const tabButtons = document.querySelectorAll('.flexlayout__tab_button')
    const secondTab = tabButtons[1] as HTMLElement

    if (!secondTab) {
      throw new Error('Second tab not found')
    }

    const layoutElement = document.querySelector(
      '.flexlayout__layout',
    ) as HTMLElement

    const tabRect = secondTab.getBoundingClientRect()
    const startX = tabRect.left + tabRect.width / 2
    const startY = tabRect.top + tabRect.height / 2

    const createDataTransfer = () => new DataTransfer()

    secondTab.dispatchEvent(
      new DragEvent('dragstart', {
        bubbles: true,
        cancelable: true,
        clientX: startX,
        clientY: startY,
        dataTransfer: createDataTransfer(),
      }),
    )

    layoutElement.dispatchEvent(
      new DragEvent('dragenter', {
        bubbles: true,
        cancelable: true,
        clientX: startX,
        clientY: startY,
        dataTransfer: createDataTransfer(),
      }),
    )

    layoutElement.dispatchEvent(
      new DragEvent('dragover', {
        bubbles: true,
        cancelable: true,
        clientX: startX,
        clientY: startY + 10,
        dataTransfer: createDataTransfer(),
      }),
    )

    await expect
      .poll(() => document.querySelector('.flexlayout__outline_rect'))
      .not.toBeNull()

    const initialOutline = document.querySelector('.flexlayout__outline_rect')
    expect(initialOutline).not.toBeNull()

    layoutElement.dispatchEvent(
      new DragEvent('dragover', {
        bubbles: true,
        cancelable: true,
        clientX: startX,
        clientY: startY + 100,
        dataTransfer: createDataTransfer(),
      }),
    )

    const outlineAfterDrag = document.querySelector(
      '.flexlayout__outline_rect',
    ) as HTMLElement

    expect(outlineAfterDrag).not.toBeNull()
    if (outlineAfterDrag) {
      expect(outlineAfterDrag.style.visibility).toBe('visible')
    }

    secondTab.dispatchEvent(
      new DragEvent('dragend', {
        bubbles: true,
        cancelable: true,
        clientX: startX,
        clientY: startY + 100,
        dataTransfer: createDataTransfer(),
      }),
    )
  })

  it('keeps drag overlay visible over nested FlexLayout', async () => {
    const { Layout, Model } = await import('@aptre/flex-layout')

    function NestedLayoutContent() {
      const nestedModel = Model.fromJson({
        global: {},
        layout: {
          type: 'row',
          children: [
            {
              type: 'tabset',
              children: [
                { type: 'tab', name: 'Nested Tab', component: 'test' },
              ],
            },
          ],
        },
      })

      return (
        <div
          style={{ width: '100%', height: '100%', position: 'relative' }}
          data-testid="nested-layout-wrapper"
        >
          <Layout
            model={nestedModel}
            factory={() => <div>Nested content</div>}
          />
        </div>
      )
    }

    const shellModel = Model.fromJson({
      global: {},
      layout: {
        type: 'row',
        children: [
          {
            type: 'tabset',
            children: [
              { type: 'tab', name: 'Tab 1', component: 'nested', id: 'tab1' },
              { type: 'tab', name: 'Tab 2', component: 'simple', id: 'tab2' },
            ],
          },
        ],
      },
    })

    await render(
      <Layout
        model={shellModel}
        factory={(node) => {
          if (node.getComponent() === 'nested') {
            return <NestedLayoutContent />
          }
          return <div>Simple content for {node.getName()}</div>
        }}
      />,
    )

    await expect
      .poll(
        () => {
          const layouts = document.querySelectorAll('.flexlayout__layout')
          return layouts.length
        },
        { timeout: 5000 },
      )
      .toBeGreaterThanOrEqual(2)

    const shellLayout = document.querySelector(
      '.flexlayout__layout',
    ) as HTMLElement
    const tabButtons = shellLayout.querySelectorAll('.flexlayout__tab_button')

    expect(tabButtons.length).toBeGreaterThanOrEqual(2)

    const secondTab = tabButtons[1] as HTMLElement
    const tabRect = secondTab.getBoundingClientRect()
    const startX = tabRect.left + tabRect.width / 2
    const startY = tabRect.top + tabRect.height / 2

    const createDataTransfer = () => new DataTransfer()

    secondTab.dispatchEvent(
      new DragEvent('dragstart', {
        bubbles: true,
        cancelable: true,
        clientX: startX,
        clientY: startY,
        dataTransfer: createDataTransfer(),
      }),
    )

    shellLayout.dispatchEvent(
      new DragEvent('dragenter', {
        bubbles: true,
        cancelable: true,
        clientX: startX,
        clientY: startY,
        dataTransfer: createDataTransfer(),
      }),
    )

    shellLayout.dispatchEvent(
      new DragEvent('dragover', {
        bubbles: true,
        cancelable: true,
        clientX: startX,
        clientY: startY + 10,
        dataTransfer: createDataTransfer(),
      }),
    )

    await expect
      .poll(() => document.querySelector('.flexlayout__outline_rect'))
      .not.toBeNull()

    const outlineAtTabBar = document.querySelector(
      '.flexlayout__outline_rect',
    ) as HTMLElement
    expect(outlineAtTabBar).not.toBeNull()

    const nestedWrapper = document.querySelector(
      '[data-testid="nested-layout-wrapper"]',
    ) as HTMLElement
    const nestedRect = nestedWrapper?.getBoundingClientRect()

    if (nestedRect) {
      const nestedX = nestedRect.left + nestedRect.width / 2
      const nestedY = nestedRect.top + nestedRect.height / 2

      const nestedLayout = nestedWrapper.querySelector(
        '.flexlayout__layout',
      ) as HTMLElement
      if (nestedLayout) {
        nestedLayout.dispatchEvent(
          new DragEvent('dragenter', {
            bubbles: true,
            cancelable: true,
            clientX: nestedX,
            clientY: nestedY,
            dataTransfer: createDataTransfer(),
          }),
        )
      }

      shellLayout.dispatchEvent(
        new DragEvent('dragover', {
          bubbles: true,
          cancelable: true,
          clientX: nestedX,
          clientY: nestedY,
          dataTransfer: createDataTransfer(),
        }),
      )

      const outlineAfterNestedEnter = document.querySelector(
        '.flexlayout__outline_rect',
      ) as HTMLElement

      expect(outlineAfterNestedEnter).not.toBeNull()
      if (outlineAfterNestedEnter) {
        expect(outlineAfterNestedEnter.style.visibility).toBe('visible')
      }
    }

    secondTab.dispatchEvent(
      new DragEvent('dragend', {
        bubbles: true,
        cancelable: true,
        clientX: startX,
        clientY: startY + 100,
        dataTransfer: createDataTransfer(),
      }),
    )
  })

  it('keeps drag overlay visible through internal dragleave transitions', async () => {
    const { Layout, Model } = await import('@aptre/flex-layout')

    function NestedLayoutContent() {
      const nestedModel = Model.fromJson({
        global: {},
        layout: {
          type: 'row',
          children: [
            {
              type: 'tabset',
              children: [
                { type: 'tab', name: 'Nested Tab', component: 'test' },
              ],
            },
          ],
        },
      })

      return (
        <div
          style={{ width: '100%', height: '100%', position: 'relative' }}
          data-testid="nested-layout-wrapper-v2"
        >
          <Layout
            model={nestedModel}
            factory={() => <div>Nested content</div>}
          />
        </div>
      )
    }

    const shellModel = Model.fromJson({
      global: {},
      layout: {
        type: 'row',
        children: [
          {
            type: 'tabset',
            children: [
              { type: 'tab', name: 'Tab 1', component: 'nested', id: 'tab1' },
              { type: 'tab', name: 'Tab 2', component: 'simple', id: 'tab2' },
            ],
          },
        ],
      },
    })

    await render(
      <Layout
        model={shellModel}
        factory={(node) => {
          if (node.getComponent() === 'nested') {
            return <NestedLayoutContent />
          }
          return <div data-testid="simple-content">Simple content</div>
        }}
      />,
    )

    await expect
      .poll(() => document.querySelectorAll('.flexlayout__layout').length, {
        timeout: 5000,
      })
      .toBeGreaterThanOrEqual(2)

    const shellLayout = document.querySelector(
      '.flexlayout__layout',
    ) as HTMLElement
    const tabButtons = shellLayout.querySelectorAll('.flexlayout__tab_button')
    const secondTab = tabButtons[1] as HTMLElement
    const tabRect = secondTab.getBoundingClientRect()
    const startX = tabRect.left + tabRect.width / 2
    const startY = tabRect.top + tabRect.height / 2

    const createDataTransfer = () => new DataTransfer()
    const dt = createDataTransfer()

    secondTab.dispatchEvent(
      new DragEvent('dragstart', {
        bubbles: true,
        cancelable: true,
        clientX: startX,
        clientY: startY,
        dataTransfer: dt,
      }),
    )

    shellLayout.dispatchEvent(
      new DragEvent('dragenter', {
        bubbles: true,
        cancelable: true,
        clientX: startX,
        clientY: startY,
        dataTransfer: dt,
      }),
    )

    shellLayout.dispatchEvent(
      new DragEvent('dragover', {
        bubbles: true,
        cancelable: true,
        clientX: startX,
        clientY: startY + 10,
        dataTransfer: dt,
      }),
    )

    await expect
      .poll(() => document.querySelector('.flexlayout__outline_rect'))
      .not.toBeNull()

    let outline = document.querySelector(
      '.flexlayout__outline_rect',
    ) as HTMLElement
    expect(outline).not.toBeNull()
    expect(outline.style.visibility).toBe('visible')

    // A child transition reports dragleave on the shell with a relatedTarget inside it.
    const tabContent = shellLayout.querySelector(
      '.flexlayout__tab',
    ) as HTMLElement

    if (tabContent) {
      tabContent.dispatchEvent(
        new DragEvent('dragleave', {
          bubbles: true,
          cancelable: true,
          clientX: startX,
          clientY: startY + 100,
          dataTransfer: dt,
          relatedTarget: tabContent,
        }),
      )
    }

    shellLayout.dispatchEvent(
      new DragEvent('dragleave', {
        bubbles: true,
        cancelable: true,
        clientX: startX,
        clientY: startY + 150,
        dataTransfer: dt,
        relatedTarget: tabContent,
      }),
    )

    outline = document.querySelector('.flexlayout__outline_rect') as HTMLElement

    expect(outline).not.toBeNull()
    expect(outline.style.visibility).toBe('visible')

    secondTab.dispatchEvent(
      new DragEvent('dragend', {
        bubbles: true,
        cancelable: true,
        clientX: startX,
        clientY: startY + 100,
        dataTransfer: dt,
      }),
    )
  })
})

describe('Grid Layout', () => {
  beforeEach(() => {
    void cleanup()
    localStorage.clear()
    window.location.hash = ''
  })

  afterEach(() => {
    void cleanup()
  })

  it('grid mode layout has correct positioning and click behavior', async () => {
    const { Layout, Model, Actions } = await import('@aptre/flex-layout')

    const gridModel = Model.fromJson({
      global: {
        splitterSize: 4,
        splitterExtra: 4,
        enableEdgeDock: true,
        tabEnableClose: false,
        tabSetEnableMaximize: false,
        tabSetEnableDivide: true,
        tabSetEnableDeleteWhenEmpty: true,
      },
      layout: {
        type: 'row',
        weight: 100,
        children: [
          {
            type: 'tabset',
            id: 'left-tabset',
            weight: 50,
            children: [
              {
                type: 'tab',
                id: 'left-tab',
                name: 'Left Tab',
                component: 'test',
              },
            ],
          },
          {
            type: 'tabset',
            id: 'right-tabset',
            weight: 50,
            children: [
              {
                type: 'tab',
                id: 'right-tab',
                name: 'Right Tab',
                component: 'test',
              },
            ],
          },
        ],
      },
    })

    await render(
      <div className="shell-flexlayout bg-editor-border flex flex-1 flex-col gap-1 overflow-hidden p-1">
        <Layout
          model={gridModel}
          factory={(node) => (
            <div
              role="button"
              tabIndex={0}
              data-testid={`content-${node.getId()}`}
              style={{ width: '100%', height: '100%', padding: '20px' }}
              onClick={() => {
                let parent = node.getParent()
                while (parent && parent.getType() !== 'tabset') {
                  parent = parent.getParent()
                }
                if (parent) {
                  gridModel.doAction(Actions.selectTab(node.getId()))
                }
              }}
              onKeyDown={(e) => {
                if (e.key !== 'Enter' && e.key !== ' ') return
                e.preventDefault()
                let parent = node.getParent()
                while (parent && parent.getType() !== 'tabset') {
                  parent = parent.getParent()
                }
                if (parent) {
                  gridModel.doAction(Actions.selectTab(node.getId()))
                }
              }}
            >
              Content for {node.getName()}
            </div>
          )}
        />
      </div>,
    )

    await expect
      .poll(
        () => {
          const tabsets = document.querySelectorAll('.flexlayout__tabset')
          return tabsets.length
        },
        { timeout: 5000 },
      )
      .toBe(2)

    const tabsets = document.querySelectorAll('.flexlayout__tabset')
    const leftTabset = tabsets[0] as HTMLElement
    const rightTabset = tabsets[1] as HTMLElement

    const leftRect = leftTabset.getBoundingClientRect()
    const rightRect = rightTabset.getBoundingClientRect()

    expect(leftRect.right).toBeLessThanOrEqual(rightRect.left + 10) // Allow for splitter

    const container = document.querySelector('.shell-flexlayout') as HTMLElement
    const layoutElement = container.querySelector(
      '.flexlayout__layout',
    ) as HTMLElement
    const layoutRect = layoutElement.getBoundingClientRect()

    const leftTabBar = leftTabset.querySelector(
      '.flexlayout__tabset_tabbar_outer_top',
    ) as HTMLElement
    if (leftTabBar) {
      const leftTabBarRect = leftTabBar.getBoundingClientRect()

      expect(leftTabBarRect.top).toBeGreaterThanOrEqual(layoutRect.top - 1)
    }

    const rightContent = document.querySelector(
      '[data-testid="content-right-tab"]',
    ) as HTMLElement

    if (rightContent) {
      rightContent.click()
    }
  })

  it('entering grid mode via drag creates proper layout', async () => {
    const { Layout, Model, Actions, DockLocation } =
      await import('@aptre/flex-layout')

    const model = Model.fromJson({
      global: {
        splitterSize: 4,
        splitterExtra: 4,
        enableEdgeDock: true,
        tabEnableClose: false,
        tabSetEnableMaximize: false,
        tabSetEnableDivide: true,
        tabSetEnableDeleteWhenEmpty: true,
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
              { type: 'tab', id: 'tab1', name: 'Tab 1', component: 'test' },
              { type: 'tab', id: 'tab2', name: 'Tab 2', component: 'test' },
            ],
          },
        ],
      },
    })

    await render(
      <div className="shell-flexlayout bg-editor-border flex flex-1 flex-col gap-1 overflow-hidden p-1">
        <Layout
          model={model}
          factory={(node) => (
            <div data-testid={`content-${node.getId()}`}>
              Content for {node.getName()}
            </div>
          )}
          onModelChange={() => {}}
        />
      </div>,
    )

    await expect
      .poll(
        () => {
          const tabButtons = document.querySelectorAll(
            '.flexlayout__tab_button',
          )
          return tabButtons.length
        },
        { timeout: 5000 },
      )
      .toBe(2)

    let tabsets = document.querySelectorAll('.flexlayout__tabset')
    expect(tabsets.length).toBe(1)

    // The model action matches dropping tab2 on the main tabset's right edge.
    model.doAction(
      Actions.moveNode('tab2', 'main-tabset', DockLocation.RIGHT, -1),
    )

    await expect
      .poll(() => document.querySelectorAll('.flexlayout__tabset').length)
      .toBe(2)

    tabsets = document.querySelectorAll('.flexlayout__tabset')

    expect(tabsets.length).toBe(2)

    const leftTabset = tabsets[0] as HTMLElement
    const rightTabset = tabsets[1] as HTMLElement
    const leftRect = leftTabset.getBoundingClientRect()
    const rightRect = rightTabset.getBoundingClientRect()

    expect(Math.abs(leftRect.top - rightRect.top)).toBeLessThan(2)

    expect(leftRect.width).toBeGreaterThan(100)
    expect(rightRect.width).toBeGreaterThan(100)
  })

  // OptimizedLayout renders tab content beside FlexLayout, so pointerdown selects its parent tabset.
  it('OptimizedLayout click-to-select activates parent tabset', async () => {
    const flexLayout = await import('@aptre/flex-layout')
    const { OptimizedLayout, Model, Actions } = flexLayout

    const model = Model.fromJson({
      global: {
        splitterSize: 4,
        splitterExtra: 4,
        enableEdgeDock: true,
        tabEnableClose: false,
        tabSetEnableMaximize: false,
        tabSetEnableDivide: true,
        tabSetEnableDeleteWhenEmpty: true,
      },
      layout: {
        type: 'row',
        weight: 100,
        children: [
          {
            type: 'tabset',
            id: 'left-tabset',
            weight: 50,
            selected: 0,
            children: [
              {
                type: 'tab',
                id: 'left-tab',
                name: 'Left Tab',
                component: 'test',
              },
            ],
          },
          {
            type: 'tabset',
            id: 'right-tabset',
            weight: 50,
            selected: 0,
            children: [
              {
                type: 'tab',
                id: 'right-tab',
                name: 'Right Tab',
                component: 'test',
              },
            ],
          },
        ],
      },
    })

    model.doAction(Actions.setActiveTabset('left-tabset'))

    await render(
      <div
        className="shell-flexlayout bg-editor-border flex flex-1 flex-col gap-1 overflow-hidden p-1"
        style={{ position: 'relative' }}
      >
        <OptimizedLayout
          model={model}
          renderTab={(node) => (
            <div
              data-testid={`optimized-content-${node.getId()}`}
              style={{
                width: '100%',
                height: '100%',
                padding: '20px',
                backgroundColor:
                  node.getId() === 'left-tab' ? '#2a2a4a' : '#4a2a2a',
              }}
            >
              OptimizedLayout Content for {node.getName()}
            </div>
          )}
        />
      </div>,
    )

    await expect
      .poll(
        () => {
          const tabsets = document.querySelectorAll('.flexlayout__tabset')
          return tabsets.length
        },
        { timeout: 5000 },
      )
      .toBe(2)

    await expect
      .poll(
        () => {
          const tabPanels = document.querySelectorAll('[role="tabpanel"]')
          return tabPanels.length
        },
        { timeout: 5000 },
      )
      .toBe(2)

    const getActiveTabsetId = (): string | undefined => {
      let activeId: string | undefined
      model.visitNodes((node) => {
        if (node.getType() === 'tabset') {
          const tabset = node as InstanceType<typeof flexLayout.TabSetNode>
          if (tabset.isActive()) {
            activeId = tabset.getId()
          }
        }
      })
      return activeId
    }

    expect(getActiveTabsetId()).toBe('left-tabset')

    const rightTabPanel = document.querySelector(
      '[data-tab-id="right-tab"]',
    ) as HTMLElement
    expect(rightTabPanel).not.toBeNull()

    rightTabPanel.dispatchEvent(
      new PointerEvent('pointerdown', {
        bubbles: true,
        cancelable: true,
      }),
    )

    await expect.poll(() => getActiveTabsetId()).toBe('right-tabset')

    expect(getActiveTabsetId()).toBe('right-tabset')

    const leftTabPanel = document.querySelector(
      '[data-tab-id="left-tab"]',
    ) as HTMLElement
    expect(leftTabPanel).not.toBeNull()

    leftTabPanel.dispatchEvent(
      new PointerEvent('pointerdown', {
        bubbles: true,
        cancelable: true,
      }),
    )

    await expect.poll(() => getActiveTabsetId()).toBe('left-tabset')

    expect(getActiveTabsetId()).toBe('left-tabset')
  })
})

describe('Grid Layout Styling', () => {
  beforeEach(() => {
    void cleanup()
    localStorage.clear()
    window.location.hash = ''
  })

  afterEach(() => {
    void cleanup()
  })

  it('grid mode tabsets have consistent padding on all sides', async () => {
    const { Layout, Model } = await import('@aptre/flex-layout')

    const gridModel = Model.fromJson({
      global: {
        splitterSize: 4,
        splitterExtra: 4,
        tabEnableClose: false,
      },
      layout: {
        type: 'row',
        weight: 100,
        children: [
          {
            type: 'tabset',
            id: 'left-tabset',
            weight: 50,
            children: [
              {
                type: 'tab',
                id: 'left-tab',
                name: 'Left Tab',
                component: 'test',
              },
            ],
          },
          {
            type: 'tabset',
            id: 'right-tabset',
            weight: 50,
            children: [
              {
                type: 'tab',
                id: 'right-tab',
                name: 'Right Tab',
                component: 'test',
              },
            ],
          },
        ],
      },
    })

    await render(
      <div
        style={{
          width: '1024px',
          height: '768px',
          position: 'relative',
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}
      >
        <div
          className="shell-flexlayout bg-editor-border flex flex-1 flex-col gap-1 overflow-hidden p-1"
          data-testid="shell-container"
        >
          <Layout
            model={gridModel}
            factory={(node) => (
              <div data-testid={`content-${node.getId()}`}>
                Content for {node.getName()}
              </div>
            )}
          />
        </div>
      </div>,
    )

    await expect
      .poll(() => document.querySelectorAll('.flexlayout__tabset').length, {
        timeout: 5000,
      })
      .toBe(2)

    const shellContainer = document.querySelector(
      '[data-testid="shell-container"]',
    ) as HTMLElement
    const layoutElement = shellContainer.querySelector(
      '.flexlayout__layout',
    ) as HTMLElement
    const tabsets = document.querySelectorAll('.flexlayout__tabset')
    const leftTabset = tabsets[0] as HTMLElement
    const rightTabset = tabsets[1] as HTMLElement

    const containerRect = shellContainer.getBoundingClientRect()
    const layoutRect = layoutElement.getBoundingClientRect()
    const leftRect = leftTabset.getBoundingClientRect()
    const rightRect = rightTabset.getBoundingClientRect()

    const paddingLeft = layoutRect.left - containerRect.left
    const paddingRight = containerRect.right - layoutRect.right

    const horizontalPaddingDiff = Math.abs(paddingLeft - paddingRight)
    expect(horizontalPaddingDiff).toBeLessThanOrEqual(2)

    const leftTabsetLeftPadding = leftRect.left - layoutRect.left
    const rightTabsetRightPadding = layoutRect.right - rightRect.right

    const tabsetPaddingDiff = Math.abs(
      leftTabsetLeftPadding - rightTabsetRightPadding,
    )
    expect(tabsetPaddingDiff).toBeLessThanOrEqual(2)
  })

  it('grid mode tab bars do not overlap container boundaries', async () => {
    const { Layout, Model } = await import('@aptre/flex-layout')

    const gridModel = Model.fromJson({
      global: {
        splitterSize: 4,
        splitterExtra: 4,
        tabEnableClose: false,
      },
      layout: {
        type: 'row',
        weight: 100,
        children: [
          {
            type: 'tabset',
            id: 'left-tabset',
            weight: 50,
            children: [
              {
                type: 'tab',
                id: 'left-tab',
                name: 'Left Tab',
                component: 'test',
              },
            ],
          },
          {
            type: 'tabset',
            id: 'right-tabset',
            weight: 50,
            children: [
              {
                type: 'tab',
                id: 'right-tab',
                name: 'Right Tab',
                component: 'test',
              },
            ],
          },
        ],
      },
    })

    await render(
      <div
        style={{
          width: '1024px',
          height: '768px',
          position: 'relative',
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}
      >
        <div
          className="shell-flexlayout bg-editor-border flex flex-1 flex-col gap-1 overflow-hidden p-1"
          data-testid="shell-container"
        >
          <Layout
            model={gridModel}
            factory={(node) => (
              <div data-testid={`content-${node.getId()}`}>
                Content for {node.getName()}
              </div>
            )}
          />
        </div>
      </div>,
    )

    await expect
      .poll(() => document.querySelectorAll('.flexlayout__tabset').length, {
        timeout: 5000,
      })
      .toBe(2)

    const shellContainer = document.querySelector(
      '[data-testid="shell-container"]',
    ) as HTMLElement
    const layoutElement = shellContainer.querySelector(
      '.flexlayout__layout',
    ) as HTMLElement

    const containerRect = shellContainer.getBoundingClientRect()
    const layoutRect = layoutElement.getBoundingClientRect()

    const tabBars = document.querySelectorAll(
      '.flexlayout__tabset_tabbar_outer_top',
    )

    tabBars.forEach((tabBar) => {
      const tabBarRect = tabBar.getBoundingClientRect()

      expect(tabBarRect.top).toBeGreaterThanOrEqual(layoutRect.top - 1) // 1px tolerance

      expect(tabBarRect.top).toBeGreaterThanOrEqual(containerRect.top - 1)
    })
  })

  it('grid mode tabsets have proper border radius and spacing', async () => {
    const { Layout, Model } = await import('@aptre/flex-layout')

    const gridModel = Model.fromJson({
      global: {
        splitterSize: 4,
        splitterExtra: 4,
        tabEnableClose: false,
      },
      layout: {
        type: 'row',
        weight: 100,
        children: [
          {
            type: 'tabset',
            id: 'left-tabset',
            weight: 50,
            children: [
              {
                type: 'tab',
                id: 'left-tab',
                name: 'Left Tab',
                component: 'test',
              },
            ],
          },
          {
            type: 'tabset',
            id: 'right-tabset',
            weight: 50,
            children: [
              {
                type: 'tab',
                id: 'right-tab',
                name: 'Right Tab',
                component: 'test',
              },
            ],
          },
        ],
      },
    })

    await render(
      <div
        style={{
          width: '1024px',
          height: '768px',
          position: 'relative',
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}
      >
        <div className="shell-flexlayout bg-editor-border flex flex-1 flex-col gap-1 overflow-hidden p-1">
          <Layout
            model={gridModel}
            factory={(node) => (
              <div data-testid={`content-${node.getId()}`}>
                Content for {node.getName()}
              </div>
            )}
          />
        </div>
      </div>,
    )

    await expect
      .poll(() => document.querySelectorAll('.flexlayout__tabset').length, {
        timeout: 5000,
      })
      .toBe(2)

    const tabsets = document.querySelectorAll('.flexlayout__tabset')
    const leftTabset = tabsets[0] as HTMLElement
    const rightTabset = tabsets[1] as HTMLElement

    const leftRect = leftTabset.getBoundingClientRect()
    const rightRect = rightTabset.getBoundingClientRect()

    const gapBetweenTabsets = rightRect.left - leftRect.right

    expect(gapBetweenTabsets).toBeGreaterThanOrEqual(2)
    expect(gapBetweenTabsets).toBeLessThanOrEqual(10)

    const leftBorderRadius = window.getComputedStyle(leftTabset).borderRadius
    const rightBorderRadius = window.getComputedStyle(rightTabset).borderRadius

    // Border radius follows --radius-editor and need not have a fixed pixel value.
    expect(leftBorderRadius).not.toBe('0px')
    expect(rightBorderRadius).not.toBe('0px')
  })

  it('grid mode tabsets fill available height correctly', async () => {
    const { Layout, Model } = await import('@aptre/flex-layout')

    const gridModel = Model.fromJson({
      global: {
        splitterSize: 4,
        splitterExtra: 4,
        tabEnableClose: false,
      },
      layout: {
        type: 'row',
        weight: 100,
        children: [
          {
            type: 'tabset',
            id: 'left-tabset',
            weight: 50,
            children: [
              {
                type: 'tab',
                id: 'left-tab',
                name: 'Left Tab',
                component: 'test',
              },
            ],
          },
          {
            type: 'tabset',
            id: 'right-tabset',
            weight: 50,
            children: [
              {
                type: 'tab',
                id: 'right-tab',
                name: 'Right Tab',
                component: 'test',
              },
            ],
          },
        ],
      },
    })

    await render(
      <div
        style={{
          width: '1024px',
          height: '768px',
          position: 'relative',
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}
      >
        <div
          className="shell-flexlayout bg-editor-border flex flex-1 flex-col gap-1 overflow-hidden p-1"
          data-testid="shell-container"
        >
          <Layout
            model={gridModel}
            factory={(node) => (
              <div data-testid={`content-${node.getId()}`}>
                Content for {node.getName()}
              </div>
            )}
          />
        </div>
      </div>,
    )

    await expect
      .poll(() => document.querySelectorAll('.flexlayout__tabset').length, {
        timeout: 5000,
      })
      .toBe(2)

    const shellContainer = document.querySelector(
      '[data-testid="shell-container"]',
    ) as HTMLElement
    const layoutElement = shellContainer.querySelector(
      '.flexlayout__layout',
    ) as HTMLElement
    const tabsets = document.querySelectorAll('.flexlayout__tabset')

    const containerRect = shellContainer.getBoundingClientRect()
    const layoutRect = layoutElement.getBoundingClientRect()
    const leftRect = (tabsets[0] as HTMLElement).getBoundingClientRect()
    const rightRect = (tabsets[1] as HTMLElement).getBoundingClientRect()

    const layoutToContainerRatio = layoutRect.height / containerRect.height
    expect(layoutToContainerRatio).toBeGreaterThan(0.95)

    expect(Math.abs(leftRect.height - rightRect.height)).toBeLessThan(2)

    const tabsetToLayoutRatio = leftRect.height / layoutRect.height
    expect(tabsetToLayoutRatio).toBeGreaterThan(0.95)
  })

  it('single tabset mode has correct menu bar padding', async () => {
    const { OptimizedLayout, Model } = await import('@aptre/flex-layout')

    const singleModel = Model.fromJson({
      global: {
        tabEnableClose: false,
      },
      layout: {
        type: 'row',
        children: [
          {
            type: 'tabset',
            id: 'main-tabset',
            children: [
              { type: 'tab', id: 'tab1', name: 'Tab 1', component: 'test' },
              { type: 'tab', id: 'tab2', name: 'Tab 2', component: 'test' },
            ],
          },
        ],
      },
    })

    await render(
      <div
        style={{
          width: '1024px',
          height: '768px',
          position: 'relative',
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}
      >
        <div
          className="shell-flexlayout shell-flexlayout--with-menu bg-editor-border flex flex-1 flex-col gap-1 overflow-hidden p-1"
          style={{ '--menu-bar-width': '233px' }}
          data-testid="shell-container"
        >
          <OptimizedLayout
            model={singleModel}
            renderTab={(node) => (
              <div data-testid={`content-${node.getId()}`}>
                Content for {node.getName()}
              </div>
            )}
          />
        </div>
      </div>,
    )

    await expect
      .poll(() => document.querySelectorAll('.flexlayout__tabset').length, {
        timeout: 5000,
      })
      .toBe(1)

    const tabBar = document.querySelector(
      '.flexlayout__tabset_tabbar_outer_top',
    ) as HTMLElement

    if (tabBar) {
      const computedStyle = window.getComputedStyle(tabBar)
      const paddingLeft = computedStyle.paddingLeft

      // --menu-bar-width leaves more than 200px for the menu overlay.
      const paddingValue = parseFloat(paddingLeft)
      expect(paddingValue).toBeGreaterThan(200)
    }
  })

  it('grid mode vs single tabset mode have different styling', async () => {
    const { OptimizedLayout, Model } = await import('@aptre/flex-layout')

    const gridModel = Model.fromJson({
      global: { tabEnableClose: false },
      layout: {
        type: 'row',
        children: [
          {
            type: 'tabset',
            id: 'left-tabset',
            weight: 50,
            children: [
              { type: 'tab', id: 'left-tab', name: 'Left', component: 'test' },
            ],
          },
          {
            type: 'tabset',
            id: 'right-tabset',
            weight: 50,
            children: [
              {
                type: 'tab',
                id: 'right-tab',
                name: 'Right',
                component: 'test',
              },
            ],
          },
        ],
      },
    })

    const { unmount } = await render(
      <div
        style={{
          width: '1024px',
          height: '768px',
          position: 'relative',
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}
      >
        <div
          className="shell-flexlayout bg-editor-border flex flex-1 flex-col gap-1 overflow-hidden p-1"
          data-testid="shell-container"
        >
          <OptimizedLayout
            model={gridModel}
            renderTab={(node) => <div>Content for {node.getName()}</div>}
          />
        </div>
      </div>,
    )

    await expect
      .poll(() => document.querySelectorAll('.flexlayout__tabset').length, {
        timeout: 5000,
      })
      .toBe(2)

    const gridTabset = document.querySelector(
      '.flexlayout__tabset',
    ) as HTMLElement
    const gridTabsetStyle = window.getComputedStyle(gridTabset)
    const gridBorderRadius = gridTabsetStyle.borderRadius

    expect(gridBorderRadius).not.toBe('0px')

    await unmount()

    const singleModel = Model.fromJson({
      global: { tabEnableClose: false },
      layout: {
        type: 'row',
        children: [
          {
            type: 'tabset',
            id: 'main-tabset',
            children: [
              { type: 'tab', id: 'tab1', name: 'Tab 1', component: 'test' },
            ],
          },
        ],
      },
    })

    await render(
      <div
        style={{
          width: '1024px',
          height: '768px',
          position: 'relative',
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}
      >
        <div
          className="shell-flexlayout shell-flexlayout--with-menu bg-editor-border flex flex-1 flex-col gap-1 overflow-hidden p-1"
          data-testid="shell-container"
        >
          <OptimizedLayout
            model={singleModel}
            renderTab={(node) => <div>Content for {node.getName()}</div>}
          />
        </div>
      </div>,
    )

    await expect
      .poll(() => document.querySelectorAll('.flexlayout__tabset').length, {
        timeout: 5000,
      })
      .toBe(1)

    const singleTabset = document.querySelector(
      '.flexlayout__tabset',
    ) as HTMLElement
    const singleTabsetStyle = window.getComputedStyle(singleTabset)
    const singleBorderRadius = singleTabsetStyle.borderRadius

    expect(singleBorderRadius).toBe('0px')
  })
})
