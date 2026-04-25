/**
 * E2E tests for BaseLayout component.
 *
 * These tests verify the integration between BaseLayout and the Go backend
 * via the LayoutHost RPC service.
 *
 * To run these tests:
 * 1. Start the Go test server (provides VITE_E2E_SERVER_PORT)
 * 2. Run: yarn test:browser
 */
import { describe, it, expect, beforeAll, afterAll, beforeEach } from 'vitest'
import { page } from 'vitest/browser'
import { render, cleanup } from 'vitest-browser-react'
import { createRef } from 'react'
import { Actions, DockLocation, type IJsonModel } from '@aptre/flex-layout'

// Import CSS for proper layout sizing
import '@s4wave/web/style/app.css'

import { BaseLayout, IBaseLayoutProps } from './BaseLayout.js'
import { BaseLayoutContextProvider } from './BaseLayoutContext.js'
import {
  E2ETestClient,
  createE2EClient,
  getTestServerPort,
} from '@s4wave/web/test/e2e-client.js'
import { jsonModelToLayoutModel } from '@s4wave/sdk/layout/layout.js'
import type { LayoutHost } from '@s4wave/sdk/layout/layout_srpc.pb.js'
import type {
  LayoutModel,
  TabDef,
  WatchLayoutModelRequest,
} from '@s4wave/sdk/layout/layout.pb.js'
import { BASE_MODEL } from './layout.js'

