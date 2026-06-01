import { expect, test, type Page } from '@playwright/test'

interface ProofState {
  count: number
  events: string[]
}

declare global {
  interface Window {
    __bottomBarContextMenuProof?: ProofState
  }
}

async function proofState(page: Page): Promise<ProofState> {
  return page.evaluate(
    () => window.__bottomBarContextMenuProof ?? { count: 0, events: [] },
  )
}

async function expectProofEvents(page: Page, events: string[]) {
  await expect.poll(async () => (await proofState(page)).events).toEqual(events)
}

test.describe('ViewerFrame bottom-bar context menu', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/e2e/webframe/bottom-bar-context-menu.html')
    await expect(page.getByText('Browser proof content')).toBeVisible()
  })

  test('opens actions from right-click on a real bottom-bar item', async ({
    page,
  }) => {
    await page.getByRole('button', { name: 'Space' }).click({
      button: 'right',
    })

    await page.getByRole('menuitem', { name: 'Switch Object Here' }).click()
    await expectProofEvents(page, ['mouse'])
  })

  test('opens actions from the keyboard context-menu command', async ({
    page,
  }) => {
    await page.getByRole('button', { name: 'Space' }).focus()
    await page.keyboard.press('Shift+F10')

    await page.getByRole('menuitem', { name: 'Switch Object Here' }).click()
    await expectProofEvents(page, ['keyboard'])
  })

  test('opens actions from touch long-press on a real bottom-bar item', async ({
    page,
  }) => {
    const trigger = page.getByRole('button', { name: 'Space' })
    const point = await trigger.evaluate((element) => {
      const rect = element.getBoundingClientRect()
      return { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 }
    })

    await trigger.dispatchEvent('pointerdown', {
      pointerId: 1,
      pointerType: 'touch',
      clientX: point.x,
      clientY: point.y,
      bubbles: true,
      cancelable: true,
    })
    await page.waitForTimeout(600)

    await page.getByRole('menuitem', { name: 'Switch Object Here' }).click()
    await expectProofEvents(page, ['touch'])
  })
})
