import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { PairingChannelProgress } from './PairingChannelProgress.js'

describe('PairingChannelProgress', () => {
  it('presents channel setup as ordered progress phases', () => {
    render(<PairingChannelProgress />)

    expect(
      screen.getByRole('heading', { name: 'Establishing encrypted channel' }),
    ).toBeDefined()
    expect(
      screen.getByText('Setting up the connection with your other device…'),
    ).toBeDefined()
    expect(screen.getByText('Connecting to your other device')).toBeDefined()
    expect(screen.getAllByText('Establishing encrypted channel')).toHaveLength(
      2,
    )
    expect(screen.getByText('Preparing connection verification')).toBeDefined()
  })
})
