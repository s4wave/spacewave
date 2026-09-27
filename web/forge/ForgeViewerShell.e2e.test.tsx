import { expect, it, vi } from 'vitest'
import { page, userEvent } from 'vitest/browser'
import { render } from 'vitest-browser-react'

import { ForgeViewerShell } from './ForgeViewerShell.js'

it('keeps Forge navigation and actions reachable through touch rotation', async () => {
  // Exercise each control region, including compact controls supplied by a viewer.
  const coarse = matchMedia('(pointer: coarse)').matches
  const openTask = vi.fn()
  await render(
    <div className="h-dvh w-full">
      <ForgeViewerShell
        icon={<span>F</span>}
        title="Job"
        stateKey="touch-job"
        headerActions={[{ label: 'Create Job', onClick: vi.fn() }]}
        actions={[{ label: 'Start Worker', onClick: vi.fn() }]}
        tabs={[
          { id: 'overview', label: 'Overview', content: 'Overview' },
          {
            id: 'tasks',
            label: 'Tasks',
            content: (
              <button
                type="button"
                className="h-7 px-2 text-xs"
                onClick={openTask}
              >
                Open task
              </button>
            ),
          },
        ]}
      />
    </div>,
  )
  await userEvent.click(
    page.getByRole('button', { name: 'Tasks', exact: true }),
  )

  // The shell supplies the touch floor while the mouse layout keeps its compact controls.
  for (const [width, height] of [
    [390, 844],
    [360, 780],
    [844, 390],
  ]) {
    await page.viewport(width, height)
    for (const name of [
      'Create Job',
      'Overview',
      'Tasks',
      'Open task',
      'Start Worker',
    ]) {
      const rect = page
        .getByRole('button', { name, exact: true })
        .element()
        .getBoundingClientRect()
      if (coarse || width < 640) {
        expect(rect.width).toBeGreaterThanOrEqual(44)
        expect(rect.height).toBeGreaterThanOrEqual(44)
      } else {
        expect(rect.height).toBeLessThan(44)
      }
      expect(rect.x).toBeGreaterThanOrEqual(0)
      expect(rect.y).toBeGreaterThanOrEqual(0)
      expect(rect.right).toBeLessThanOrEqual(width)
      expect(rect.bottom).toBeLessThanOrEqual(height)
    }
    expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(width)
  }

  // The enlarged content target retains its action.
  await userEvent.click(
    page.getByRole('button', { name: 'Open task', exact: true }),
  )
  expect(openTask).toHaveBeenCalledOnce()
})
