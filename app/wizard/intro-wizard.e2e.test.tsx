import { useState } from 'react'
import { describe, expect, it } from 'vitest'
import { page } from 'vitest/browser'
import { cleanup, render } from 'vitest-browser-react'

import { driveIntroConfig } from './intro.js'
import { IntroWizardOverlay } from './IntroWizardOverlay.js'

// IntroWizardSurface renders the fixed-anchor intro overlay over a neutral
// file-browser backdrop so the callouts, arrows, and control panel are captured
// against realistic content. Callout placement is region-anchored, so the
// overlay alone is faithful without mounting the full ObjectViewer.
function IntroWizardSurface({ finishing }: { finishing: boolean }) {
  const config = driveIntroConfig()
  const [finished, setFinished] = useState(false)

  return (
    <div className="bg-background text-foreground fixed inset-0">
      <div className="relative h-full w-full">
        <div className="grid grid-cols-4 content-start gap-4 p-6">
          {[
            'Documents',
            'Photos',
            'Music',
            'Projects',
            'Archive',
            'Shared',
            'Invoices',
            'Trips',
          ].map((name) => (
            <div
              key={name}
              className="border-frame-overlay-border bg-frame-overlay flex h-24 flex-col justify-end rounded-md border p-3"
            >
              <span className="text-foreground text-sm">{name}</span>
              <span className="text-foreground-alt text-xs">Folder</span>
            </div>
          ))}
        </div>
        <IntroWizardOverlay
          headline={config.headline ?? ''}
          subhead={config.subhead ?? ''}
          finishLabel={config.finishLabel ?? ''}
          callouts={config.callouts ?? []}
          finishing={finishing || finished}
          onFinish={() => setFinished(true)}
          onSkip={() => setFinished(true)}
        />
      </div>
    </div>
  )
}

async function capture(name: string) {
  return page.screenshot({ path: `__screenshots__/intro-wizard/${name}.png` })
}

describe('intro wizard overlay', () => {
  it('draws region-anchored callouts and the control panel over the frame', async () => {
    await render(<IntroWizardSurface finishing={false} />)

    await expect
      .element(page.getByText('Welcome to your Drive'))
      .toBeInTheDocument()
    await expect.element(page.getByText('Add files')).toBeInTheDocument()
    await expect
      .element(page.getByText('Your files', { exact: true }))
      .toBeInTheDocument()
    await expect.element(page.getByText('Upload progress')).toBeInTheDocument()
    await expect
      .element(page.getByRole('button', { name: 'Got it, start exploring' }))
      .toBeInTheDocument()

    await capture('overlay')
    await cleanup()
  })

  it('shows the finishing state on the control panel button', async () => {
    await render(<IntroWizardSurface finishing={true} />)

    await expect
      .element(page.getByText('Welcome to your Drive'))
      .toBeInTheDocument()
    await expect
      .element(page.getByRole('button', { name: 'Opening...' }))
      .toBeInTheDocument()

    await capture('finishing')
    await cleanup()
  })
})
