/**
 * User Story E2E Test
 *
 * Tests the main user flow for spacewave-app using the full App component:
 * 1. Load the app (see landing page)
 * 2. Click "Create a Drive" (quickstart option)
 * 3. Confirm file browser is visible with expected files
 * 4. Double-click on getting-started.md
 * 5. Confirm file contents are shown
 *
 * To run: bun run test:browser
 */
import { describe, it, expect, beforeAll, beforeEach } from 'vitest'
import { page } from 'vitest/browser'
import { render, cleanup } from 'vitest-browser-react'
import { BldrContext, type IBldrContext } from '@aptre/bldr-react'
import type { OpenStreamFunc } from 'starpc'

import '@s4wave/web/style/app.css'

import { App } from './App.js'
import {
  createE2EClient,
  getTestServerPort,
  type E2ETestClient,
} from '@s4wave/web/test/e2e-client.js'

describe('User Story: Create Drive and View File', () => {
  let e2eClient: E2ETestClient | undefined
  let openStreamFunc: OpenStreamFunc | undefined

  beforeAll(async () => {
    let port: number
    try {
      port = getTestServerPort()
    } catch {
      return
    }

    e2eClient = await createE2EClient(port)
    openStreamFunc = e2eClient.getOpenStreamFunc()
  })

  beforeEach(async () => {
    await cleanup()
    localStorage.clear()
    window.location.hash = ''
  })

  it(
    'creates drive, views file browser, opens file and sees contents',
    { timeout: 60000 },
    async ({ skip }) => {
      skip(!openStreamFunc, 'No backend available')

      // Create mock BldrContext that provides our E2E WebSocket connection.
      // AppAPI only uses webDocument.buildWebViewHostOpenStream() and webView.getUuid()
      // so we only need to mock those methods. Cast through unknown to satisfy TypeScript.
      const mockBldrContext = {
        webDocument: {
          buildWebViewHostOpenStream: () => openStreamFunc!,
          registerWebView: () => ({ release: () => {} }),
          webDocumentUuid: 'e2e-test-doc',
        },
        webView: {
          getUuid: () => 'e2e-test-webview',
        },
      } as unknown as IBldrContext

      // Step 1: Load the full App with mocked BldrContext
      await render(
        <BldrContext.Provider value={mockBldrContext}>
          <App />
        </BldrContext.Provider>,
      )

      // Wait for app to initialize (loading overlay should disappear)
      await expect
        .poll(
          () => {
            const loading = document.querySelector(
              '[role="status"][aria-label="Loading"]',
            )
            const initText = document.body.textContent?.includes('Initializing')
            return !loading && !initText
          },
          { timeout: 15000 },
        )
        .toBe(true)

      // Verify landing page shows [SPACEWAVE] title
      await expect
        .poll(() => page.getByText('[SPACEWAVE]').element() !== null, {
          timeout: 10000,
        })
        .toBe(true)

      // Step 2: Click "Create a Drive"
      await expect
        .poll(
          () => {
            const item = page.getByText('Create a Drive').element()
            return item?.closest('[data-slot="command-item"]') !== null
          },
          { timeout: 5000 },
        )
        .toBe(true)

      const driveItemEl = page.getByText('Create a Drive').element()
      const driveItem = driveItemEl?.closest(
        '[data-slot="command-item"]',
      ) as HTMLElement
      driveItem.click()

      // Wait for quickstart to complete - URL should contain /u/ and /so/
      await expect
        .poll(
          () => {
            const hash = window.location.hash
            return hash.includes('/u/') && hash.includes('/so/')
          },
          { timeout: 30000 },
        )
        .toBe(true)

      // Step 3: Verify file browser is visible with the starter guide
      // Note: The file browser may take time to load after navigation completes
      await expect
        .poll(
          () => {
            const browser = document.querySelector(
              '[data-testid="unixfs-browser"]',
            )
            if (!browser) return false
            const text = browser.textContent ?? ''
            return text.includes('getting-started.md')
          },
          { timeout: 30000 },
        )
        .toBe(true)

      // Step 4: Double-click on getting-started.md
      // File rows have role="row" attribute
      await expect
        .poll(
          () => {
            const rows = document.querySelectorAll('[role="row"]')
            return (
              Array.from(rows).find((row) =>
                row.textContent?.includes('getting-started.md'),
              ) !== undefined
            )
          },
          { timeout: 5000 },
        )
        .toBe(true)

      const rows = document.querySelectorAll('[role="row"]')
      const fileRow = Array.from(rows).find((row) =>
        row.textContent?.includes('getting-started.md'),
      ) as HTMLElement

      // Dispatch double-click event
      const dblClickEvent = new MouseEvent('dblclick', {
        bubbles: true,
        cancelable: true,
        view: window,
      })
      fileRow.dispatchEvent(dblClickEvent)

      // Step 5: Verify file contents are shown
      // The file content should include the welcome message from getting-started.md
      await expect
        .poll(
          () => {
            const browser = document.querySelector(
              '[data-testid="unixfs-browser"]',
            )
            if (!browser) return false
            const text = browser.textContent ?? ''
            // getting-started.md contains "Welcome to your new drive"
            return text.includes('Welcome to your new drive')
          },
          { timeout: 15000 },
        )
        .toBe(true)

      // Steps 6-9 (maximize/restore) removed: flexlayout's canMaximize()
      // returns false when root has a single tabset child, which is the
      // default quickstart layout. The maximize button only renders when
      // there are 2+ tabsets in the layout.
    },
  )
})
