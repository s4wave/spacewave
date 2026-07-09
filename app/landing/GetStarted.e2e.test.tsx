import { describe, expect, it } from 'vitest'
import { page, userEvent } from 'vitest/browser'
import { render } from 'vitest-browser-react'

import { RouterProvider } from '@s4wave/web/router/router.js'

import GetStarted from './GetStarted.js'
import { Landing } from './Landing.js'

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

function expectHTMLElement(
  element: Element | null | undefined,
): asserts element is HTMLElement {
  expect(element).toBeInstanceOf(HTMLElement)
}

function compositedContrastRatio(background: string, overlay: string): number {
  const canvas = document.createElement('canvas')
  canvas.width = 1
  canvas.height = 1
  const context = canvas.getContext('2d')
  if (!context) throw new Error('no canvas 2d context')

  context.fillStyle = background
  context.fillRect(0, 0, 1, 1)
  const backgroundRgb = Array.from(
    context.getImageData(0, 0, 1, 1).data.slice(0, 3),
  )

  context.fillStyle = overlay
  context.fillRect(0, 0, 1, 1)
  const compositedRgb = Array.from(
    context.getImageData(0, 0, 1, 1).data.slice(0, 3),
  )

  const relativeLuminance = (rgb: number[]) =>
    rgb
      .map((channel) => channel / 255)
      .map((channel) =>
        channel <= 0.04045
          ? channel / 12.92
          : Math.pow((channel + 0.055) / 1.055, 2.4),
      )
      .reduce(
        (sum, channel, index) =>
          sum + channel * ([0.2126, 0.7152, 0.0722][index] ?? 0),
        0,
      )

  const darker = relativeLuminance(backgroundRgb)
  const lighter = relativeLuminance(compositedRgb)
  return (Math.max(darker, lighter) + 0.05) / (Math.min(darker, lighter) + 0.05)
}

describe('GetStarted browser layout', () => {
  it('keeps quickstart icon boxes square and distinct from the palette surface', async () => {
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
    const command = driveItem?.closest('[data-slot="command"]')
    const iconBox = driveItem?.querySelector(':scope > div')

    expectHTMLElement(driveItem)
    expectHTMLElement(command)
    expectHTMLElement(iconBox)

    const iconRect = iconBox.getBoundingClientRect()

    expect(iconRect.width).toBeGreaterThanOrEqual(35)
    expect(Math.abs(iconRect.width - iconRect.height)).toBeLessThan(0.5)

    const paletteBackground = getComputedStyle(command).backgroundColor
    const iconBackground = getComputedStyle(iconBox).backgroundColor
    expect(
      compositedContrastRatio(paletteBackground, iconBackground),
    ).toBeGreaterThanOrEqual(1.2)
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

  it('gives Learn more a purpose label and a working keyboard action', async () => {
    await page.viewport(1280, 800)
    await render(
      <RouterProvider path="/" onNavigate={() => {}}>
        <div style={{ width: '100vw', height: '100vh', display: 'flex' }}>
          <Landing />
        </div>
      </RouterProvider>,
    )

    const learnMore = page.getByRole('button', {
      name: 'Scroll down to learn more',
    })
    await expect.poll(() => learnMore.element() !== null).toBe(true)

    const control = learnMore.element()
    expectHTMLElement(control)
    expect(control.textContent).toContain('Learn more')
    expect(control.tabIndex).toBe(0)

    control.focus()
    expect(document.activeElement).toBe(control)
    await userEvent.keyboard('{Enter}')

    await expect.poll(() => control.tabIndex).toBe(-1)
  })

  it('keeps the attribution footer visually secondary to the product value', async () => {
    await page.viewport(1280, 800)
    await render(
      <RouterProvider path="/" onNavigate={() => {}}>
        <div
          data-testid="landing-hierarchy-host"
          style={{ width: '100vw', height: '100vh', display: 'flex' }}
        >
          <Landing />
        </div>
      </RouterProvider>,
    )

    await expect
      .poll(
        () =>
          page.getByRole('button', { name: 'the community' }).element() !==
          null,
      )
      .toBe(true)

    const community = page
      .getByRole('button', { name: 'the community' })
      .element()
    const productValue = page.getByText(/local-first/).element()
    const landingSurface = document.querySelector(
      '[data-testid="landing-hierarchy-host"]',
    )?.firstElementChild

    expectHTMLElement(community)
    expectHTMLElement(productValue)
    expectHTMLElement(landingSurface)

    const surfaceBackground = getComputedStyle(landingSurface).backgroundColor
    const footerContrast = compositedContrastRatio(
      surfaceBackground,
      getComputedStyle(community).color,
    )
    const productValueContrast = compositedContrastRatio(
      surfaceBackground,
      getComputedStyle(productValue).color,
    )

    expect(footerContrast).toBeLessThan(productValueContrast)
  })
})
