import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { IntroWizardRegion } from '@s4wave/sdk/world/wizard/wizard.pb.js'

import { IntroWizardOverlay } from './IntroWizardOverlay.js'
import { driveIntroConfig } from './intro.js'

const config = driveIntroConfig()

function renderOverlay(overrides?: {
  finishing?: boolean
  onFinish?: () => void
  onSkip?: () => void
}) {
  return render(
    <IntroWizardOverlay
      headline={config.headline ?? ''}
      subhead={config.subhead ?? ''}
      finishLabel={config.finishLabel ?? ''}
      callouts={config.callouts ?? []}
      finishing={overrides?.finishing ?? false}
      onFinish={overrides?.onFinish ?? vi.fn()}
      onSkip={overrides?.onSkip ?? vi.fn()}
    />,
  )
}

describe('IntroWizardOverlay', () => {
  afterEach(() => {
    cleanup()
  })

  it('draws the callouts and control panel around the frame', () => {
    renderOverlay()

    expect(screen.getByText('Welcome to your Drive')).toBeTruthy()
    expect(screen.getByText('Add files')).toBeTruthy()
    expect(screen.getByText('Upload progress')).toBeTruthy()
    expect(
      screen.getByRole('button', { name: 'Got it, start exploring' }),
    ).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Skip' })).toBeTruthy()
  })

  it('leaves the bottom-right upload-indicator anchor lit via a two-panel scrim', () => {
    renderOverlay()

    // A single full-inset scrim would dim the indicator the BOTTOM_RIGHT callout
    // points at; the cutout tiles two panels around a lit corner window instead.
    const scrims = screen.getAllByTestId('intro-scrim')
    expect(scrims).toHaveLength(2)
    for (const scrim of scrims) {
      expect(scrim.className).not.toContain('inset-0')
    }
    expect(
      (config.callouts ?? []).some(
        (callout) => callout.region === IntroWizardRegion.BOTTOM_RIGHT,
      ),
    ).toBe(true)
  })

  it('shows the finishing state and disables both controls', () => {
    renderOverlay({ finishing: true })

    const finish = screen.getByRole('button', { name: 'Opening...' })
    const skip = screen.getByRole('button', { name: 'Skip' })
    expect((finish as HTMLButtonElement).disabled).toBe(true)
    expect((skip as HTMLButtonElement).disabled).toBe(true)
  })

  it('invokes finish and skip callbacks', async () => {
    const user = userEvent.setup()
    const onFinish = vi.fn()
    const onSkip = vi.fn()
    renderOverlay({ onFinish, onSkip })

    await user.click(
      screen.getByRole('button', { name: 'Got it, start exploring' }),
    )
    await user.click(screen.getByRole('button', { name: 'Skip' }))

    expect(onFinish).toHaveBeenCalledTimes(1)
    expect(onSkip).toHaveBeenCalledTimes(1)
  })
})
