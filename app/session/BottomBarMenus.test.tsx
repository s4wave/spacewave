import { useState } from 'react'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'
import { BottomBarRoot } from '@s4wave/web/frame/bottom-bar-root.js'
import { ViewerFrame } from '@s4wave/web/frame/ViewerFrame.js'
import { SystemStatusButton } from '@s4wave/app/system/SystemStatusButton.js'
import { SessionSyncStatusButton } from './SessionSyncStatusButton.js'
import { SessionBackgroundPluginsButton } from './SessionBackgroundPluginsButton.js'

vi.mock('./useBackgroundPlugins.js', () => ({
  useBackgroundPlugins: () => ({
    entries: [{ spaceId: 'space-1', pluginId: 'glados-matrix' }],
    set: vi.fn(),
  }),
}))
vi.mock('@s4wave/app/system/useSystemStatus.js', () => ({
  useWatchSpacesList: () => [],
}))
vi.mock('@s4wave/app/system/SystemStatusDashboard.js', () => ({
  SystemStatusDashboard: ({ onClose }: { onClose: () => void }) => (
    <button type="button" onClick={onClose}>
      Close dashboard
    </button>
  ),
}))

function Harness() {
  const [openMenu, setOpenMenu] = useState('')
  return (
    <BottomBarRoot openMenu={openMenu} setOpenMenu={setOpenMenu}>
      <SessionBackgroundPluginsButton />
      <SystemStatusButton />
      <SessionSyncStatusButton />
      <ViewerFrame>
        <button type="button">Content</button>
      </ViewerFrame>
      <output data-testid="open-menu">{openMenu}</output>
    </BottomBarRoot>
  )
}

afterEach(cleanup)
it('switches from Background plugins to only System status on pointer activation', async () => {
  const user = userEvent.setup()
  render(<Harness />)
  await user.click(screen.getByLabelText('Open system status view'))
  expect(screen.getByTestId('open-menu').textContent).toBe('system-status')
  expect(screen.getByRole('button', { name: 'Close dashboard' })).toBeTruthy()
  expect(screen.queryByText('Background plugins')).toBeNull()
  await user.click(screen.getByLabelText('Background plugins: 1 running'))
  expect(screen.getByTestId('open-menu').textContent).toBe(
    'session-background-plugins',
  )
  expect(screen.getByText('Background plugins')).toBeTruthy()
  expect(screen.queryByRole('button', { name: 'Close dashboard' })).toBeNull()
  await user.click(screen.getByLabelText('Open system status view'))
  expect(screen.getByTestId('open-menu').textContent).toBe('system-status')
  expect(screen.getByRole('button', { name: 'Close dashboard' })).toBeTruthy()
  expect(screen.queryByText('Background plugins')).toBeNull()
})

it('switches away from Sync and closes the selected popover from outside or its trigger', async () => {
  const user = userEvent.setup()
  render(<Harness />)
  const sync = screen.getByRole('button', {
    name: 'Session sync status: checking',
  })
  await user.click(sync)
  expect(screen.getByText('Checking sync status')).toBeTruthy()

  await user.click(
    screen.getByRole('button', { name: 'Open system status view' }),
  )
  expect(screen.getByRole('button', { name: 'Close dashboard' })).toBeTruthy()
  expect(screen.queryByText('Checking sync status')).toBeNull()
  await user.click(screen.getByRole('button', { name: 'Close dashboard' }))
  expect(screen.getByTestId('open-menu').textContent).toBe('')

  await user.click(sync)
  await user.click(screen.getByRole('button', { name: 'Content' }))
  expect(screen.queryByText('Checking sync status')).toBeNull()

  await user.click(sync)
  await user.click(sync)
  expect(screen.queryByText('Checking sync status')).toBeNull()
})

it('supports keyboard activation and Escape dismissal', async () => {
  const user = userEvent.setup()
  render(<Harness />)
  await user.tab()
  await user.tab()
  const plugins = screen.getByRole('button', {
    name: 'Background plugins: 1 running',
  })
  expect(document.activeElement).toBe(plugins)
  await user.keyboard('{Enter}')
  expect(screen.getByText('Background plugins')).toBeTruthy()
  await user.keyboard('{Escape}')
  expect(screen.queryByText('Background plugins')).toBeNull()
  expect(document.activeElement).toBe(plugins)

  await user.tab()
  await user.keyboard(' ')
  expect(screen.getByRole('button', { name: 'Close dashboard' })).toBeTruthy()
  await user.keyboard('{Escape}')
  expect(screen.queryByRole('button', { name: 'Close dashboard' })).toBeNull()
})
