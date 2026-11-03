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

// viewerReady requires loaded viewer content beyond the session shell and its loading status.
async function viewerReady(page: Page, id: string): Promise<void> {
  await quickstartReady(page)
  switch (id) {
    case 'space':
      await page
        .getByRole('heading', { name: 'Choose an object', exact: true })
        .waitFor()
      break
    case 'git':
      await page.getByRole('button', { name: 'Next', exact: true }).click()
      await page
        .getByRole('textbox', { name: 'Repository Name', exact: true })
        .fill('Parity repository')
      await page.getByRole('button', { name: 'Create', exact: true }).click()
      await page
        .getByText('Empty Repository', { exact: true })
        .or(page.getByText('Make panel larger to view', { exact: true }))
        .waitFor()
      await page
        .getByText('Created Parity repository', { exact: true })
        .waitFor({ state: 'hidden' })
      break
    case 'notebook':
      await page
        .getByTestId('notes-note-row')
        .first()
        .waitFor({ state: 'attached' })
      break
    case 'chat':
      await page
        .getByRole('region', { name: 'Chat channel', exact: true })
        .waitFor()
      await page.waitForFunction(
        () =>
          document.querySelector<HTMLTextAreaElement>(
            'textarea[aria-label="Message"]',
          )?.disabled === false,
      )
      break
    case 'kv':
      await page
        .getByRole('listbox', { name: 'Keys', exact: true })
        .getByRole('option')
        .first()
        .waitFor()
      break
    case 'sql':
      await page
        .getByRole('tree', { name: 'Schemas', exact: true })
        .getByRole('treeitem', { name: 'quickstart', exact: true })
        .waitFor()
      break
    case 'docs':
      // The existing narrow layout collapses the content pane between its two sidebars.
      await page
        .getByTestId('notes-content-view')
        .waitFor({ state: 'attached' })
      await page
        .getByRole('heading', { name: 'Documentation', exact: true })
        .waitFor({ state: 'attached' })
      break
    case 'blog':
      await page
        .getByRole('heading', { name: 'Hello World', exact: true })
        .waitFor()
      break
    case 'device':
      await page
        .getByRole('heading', { name: 'No computers added', exact: true })
        .waitFor()
      break
    case 'forge':
      await page.getByTestId('forge-viewer').waitFor()
      await page.getByText('Entity health', { exact: true }).waitFor()
      break
    default:
      throw new Error(`Missing readiness contract for ${id}`)
  }
}

// sessionState preserves the running document when opening a session page.
function sessionState(
  id: string,
  path: string,
  ready: ParityState['ready'],
): ParityState {
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
      ['pair', 'input[placeholder="XXXX XXXX"]'],
      ['docs', 'h1:has-text("Documentation")'],
      ['debug', 'h1:has-text("Debug tools")'],
    ].map(([id, ready]) => ({ id: id!, path: '/#/' + id, ready: ready! })),
    {
      id: 'changelog',
      path: '/#/changelog',
      ready: async (page) => {
        // The resource's title appears before its embedded release list arrives.
        await page
          .getByRole('heading', { name: 'Changelog', exact: true })
          .waitFor()
        await page.locator('a[href*="/changelog/v"]').first().waitFor()
      },
    },
    {
      id: 'sessions-empty',
      path: '/#/sessions',
      ready: async (page) => {
        await page.waitForFunction(() => location.hash === '#/landing')
        await page.getByRole('combobox').waitFor()
      },
    },
    {
      id: 'sessions',
      path: '/#/quickstart/drive',
      setup: async (page) => {
        await driveReady(page)
        await page.evaluate(() => {
          location.hash = '/sessions'
        })
      },
      ready: '[data-testid="session-selector"]',
    },
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
      'device',
      'forge',
    ].map((id) => ({
      id: 'viewer-' + id,
      path: '/#/quickstart/' + id,
      ready: (page: Page) => viewerReady(page, id),
    })),
    {
      // This isolated fixture has no optional VM image catalog; capture its actual creation state.
      id: 'wizard-vm-catalog-loading',
      path: '/#/quickstart/v86',
      ready: async (page) => {
        await quickstartReady(page)
        await page
          .getByRole('textbox', { name: 'VM name', exact: true })
          .waitFor()
        await page
          .getByText('Loading the catalog image', { exact: true })
          .waitFor()
      },
    },
    sessionState(
      'session-storage',
      '/settings/storage',
      'h1:has-text("Storage health")',
    ),
    sessionState(
      'session-plugins',
      '/settings/plugins',
      'h1:has-text("Plugins")',
    ),
    sessionState(
      'session-transfers',
      '/settings/transfer',
      'h1:has-text("Transfer Sessions")',
    ),
    sessionState('session-cli', '/settings/cli', 'h1:has-text("Command Line")'),
    sessionState('session-billing-unavailable', '/billing', async (page) => {
      await page
        .getByRole('button', { name: 'Reload page', exact: true })
        .waitFor()
      await page
        .getByRole('main')
        .filter({ hasText: 'SpacewaveOrgList context not found' })
        .waitFor({ state: 'attached' })
    }),
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
        await page
          .getByRole('button', { name: 'Open command palette', exact: true })
          .click()
      },
      ready: '[role="dialog"] [cmdk-input]',
    },
  ],
}

export default routesManifest
