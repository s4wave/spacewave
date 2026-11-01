import type { Page } from 'playwright'

import publicManifest from './public.manifest.ts'
import type { ParityManifest, ParityState } from './capture.js'

// hydratedPage requires the actual client page, rather than its prerendered heading alone.
async function hydratedPage(page: Page): Promise<void> {
  await page.waitForFunction(() => '__swPrerenderRoot' in globalThis)
  await page.locator('h1').first().waitFor()
}

// driveReady follows the existing recovery control once before requiring seeded content.
async function driveReady(page: Page, allowRetry = true): Promise<void> {
  // Quickstart exposes cancellation as a recoverable state, which must not become a selector timeout.
  const status = await page.waitForFunction((allowRetry) => {
    // Preserve the application’s failure and timing evidence before following its Retry control.
    const failed = [...document.querySelectorAll('h3')].some(
      (heading) => heading.textContent === 'Setup Failed',
    )
    if (failed) {
      const text = document.body.textContent ?? ''
      if (allowRetry && text.includes('ERR_RPC_ABORT')) return 'retry'
      const timing = (
        globalThis as typeof globalThis & {
          __s4waveQuickstartTiming?: unknown
        }
      ).__s4waveQuickstartTiming
      throw new Error(`Drive setup failed: ${JSON.stringify(timing)}; ${text}`)
    }

    // Finish the intro only after its file seed is visible, so creation and navigation cannot race.
    const browser = document.querySelector('[data-testid="unixfs-browser"]')
    const seeded = browser?.textContent?.includes('getting-started.md')
    const actions = ['Next', 'Got it, start exploring', 'Open files']
    const button = [...document.querySelectorAll('button')].find(
      (element) =>
        !element.disabled &&
        actions.includes(element.textContent?.trim() ?? ''),
    )
    if (
      button &&
      (button.textContent?.trim() !== 'Got it, start exploring' || seeded)
    )
      button.click()
    return seeded && !location.hash.includes('/wizard/') ? 'ready' : false
  }, allowRetry)
  if ((await status.jsonValue()) === 'retry') {
    console.log(
      'Drive setup returned ERR_RPC_ABORT; following its Retry control once',
    )
    await page.getByRole('button', { name: 'Retry', exact: true }).click()
    await driveReady(page, false)
    return
  }

  // The visible browser is required after the seed and route become ready.
  await page.locator('[data-testid="unixfs-browser"]:visible').first().waitFor()
}

// quickstartReady requires the ordinary setup's content boundary and mounted Space route.
async function quickstartReady(page: Page): Promise<void> {
  await page.waitForFunction(() => {
    const state = (
      globalThis as typeof globalThis & {
        __s4waveQuickstartTiming?: { state: string; error?: string }
      }
    ).__s4waveQuickstartTiming
    if (state?.state === 'error')
      throw new Error(state.error ?? 'Quickstart failed')
    return (
      state?.state === 'content-ready' && /^#\/u\/\d+\/so\//.test(location.hash)
    )
  })
  await page.locator('[data-testid="session-account-menu-button"]').waitFor()
}

// sessionState preserves the running document when opening a session page.
function sessionState(id: string, path: string, ready: string): ParityState {
  return {
    id,
    path: '/#/quickstart/drive',
    setup: async (page) => {
      await driveReady(page)
      await page.evaluate((path) => {
        location.hash = '/u/1' + path
      }, path)
    },
    ready,
  }
}

/** routesManifest grows the baseline through public, account, viewer and overlay families. */
const routesManifest: ParityManifest = {
  states: [
    ...publicManifest.states,
    ...[
      '/landing',
      '/landing/drive',
      '/landing/chat',
      '/landing/devices',
      '/landing/plugins',
      '/landing/notes',
      '/landing/cli',
      '/landing/hydra',
      '/landing/bifrost',
      '/landing/controllerbus',
      '/community',
      '/tos',
      '/privacy',
      '/dmca',
      '/licenses',
      '/download/cli',
      '/blog',
    ].map((path) => ({
      id:
        path === '/landing'
          ? 'landing-page'
          : path.slice(1).replaceAll('/', '-'),
      path,
      ready: hydratedPage,
    })),
    ...[
      ['login', 'input[placeholder=alice]'],
      ['signup', 'input[placeholder=alice]'],
      ['recover', 'h1:has-text("Recover Account")'],
      ['sessions', '[data-testid="session-selector"]'],
      ['pair', 'input[placeholder="XXXX XXXX"]'],
      ['docs', 'h1:has-text("Documentation")'],
      ['changelog', 'h1:has-text("Changelog")'],
      ['debug', 'h1:has-text("Debug tools")'],
    ].map(([id, ready]) => ({ id: id!, path: '/#/' + id, ready: ready! })),
    {
      id: 'viewer-drive',
      path: '/#/quickstart/drive',
      ready: driveReady,
    },
    {
      id: 'viewer-canvas',
      path: '/#/quickstart/canvas',
      ready: async (page) => {
        await quickstartReady(page)
        await page
          .locator(
            '[data-testid="canvas-viewport"]:visible [data-testid="unixfs-browser"]:visible',
          )
          .first()
          .waitFor()
        await page
          .getByText('This folder is empty', { exact: true })
          .first()
          .waitFor()
      },
    },
    ...[
      'space',
      'git',
      'notebook',
      'chat',
      'kv',
      'sql',
      'docs',
      'blog',
      'v86',
      'device',
      'forge',
    ].map((id) => ({
      id: 'viewer-' + id,
      path: '/#/quickstart/' + id,
      ready: quickstartReady,
    })),
    sessionState('session-storage', '/settings/storage', 'h1'),
    sessionState('session-plugins', '/settings/plugins', 'h1'),
    sessionState('session-transfers', '/settings/transfer', 'h1'),
    sessionState('session-cli', '/settings/cli', 'h1'),
    sessionState('session-billing', '/billing', 'h1'),
    {
      id: 'overlay-account',
      path: '/#/quickstart/drive',
      setup: async (page) => {
        await driveReady(page)
        await page.getByTestId('session-account-menu-button').click()
      },
      ready: async (page) => {
        await page
          .getByRole('button', { name: 'Close account menu', exact: true })
          .waitFor()
      },
    },
    {
      id: 'overlay-invite',
      path: '/#/quickstart/drive',
      setup: async (page) => {
        await driveReady(page)
        await page.getByTestId('drive-invite-cta').first().click()
      },
      ready: '[role="dialog"]:has-text("Add User")',
    },
    {
      id: 'overlay-command',
      path: '/#/quickstart/drive',
      setup: async (page) => {
        await driveReady(page)
        await page.keyboard.press('Control+k')
      },
      ready: '[role="dialog"] [cmdk-input]',
    },
  ],
}

export default routesManifest
