import { afterEach, describe, expect, it } from 'vitest'
import { page } from 'vitest/browser'

import spacewaveIcon from '@s4wave/web/images/spacewave-icon.png'

import { buildStartupShell } from './startup-shell.js'
import {
  writeBrowserBootStatus,
  resetBrowserStartupMarksForTest,
} from './boot-status.js'

let host: HTMLDivElement | undefined

afterEach(() => {
  host?.remove()
  host = undefined
  globalThis.__swBootStatus = undefined
  resetBrowserStartupMarksForTest()
})

describe('initial startup HTML', () => {
  it('keeps status updates inside details and preserves usable error recovery', async () => {
    await page.viewport(1518, 1094)
    host = document.createElement('div')
    host.style.cssText = 'position:fixed;inset:0;display:flex'
    host.innerHTML = buildStartupShell(spacewaveIcon)
    document.body.append(host)
    const shell = host.querySelector<HTMLElement>('#sw-loading')!
    shell.style.display = 'block'

    writeBrowserBootStatus({
      phase: 'runtime',
      state: 'loading',
      detail: 'Runtime channel opened.',
      progress: 0.6,
    })
    await expect.element(page.getByText('Opening Spacewave')).toBeVisible()
    expect(host.querySelector('details')?.open).toBe(false)
    expect(
      host.querySelector('[role="progressbar"]')?.getAttribute('aria-valuenow'),
    ).toBeNull()
    expect(host.querySelector('[data-sw-boot-status]')?.textContent).toContain(
      'Connecting the Spacewave runtime.',
    )
    await page.screenshot({
      path: '__screenshots__/browser-startup/initial-html-desktop.png',
    })

    await page.getByText('Show details', { exact: true }).click()
    await expect
      .element(page.getByText(/Connecting the Spacewave runtime/))
      .toBeVisible()
    writeBrowserBootStatus({
      phase: 'runtime-error',
      state: 'error',
      detail: 'Network unavailable.',
    })
    await expect.element(page.getByRole('alert')).toBeVisible()
    await expect
      .element(page.getByRole('button', { name: 'Retry' }))
      .toBeVisible()
    await expect
      .element(page.getByRole('button', { name: 'Back', exact: true }))
      .toBeVisible()
    expect(getComputedStyle(host.querySelector('.swb-activity')!).display).toBe(
      'none',
    )
  })
})
