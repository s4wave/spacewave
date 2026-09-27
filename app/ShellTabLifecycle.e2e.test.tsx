import { afterEach, describe, expect, it, vi } from 'vitest'
import { page } from 'vitest/browser'
import { cleanup, render } from 'vitest-browser-react'

import { getAppPath } from '@s4wave/web/router/app-path.js'
import { useNavigate, usePath } from '@s4wave/web/router/router.js'

import { resetBrowserShellTabsStoreForTests } from './BrowserShellTabsStore.js'
import type { ShellDocumentEntry } from './ShellDocumentEntry.js'
import { ShellTabStrip } from './ShellFlexLayout.js'
import { useTabId } from './ShellTabContext.js'
import { readShellTabsSnapshot, seedShellTabs } from './ShellTabTestHarness.js'

// Keep account startup outside this shell test while using the real tab store,
// panel router, FlexLayout model, render lifecycle, and stylesheet.
vi.mock('./routes/AppRoutes.js', () => ({
  AppRoutes: function AppRoutes() {
    // Navigate through the panel's real router and committed tab state.
    const tabId = useTabId()
    const path = usePath()
    const navigate = useNavigate()

    // Retain a draft so tab changes also check the viewer's mount lifetime.
    return (
      <div className="h-full w-full" data-viewer-id={tabId}>
        <output>{path}</output>
        <input aria-label="Draft" defaultValue="" />
        <button
          type="button"
          onClick={() => navigate({ path: '/u/1/new/drive' })}
        >
          Create a Drive
        </button>
      </div>
    )
  },
}))

const entry: ShellDocumentEntry = {
  kind: 'continuation',
  path: '/u/1',
  params: {},
  incarnation: 'shell-lifecycle-test',
}
const observers = new Set<MutationObserver>()

/** onDOMChange resolves when a DOM revision satisfies the rendered contract. */
function onDOMChange(check: () => boolean): Promise<void> {
  return new Promise((resolve) => {
    // Release the subscription as soon as the rendered state is accepted.
    const accept = () => {
      if (!check()) return
      observer.disconnect()
      observers.delete(observer)
      resolve()
    }

    // Subscribe before the first read so a committed DOM change cannot be missed.
    const observer = new MutationObserver(accept)
    observers.add(observer)
    observer.observe(document.body, {
      childList: true,
      subtree: true,
      attributes: true,
      characterData: true,
    })
    accept()
  })
}

/** activePanel returns the shell's visible content panel. */
function activePanel(): HTMLElement | undefined {
  return Array.from(
    document.querySelectorAll<HTMLElement>(
      '.shell-flexlayout [role="tabpanel"]',
    ),
  ).find((panel) => getComputedStyle(panel).visibility !== 'hidden')
}

/** mountShell restores the document's tab selection with the real shell. */
async function mountShell() {
  await render(
    <div className="fixed inset-0 flex flex-col">
      <ShellTabStrip entry={entry} />
    </div>,
  )
}

afterEach(async () => {
  // Disconnect observers even if a failed assertion ends a pending wait.
  observers.forEach((observer) => observer.disconnect())
  observers.clear()

  // Release the shell before discarding this test's document state.
  await cleanup()
  sessionStorage.clear()
  resetBrowserShellTabsStoreForTests()
})

describe('Shell tab lifecycle with FlexLayout', () => {
  for (const [width, height] of [
    [390, 844],
    [360, 780],
    [844, 390],
  ]) {
    it(`adds, resizes, navigates, and restores tabs at ${width}x${height}`, async () => {
      // Begin in a Session with one committed tab.
      await page.viewport(width, height)
      window.location.hash = '/u/1'
      seedShellTabs([{ id: 'session', name: 'Session', path: '/u/1' }])
      await mountShell()
      await onDOMChange(
        () => activePanel()?.getBoundingClientRect().width === width,
      )
      const firstViewer = document.querySelector('[data-viewer-id="session"]')
      const firstPanel = activePanel()!
      await page.getByRole('textbox', { name: 'Draft' }).fill('keep this draft')

      // A viewport resize updates the existing viewer through FlexLayout.
      await page.viewport(844, 390)
      await onDOMChange(() => firstPanel.getBoundingClientRect().width === 844)
      const content = document.querySelector('.flexlayout__tabset_content')!
      expect(firstPanel.getBoundingClientRect().height).toBe(
        content.getBoundingClientRect().height,
      )
      expect(firstViewer).toBe(
        document.querySelector('[data-viewer-id="session"]'),
      )
      expect(firstViewer!.getBoundingClientRect().width).toBe(844)

      // New tab commits a second record and selects its rendered content.
      await page.getByTitle('New tab', { exact: true }).click()
      await onDOMChange(
        () =>
          document.querySelectorAll('.shell-flexlayout .flexlayout__tab_button')
            .length === 2 &&
          !!activePanel() &&
          activePanel()!.dataset.tabId !== 'session',
      )
      const selectedId = activePanel()!.dataset.tabId!
      expect(readShellTabsSnapshot().records).toHaveLength(2)
      expect(
        (document.querySelector('[title="Close tab"]') as HTMLButtonElement)
          .disabled,
      ).toBe(false)
      expect(activePanel()!.getBoundingClientRect().width).toBe(844)

      // Navigation updates only the new tab; returning preserves the first viewer.
      activePanel()!.querySelector('button')!.click()
      await onDOMChange(
        () =>
          activePanel()?.querySelector('output')?.textContent ===
            '/u/1/new/drive' && getAppPath() === '/u/1/new/drive',
      )
      expect(
        readShellTabsSnapshot().records.find(
          (record) => record.id === 'session',
        )?.path,
      ).toBe('/u/1')
      document.querySelector<HTMLElement>('.flexlayout__tab_button')!.click()
      await onDOMChange(() => activePanel()?.dataset.tabId === 'session')
      expect(firstViewer).toBe(
        document.querySelector('[data-viewer-id="session"]'),
      )
      expect(firstViewer!.querySelector('input')!.value).toBe('keep this draft')
      document
        .querySelectorAll<HTMLElement>('.flexlayout__tab_button')[1]
        .click()
      await onDOMChange(
        () =>
          activePanel()?.dataset.tabId === selectedId &&
          getAppPath() === '/u/1/new/drive',
      )

      // A document continuation restores both records and the selected path.
      await cleanup()
      resetBrowserShellTabsStoreForTests()
      await mountShell()
      await onDOMChange(
        () =>
          activePanel()?.dataset.tabId === selectedId &&
          activePanel()?.querySelector('output')?.textContent ===
            '/u/1/new/drive',
      )
      expect(document.querySelectorAll('.flexlayout__tab_button')).toHaveLength(
        2,
      )
      expect(activePanel()!.getBoundingClientRect().width).toBe(844)

      // Closing the new tab selects the surviving Session without losing its route.
      await page.getByTitle('Close tab', { exact: true }).click()
      await onDOMChange(
        () =>
          document.querySelectorAll('.flexlayout__tab_button').length === 1 &&
          activePanel()?.dataset.tabId === 'session',
      )
      expect(readShellTabsSnapshot().records).toHaveLength(1)
      expect(getAppPath()).toBe('/u/1')
    }, 15000)
  }
})
