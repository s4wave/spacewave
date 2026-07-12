import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'

import { PairingCodeChip } from './PairingCodeChip.js'

describe('PairingCodeChip', () => {
  const writeText = vi.fn().mockResolvedValue(undefined)

  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  function setup() {
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      writable: true,
      configurable: true,
    })
    writeText.mockClear()
  }

  it('renders the code grouped into two blocks of four', () => {
    setup()
    render(<PairingCodeChip code="ABCD1234" />)
    expect(screen.getByText('ABCD 1234')).toBeDefined()
    const codeButton = screen.getByRole('button', { name: 'Copy pairing code' })
    expect(codeButton.querySelector('svg')).not.toBeNull()
  })

  it('copies from the code itself and shows a brief confirmation', async () => {
    setup()
    render(<PairingCodeChip code="ABCD1234" />)

    const codeButton = screen.getByRole('button', { name: 'Copy pairing code' })
    fireEvent.click(codeButton)

    expect(writeText).toHaveBeenCalledWith('ABCD1234')
    await waitFor(() => {
      expect(screen.getByText('Copied')).toBeDefined()
    })
    expect(screen.getByText('Copied').closest('span')?.className).toContain(
      'text-success',
    )
    expect(codeButton.textContent).toContain('ABCD 1234')
    expect(codeButton.querySelector('svg')).not.toBeNull()
    expect(screen.getAllByRole('button')).toHaveLength(1)
  })
})
