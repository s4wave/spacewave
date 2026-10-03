import type { ReactNode } from 'react'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { BackgroundPlugin } from '@s4wave/core/session/session.pb.js'

import { SessionBackgroundPluginsButton } from './SessionBackgroundPluginsButton.js'

const mocks = vi.hoisted(() => ({
  set: vi.fn().mockResolvedValue(undefined),
  toggle: vi.fn(),
}))

let entries: BackgroundPlugin[] = []

vi.mock('./useBackgroundPlugins.js', () => ({
  useBackgroundPlugins: () => ({ entries, set: mocks.set }),
}))

vi.mock('@s4wave/app/system/useSystemStatus.js', () => ({
  useWatchSpacesList: () => [
    {
      entry: { ref: { providerResourceRef: { id: 'space-1' } } },
      spaceMeta: { name: 'Home' },
    },
  ],
}))

vi.mock('@s4wave/web/frame/bottom-bar-level.js', () => ({
  BottomBarLevel: (props: {
    id: string
    button: (
      selected: boolean,
      onClick: () => void,
      className?: string,
    ) => ReactNode
  }) => (
    <div data-testid={`bottom-bar-level-${props.id}`}>
      {props.button(true, mocks.toggle, '')}
    </div>
  ),
}))

describe('SessionBackgroundPluginsButton', () => {
  beforeEach(() => {
    entries = []
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('hides while no plugin runs in the background', () => {
    render(<SessionBackgroundPluginsButton />)

    expect(screen.queryByTestId('session-background-plugins-button')).toBeNull()
  })

  it('lists each Space and suspends exactly the chosen plugin', async () => {
    entries = [
      { spaceId: 'space-1', pluginId: 'gizmo-matrix' },
      { spaceId: 'space-1', pluginId: 'spacewave-notes', suspended: true },
    ]

    render(<SessionBackgroundPluginsButton />)

    expect(screen.getByText('Home')).toBeDefined()
    expect(screen.getByText('Running')).toBeDefined()
    expect(screen.getByText('Suspended')).toBeDefined()
    fireEvent.click(
      screen.getByRole('button', { name: 'Suspend gizmo-matrix' }),
    )

    await waitFor(() =>
      expect(mocks.set).toHaveBeenCalledWith(
        'space-1',
        'gizmo-matrix',
        true,
        true,
      ),
    )
  })

  it('resumes a suspended plugin', async () => {
    entries = [
      { spaceId: 'space-1', pluginId: 'gizmo-matrix', suspended: true },
    ]

    render(<SessionBackgroundPluginsButton />)
    fireEvent.click(screen.getByRole('button', { name: 'Resume gizmo-matrix' }))

    await waitFor(() =>
      expect(mocks.set).toHaveBeenCalledWith(
        'space-1',
        'gizmo-matrix',
        true,
        false,
      ),
    )
  })
})