// Test wrapper component that provides necessary context
function TestBaseLayout(
  props: Omit<IBaseLayoutProps, 'layoutHost'> & { layoutHost: LayoutHost },
) {
  return (
    <BaseLayoutContextProvider>
      <div
        style={{
          width: '800px',
          height: '600px',
          position: 'relative',
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}
      >
        <BaseLayout {...props} />
      </div>
    </BaseLayoutContextProvider>
  )
}

function buildStaticLayoutHost(layoutModel: LayoutModel): LayoutHost {
  const stream: AsyncIterable<LayoutModel> = {
    [Symbol.asyncIterator]() {
      let done = false
      return {
        next: (): Promise<IteratorResult<LayoutModel>> => {
          if (done) {
            return Promise.resolve({ value: undefined as never, done: true })
          }
          done = true
          return Promise.resolve({ value: layoutModel, done: false })
        },
      }
    },
  }

  return {
    WatchLayoutModel: () => stream as never,
    NavigateTab: () => Promise.resolve({}),
    AddTab: (request) =>
      Promise.resolve({
        tabId: request.tab?.id ?? '',
      }),
  }
}

function buildSingleTabLayoutModel(): LayoutModel {
  const modelJson: IJsonModel = {
    ...BASE_MODEL,
    layout: {
      type: 'row',
      children: [
        {
          type: 'tabset',
          id: 'tabset-1',
          children: [{ type: 'tab', id: 'tab-1', name: 'Tab 1' }],
        },
      ],
    },
  }
  return jsonModelToLayoutModel(modelJson, {})
}

function createAsyncQueue<T>() {
  const queue: T[] = []
  let resolve: ((result: IteratorResult<T>) => void) | null = null
  let done = false

  const push = (value: T) => {
    if (done) return
    if (resolve) {
      const r = resolve
      resolve = null
      r({ value, done: false })
      return
    }
    queue.push(value)
  }

  const end = () => {
    done = true
    if (resolve) {
      const r = resolve
      resolve = null
      r({ value: undefined as never, done: true })
    }
  }

  const iterable: AsyncIterable<T> = {
    [Symbol.asyncIterator]() {
      return {
        next(): Promise<IteratorResult<T>> {
          if (queue.length > 0) {
            return Promise.resolve({ value: queue.shift()!, done: false })
          }
          if (done) {
            return Promise.resolve({ value: undefined as never, done: true })
          }
          return new Promise<IteratorResult<T>>((r) => {
            resolve = r
          })
        },
      }
    },
  }

  return { push, end, iterable }
}

function buildRecordingLayoutHost(initialLayoutModel: LayoutModel) {
  const responses = createAsyncQueue<LayoutModel>()
  const requests: WatchLayoutModelRequest[] = []
  responses.push(initialLayoutModel)

  const host: LayoutHost = {
    WatchLayoutModel(requestStream) {
      void (async () => {
        for await (const request of requestStream as AsyncIterable<WatchLayoutModelRequest>) {
          requests.push(request)
        }
      })()
      return responses.iterable as never
    },
    NavigateTab: () => Promise.resolve({}),
    AddTab: (request) =>
      Promise.resolve({
        tabId: request.tab?.id ?? '',
      }),
  }

  return { host, requests, pushResponse: responses.push, end: responses.end }
}

function findTabDefById(
  layoutModel: LayoutModel,
  tabID: string,
): TabDef | undefined {
  const visit = (
    defs:
      | Array<{
          node?:
            | { case?: 'row'; value?: { children?: unknown[] } }
            | { case?: 'tabSet'; value?: { children?: TabDef[] } }
        }>
      | undefined,
  ): TabDef | undefined => {
    for (const def of defs ?? []) {
      const node = def.node
      if (node?.case === 'tabSet') {
        const tabSet = node.value
        const tab = tabSet?.children?.find((child) => child.id === tabID)
        if (tab) return tab
      }
      if (node?.case === 'row') {
        const rowChildren = node.value?.children as Parameters<typeof visit>[0]
        const tab = visit(rowChildren)
        if (tab) return tab
      }
    }
    return undefined
  }

  return visit(layoutModel.layout?.children as Parameters<typeof visit>[0])
}

describe('BaseLayout E2E', () => {
  let client: E2ETestClient

  beforeAll(async () => {
    // Get the test server port from environment
    let port: number
    try {
      port = getTestServerPort()
    } catch {
      // For development, skip if no server is running
      console.warn('Skipping E2E tests: no test server available')
      return
    }

    // Connect to the test server
    client = await createE2EClient(port)
  })

  afterAll(() => {
    if (client) {
      client.disconnect()
    }
  })

  beforeEach(() => {
    void cleanup()
  })

  it('connects to the test server', () => {
    if (!client) {
      return // Skip if no server
    }

    expect(client.isConnected()).toBe(true)
  })

  it('renders loading state initially when no model available', async () => {
    if (!client) {
      return
    }

    const layoutHost = client.getLayoutHost()

    await render(
      <TestBaseLayout
        layoutHost={layoutHost}
        renderTab={({ tabID }) => (
          <div data-testid={`tab-${tabID}`}>Tab {tabID}</div>
        )}
      />,
    )

    // The initial render should either show loading or the layout
    // depending on how quickly the server responds
    // Just verify the component renders without error
    await expect.element(page.getByText(/Tab|loading/i)).toBeInTheDocument()
  })

  it('loads initial model from server and renders layout', async () => {
    if (!client) {
      return
    }

    const layoutHost = client.getLayoutHost()

    await render(
      <TestBaseLayout
        layoutHost={layoutHost}
        renderTab={({ tabID }) => (
          <div data-testid={`tab-content-${tabID}`}>Content for {tabID}</div>
        )}
      />,
    )

    // Wait for the tab content to render
    await expect
      .poll(() => page.getByTestId('tab-content-tab-1').element(), {
        timeout: 5000,
      })
      .not.toBeNull()

    // Verify both tab buttons are present in the tab bar (use role to get specific button)
    await expect
      .element(page.getByRole('button', { name: 'Tab 1' }))
      .toBeInTheDocument()
    await expect
      .element(page.getByRole('button', { name: 'Tab 2' }))
      .toBeInTheDocument()
  })

  it('allows switching tabs by clicking tab buttons', async () => {
    if (!client) {
      return
    }

    const layoutHost = client.getLayoutHost()

    await render(
      <TestBaseLayout
        layoutHost={layoutHost}
        renderTab={({ tabID }) => (
          <div data-testid={`tab-content-${tabID}`}>Content for {tabID}</div>
        )}
      />,
    )

    // Wait for the initial tab content
    await expect
      .poll(() => page.getByTestId('tab-content-tab-1').element(), {
        timeout: 5000,
      })
      .not.toBeNull()

    // Tab 1 should be selected (has aria-pressed="true")
    const tab1Button = page.getByRole('button', { name: 'Tab 1' })
    await expect.element(tab1Button).toHaveAttribute('aria-pressed', 'true')

    // Tab 2 should not be selected
    const tab2Button = page.getByRole('button', { name: 'Tab 2' })
    await expect.element(tab2Button).toHaveAttribute('aria-pressed', 'false')

    // Click on Tab 2
    await tab2Button.click()

    // Tab 2 should now be selected
    await expect.element(tab2Button).toHaveAttribute('aria-pressed', 'true')

    // Tab 1 should no longer be selected
    await expect.element(tab1Button).toHaveAttribute('aria-pressed', 'false')

    // Tab 2 content should be visible
    await expect
      .poll(() => page.getByTestId('tab-content-tab-2').element(), {
        timeout: 5000,
      })
      .not.toBeNull()
  })

  it('passes tabData to renderTab callback', async () => {
    if (!client) {
      return
    }

    const layoutHost = client.getLayoutHost()

    // Render with a callback that displays tabData info
    await render(
      <TestBaseLayout
        layoutHost={layoutHost}
        renderTab={({ tabID, tabData }) => {
          // tabData is always a Uint8Array - empty when no data is set
          const decoded = tabData ? new TextDecoder().decode(tabData) : ''
          const hasData = tabData && tabData.length > 0
          return (
            <div data-testid={`tab-content-${tabID}`}>
              <span data-testid={`tab-id-${tabID}`}>{tabID}</span>
              <span data-testid={`tab-has-data-${tabID}`}>
                {hasData ? 'has-data' : 'no-data'}
              </span>
              <span data-testid={`tab-data-decoded-${tabID}`}>{decoded}</span>
            </div>
          )
        }}
      />,
    )

    // Wait for the tab panel to render
    await expect
      .poll(() => page.getByRole('tabpanel').element(), { timeout: 5000 })
      .not.toBeNull()

    // Initial model has no tab data, so tabData.length should be 0
    await expect
      .element(page.getByTestId('tab-has-data-tab-1'))
      .toHaveTextContent('no-data')

    // Decoded empty data should be empty string
    await expect
      .element(page.getByTestId('tab-data-decoded-tab-1'))
      .toHaveTextContent('')
  })

  it('provides navigate function to tab components', async () => {
    if (!client) {
      return
    }

    const layoutHost = client.getLayoutHost()

    // Render with a button that calls navigate
    await render(
      <TestBaseLayout
        layoutHost={layoutHost}
        renderTab={({ tabID, navigate }) => (
          <div data-testid={`tab-content-${tabID}`}>
            <button
              data-testid={`navigate-btn-${tabID}`}
              onClick={() => void navigate('/test/path')}
            >
              Navigate
            </button>
          </div>
        )}
      />,
    )

    // Wait for tab content to render
    await expect
      .poll(() => page.getByTestId('tab-content-tab-1').element(), {
        timeout: 5000,
      })
      .not.toBeNull()

    // Click the navigate button
    const navigateBtn = page.getByTestId('navigate-btn-tab-1')
    await navigateBtn.click()

    // The navigate function should call the RPC without throwing
    // (The Go server will receive it, but we can't easily verify that from the browser)
    // Just verify the button exists and is clickable
    await expect.element(navigateBtn).toBeInTheDocument()
  })

  it('renders tabs in correct positions with absolute positioning', async () => {
    if (!client) {
      return
    }

    const layoutHost = client.getLayoutHost()

    await render(
      <TestBaseLayout
        layoutHost={layoutHost}
        renderTab={({ tabID }) => (
          <div data-testid={`tab-content-${tabID}`}>Content for {tabID}</div>
        )}
      />,
    )

    // Wait for tab content to render
    await expect
      .poll(() => page.getByTestId('tab-content-tab-1').element(), {
        timeout: 5000,
      })
      .not.toBeNull()

    // Find the tab panel element (the parent div with role="tabpanel")
    const tabPanel = page.getByRole('tabpanel')
    await expect.element(tabPanel).toBeInTheDocument()

    // Verify it has absolute positioning style applied
    // The tab panel should have position: absolute and dimensions set
    const element = tabPanel.element()
    const style = element?.style
    expect(style?.position).toBe('absolute')
  })

  it('renders multiple tab content elements for all tabs', async () => {
    if (!client) {
      return
    }

    const layoutHost = client.getLayoutHost()

    await render(
      <TestBaseLayout
        layoutHost={layoutHost}
        renderTab={({ tabID }) => (
          <div data-testid={`tab-content-${tabID}`}>Content for {tabID}</div>
        )}
      />,
    )

    // Wait for tab 1 content
    await expect
      .poll(() => page.getByTestId('tab-content-tab-1').element(), {
        timeout: 5000,
      })
      .not.toBeNull()

    // Both tabs should have content rendered (even if one is hidden)
    // Tab 2 content exists but may be hidden via display: none
    await expect
      .element(page.getByTestId('tab-content-tab-1'))
      .toBeInTheDocument()

    // Click tab 2 to trigger its content to be created
    const tab2Button = page.getByRole('button', { name: 'Tab 2' })
    await tab2Button.click()

    // Now tab 2 content should be rendered
    await expect
      .poll(() => page.getByTestId('tab-content-tab-2').element(), {
        timeout: 5000,
      })
      .not.toBeNull()
  })

  it('renders close button for closable tabs', async () => {
    if (!client) {
      return
    }

    const layoutHost = client.getLayoutHost()

    await render(
      <TestBaseLayout
        layoutHost={layoutHost}
        renderTab={({ tabID }) => (
          <div data-testid={`tab-content-${tabID}`}>Content for {tabID}</div>
        )}
      />,
    )

    // Wait for the layout to render
    await expect
      .poll(() => page.getByTestId('tab-content-tab-1').element(), {
        timeout: 5000,
      })
      .not.toBeNull()

    // The closable tab should be present
    await expect
      .element(page.getByRole('button', { name: 'Closable Tab' }))
      .toBeInTheDocument()

    // Click on the closable tab to select it
    const closableTabButton = page.getByRole('button', { name: 'Closable Tab' })
    await closableTabButton.click()

    // After clicking, the closable tab should be selected
    await expect
      .element(closableTabButton)
      .toHaveAttribute('aria-pressed', 'true')

    // The closable tab content should be visible
    await expect
      .poll(() => page.getByTestId('tab-content-tab-closable').element(), {
        timeout: 5000,
      })
      .not.toBeNull()
  })

  it('sends layout changes to server when tabs are switched', async () => {
    if (!client) {
      return
    }

    const layoutHost = client.getLayoutHost()

    await render(
      <TestBaseLayout
        layoutHost={layoutHost}
        renderTab={({ tabID }) => (
          <div data-testid={`tab-content-${tabID}`}>Content for {tabID}</div>
        )}
      />,
    )

    // Wait for the layout to render
    await expect
      .poll(() => page.getByTestId('tab-content-tab-1').element(), {
        timeout: 5000,
      })
      .not.toBeNull()

    // Switch to Tab 2
    const tab2Button = page.getByRole('button', { name: 'Tab 2' })
    await tab2Button.click()

    // Tab 2 should be selected
    await expect.element(tab2Button).toHaveAttribute('aria-pressed', 'true')

    // The layout change should have been sent to the server
    // We verify this indirectly by checking that the tab content is visible
    // (the RPC communication was logged on the server side in debug mode)
    await expect
      .poll(() => page.getByTestId('tab-content-tab-2').element(), {
        timeout: 5000,
      })
      .not.toBeNull()

    // Switch back to Tab 1 to verify bidirectional updates work
    const tab1Button = page.getByRole('button', { name: 'Tab 1' })
    await tab1Button.click()

    await expect.element(tab1Button).toHaveAttribute('aria-pressed', 'true')
  })

  it('supports tab dragging without losing layout state', async () => {
    if (!client) {
      return
    }

    const layoutHost = client.getLayoutHost()

    await render(
      <TestBaseLayout
        layoutHost={layoutHost}
        renderTab={({ tabID }) => (
          <div data-testid={`tab-content-${tabID}`}>Content for {tabID}</div>
        )}
      />,
    )

    // Wait for the layout to render
    await expect
      .poll(() => page.getByTestId('tab-content-tab-1').element(), {
        timeout: 5000,
      })
      .not.toBeNull()

    // Get the tab button element for Tab 1
    const tab1Button = page.getByRole('button', { name: 'Tab 1' })
    await expect.element(tab1Button).toBeInTheDocument()

    // Get the tab button element
    const tab1Element = tab1Button.element() as HTMLElement
    if (!tab1Element) {
      throw new Error('Tab 1 button not found')
    }

    // Get the layout element (drop target)
    const layoutElement = document.querySelector(
      '.flexlayout__layout',
    ) as HTMLElement
    if (!layoutElement) {
      throw new Error('Layout element not found')
    }

    // Get the tab panel (the content area rendered outside FlexLayout)
    const tabPanel = document.querySelector('[role="tabpanel"]') as HTMLElement
    if (!tabPanel) {
      throw new Error('Tab panel not found')
    }

    // Get bounding rects for drag simulation
    const tabRect = tab1Element.getBoundingClientRect()
    const tabPanelRect = tabPanel.getBoundingClientRect()
    const startX = tabRect.left + tabRect.width / 2
    const startY = tabRect.top + tabRect.height / 2

    // Helper to create DataTransfer mock
    const createDataTransfer = () => new DataTransfer()

    // FlexLayout uses HTML5 Drag and Drop API, not mouse events
    // 1. Start drag on the tab button
    const dragStartEvent = new DragEvent('dragstart', {
      bubbles: true,
      cancelable: true,
      clientX: startX,
      clientY: startY,
      dataTransfer: createDataTransfer(),
    })
    tab1Element.dispatchEvent(dragStartEvent)

    // 2. Drag enters the layout (tab bar area)
    const tabBarX = startX + 50
    const tabBarY = startY
    const dragEnterEvent = new DragEvent('dragenter', {
      bubbles: true,
      cancelable: true,
      clientX: tabBarX,
      clientY: tabBarY,
      dataTransfer: createDataTransfer(),
    })
    layoutElement.dispatchEvent(dragEnterEvent)

    // 3. Drag over tab bar area
    const dragOverTabBar = new DragEvent('dragover', {
      bubbles: true,
      cancelable: true,
      clientX: tabBarX,
      clientY: tabBarY,
      dataTransfer: createDataTransfer(),
    })
    layoutElement.dispatchEvent(dragOverTabBar)

    // Wait for outline rect to appear
    await expect
      .poll(() => document.querySelector('.flexlayout__outline_rect'))
      .not.toBeNull()

    // Verify drag UI appears (outline rect should exist after dragenter)
    const outlineRect1 = document.querySelector('.flexlayout__outline_rect')
    console.log(
      '=== BaseLayout: Dragging over TAB BAR ===',
      outlineRect1 ? 'outline_rect FOUND' : 'outline_rect NOT FOUND',
    )
    expect(outlineRect1).not.toBeNull()

    // 4. Now drag over the tab panel content area (rendered outside FlexLayout)
    // This is the key test - BaseLayout renders tabs with absolute positioning
    // outside the FlexLayout DOM structure
    const bodyX = tabPanelRect.left + tabPanelRect.width / 2
    const bodyY = tabPanelRect.top + tabPanelRect.height / 2

    const dragOverBody = new DragEvent('dragover', {
      bubbles: true,
      cancelable: true,
      clientX: bodyX,
      clientY: bodyY,
      dataTransfer: createDataTransfer(),
    })
    layoutElement.dispatchEvent(dragOverBody)

    // BUG CHECK: The outline rect should still be visible when dragging over tab body
    const outlineRect2 = document.querySelector('.flexlayout__outline_rect')
    console.log(
      '=== BaseLayout: Dragging over TAB BODY ===',
      outlineRect2 ? 'outline_rect FOUND' : 'outline_rect NOT FOUND',
    )
    expect(outlineRect2).not.toBeNull()

    // 5. End the drag
    const dragEndEvent = new DragEvent('dragend', {
      bubbles: true,
      cancelable: true,
      clientX: bodyX,
      clientY: bodyY,
      dataTransfer: createDataTransfer(),
    })
    tab1Element.dispatchEvent(dragEndEvent)

    // Verify the layout is still intact after the drag simulation
    // Tab 1 should still be present and functional
    await expect.element(tab1Button).toBeInTheDocument()

    // Tab 2 should also still be present
    const tab2Button = page.getByRole('button', { name: 'Tab 2' })
    await expect.element(tab2Button).toBeInTheDocument()

    // Verify we can still switch tabs (proves layout state wasn't corrupted)
    await tab2Button.click()
    await expect.element(tab2Button).toHaveAttribute('aria-pressed', 'true')

    // And switch back
    await tab1Button.click()
    await expect.element(tab1Button).toHaveAttribute('aria-pressed', 'true')
  })

  it('maintains tab content during rapid model changes', async () => {
    if (!client) {
      return
    }

    const layoutHost = client.getLayoutHost()

    await render(
      <TestBaseLayout
        layoutHost={layoutHost}
        renderTab={({ tabID }) => (
          <div data-testid={`tab-content-${tabID}`}>Content for {tabID}</div>
        )}
      />,
    )

    // Wait for the layout to render
    await expect
      .poll(() => page.getByTestId('tab-content-tab-1').element(), {
        timeout: 5000,
      })
      .not.toBeNull()

    // Rapidly switch tabs multiple times to stress test model updates
    const tab1Button = page.getByRole('button', { name: 'Tab 1' })
    const tab2Button = page.getByRole('button', { name: 'Tab 2' })
    const closableTab = page.getByRole('button', { name: 'Closable Tab' })

    // Rapid switching
    await tab2Button.click()
    await closableTab.click()
    await tab1Button.click()
    await tab2Button.click()
    await tab1Button.click()

    // After rapid switching, verify final state is correct
    await expect.element(tab1Button).toHaveAttribute('aria-pressed', 'true')

    // Verify all tabs are still present (none were lost during rapid updates)
    await expect.element(tab1Button).toBeInTheDocument()
    await expect.element(tab2Button).toBeInTheDocument()
    await expect.element(closableTab).toBeInTheDocument()

    // Verify tab content is still functional
    await expect
      .element(page.getByTestId('tab-content-tab-1'))
      .toBeInTheDocument()
  })

  it('renders config-backed tab data immediately for externally added tabs', async () => {
    const ref = createRef<BaseLayout>()
    const layoutHost = buildStaticLayoutHost(buildSingleTabLayoutModel())

    await render(
      <BaseLayoutContextProvider>
        <div
          style={{
            width: '800px',
            height: '600px',
            position: 'relative',
            display: 'flex',
            flexDirection: 'column',
            overflow: 'hidden',
          }}
        >
          <BaseLayout
            ref={ref}
            layoutHost={layoutHost}
            renderTab={({ tabID, tabData }) => (
              <div data-testid={`tab-data-${tabID}`}>
                {tabData ? new TextDecoder().decode(tabData) : ''}
              </div>
            )}
          />
        </div>
      </BaseLayoutContextProvider>,
    )

    await expect
      .poll(() => page.getByTestId('tab-data-tab-1').element(), {
        timeout: 5000,
      })
      .not.toBeNull()

    const model = ref.current?.state.model
    if (!model) {
      throw new Error('BaseLayout model not initialized')
    }

    model.doAction(
      Actions.addNode(
        {
          type: 'tab',
          id: 'external-tab',
          name: 'External Tab',
          component: 'tab-content',
          config: new TextEncoder().encode('config-backed-tab'),
        },
        'tabset-1',
        DockLocation.CENTER,
        -1,
        true,
      ),
    )

    await expect
      .poll(
        () => page.getByTestId('tab-data-external-tab').element()?.textContent,
        { timeout: 5000 },
      )
      .toBe('config-backed-tab')
  })

  it('persists config-backed external tabs through the layout model and reload path', async () => {
    const ref = createRef<BaseLayout>()
    const recordingHost = buildRecordingLayoutHost(buildSingleTabLayoutModel())

    const view = await render(
      <BaseLayoutContextProvider>
        <div
          style={{
            width: '800px',
            height: '600px',
            position: 'relative',
            display: 'flex',
            flexDirection: 'column',
            overflow: 'hidden',
          }}
        >
          <BaseLayout
            key="initial"
            ref={ref}
            layoutHost={recordingHost.host}
            renderTab={({ tabID, tabData }) => (
              <div data-testid={`persisted-tab-data-${tabID}`}>
                {tabData ? new TextDecoder().decode(tabData) : ''}
              </div>
            )}
          />
        </div>
      </BaseLayoutContextProvider>,
    )

    await expect
      .poll(() => page.getByTestId('persisted-tab-data-tab-1').element(), {
        timeout: 5000,
      })
      .not.toBeNull()

    const model = ref.current?.state.model
    if (!model) {
      throw new Error('BaseLayout model not initialized')
    }

    model.doAction(
      Actions.addNode(
        {
          type: 'tab',
          id: 'persisted-tab',
          name: 'Persisted Tab',
          component: 'tab-content',
          config: new TextEncoder().encode('persisted-config-tab'),
        },
        'tabset-1',
        DockLocation.CENTER,
        -1,
        true,
      ),
    )

    await expect
      .poll(
        () =>
          recordingHost.requests.some(
            (request) => request.body?.case === 'setModel',
          ),
        { timeout: 5000 },
      )
      .toBe(true)

    const request = recordingHost.requests.find(
      (request) => request.body?.case === 'setModel',
    )
    if (request?.body?.case !== 'setModel') {
      throw new Error('Persisted layout model was not captured')
    }

    const persistedTab = findTabDefById(request.body.value, 'persisted-tab')
    expect(
      new TextDecoder().decode(persistedTab?.data ?? new Uint8Array()),
    ).toBe('persisted-config-tab')

    recordingHost.end()

    await view.rerender(
      <BaseLayoutContextProvider>
        <div
          style={{
            width: '800px',
            height: '600px',
            position: 'relative',
            display: 'flex',
            flexDirection: 'column',
            overflow: 'hidden',
          }}
        >
          <BaseLayout
            key="reloaded"
            layoutHost={buildStaticLayoutHost(request.body.value)}
            renderTab={({ tabID, tabData }) => (
              <div data-testid={`reloaded-tab-data-${tabID}`}>
                {tabData ? new TextDecoder().decode(tabData) : ''}
              </div>
            )}
          />
        </div>
      </BaseLayoutContextProvider>,
    )

    const persistedTabButton = page.getByRole('button', {
      name: 'Persisted Tab',
    })
    await expect.element(persistedTabButton).toBeInTheDocument()
    await persistedTabButton.click()

    await expect
      .poll(
        () =>
          page.getByTestId('reloaded-tab-data-persisted-tab').element()
            ?.textContent,
        { timeout: 5000 },
      )
      .toBe('persisted-config-tab')
  })
})
