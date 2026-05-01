import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { SetSpaceSettingsOp } from '@s4wave/core/space/world/ops/ops.pb.js'

const mockOpenCommand = vi.hoisted(() => vi.fn())
const mockUseSpaceContainer = vi.hoisted(() => vi.fn())
const mockToastSuccess = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/command/CommandContext.js', () => ({
  useOpenCommand: () => mockOpenCommand,
}))

vi.mock('@s4wave/web/contexts/SpaceContainerContext.js', () => ({
  SpaceContainerContext: {
    useContext: mockUseSpaceContainer,
  },
}))

vi.mock('@s4wave/web/ui/toaster.js', () => ({
  toast: {
    success: mockToastSuccess,
  },
}))

import { SpaceIndex } from './SpaceIndex.js'

describe('SpaceIndex', () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('redirects to the configured index path through the space navigator', async () => {
    const navigateToSubPath = vi.fn()
    const applyWorldOp = vi.fn()
    mockUseSpaceContainer.mockReturnValue({
      spaceState: {
        settings: { indexPath: 'files' },
        worldContents: {
          objects: [{ objectKey: 'files', objectType: 'unixfs/fs-node' }],
        },
      },
      spaceWorld: { applyWorldOp },
      navigateToSubPath,
    })

    render(<SpaceIndex />)

    await waitFor(() => {
      expect(navigateToSubPath).toHaveBeenCalledWith('files')
    })
    expect(navigateToSubPath).toHaveBeenCalledTimes(1)
    expect(applyWorldOp).not.toHaveBeenCalled()
  })

  it('repairs a stale index path to the matching numbered object', async () => {
    const navigateToSubPath = vi.fn()
    const applyWorldOp = vi.fn().mockResolvedValue({ seqno: 1n, sysErr: false })
    mockUseSpaceContainer.mockReturnValue({
      spaceState: {
        settings: { indexPath: 'files', pluginIds: ['spacewave-app'] },
        worldContents: {
          objects: [
            { objectKey: 'files-1', objectType: 'unixfs/fs-node' },
            { objectKey: 'settings', objectType: 'space/settings' },
          ],
        },
      },
      spaceWorld: { applyWorldOp },
      navigateToSubPath,
    })

    render(<SpaceIndex />)

    await waitFor(() => {
      expect(navigateToSubPath).toHaveBeenCalledWith('files-1')
    })
    await waitFor(() => {
      expect(applyWorldOp).toHaveBeenCalledTimes(1)
    })

    const opData = applyWorldOp.mock.calls[0]?.[1]
    const op = SetSpaceSettingsOp.fromBinary(opData)
    expect(op.settings?.indexPath).toBe('files-1')
    expect(op.settings?.pluginIds).toEqual(['spacewave-app'])
    expect(mockToastSuccess).toHaveBeenCalledWith(
      'Default object updated to files-1',
    )
  })

  it('renders the empty state when no index path is configured', () => {
    mockUseSpaceContainer.mockReturnValue({
      spaceState: {
        settings: { indexPath: '' },
        worldContents: { objects: [] },
      },
      spaceWorld: { applyWorldOp: vi.fn() },
      navigateToSubPath: vi.fn(),
    })

    render(<SpaceIndex />)

    expect(screen.getByText('Empty Space')).toBeDefined()
  })
})
