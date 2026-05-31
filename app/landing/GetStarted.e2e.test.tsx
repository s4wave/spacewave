import { describe, expect, it } from 'vitest'
import { page } from 'vitest/browser'
import { render } from 'vitest-browser-react'

import GetStarted from './GetStarted.js'
import { Landing } from './Landing.js'
import { RouterProvider } from '@s4wave/web/router/router.js'

function renderGetStarted() {
  return render(
    <RouterProvider path="/" onNavigate={() => {}}>
      <div
        style={{
          width: '640px',
          height: '540px',
          padding: '32px',
          background: '#111',
        }}
      >
        <GetStarted />
      </div>
    </RouterProvider>,
  )
}

describe('GetStarted browser layout', () => {
  it('keeps quickstart icon boxes square under narrow text pressure', async () => {
    await render(
      <RouterProvider path="/" onNavigate={() => {}}>
        <div
          style={{
            width: '270px',
            height: '540px',
            padding: '16px',
            background: '#111',
          }}
        >
          <GetStarted />
        </div>
      </RouterProvider>,
    )

    await expect
      .poll(() => page.getByText('Create a Drive').element() !== null)
      .toBe(true)

    const driveItem = page
      .getByText('Create a Drive')
      .element()
      ?.closest('[data-slot="command-item"]')
    const iconBox = driveItem?.querySelector(':scope > div')

    expect(driveItem).toBeInstanceOf(HTMLElement)
    expect(iconBox).toBeInstanceOf(HTMLElement)

    const iconRect = (iconBox as HTMLElement).getBoundingClientRect()

    expect(iconRect.width).toBeGreaterThanOrEqual(35)
    expect(Math.abs(iconRect.width - iconRect.height)).toBeLessThan(0.5)
  })

  it('keeps the first command item below the input row', async () => {
    await renderGetStarted()

    await expect
      .poll(() => page.getByText('Open a local state root').element() !== null)
      .toBe(true)

    const command = document.querySelector('[data-slot="command"]')
    const inputWrapper = document.querySelector(
      '[data-slot="command-input-wrapper"]',
    )
    const firstItem = document.querySelector('[data-slot="command-item"]')

    expect(command).toBeInstanceOf(HTMLElement)
    expect(inputWrapper).toBeInstanceOf(HTMLElement)
    expect(firstItem).toBeInstanceOf(HTMLElement)

    const commandRect = (command as HTMLElement).getBoundingClientRect()
    const inputRect = (inputWrapper as HTMLElement).getBoundingClientRect()
    const itemRect = (firstItem as HTMLElement).getBoundingClientRect()

    expect(inputRect.top).toBeGreaterThanOrEqual(commandRect.top)
    expect(itemRect.top).toBeGreaterThanOrEqual(inputRect.bottom)
  })

  it('does not clip the first visible action in the landing frame', async () => {
    await page.viewport(710, 600)
    await render(
      <RouterProvider path="/" onNavigate={() => {}}>
        <div style={{ width: '100vw', height: '100vh', display: 'flex' }}>
          <Landing />
        </div>
      </RouterProvider>,
    )

    await expect
      .poll(() => page.getByText('Open a local state root').element() !== null)
      .toBe(true)

    const command = document.querySelector('[data-slot="command"]')
    const inputWrapper = document.querySelector(
      '[data-slot="command-input-wrapper"]',
    )
    const firstVisibleItem = Array.from(
      document.querySelectorAll('[data-slot="command-item"]'),
    ).find((item) => {
      const rect = (item as HTMLElement).getBoundingClientRect()
      return rect.width > 0 && rect.height > 0
    })

    expect(command).toBeInstanceOf(HTMLElement)
    expect(inputWrapper).toBeInstanceOf(HTMLElement)
    expect(firstVisibleItem).toBeInstanceOf(HTMLElement)

    const commandRect = (command as HTMLElement).getBoundingClientRect()
    const inputRect = (inputWrapper as HTMLElement).getBoundingClientRect()
    const itemRect = (firstVisibleItem as HTMLElement).getBoundingClientRect()

    expect(inputRect.top).toBeGreaterThanOrEqual(commandRect.top)
    expect(itemRect.top).toBeGreaterThanOrEqual(inputRect.bottom)
    expect(itemRect.top).toBeGreaterThanOrEqual(commandRect.top)
  })
})
