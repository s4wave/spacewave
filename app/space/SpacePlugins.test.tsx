import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  SpacePluginLifecycleState,
  type SpaceContentsState,
} from '@s4wave/sdk/space/space.pb.js'

const mocks = vi.hoisted(() => ({
  useResourceValue: vi.fn(),
  useWatchStateRpc: vi.fn(),
}))

let contentsState: SpaceContentsState | null = null

vi.mock('@aptre/bldr-react', () => ({
  useWatchStateRpc: mocks.useWatchStateRpc,
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: mocks.useResourceValue,
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SpaceContentsContext: {
    useContext: () => ({
      value: {},
      loading: false,
      error: null,
      retry: vi.fn(),
    }),
  },
}))

import { SpacePlugins } from './SpacePlugins.js'

describe('SpacePlugins', () => {
  beforeEach(() => {
    contentsState = null
    mocks.useResourceValue.mockReturnValue({})
    mocks.useWatchStateRpc.mockImplementation(() => contentsState)
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('shows configured plugins with loading state', () => {
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
})
