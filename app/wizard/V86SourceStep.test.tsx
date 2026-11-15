import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { V86Image } from '@s4wave/sdk/vm/v86.pb.js'
import { V86WizardConfig } from '@s4wave/sdk/vm/v86-wizard.pb.js'

import { V86SourceStep } from './V86SourceStep.js'

afterEach(cleanup)

// retryOn returns the Retry button of the card titled title: the nearest
// ancestor of the title that holds a Retry button.
function retryOn(title: string): HTMLElement {
  let el: HTMLElement | null = screen.getByText(title)
  while (el && !within(el).queryByRole('button', { name: 'Retry' })) {
    el = el.parentElement
  }
  if (!el) throw new Error('no Retry near ' + title)
  return within(el).getByRole('button', { name: 'Retry' })
}

// partialImages is the first page of a listing still being read.
const partialImages = [
  { objectKey: 'vm/image/1', image: V86Image.create({ name: 'Alpine' }) },
]

describe('V86SourceStep', () => {
  it('shows a failed listing with Retry instead of loading', async () => {
    const user = userEvent.setup()
    const retry = vi.fn()
    render(
      <V86SourceStep
        cfg={V86WizardConfig.create()}
        existingDefault={undefined}
        inSpaceImages={[]}
        pending
        failures={[
          {
            title: 'Could not read the VM images in this Space',
            message: 'world closed',
            retry,
          },
        ]}
        onSelectInSpace={() => {}}
        onOpenCatalog={() => {}}
      />,
    )

    expect(
      screen.getByText('Could not read the VM images in this Space'),
    ).toBeTruthy()
    expect(screen.getByText('world closed')).toBeTruthy()
    expect(screen.queryByText('Looking for VM images in this Space')).toBeNull()
    expect(screen.queryByText('No VM images in this Space')).toBeNull()
    await user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(retry).toHaveBeenCalledOnce()
  })

  it('shows loading beside the images read so far', () => {
    render(
      <V86SourceStep
        cfg={V86WizardConfig.create()}
        existingDefault={undefined}
        inSpaceImages={partialImages}
        pending
        onSelectInSpace={() => {}}
        onOpenCatalog={() => {}}
      />,
    )

    expect(screen.getByText('Alpine')).toBeTruthy()
    expect(screen.getByText('Looking for VM images in this Space')).toBeTruthy()
    expect(screen.queryByText('Add image from catalog')).toBeNull()
  })

  it('keeps the images read so far under each failure', async () => {
    const user = userEvent.setup()
    const retryImages = vi.fn()
    const retryVms = vi.fn()
    render(
      <V86SourceStep
        cfg={V86WizardConfig.create()}
        existingDefault={undefined}
        inSpaceImages={partialImages}
        pending
        failures={[
          {
            title: 'Could not read the VM images in this Space',
            message: 'stream reset',
            retry: retryImages,
          },
          {
            title: 'Could not read the VMs in this Space',
            message: 'world closed',
            retry: retryVms,
          },
        ]}
        onSelectInSpace={() => {}}
        onOpenCatalog={() => {}}
      />,
    )

    expect(
      screen.getByText('Could not read the VM images in this Space'),
    ).toBeTruthy()
    expect(
      screen.getByText('Could not read the VMs in this Space'),
    ).toBeTruthy()
    expect(screen.getAllByRole('button', { name: 'Retry' })).toHaveLength(2)
    expect(screen.getByText('Alpine')).toBeTruthy()
    expect(screen.queryByText('Looking for VM images in this Space')).toBeNull()

    await user.click(retryOn('Could not read the VMs in this Space'))
    expect(retryVms).toHaveBeenCalledOnce()
    expect(retryImages).not.toHaveBeenCalled()
    await user.click(retryOn('Could not read the VM images in this Space'))
    expect(retryImages).toHaveBeenCalledOnce()
  })
})
