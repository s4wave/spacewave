import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { State } from '@go/github.com/s4wave/spacewave/forge/task/task.pb.js'

import type { PluginRepositoryInventory } from './plugin-repositories.js'

const mocks = vi.hoisted(() => ({
  space: {
    fetchPluginRepository: vi.fn(),
    validatePluginRepository: vi.fn(),
    buildSpacePlugin: vi.fn(),
  },
  inventory: undefined as PluginRepositoryInventory | undefined,
  watched: undefined as
    | {
        task: { taskState?: number; result?: { success?: boolean } }
        last: boolean
      }
    | undefined,
}))

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResource: () => ({ value: mocks.space, loading: false, error: null }),
}))

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: () => ({
    value: mocks.watched,
    loading: false,
    error: null,
  }),
}))

vi.mock('@s4wave/web/hooks/useWorldQuery.js', () => ({
  useWorldQuery: () => ({
    value: mocks.inventory,
    loading: false,
    error: null,
  }),
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SessionContext: { useContext: () => ({ value: null }) },
}))

vi.mock('@s4wave/web/router/router.js', () => ({
  useNavigate: () => vi.fn(),
}))

import { PluginRepositoriesPage } from './PluginRepositoriesPage.js'

const pinned = 'aaaaaaa1111111111111111111111111111111111'
const newer = 'bbbbbbb2222222222222222222222222222222222'

describe('PluginRepositoriesPage', () => {
  beforeEach(() => {
    for (const mock of Object.values(mocks.space)) mock.mockReset()
    mocks.space.fetchPluginRepository.mockResolvedValue({
      taskKey: 'fetch/task',
    })
    mocks.space.buildSpacePlugin.mockResolvedValue({ taskKey: 'build/task' })
    mocks.inventory = { devices: ['devices/laptop'], repositories: [] }
    mocks.watched = undefined
  })
  afterEach(cleanup)

  it('asks for a linked Device when the Space has none', () => {
    // Render a Space without Devices.
    mocks.inventory = { devices: [], repositories: [] }
    render(<PluginRepositoriesPage />)

    // The page explains the missing Device and refuses to add.
    expect(
      screen.getByText(/needs a desktop or command-line Device/),
    ).toBeDefined()
    fireEvent.change(screen.getByPlaceholderText('owner/repo'), {
      target: { value: 's4wave/spreadsheet' },
    })
    expect(
      screen.getByRole('button', { name: 'Add' }).hasAttribute('disabled'),
    ).toBe(true)
  })

  it('queues the fetch on the Device and reports its Task', async () => {
    // Submit a repository with surrounding spaces.
    const { rerender } = render(<PluginRepositoriesPage />)
    fireEvent.change(screen.getByPlaceholderText('owner/repo'), {
      target: { value: ' s4wave/spreadsheet ' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Add' }))

    // The fetch runs on the only Device and shows progress.
    expect(
      await screen.findByText('Fetching s4wave/spreadsheet on your device.'),
    ).toBeDefined()
    expect(mocks.space.fetchPluginRepository).toHaveBeenCalledWith({
      repository: 's4wave/spreadsheet',
      deviceKey: 'devices/laptop',
    })

    // The completed Task replaces the progress line.
    mocks.watched = {
      task: { taskState: State.TaskState_COMPLETE, result: { success: true } },
      last: true,
    }
    rerender(<PluginRepositoriesPage />)
    expect(screen.getByText('Fetched s4wave/spreadsheet.')).toBeDefined()
  })

  it('shows a fetched commit newer than the pinned one', () => {
    // Render one current and one stale repository.
    mocks.inventory = {
      devices: ['devices/laptop'],
      repositories: [
        { name: 's4wave/current', pinnedCommit: pinned, fetchedCommit: pinned },
        { name: 's4wave/stale', pinnedCommit: pinned, fetchedCommit: newer },
      ],
    }
    render(<PluginRepositoriesPage />)

    // Only the stale one offers its newer commit; checking it refetches.
    expect(screen.getByText('Pinned at aaaaaaa · Up to date')).toBeDefined()
    expect(
      screen.getByText('Pinned at aaaaaaa · Update available: bbbbbbb'),
    ).toBeDefined()
    fireEvent.click(
      screen.getAllByRole('button', { name: 'Check for updates' })[1],
    )
    expect(mocks.space.fetchPluginRepository).toHaveBeenCalledWith({
      repository: 's4wave/stale',
      deviceKey: 'devices/laptop',
    })
  })

  it('reviews the pinned commit and builds it on confirm', async () => {
    // Render one repository whose pinned commit validates.
    mocks.inventory = {
      devices: ['devices/laptop'],
      repositories: [
        { name: 's4wave/colors', pinnedCommit: pinned, fetchedCommit: pinned },
      ],
    }
    mocks.space.validatePluginRepository.mockResolvedValue({
      commit: pinned,
      sourceKey: 'plugin-repos/s4wave/colors/workdir',
      validation: { plugins: [{ manifestId: 'colors' }] },
    })
    render(<PluginRepositoriesPage />)

    // Reviewing validates the repository and opens the sheet.
    fireEvent.click(screen.getByRole('button', { name: 'Review' }))
    expect(await screen.findByText('Review s4wave/colors')).toBeDefined()
    expect(mocks.space.validatePluginRepository).toHaveBeenCalledWith({
      repository: 's4wave/colors',
    })

    // Confirming builds the reviewed commit on the Device.
    fireEvent.click(screen.getByRole('button', { name: 'Build' }))
    expect(
      await screen.findByText('Building s4wave/colors on your device.'),
    ).toBeDefined()
    expect(mocks.space.buildSpacePlugin).toHaveBeenCalledWith({
      sourceKey: 'plugin-repos/s4wave/colors/workdir',
      commit: pinned,
      manifestId: 'colors',
      deviceKey: 'devices/laptop',
      milliCpu: 1000n,
      memoryBytes: 2n << 30n,
    })
  })
})
