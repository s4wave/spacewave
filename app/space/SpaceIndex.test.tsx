import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'

const mockOpenCommand = vi.hoisted(() => vi.fn())
const mockUseSpaceContainer = vi.hoisted(() => vi.fn())

vi.mock('@s4wave/web/command/CommandContext.js', () => ({
  useOpenCommand: () => mockOpenCommand,
}))

vi.mock('@s4wave/web/contexts/SpaceContainerContext.js', () => ({
  SpaceContainerContext: {
    useContext: mockUseSpaceContainer,
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
    mockUseSpaceContainer.mockReturnValue({
      spaceState: { settings: { indexPath: 'files' } },
      navigateToSubPath,
    })

    render(<SpaceIndex />)

    await waitFor(() => {
      expect(navigateToSubPath).toHaveBeenCalledWith('files')
    })
    expect(navigateToSubPath).toHaveBeenCalledTimes(1)
  })

  it('renders the empty state when no index path is configured', () => {
    mockUseSpaceContainer.mockReturnValue({
      spaceState: { settings: { indexPath: '' } },
      navigateToSubPath: vi.fn(),
    })

    render(<SpaceIndex />)

    expect(screen.getByText('Empty Space')).toBeDefined()
  })
})
