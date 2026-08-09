import { describe, it, expect, beforeAll, beforeEach } from 'vitest'
import { page } from 'vitest/browser'
import { render, cleanup } from 'vitest-browser-react'
import { BldrContext, type IBldrContext } from '@aptre/bldr-react'
import type { OpenStreamFunc } from 'starpc'

import '@s4wave/web/style/app.css'
import {
  createE2EClient,
  getTestServerPort,
  type E2ETestClient,
} from '@s4wave/web/test/e2e-client.js'

import { App } from './App.js'

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
    { timeout: 180000 },
    async ({ skip }) => {
      skip(!openStreamFunc, 'No backend available')

      // AppAPI uses only the E2E stream and WebView UUID across the window boundary.
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

      await render(
        <BldrContext.Provider value={mockBldrContext}>
          <App />
        </BldrContext.Provider>,
      )

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

      await expect
        .poll(() => page.getByText('[SPACEWAVE]').element() !== null, {
          timeout: 10000,
        })
        .toBe(true)

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

      await expect
        .poll(
          () => {
            const hash = window.location.hash
            return hash.includes('/u/') && hash.includes('/so/')
          },
          { timeout: 30000 },
        )
        .toBe(true)

      await expect
        .poll(
          async () => {
            const buttons = Array.from(document.querySelectorAll('button'))
            const finish = buttons.find((button) =>
              button.textContent?.includes('Got it, start exploring'),
            )
            if (finish instanceof HTMLButtonElement) {
              finish.click()
              await new Promise<void>((resolve) => {
                requestAnimationFrame(() => resolve())
              })
              return false
            }
            const next = buttons.find((button) => button.textContent === 'Next')
            if (next instanceof HTMLButtonElement && !next.disabled) {
              next.click()
              await new Promise<void>((resolve) => {
                requestAnimationFrame(() => resolve())
              })
              return false
            }

            const browser = document.querySelector(
              '[data-testid="unixfs-browser"]',
            )
            return (
              browser !== null && !window.location.hash.includes('/wizard/')
            )
          },
          { timeout: 120000 },
        )
        .toBe(true)

      // The file browser can finish loading after the wizard closes.
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

      expect(
        document.querySelector('[data-testid="drive-welcome"]'),
      ).toBeTruthy()

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

      const dblClickEvent = new MouseEvent('dblclick', {
        bubbles: true,
        cancelable: true,
        view: window,
      })
      fileRow.dispatchEvent(dblClickEvent)

      await expect
        .poll(
          () => {
            const browser = document.querySelector(
              '[data-testid="unixfs-browser"]',
            )
            if (!browser) return false
            const text = browser.textContent ?? ''
            return text.includes('Welcome to your new drive')
          },
          { timeout: 15000 },
        )
        .toBe(true)
    },
  )
})
