import { describe, expect, it, vi } from 'vitest'
import { page, userEvent } from 'vitest/browser'
import { render } from 'vitest-browser-react'

import { ViewerBar, type ViewerBarAction } from './ViewerBar.js'

// Pane widths in each band: cramped under 20rem, narrow under 28rem, wide.
const cramped = 'w-72'
const narrow = 'w-96'
const wide = 'w-128'

function Pane({
  width,
  actions,
}: {
  width: string
  actions: ViewerBarAction[]
}) {
  return (
    <div className={`viewer-pane h-40 ${width}`}>
      <ViewerBar title="Production bastion SSH host" actions={actions} />
    </div>
  )
}

function press(id: string, extra?: Partial<ViewerBarAction>): ViewerBarAction {
  return { id, label: id, onClick: vi.fn(), ...extra }
}

// shown lists the bar's presses a person can see, More included.
function shown(): string[] {
  const header = document.querySelector('header')
  if (!header) return []
  return Array.from(header.querySelectorAll('button'))
    .filter((el) => el.getClientRects().length > 0)
    .map((el) => el.getAttribute('aria-label') ?? el.textContent ?? '')
}

function listed(): string[] {
  return Array.from(document.querySelectorAll('[role="menuitem"]')).map(
    (el) => el.textContent ?? '',
  )
}

function focused(): string {
  const el = document.activeElement
  if (!el || el === document.body) return 'page'
  if (el.tagName === 'HEADER') return 'bar'
  return el.getAttribute('aria-label') ?? el.textContent ?? el.tagName
}

describe('ViewerBar', () => {
  it('folds each press at its fold point and More lists the folded ones', async () => {
    const actions = [press('a'), press('b', { fold: 'md' })]
    const screen = await render(<Pane width={cramped} actions={actions} />)
    expect(shown()).toEqual(['More'])

    await screen.rerender(<Pane width={narrow} actions={actions} />)
    expect(shown()).toEqual(['a', 'More'])

    await screen.rerender(<Pane width={wide} actions={actions} />)
    expect(shown()).toEqual(['a', 'b'])

    // A "menu" press stays in More at every width.
    const menu = [...actions, press('c', { fold: 'menu' })]
    await screen.rerender(<Pane width={wide} actions={menu} />)
    expect(shown()).toEqual(['a', 'b', 'More'])
    await userEvent.click(page.getByRole('button', { name: 'More' }))
    await expect.poll(listed).toEqual(['c'])
  })

  it('rereads an open More on resize and sends focus to the bar when it closes', async () => {
    const actions = [press('a', { fold: 'md' }), press('b', { fold: 'md' })]
    const screen = await render(<Pane width={cramped} actions={actions} />)
    await userEvent.click(page.getByRole('button', { name: 'More' }))
    await expect.poll(listed).toEqual(['a', 'b'])

    await screen.rerender(<Pane width={narrow} actions={actions} />)
    await expect.poll(listed).toEqual(['a', 'b'])

    // Past the last fold More leaves the bar; focus goes to the first press.
    await screen.rerender(<Pane width={wide} actions={actions} />)
    await expect.poll(listed).toEqual([])
    await expect.poll(focused).toBe('a')
  })

  it('rereads an open More when the presses change', async () => {
    const one = [press('a'), press('b', { fold: 'md' })]
    const screen = await render(<Pane width={narrow} actions={one} />)
    await userEvent.click(page.getByRole('button', { name: 'More' }))
    await expect.poll(listed).toEqual(['b'])

    const two = [...one, press('c', { fold: 'md' })]
    await screen.rerender(<Pane width={narrow} actions={two} />)
    await expect.poll(listed).toEqual(['b', 'c'])

    // With nothing folded More closes, and focus stays in the bar.
    await screen.rerender(<Pane width={narrow} actions={[press('a')]} />)
    await expect.poll(listed).toEqual([])
    await expect.poll(focused).toBe('a')
  })

  it('keeps focus on a press while its action runs', async () => {
    const run = vi.fn()
    const idle = [press('open', { label: 'Open', onClick: run })]
    const screen = await render(<Pane width={wide} actions={idle} />)
    // A keyboard user starts the action; WebKit does not focus a clicked button.
    ;(
      page.getByRole('button', { name: 'Open' }).element() as HTMLElement
    ).focus()
    await userEvent.keyboard('{Enter}')
    expect(run).toHaveBeenCalledOnce()

    const running = [
      press('open', { label: 'Opening…', onClick: run, disabled: true }),
    ]
    await screen.rerender(<Pane width={wide} actions={running} />)
    expect(focused()).toBe('Opening…')
    await userEvent.keyboard('{Enter}')
    expect(run).toHaveBeenCalledOnce()
  })
})
