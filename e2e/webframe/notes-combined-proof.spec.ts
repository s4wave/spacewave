import { expect, test as base, type Page } from '@playwright/test'

// All tests in one engine share a single browser context: the app-bundle
// download cache is keyed to the browser profile, and a fresh context pays
// the whole multi-minute download again. Viewports are still set per test.
const test = base.extend({
  page: async ({ browser }, use) => {
    const context = await browser.newContext()
    const page = await context.newPage()
    // Playwright fixture teardown hook, not a React hook.
    // eslint-disable-next-line react-hooks/rules-of-hooks
    await use(page)
    await context.close()
  },
})

const ROUTE = '/#/quickstart/notebook'
const WELCOME_ROW = "[data-testid='notes-note-row']"
const CONTENT_VIEW = "[data-testid='notes-content-view']"
const SOURCE_TOGGLE = "[data-testid='notes-source-toggle']"

// The first launch downloads the app bundle and seeds the notebook, which
// takes minutes — longer on webkit, where the download alone ran past five
// minutes at roughly eighty percent when timed out. One shared context per
// engine pays this once.
const COLD_START_TIMEOUT = 900_000

async function waitNotebookReady(page: Page) {
  await page.locator("input[placeholder='Search notes…']").waitFor({
    state: 'visible',
    timeout: COLD_START_TIMEOUT,
  })
  await page
    .locator(WELCOME_ROW)
    .filter({ hasText: 'Welcome' })
    .first()
    .waitFor({
      state: 'visible',
      timeout: COLD_START_TIMEOUT,
    })
}

// Navigates to the quickstart route; every visit creates a new seeded space,
// so tests that must return to an existing notebook call waitNotebookReady
// on the URL they already hold instead of routing here again.
async function openNotebook(page: Page) {
  await page.goto(ROUTE, { waitUntil: 'domcontentloaded' })
  await waitNotebookReady(page)
}

async function openWelcome(page: Page) {
  const row = page.locator(WELCOME_ROW).filter({ hasText: 'Welcome' }).first()
  await row.click()
  await page.locator(CONTENT_VIEW).waitFor({ state: 'visible' })
}

/** Switches the note editor between source text and WYSIWYG rendering. */
async function toggleSourceMode(page: Page) {
  const toggle = page.locator(SOURCE_TOGGLE).first()
  await toggle.focus()
  await page.keyboard.press('Enter')
}

/**
 * Fills the note with a unique nonce and waits for the app to persist it.
 * Returns the nonce so a later test can prove the bytes survived a reload.
 */
async function saveNonce(page: Page, engine: string): Promise<string> {
  const nonce = `notes-combined-${engine}-${Date.now()}`
  await toggleSourceMode(page)
  const editor = page.getByLabel('Note source')
  await editor.waitFor({ state: 'visible' })
  await editor.fill(`# Welcome\n\n${nonce}`)
  await toggleSourceMode(page)
  await page
    .getByRole('status')
    .filter({ hasText: 'Saved' })
    .waitFor({ state: 'visible' })
  return nonce
}

async function expectNoHorizontalOverflow(page: Page) {
  const overflow = await page.evaluate(
    () =>
      document.documentElement.scrollWidth >
      document.documentElement.clientWidth,
  )
  expect(overflow).toBe(false)
}

test.describe('Notes combined proof', () => {
  test('desktop save flow shows Saved status without overflow', async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await openNotebook(page)
    await openWelcome(page)
    await page
      .locator("[data-testid='notes-note-list']")
      .waitFor({ state: 'visible' })

    await saveNonce(page, testInfo.project.name)
    await expectNoHorizontalOverflow(page)
  })

  test('mobile save flow toggles notebook navigation', async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await openNotebook(page)

    const navToggle = page.getByRole('button', {
      name: 'Open notebook navigation',
    })
    await navToggle.waitFor({ state: 'visible' })
    await navToggle.click()
    await page
      .getByRole('button', { name: 'Close notebook navigation' })
      .click()
    await navToggle.waitFor({ state: 'visible' })

    await openWelcome(page)
    await saveNonce(page, testInfo.project.name)
    await expectNoHorizontalOverflow(page)
  })

  test('desktop search filters notes and clears back to Welcome', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await openNotebook(page)

    const search = page.locator("input[placeholder='Search notes…']")
    await search.fill('Welcome')
    await expect(
      page.locator(WELCOME_ROW).filter({ hasText: 'Welcome' }).first(),
    ).toBeVisible()

    await search.fill(`zzqx-no-such-note-${Date.now()}`)
    await expect(page.locator(WELCOME_ROW)).toHaveCount(0)

    await search.fill('')
    await openWelcome(page)
    await page.locator(CONTENT_VIEW).waitFor({ state: 'visible' })
  })

  test('cold reopen keeps the saved nonce in source bytes', async ({
    page,
  }, testInfo) => {
    test.setTimeout(300_000)
    await page.setViewportSize({ width: 1440, height: 900 })
    await openNotebook(page)
    await openWelcome(page)
    const nonce = await saveNonce(page, testInfo.project.name)

    // Reload holds this space's own URL. Routing to the quickstart route
    // here would create a second seeded space instead of proving bytes.
    const spaceUrl = page.url()
    await page.reload({ waitUntil: 'domcontentloaded' })
    expect(page.url()).toBe(spaceUrl)
    await waitNotebookReady(page)
    await openWelcome(page)

    await toggleSourceMode(page)
    const editor = page.getByLabel('Note source')
    await editor.waitFor({ state: 'visible' })
    await expect(editor).toHaveValue(new RegExp(nonce))
    await expectNoHorizontalOverflow(page)
  })
})
