import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  type VmCreationProgress,
  VmCreationProgressScreen,
} from './VmCreationProgressScreen.js'

afterEach(cleanup)

function renderProgress(
  progress: VmCreationProgress,
  options?: { error?: string; onRetry?: () => void },
) {
  return render(
    <VmCreationProgressScreen
      progress={progress}
      vmName="Debian lab"
      includesCdnCopy
      error={options?.error}
      onRetry={options?.onRetry}
    />,
  )
}

describe('VmCreationProgressScreen', () => {
  it.each([
    ['fetching', 'Resolving the image and destination Space.'],
    ['creating', 'Image ready. Writing the VM object.'],
    ['ready', 'Debian lab is ready. Opening it now.'],
  ] as const)('renders the %s progress event', (stage, detail) => {
    renderProgress({ stage })

    expect(screen.getByText(detail)).toBeTruthy()
    expect(screen.getByText('Fetching image from CDN')).toBeTruthy()
    expect(screen.getByText('Copying blocks')).toBeTruthy()
    expect(screen.getByText('Creating VM')).toBeTruthy()
    expect(screen.getByText('Ready')).toBeTruthy()
  })

  it('renders real block accounting without inventing a fraction', () => {
    renderProgress({
      stage: 'copying',
      blocksSeen: 14n,
      blocksCopied: 12n,
      blocksWritten: 9n,
      logicalSourceBytes: 1572864n,
    })

    expect(
      screen.getByText('14 seen · 12 copied · 9 written · 1.5 MB'),
    ).toBeTruthy()
    expect(screen.queryByText(/%$/)).toBeNull()
  })

  it('renders a recoverable error and retries the stream', async () => {
    const user = userEvent.setup()
    const onRetry = vi.fn()
    renderProgress(
      { stage: 'copying', blocksCopied: 3n, blocksWritten: 2n },
      {
        error: 'The image copy stopped. Try again to continue.',
        onRetry,
      },
    )

    expect(screen.getByText('VM could not be created')).toBeTruthy()
    expect(
      screen.getByText('The image copy stopped. Try again to continue.'),
    ).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(onRetry).toHaveBeenCalledTimes(1)
  })
})
