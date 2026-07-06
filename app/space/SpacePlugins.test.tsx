import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  SpacePluginLifecycleState,
  type SpaceContentsState,
} from '@s4wave/sdk/space/space.pb.js'

const mocks = vi.hoisted(() => ({
  useResourceValue: vi.fn(),
  useWatchStateRpc: vi.fn(),
  addSpacePlugin: vi.fn().mockResolvedValue(undefined),
  removeSpacePlugin: vi.fn().mockResolvedValue(undefined),
  toastError: vi.fn(),
}))

let contentsState: SpaceContentsState | null = null

const spaceResource = { kind: 'space' }
const contentsResource = { kind: 'contents' }
const spaceMock = {
  addSpacePlugin: mocks.addSpacePlugin,
  removeSpacePlugin: mocks.removeSpacePlugin,
}

vi.mock('@aptre/bldr-react', () => ({
  useWatchStateRpc: mocks.useWatchStateRpc,
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: mocks.useResourceValue,
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SpaceContext: { useContext: () => spaceResource },
  SpaceContentsContext: { useContext: () => contentsResource },
}))

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: { error: mocks.toastError },
}))

import { SpacePlugins } from './SpacePlugins.js'

describe('SpacePlugins', () => {
  beforeEach(() => {
    contentsState = null
    mocks.useResourceValue.mockImplementation((res: unknown) =>
      res === spaceResource ? spaceMock : {},
    )
    mocks.useWatchStateRpc.mockImplementation(() => contentsState)
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('shows configured plugins with lifecycle badges', () => {
    contentsState = {
      plugins: [
        {
          pluginId: 'loading-plugin',
          loaded: false,
          state: SpacePluginLifecycleState.SpacePluginLifecycleState_LOADING,
        },
        {
          pluginId: 'loaded-plugin',
          loaded: true,
          state: SpacePluginLifecycleState.SpacePluginLifecycleState_LOADED,
        },
      ],
    }

    render(<SpacePlugins />)

    expect(screen.getByText('loading-plugin')).toBeDefined()
    expect(screen.getByText('loaded-plugin')).toBeDefined()
    expect(screen.getByText('Loading')).toBeDefined()
    expect(screen.getByText('Loaded')).toBeDefined()
    expect(screen.getByText('2 installed')).toBeDefined()
  })

  it('shows every projected lifecycle label and scheduler detail', () => {
    contentsState = {
      plugins: [
        {
          pluginId: 'configured-plugin',
          state: SpacePluginLifecycleState.SpacePluginLifecycleState_CONFIGURED,
        },
        {
          pluginId: 'failed-plugin',
          state: SpacePluginLifecycleState.SpacePluginLifecycleState_FAILED,
          detail: 'fetch plugin manifest: copy failed',
        },
        {
          pluginId: 'retrying-plugin',
          state: SpacePluginLifecycleState.SpacePluginLifecycleState_RETRYING,
        },
        {
          pluginId: 'removed-plugin',
          state: SpacePluginLifecycleState.SpacePluginLifecycleState_REMOVED,
        },
        {
          pluginId: 'upgraded-plugin',
          state: SpacePluginLifecycleState.SpacePluginLifecycleState_UPGRADED,
        },
      ],
    }

    render(<SpacePlugins />)

    for (const label of [
      'Configured',
      'Failed',
      'Retrying',
      'Removed',
      'Upgraded',
    ]) {
      expect(screen.getByText(label)).toBeDefined()
    }
    expect(screen.getByText('fetch plugin manifest: copy failed')).toBeDefined()
  })

  it('shows an empty state and add control with no plugins', () => {
    render(<SpacePlugins />)

    expect(screen.getByText('No plugins installed')).toBeDefined()
    expect(screen.getByText('0 installed')).toBeDefined()
    expect(screen.getByLabelText('Add plugin')).toBeDefined()
  })

  it('opens the add form with known-plugin suggestions', () => {
    render(<SpacePlugins />)

    fireEvent.click(screen.getByLabelText('Add plugin'))

    expect(screen.getByLabelText('Plugin manifest ID')).toBeDefined()
    expect(screen.getByText('Notes')).toBeDefined()
    expect(screen.getByText('V86 VM')).toBeDefined()
  })

  it('installs a suggested plugin via addSpacePlugin', () => {
    render(<SpacePlugins />)

    fireEvent.click(screen.getByLabelText('Add plugin'))
    fireEvent.click(screen.getByText('Notes'))

    expect(mocks.addSpacePlugin).toHaveBeenCalledWith('spacewave-notes')
  })

  it('hides already-installed plugins from suggestions', () => {
    contentsState = {
      plugins: [
        {
          pluginId: 'spacewave-notes',
          state: SpacePluginLifecycleState.SpacePluginLifecycleState_LOADED,
        },
      ],
    }

    render(<SpacePlugins />)
    fireEvent.click(screen.getByLabelText('Add plugin'))

    expect(screen.queryByText('Notes')).toBeNull()
    expect(screen.getByText('V86 VM')).toBeDefined()
  })

  it('validates and installs a manually entered manifest ID', () => {
    render(<SpacePlugins />)

    fireEvent.click(screen.getByLabelText('Add plugin'))
    const input = screen.getByLabelText('Plugin manifest ID')

    fireEvent.change(input, { target: { value: 'Bad Id' } })
    expect(
      screen.getByText('Use a lowercase manifest ID (letters, digits, dashes)'),
    ).toBeDefined()

    fireEvent.change(input, { target: { value: 'my-plugin' } })
    fireEvent.click(screen.getByRole('button', { name: 'Add' }))

    expect(mocks.addSpacePlugin).toHaveBeenCalledWith('my-plugin')
  })

  it('removes a plugin only after inline confirmation', () => {
    contentsState = {
      plugins: [
        {
          pluginId: 'spacewave-v86',
          state: SpacePluginLifecycleState.SpacePluginLifecycleState_LOADED,
        },
      ],
    }

    render(<SpacePlugins />)

    fireEvent.click(screen.getByLabelText('Remove spacewave-v86'))
    expect(mocks.removeSpacePlugin).not.toHaveBeenCalled()

    expect(screen.getByText('Remove spacewave-v86?')).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: 'Confirm' }))

    expect(mocks.removeSpacePlugin).toHaveBeenCalledWith('spacewave-v86')
  })
})
