import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'

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
  })

  it('copies the ungrouped code to the clipboard', () => {
    setup()
    render(<PairingCodeChip code="ABCD1234" />)
    fireEvent.click(screen.getByRole('button', { name: 'Copy pairing code' }))
    expect(writeText).toHaveBeenCalledWith('ABCD1234')
  })
})
