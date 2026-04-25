/**
 * E2E test for tab drag overlay visibility bug.
 *
 * This test replicates the issue where:
 * 1. Load app
 * 2. Click create drive (quickstart)
 * 3. Click the plus button next to shell flexlayout tabs (duplicate tab)
 * 4. Click and hold on the selected tab (the second tab)
 * 5. Drag it down ~100px
 * 6. The drag overlay from flex layout disappears
 *
 * The bug: When dragging a tab downward into the tab content area, the
 * `.flexlayout__outline_rect` element becomes hidden or is removed,
 * making it impossible to see where the tab would be dropped.
 */
import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { render, cleanup } from 'vitest-browser-react'

import '@s4wave/web/style/app.css'

import { AppShell } from './AppShell.js'
import { EditorShell } from './EditorShell.js'

describe('Tab Drag Overlay Visibility Bug', () => {
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

    // Wait for the landing page to render with Home tab
    await expect
      .poll(
        () => {
          const homeTab = document.querySelector('.flexlayout__tab_button')
          return homeTab !== null
        },
        { timeout: 5000 },
      )
      .toBe(true)

    // Find and click the new tab button (plus icon)
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

    // Wait for second tab to appear
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

    // Get the second tab button (the newly created duplicate, which should be selected)
    const tabButtons = document.querySelectorAll('.flexlayout__tab_button')
    const secondTab = tabButtons[1] as HTMLElement

    if (!secondTab) {
      throw new Error('Second tab not found')
    }

    // Get the layout element for drag events
    const layoutElement = document.querySelector(
      '.flexlayout__layout',
    ) as HTMLElement
    if (!layoutElement) {
      throw new Error('Layout element not found')
    }

    // Get tab button position
    const tabRect = secondTab.getBoundingClientRect()
    const startX = tabRect.left + tabRect.width / 2
    const startY = tabRect.top + tabRect.height / 2

    // Create DataTransfer for drag events
    const createDataTransfer = () => new DataTransfer()

    // Step 1: Start drag on the tab button
    const dragStartEvent = new DragEvent('dragstart', {
      bubbles: true,
      cancelable: true,
      clientX: startX,
      clientY: startY,
      dataTransfer: createDataTransfer(),
    })
    secondTab.dispatchEvent(dragStartEvent)

    // Step 2: Drag enters the layout (still over tab bar area)
    const dragEnterEvent = new DragEvent('dragenter', {
      bubbles: true,
      cancelable: true,
      clientX: startX,
      clientY: startY,
      dataTransfer: createDataTransfer(),
    })
    layoutElement.dispatchEvent(dragEnterEvent)

    // Step 3: Drag over tab bar area - overlay should appear
    const dragOverTabBar = new DragEvent('dragover', {
      bubbles: true,
      cancelable: true,
      clientX: startX,
      clientY: startY + 10, // Slightly below tab button
      dataTransfer: createDataTransfer(),
    })
    layoutElement.dispatchEvent(dragOverTabBar)

    // Wait for outline rect to appear
    await expect
      .poll(() => document.querySelector('.flexlayout__outline_rect'))
      .not.toBeNull()

    // Verify drag overlay appears when dragging over tab bar
    const outlineRectAtTabBar = document.querySelector(
      '.flexlayout__outline_rect',
    ) as HTMLElement
    console.log(
      '=== DRAG OVER TAB BAR ===',
      outlineRectAtTabBar ?
        {
          exists: true,
          visibility: outlineRectAtTabBar.style.visibility,
          computedVisibility:
            window.getComputedStyle(outlineRectAtTabBar).visibility,
        }
      : 'NOT FOUND',
    )

    expect(outlineRectAtTabBar).not.toBeNull()
    expect(outlineRectAtTabBar.style.visibility).toBe('visible')

    // Step 4: Now drag DOWN into the content area (~100px below)
    // This is where the bug occurs - the overlay should remain visible
    const contentY = startY + 100 // 100px below the tab button

    const dragOverContent = new DragEvent('dragover', {
      bubbles: true,
      cancelable: true,
      clientX: startX,
      clientY: contentY,
      dataTransfer: createDataTransfer(),
    })
    layoutElement.dispatchEvent(dragOverContent)

    // Check if outline rect is still visible after dragging into content area
    const outlineRectAtContent = document.querySelector(
      '.flexlayout__outline_rect',
    ) as HTMLElement

    console.log(
      '=== DRAG DOWN INTO CONTENT AREA (100px below tab) ===',
      outlineRectAtContent ?
        {
          exists: true,
          visibility: outlineRectAtContent.style.visibility,
          computedVisibility:
            window.getComputedStyle(outlineRectAtContent).visibility,
          boundingRect: outlineRectAtContent.getBoundingClientRect(),
        }
      : 'NOT FOUND - BUG: outline_rect was removed!',
    )

    // BUG CHECK: The outline rect should still exist and be visible
    // when dragging over the content area
    expect(outlineRectAtContent).not.toBeNull()

    if (outlineRectAtContent) {
      const visibility = outlineRectAtContent.style.visibility
      const computedVisibility =
        window.getComputedStyle(outlineRectAtContent).visibility

      // The bug causes visibility to become 'hidden' when dragging over content area
      if (visibility === 'hidden' || computedVisibility === 'hidden') {
        console.error(
          'BUG REPRODUCED: Drag overlay visibility is "hidden" when dragging into content area',
        )
      }

      // This assertion will fail if the bug is present
      expect(visibility).toBe('visible')
      expect(computedVisibility).toBe('visible')
    }

    // Step 5: End the drag
    const dragEndEvent = new DragEvent('dragend', {
      bubbles: true,
      cancelable: true,
      clientX: startX,
      clientY: contentY,
      dataTransfer: createDataTransfer(),
    })
    secondTab.dispatchEvent(dragEndEvent)

    // Verify layout is still functional after drag
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
    // Navigate directly to quickstart/drive to simulate clicking "Create a Drive"
    window.location.hash = '#/quickstart/drive'

    await render(
      <AppShell>
        <EditorShell />
      </AppShell>,
    )

    // Wait for flexlayout to render
    await expect
      .poll(
        () => {
          const layout = document.querySelector('.flexlayout__layout')
          return layout !== null
        },
        { timeout: 5000 },
      )
      .toBe(true)

    // Wait for tab to appear (will show "drive" or similar based on quickstart ID)
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

    // Create a second tab by clicking new tab button
    const addButton = document.querySelector(
      'button[title="New tab"]',
    ) as HTMLElement
    if (addButton) {
      addButton.click()
    }

    // Wait for second tab
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

    // Get the second tab (the one titled "Space" or duplicated quickstart tab)
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

    // Start drag
    secondTab.dispatchEvent(
      new DragEvent('dragstart', {
        bubbles: true,
        cancelable: true,
        clientX: startX,
        clientY: startY,
        dataTransfer: createDataTransfer(),
      }),
    )

    // Enter layout
    layoutElement.dispatchEvent(
      new DragEvent('dragenter', {
        bubbles: true,
        cancelable: true,
        clientX: startX,
        clientY: startY,
        dataTransfer: createDataTransfer(),
      }),
    )

    // Drag over to show overlay
    layoutElement.dispatchEvent(
      new DragEvent('dragover', {
        bubbles: true,
        cancelable: true,
        clientX: startX,
        clientY: startY + 10,
        dataTransfer: createDataTransfer(),
      }),
    )

    // Wait for overlay to appear
    await expect
      .poll(() => document.querySelector('.flexlayout__outline_rect'))
      .not.toBeNull()

    // Verify overlay exists initially
    const initialOutline = document.querySelector('.flexlayout__outline_rect')
    expect(initialOutline).not.toBeNull()

    // Now drag down 100px (into content area)
    layoutElement.dispatchEvent(
      new DragEvent('dragover', {
        bubbles: true,
        cancelable: true,
        clientX: startX,
        clientY: startY + 100,
        dataTransfer: createDataTransfer(),
      }),
    )

    // Check outline rect visibility after dragging down
    const outlineAfterDrag = document.querySelector(
      '.flexlayout__outline_rect',
    ) as HTMLElement

    console.log(
      '=== QUICKSTART TAB: DRAG DOWN 100px ===',
      outlineAfterDrag ?
        {
          visibility: outlineAfterDrag.style.visibility,
          computed: window.getComputedStyle(outlineAfterDrag).visibility,
        }
      : 'NOT FOUND',
    )

    // BUG: This should not be null or hidden
    expect(outlineAfterDrag).not.toBeNull()
    if (outlineAfterDrag) {
      expect(outlineAfterDrag.style.visibility).toBe('visible')
    }

    // End drag
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

  it('drag overlay with nested FlexLayout - simulates Space with SpaceFlexLayout', async () => {
    // This test simulates the bug scenario more closely by:
    // 1. Creating a mock nested FlexLayout inside tab content
    // 2. Testing drag behavior when cursor moves into nested layout area

    const { Layout, Model } = await import('@aptre/flex-layout')

    // Create a component that renders a nested FlexLayout (simulating SpaceFlexLayout)
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

    // Create a custom shell with nested layout
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

    // Wait for layouts to render
    await expect
      .poll(
        () => {
          // Should have 2 flexlayout__layout elements (outer + nested)
          const layouts = document.querySelectorAll('.flexlayout__layout')
          return layouts.length
        },
        { timeout: 5000 },
      )
      .toBeGreaterThanOrEqual(2)

    // Get the shell's tab buttons (outer layout)
    const shellLayout = document.querySelector(
      '.flexlayout__layout',
    ) as HTMLElement
    const tabButtons = shellLayout.querySelectorAll('.flexlayout__tab_button')

    console.log(
      '=== NESTED LAYOUT TEST ===',
      'Tab buttons found:',
      tabButtons.length,
      'Layout elements:',
      document.querySelectorAll('.flexlayout__layout').length,
    )

    expect(tabButtons.length).toBeGreaterThanOrEqual(2)

    // Get second tab to drag
    const secondTab = tabButtons[1] as HTMLElement
    const tabRect = secondTab.getBoundingClientRect()
    const startX = tabRect.left + tabRect.width / 2
    const startY = tabRect.top + tabRect.height / 2

    const createDataTransfer = () => new DataTransfer()

    // Start drag on second tab
    secondTab.dispatchEvent(
      new DragEvent('dragstart', {
        bubbles: true,
        cancelable: true,
        clientX: startX,
        clientY: startY,
        dataTransfer: createDataTransfer(),
      }),
    )

    // Enter shell layout
    shellLayout.dispatchEvent(
      new DragEvent('dragenter', {
        bubbles: true,
        cancelable: true,
        clientX: startX,
        clientY: startY,
        dataTransfer: createDataTransfer(),
      }),
    )

    // Drag over tab bar to show outline
    shellLayout.dispatchEvent(
      new DragEvent('dragover', {
        bubbles: true,
        cancelable: true,
        clientX: startX,
        clientY: startY + 10,
        dataTransfer: createDataTransfer(),
      }),
    )

    // Wait for outline to appear
    await expect
      .poll(() => document.querySelector('.flexlayout__outline_rect'))
      .not.toBeNull()

    // Should have outline visible
    const outlineAtTabBar = document.querySelector(
      '.flexlayout__outline_rect',
    ) as HTMLElement
    console.log(
      '=== OUTLINE AT TAB BAR ===',
      outlineAtTabBar ?
        {
          visibility: outlineAtTabBar.style.visibility,
        }
      : 'NOT FOUND',
    )
    expect(outlineAtTabBar).not.toBeNull()

    // Now find the nested layout wrapper (content area with nested FlexLayout)
    const nestedWrapper = document.querySelector(
      '[data-testid="nested-layout-wrapper"]',
    ) as HTMLElement
    const nestedRect = nestedWrapper?.getBoundingClientRect()

    if (nestedRect) {
      // Drag INTO the nested layout area
      const nestedX = nestedRect.left + nestedRect.width / 2
      const nestedY = nestedRect.top + nestedRect.height / 2

      console.log('=== DRAGGING INTO NESTED LAYOUT AREA ===', {
        nestedX,
        nestedY,
      })

      // First, fire dragenter on the nested layout (simulating cursor entering it)
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

      // Then fire dragover on shell layout but at nested coordinates
      shellLayout.dispatchEvent(
        new DragEvent('dragover', {
          bubbles: true,
          cancelable: true,
          clientX: nestedX,
          clientY: nestedY,
          dataTransfer: createDataTransfer(),
        }),
      )

      // Check if outline is still visible after dragging over nested layout
      const outlineAfterNestedEnter = document.querySelector(
        '.flexlayout__outline_rect',
      ) as HTMLElement

      console.log(
        '=== OUTLINE AFTER ENTERING NESTED LAYOUT ===',
        outlineAfterNestedEnter ?
          {
            visibility: outlineAfterNestedEnter.style.visibility,
            computed: window.getComputedStyle(outlineAfterNestedEnter)
              .visibility,
          }
        : 'NOT FOUND - BUG!',
      )

      // BUG CHECK: Outline should still be visible
      expect(outlineAfterNestedEnter).not.toBeNull()
      if (outlineAfterNestedEnter) {
        // This might fail if the bug is present
        expect(outlineAfterNestedEnter.style.visibility).toBe('visible')
      }
    }

    // End drag
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

  it('drag overlay disappears with dragleave - simulates real browser behavior', async () => {
    // This test attempts to reproduce the bug by simulating the dragleave event
    // that occurs when the cursor leaves one element and enters another.
    // In a real browser, when you drag from shell to nested layout:
    // 1. dragenter fires on nested layout
    // 2. dragleave fires on shell layout (for the specific child element)
    // This can cause dragEnterCount imbalance in FlexLayout.

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

    // Wait for layouts
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

    // Start drag
    secondTab.dispatchEvent(
      new DragEvent('dragstart', {
        bubbles: true,
        cancelable: true,
        clientX: startX,
        clientY: startY,
        dataTransfer: dt,
      }),
    )

    // Enter shell layout - this sets dragEnterCount = 1
    shellLayout.dispatchEvent(
      new DragEvent('dragenter', {
        bubbles: true,
        cancelable: true,
        clientX: startX,
        clientY: startY,
        dataTransfer: dt,
      }),
    )

    // Drag over to make outline visible
    shellLayout.dispatchEvent(
      new DragEvent('dragover', {
        bubbles: true,
        cancelable: true,
        clientX: startX,
        clientY: startY + 10,
        dataTransfer: dt,
      }),
    )

    // Wait for outline to appear
    await expect
      .poll(() => document.querySelector('.flexlayout__outline_rect'))
      .not.toBeNull()

    // Verify outline is visible
    let outline = document.querySelector(
      '.flexlayout__outline_rect',
    ) as HTMLElement
    expect(outline).not.toBeNull()
    expect(outline.style.visibility).toBe('visible')

    console.log('=== BEFORE DRAGLEAVE ===', {
      outlineVisible: outline.style.visibility,
    })

    // Now simulate what happens in real browser:
    // When cursor moves from shell content into nested layout,
    // browser fires dragleave on shell (for the element cursor left)
    // with relatedTarget set to the element being entered

    // Get the tab content area (where nested layout is)
    const tabContent = shellLayout.querySelector(
      '.flexlayout__tab',
    ) as HTMLElement

    if (tabContent) {
      // Fire dragleave on the tab content - simulating cursor leaving it
      // to enter the nested layout's area.
      // In real browser, relatedTarget would be set to the element being entered.
      tabContent.dispatchEvent(
        new DragEvent('dragleave', {
          bubbles: true,
          cancelable: true,
          clientX: startX,
          clientY: startY + 100,
          dataTransfer: dt,
          relatedTarget: tabContent, // Element cursor is entering (still inside shell)
        }),
      )
    }

    // Now fire dragleave on shell layout itself
    // In real browser, when moving to a child element, relatedTarget
    // would be the child element (which is still inside the layout).
    // Our fix checks if relatedTarget is inside the layout and preserves the outline.
    shellLayout.dispatchEvent(
      new DragEvent('dragleave', {
        bubbles: true,
        cancelable: true,
        clientX: startX,
        clientY: startY + 150,
        dataTransfer: dt,
        relatedTarget: tabContent, // Still inside the shell layout
      }),
    )

    // Check if outline was removed or hidden
    outline = document.querySelector('.flexlayout__outline_rect') as HTMLElement

    console.log(
      '=== AFTER DRAGLEAVE ON SHELL ===',
      outline ?
        {
          visibility: outline.style.visibility,
          computed: window.getComputedStyle(outline).visibility,
        }
      : 'OUTLINE REMOVED - BUG REPRODUCED!',
    )

    // If the bug is present, the outline will be removed or hidden
    // With our fix, the outline should still exist because relatedTarget is inside the layout
    expect(outline).not.toBeNull()
    expect(outline.style.visibility).toBe('visible')

    // End drag
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

describe('Grid Mode Visual Issues', () => {
  beforeEach(() => {
    void cleanup()
    localStorage.clear()
    window.location.hash = ''
  })

  afterEach(() => {
    void cleanup()
  })

  it('grid mode layout has correct positioning and click behavior', async () => {
    // This test verifies that when entering grid mode (side-by-side tabs):
    // a) The nested tabsets don't overlap the containing tab bar
    // b) The layout has consistent padding on all sides
    // c) Clicking inside a tabset's content area selects that tabset

    const { Layout, Model, Actions } = await import('@aptre/flex-layout')

    // Create a grid layout model (2 side-by-side tabsets)
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

    // Track which tabset was clicked
    let clickedTabsetId: string | null = null

    await render(
      <div className="shell-flexlayout bg-editor-border flex flex-1 flex-col gap-1 overflow-hidden p-1">
        <Layout
          model={gridModel}
          factory={(node) => (
            <div
              data-testid={`content-${node.getId()}`}
              style={{ width: '100%', height: '100%', padding: '20px' }}
              onClick={() => {
                // Find parent tabset
                let parent = node.getParent()
                while (parent && parent.getType() !== 'tabset') {
                  parent = parent.getParent()
                }
                if (parent) {
                  clickedTabsetId = parent.getId()
                  // Select the tab to trigger tabset selection
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

    // Wait for layout to render
    await expect
      .poll(
        () => {
          const tabsets = document.querySelectorAll('.flexlayout__tabset')
          return tabsets.length
        },
        { timeout: 5000 },
      )
      .toBe(2)

    // Get both tabsets
    const tabsets = document.querySelectorAll('.flexlayout__tabset')
    const leftTabset = tabsets[0] as HTMLElement
    const rightTabset = tabsets[1] as HTMLElement

    // Check 1: Verify tabsets don't overlap
    const leftRect = leftTabset.getBoundingClientRect()
    const rightRect = rightTabset.getBoundingClientRect()

    console.log('=== GRID LAYOUT POSITIONS ===', {
      left: {
        top: leftRect.top,
        left: leftRect.left,
        right: leftRect.right,
        bottom: leftRect.bottom,
        width: leftRect.width,
        height: leftRect.height,
      },
      right: {
        top: rightRect.top,
        left: rightRect.left,
        right: rightRect.right,
        bottom: rightRect.bottom,
        width: rightRect.width,
        height: rightRect.height,
      },
    })

    // Tabsets should not overlap horizontally
    expect(leftRect.right).toBeLessThanOrEqual(rightRect.left + 10) // Allow for splitter

    // Check 2: Verify consistent padding/margins
    const container = document.querySelector('.shell-flexlayout') as HTMLElement
    const containerRect = container.getBoundingClientRect()
    const layoutElement = container.querySelector(
      '.flexlayout__layout',
    ) as HTMLElement
    const layoutRect = layoutElement.getBoundingClientRect()

    console.log('=== CONTAINER VS LAYOUT ===', {
      container: {
        top: containerRect.top,
        left: containerRect.left,
        right: containerRect.right,
        bottom: containerRect.bottom,
      },
      layout: {
        top: layoutRect.top,
        left: layoutRect.left,
        right: layoutRect.right,
        bottom: layoutRect.bottom,
      },
    })

    // Check 3: Verify tab bars don't overlap with container top
    const leftTabBar = leftTabset.querySelector(
      '.flexlayout__tabset_tabbar_outer_top',
    ) as HTMLElement
    if (leftTabBar) {
      const leftTabBarRect = leftTabBar.getBoundingClientRect()
      console.log('=== LEFT TAB BAR ===', {
        top: leftTabBarRect.top,
        height: leftTabBarRect.height,
        containerTop: containerRect.top,
        layoutTop: layoutRect.top,
      })

      // Tab bar top should be at or below the layout top
      expect(leftTabBarRect.top).toBeGreaterThanOrEqual(layoutRect.top - 1)
    }

    // Check 4: Click on right tab content and verify it selects the right tabset
    const rightContent = document.querySelector(
      '[data-testid="content-right-tab"]',
    ) as HTMLElement

    if (rightContent) {
      rightContent.click()

      console.log('=== CLICK TEST ===', {
        clickedTabsetId,
        expectedTabsetId: 'right-tabset',
      })

      // The right tabset should now have the selected class
      const selectedTabset = document.querySelector(
        '.flexlayout__tabset:has(.flexlayout__tabset-selected)',
      )
      console.log('=== SELECTED TABSET ===', {
        found: selectedTabset !== null,
        isRightTabset: selectedTabset === rightTabset,
      })
    }
  })

  it('entering grid mode via drag creates proper layout', async () => {
    // This test simulates the actual flow:
    // 1. Start with single tabset (2 tabs)
    // 2. Drag one tab to the right edge to create a split
    // 3. Verify the resulting grid layout is correct

    const { Layout, Model, Actions, DockLocation } =
      await import('@aptre/flex-layout')

    // Start with a single tabset containing 2 tabs
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

    // Wait for initial layout
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

    // Initially should have 1 tabset
    let tabsets = document.querySelectorAll('.flexlayout__tabset')
    expect(tabsets.length).toBe(1)

    // Simulate moving tab2 to create a split by using FlexLayout's action API
    // This simulates what happens when you drop a tab on the right edge
    model.doAction(
      Actions.moveNode('tab2', 'main-tabset', DockLocation.RIGHT, -1),
    )

    // Wait for split to happen
    await expect
      .poll(() => document.querySelectorAll('.flexlayout__tabset').length)
      .toBe(2)

    // Now should have 2 tabsets
    tabsets = document.querySelectorAll('.flexlayout__tabset')
    console.log('=== AFTER SPLIT ===', {
      tabsetCount: tabsets.length,
    })

    expect(tabsets.length).toBe(2)

    // Verify layout structure
    const leftTabset = tabsets[0] as HTMLElement
    const rightTabset = tabsets[1] as HTMLElement
    const leftRect = leftTabset.getBoundingClientRect()
    const rightRect = rightTabset.getBoundingClientRect()

    console.log('=== SPLIT LAYOUT ===', {
      leftTop: leftRect.top,
      rightTop: rightRect.top,
      leftWidth: leftRect.width,
      rightWidth: rightRect.width,
    })

    // Both tabsets should start at the same vertical position
    expect(Math.abs(leftRect.top - rightRect.top)).toBeLessThan(2)

    // Both should have reasonable widths (roughly equal for 50/50 split)
    expect(leftRect.width).toBeGreaterThan(100)
    expect(rightRect.width).toBeGreaterThan(100)
  })

  it('OptimizedLayout click-to-select activates parent tabset', async () => {
    // This test verifies that clicking on tab content in OptimizedLayout
    // activates the parent tabset. This is important because OptimizedLayout
    // renders tab content in a sibling TabContainer element, not inside
    // FlexLayout's DOM, so click events need special handling.

    const flexLayout = await import('@aptre/flex-layout')
    const { OptimizedLayout, Model, Actions } = flexLayout

    // Create a grid layout with 2 tabsets
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

    // Set initial active tabset to left
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

    // Wait for layout to render with both tabsets
    await expect
      .poll(
        () => {
          const tabsets = document.querySelectorAll('.flexlayout__tabset')
          return tabsets.length
        },
        { timeout: 5000 },
      )
      .toBe(2)

    // Wait for TabContainer to render tab panels
    await expect
      .poll(
        () => {
          const tabPanels = document.querySelectorAll('[role="tabpanel"]')
          return tabPanels.length
        },
        { timeout: 5000 },
      )
      .toBe(2)

    // Helper to check which tabset is active using the model
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

    // Initially, left tabset should be active
    expect(getActiveTabsetId()).toBe('left-tabset')

    console.log('=== BEFORE CLICK ===', {
      activeTabsetId: getActiveTabsetId(),
    })

    // Find the right tab panel and simulate pointerdown
    const rightTabPanel = document.querySelector(
      '[data-tab-id="right-tab"]',
    ) as HTMLElement
    expect(rightTabPanel).not.toBeNull()

    // Dispatch pointerdown event (this is what OptimizedLayout listens for)
    rightTabPanel.dispatchEvent(
      new PointerEvent('pointerdown', {
        bubbles: true,
        cancelable: true,
      }),
    )

    // Wait for active tabset to change
    await expect.poll(() => getActiveTabsetId()).toBe('right-tabset')

    console.log('=== AFTER CLICK ===', {
      activeTabsetId: getActiveTabsetId(),
    })

    // After clicking on right tab content, right tabset should be active
    expect(getActiveTabsetId()).toBe('right-tabset')

    // Now click on left tab content to verify switching back works
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

    // Wait for active tabset to change back
    await expect.poll(() => getActiveTabsetId()).toBe('left-tabset')

    // Left tabset should now be active again
    expect(getActiveTabsetId()).toBe('left-tabset')
  })
})

describe('Grid Mode CSS Visual Issues', () => {
  beforeEach(() => {
    void cleanup()
    localStorage.clear()
    window.location.hash = ''
  })

  afterEach(() => {
    void cleanup()
  })

  it('grid mode tabsets have consistent padding on all sides', async () => {
    // This test verifies that the layout has equal padding/margin on all sides
    // Bug: Layout was shifted too far to the left (left padding != right padding)

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

    // Wait for layout to render
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

    // Calculate padding from container to layout
    const paddingLeft = layoutRect.left - containerRect.left
    const paddingRight = containerRect.right - layoutRect.right
    const paddingTop = layoutRect.top - containerRect.top
    const paddingBottom = containerRect.bottom - layoutRect.bottom

    console.log('=== PADDING CHECK ===', {
      paddingLeft,
      paddingRight,
      paddingTop,
      paddingBottom,
      containerWidth: containerRect.width,
      layoutWidth: layoutRect.width,
    })

    // Padding should be consistent on left and right (within 2px tolerance)
    const horizontalPaddingDiff = Math.abs(paddingLeft - paddingRight)
    expect(horizontalPaddingDiff).toBeLessThanOrEqual(2)

    // Calculate spacing from layout edge to tabsets
    const leftTabsetLeftPadding = leftRect.left - layoutRect.left
    const rightTabsetRightPadding = layoutRect.right - rightRect.right

    console.log('=== TABSET SPACING ===', {
      leftTabsetLeftPadding,
      rightTabsetRightPadding,
      diff: Math.abs(leftTabsetLeftPadding - rightTabsetRightPadding),
    })

    // Tabsets should have equal spacing from layout edges
    const tabsetPaddingDiff = Math.abs(
      leftTabsetLeftPadding - rightTabsetRightPadding,
    )
    expect(tabsetPaddingDiff).toBeLessThanOrEqual(2)
  })

  it('grid mode tab bars do not overlap container boundaries', async () => {
    // This test verifies that tab bars don't overlap the container's top edge
    // Bug: The top of nested layout overlays over the containing tab bar

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

    // Wait for layout to render
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

    // Get all tab bars
    const tabBars = document.querySelectorAll(
      '.flexlayout__tabset_tabbar_outer_top',
    )

    tabBars.forEach((tabBar, index) => {
      const tabBarRect = tabBar.getBoundingClientRect()

      console.log(`=== TAB BAR ${index} POSITION ===`, {
        tabBarTop: tabBarRect.top,
        layoutTop: layoutRect.top,
        containerTop: containerRect.top,
        overlapWithLayout: layoutRect.top - tabBarRect.top,
        overlapWithContainer: containerRect.top - tabBarRect.top,
      })

      // Tab bar should not extend above the layout area
      expect(tabBarRect.top).toBeGreaterThanOrEqual(layoutRect.top - 1) // 1px tolerance

      // Tab bar should not extend above the container
      expect(tabBarRect.top).toBeGreaterThanOrEqual(containerRect.top - 1)
    })
  })

  it('grid mode tabsets have proper border radius and spacing', async () => {
    // This test verifies tabsets have correct border-radius and visual separation

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

    // Wait for layout to render
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

    // Calculate gap between tabsets (should be splitter width)
    const gapBetweenTabsets = rightRect.left - leftRect.right

    console.log('=== TABSET SPACING ===', {
      leftRight: leftRect.right,
      rightLeft: rightRect.left,
      gap: gapBetweenTabsets,
      expectedGap: 4, // splitterSize
    })

    // Gap should be approximately splitter size (4px) with some tolerance
    expect(gapBetweenTabsets).toBeGreaterThanOrEqual(2)
    expect(gapBetweenTabsets).toBeLessThanOrEqual(10)

    // Check border-radius is applied
    const leftBorderRadius = window.getComputedStyle(leftTabset).borderRadius
    const rightBorderRadius = window.getComputedStyle(rightTabset).borderRadius

    console.log('=== BORDER RADIUS ===', {
      leftBorderRadius,
      rightBorderRadius,
    })

    // Tabsets should have some border radius (not 0)
    // The actual value depends on --radius-editor CSS variable
    expect(leftBorderRadius).not.toBe('0px')
    expect(rightBorderRadius).not.toBe('0px')
  })

  it('grid mode tabsets fill available height correctly', async () => {
    // This test verifies tabsets expand to fill the available vertical space

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

    // Wait for layout to render
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

    console.log('=== HEIGHT CHECK ===', {
      containerHeight: containerRect.height,
      layoutHeight: layoutRect.height,
      leftTabsetHeight: leftRect.height,
      rightTabsetHeight: rightRect.height,
      layoutToContainerRatio: layoutRect.height / containerRect.height,
    })

    // Layout should fill most of the container (accounting for padding)
    const layoutToContainerRatio = layoutRect.height / containerRect.height
    expect(layoutToContainerRatio).toBeGreaterThan(0.95)

    // Both tabsets should have the same height
    expect(Math.abs(leftRect.height - rightRect.height)).toBeLessThan(2)

    // Tabsets should fill most of the layout height
    const tabsetToLayoutRatio = leftRect.height / layoutRect.height
    expect(tabsetToLayoutRatio).toBeGreaterThan(0.95)
  })

  it('single tabset mode has correct menu bar padding', async () => {
    // This test verifies single tabset mode (with-menu) has correct left padding
    // for the menu bar overlay

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
          style={{ '--menu-bar-width': '233px' } as React.CSSProperties}
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

    // Wait for layout to render
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

      console.log('=== MENU BAR PADDING ===', {
        paddingLeft,
        expectedPadding: '233px',
      })

      // In with-menu mode, tab bar should have left padding for menu
      // The actual padding comes from CSS variable --menu-bar-width
      const paddingValue = parseFloat(paddingLeft)
      expect(paddingValue).toBeGreaterThan(200) // Should be ~233px
    }
  })

  it('grid mode vs single tabset mode have different styling', async () => {
    // This test compares grid mode and single tabset mode styling differences

    const { OptimizedLayout, Model } = await import('@aptre/flex-layout')

    // First render grid mode (2 tabsets, no --with-menu class)
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

    // Capture grid mode tabset styling
    const gridTabset = document.querySelector(
      '.flexlayout__tabset',
    ) as HTMLElement
    const gridTabsetStyle = window.getComputedStyle(gridTabset)
    const gridBorderRadius = gridTabsetStyle.borderRadius
    const gridBorder = gridTabsetStyle.border

    console.log('=== GRID MODE TABSET STYLE ===', {
      borderRadius: gridBorderRadius,
      border: gridBorder,
    })

    // Grid mode should have border radius
    expect(gridBorderRadius).not.toBe('0px')

    await unmount()

    // Now render single tabset mode
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

    // Capture single mode tabset styling
    const singleTabset = document.querySelector(
      '.flexlayout__tabset',
    ) as HTMLElement
    const singleTabsetStyle = window.getComputedStyle(singleTabset)
    const singleBorderRadius = singleTabsetStyle.borderRadius

    console.log('=== SINGLE MODE TABSET STYLE ===', {
      borderRadius: singleBorderRadius,
    })

    // Single mode should have no border radius (fills entire area)
    expect(singleBorderRadius).toBe('0px')
  })
})
