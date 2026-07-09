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

  it('shows one callout at a time in order, starting at the TOP upload toolbar', async () => {
    const user = userEvent.setup()
    const calloutTitles = (config.callouts ?? [])
      .map((callout) => callout.title)
      .filter((title): title is string => typeof title === 'string')
    const renderedCallouts = () =>
      calloutTitles.flatMap((title) => screen.queryAllByText(title))

    renderOverlay()

    // The Drive tour opens by pointing "Add files" at the upload toolbar.
    // Only that active callout is mounted, so assistive technology and sighted
    // users encounter one instruction at each step instead of a stacked set.
    expect(config.callouts?.[0]?.title).toBe('Add files')
    expect(config.callouts?.[0]?.region).toBe(IntroWizardRegion.TOP)
    expect(screen.getByText('Welcome to your Drive')).toBeTruthy()
    expect(renderedCallouts()).toHaveLength(1)
    expect(renderedCallouts()[0]?.textContent).toBe('Add files')

    await user.click(screen.getByRole('button', { name: 'Next' }))
    expect(renderedCallouts()).toHaveLength(1)
    expect(renderedCallouts()[0]?.textContent).toBe('Your files')

    await user.click(screen.getByRole('button', { name: 'Next' }))
    expect(renderedCallouts()).toHaveLength(1)
    expect(renderedCallouts()[0]?.textContent).toBe('Upload progress')
  })

  it('walks back to an earlier callout', async () => {
    const user = userEvent.setup()
    renderOverlay()

    // Back is hidden on the first step, then returns to the prior callout.
    expect(screen.queryByRole('button', { name: 'Back' })).toBeNull()
    await user.click(screen.getByRole('button', { name: 'Next' }))
    await user.click(screen.getByRole('button', { name: 'Back' }))
    expect(screen.getByText('Add files')).toBeTruthy()
    expect(screen.queryByText('Your files')).toBeNull()
  })

  it('only offers the finish action on the last step', async () => {
    const user = userEvent.setup()
    const onFinish = vi.fn()
    renderOverlay({ onFinish })

    // Next advances the tour and never finishes it before the last step.
    expect(
      screen.queryByRole('button', { name: 'Got it, start exploring' }),
    ).toBeNull()
    await user.click(screen.getByRole('button', { name: 'Next' }))
    expect(onFinish).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'Next' }))
    await user.click(
      screen.getByRole('button', { name: 'Got it, start exploring' }),
    )
    expect(onFinish).toHaveBeenCalledTimes(1)
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

  it('disables navigation and shows the opening state while finishing', async () => {
    const user = userEvent.setup()
    const { rerender } = renderOverlay({ finishing: false })

    // The parent flips finishing true only after Finish is pressed on the last
    // step, so advance there before the finishing state applies.
    await user.click(screen.getByRole('button', { name: 'Next' }))
    await user.click(screen.getByRole('button', { name: 'Next' }))
    rerender(
      <IntroWizardOverlay
        headline={config.headline ?? ''}
        subhead={config.subhead ?? ''}
        finishLabel={config.finishLabel ?? ''}
        callouts={config.callouts ?? []}
        finishing={true}
        onFinish={vi.fn()}
        onSkip={vi.fn()}
      />,
    )

    const finish = screen.getByRole('button', { name: 'Opening...' })
    const skip = screen.getByRole('button', { name: 'Skip' })
    expect((finish as HTMLButtonElement).disabled).toBe(true)
    expect((skip as HTMLButtonElement).disabled).toBe(true)
  })

  it('skips the tour from any step', async () => {
    const user = userEvent.setup()
    const onSkip = vi.fn()
    renderOverlay({ onSkip })

    await user.click(screen.getByRole('button', { name: 'Skip' }))
    expect(onSkip).toHaveBeenCalledTimes(1)
  })
})
