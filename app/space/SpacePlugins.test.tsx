import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { PluginApprovalState } from '@s4wave/core/plugin/approval/approval.pb.js'
import type { SpaceContentsState } from '@s4wave/sdk/space/space.pb.js'

const mocks = vi.hoisted(() => ({
  useResourceValue: vi.fn(),
  useWatchStateRpc: vi.fn(),
  setPluginApproval: vi.fn(),
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
      value: {
        setPluginApproval: mocks.setPluginApproval,
      },
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
    mocks.useResourceValue.mockReturnValue({
      setPluginApproval: mocks.setPluginApproval,
    })
    mocks.useWatchStateRpc.mockImplementation(() => contentsState)
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('shows pending and denied plugin approval states', () => {
    contentsState = {
      plugins: [
        {
          pluginId: 'pending-plugin',
          approvalState: PluginApprovalState.PluginApprovalState_UNSPECIFIED,
        },
        {
          pluginId: 'denied-plugin',
          approvalState: PluginApprovalState.PluginApprovalState_DENIED,
        },
      ],
    }

    render(<SpacePlugins />)

    expect(screen.getByText('pending-plugin')).toBeDefined()
    expect(screen.getByText('denied-plugin')).toBeDefined()
    expect(screen.getByText('Pending')).toBeDefined()
    expect(screen.getByText('Denied')).toBeDefined()
  })

  it('distinguishes approved loading and loaded plugin states', () => {
    contentsState = {
      plugins: [
        {
          pluginId: 'loading-plugin',
          approvalState: PluginApprovalState.PluginApprovalState_APPROVED,
          loaded: false,
        },
        {
          pluginId: 'loaded-plugin',
          approvalState: PluginApprovalState.PluginApprovalState_APPROVED,
          loaded: true,
        },
      ],
    }

    render(<SpacePlugins />)

    expect(screen.getAllByText('Approved')).toHaveLength(2)
    expect(screen.getByText('Loading')).toBeDefined()
    expect(screen.getByText('Loaded')).toBeDefined()
  })
})
