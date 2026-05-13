import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'

import { LoadingScreen } from './LoadingScreen.js'

vi.mock('@s4wave/app/landing/AnimatedLogo.js', () => ({
  default: () => <div data-testid="animated-logo" />,
}))

afterEach(() => {
  cleanup()
})

describe('quickstart LoadingScreen', () => {
  it('shows default quickstart progress before the first setup event', () => {
    render(<LoadingScreen quickstartId="drive" />)

    expect(screen.getByText('Setting up drive')).toBeDefined()
    expect(screen.getByText('Local Session')).toBeDefined()
    expect(screen.getByText('Frame-Ready')).toBeDefined()
    expect(screen.getByText('Content-Ready')).toBeDefined()
  })

  it('shows specific setup progress for the active phase', () => {
    render(
      <LoadingScreen
        quickstartId="drive"
        progress={{
          step: 'content',
          stepIndex: 4,
          stepCount: 4,
          detail: 'Seeding My Drive content',
        }}
      />,
    )

    expect(screen.getByText('Seeding My Drive content')).toBeDefined()
    expect(screen.getByText('88%')).toBeDefined()
  })

  it('keeps local-only quickstart progress scoped to session setup', () => {
    render(<LoadingScreen quickstartId="local" />)

    expect(screen.getByText('Local Session')).toBeDefined()
    expect(screen.queryByText('Space')).toBeNull()
  })
})
