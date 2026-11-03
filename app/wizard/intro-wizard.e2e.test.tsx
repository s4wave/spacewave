import { useState } from 'react'
import { describe, expect, it } from 'vitest'
import { page } from 'vitest/browser'
import { cleanup, render } from 'vitest-browser-react'

import { driveIntroConfig } from './intro.js'
import { IntroWizardOverlay } from './IntroWizardOverlay.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  layout as themeLayout,
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    inset: 0,
    backgroundColor: themeColors['--color-background'],
    color: themeColors['--color-foreground'],
    position: 'fixed',
  },
  rule2: { position: 'relative', height: '100%', width: '100%' },
  rule3: {
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    alignContent: 'flex-start',
    display: 'grid',
    gridTemplateColumns: 'repeat(4, minmax(0, 1fr))',
  },
  rule4: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: themeColors['--color-background-card'],
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'flex-end',
    height: `calc(${themeLayout['--spacing']} * 24)`,
  },
  rule5: {
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule6: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule7: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 40%, transparent)`,
    borderRadius: '3.40282e38px',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    backgroundColor: themeColors['--color-background-card'],
    display: 'flex',
    position: 'absolute',
    bottom: `calc(${themeLayout['--spacing']} * 4)`,
    right: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule8: {
    borderRadius: '3.40282e38px',
    backgroundColor: themeColors['--color-brand'],
    height: `calc(${themeLayout['--spacing']} * 2)`,
    width: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule9: {
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

// IntroWizardSurface renders the fixed-anchor intro overlay over a neutral
// file-browser backdrop so the callouts, arrows, and control panel are captured
// against realistic content. Callout placement is region-anchored, so the
// overlay alone is faithful without mounting the full ObjectViewer.
function IntroWizardSurface({ finishing }: { finishing: boolean }) {
  const config = driveIntroConfig()
  const [finished, setFinished] = useState(false)

  return (
    <div sx={stylexStyles.rule1}>
      <div sx={stylexStyles.rule2}>
        <div sx={stylexStyles.rule3}>
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
            <div key={name} sx={stylexStyles.rule4}>
              <span sx={stylexStyles.rule5}>{name}</span>
              <span sx={stylexStyles.rule6}>Folder</span>
            </div>
          ))}
        </div>
        {/* Mock bottom-right upload indicator: the "Upload progress" callout
            teaches the user to watch it, so it must stay lit under the overlay. */}
        <div sx={stylexStyles.rule7}>
          <span sx={stylexStyles.rule8} />
          <span sx={stylexStyles.rule9}>Uploads</span>
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
  it('steps through region-anchored callouts one at a time over the frame', async () => {
    await render(<IntroWizardSurface finishing={false} />)

    // The tour opens on the first callout with the control panel; the later
    // callouts appear only as the user advances, one region at a time.
    await expect
      .element(page.getByText('Welcome to your Drive'))
      .toBeInTheDocument()
    await expect.element(page.getByText('Add files')).toBeInTheDocument()
    await expect
      .element(page.getByText('Your files', { exact: true }))
      .not.toBeInTheDocument()
    await capture('overlay-step-1')

    await page.getByRole('button', { name: 'Next' }).click()
    await expect
      .element(page.getByText('Your files', { exact: true }))
      .toBeInTheDocument()
    await capture('overlay-step-2')

    await page.getByRole('button', { name: 'Next' }).click()
    await expect.element(page.getByText('Upload progress')).toBeInTheDocument()
    await expect
      .element(page.getByRole('button', { name: 'Got it, start exploring' }))
      .toBeInTheDocument()
    await capture('overlay-step-3')

    await cleanup()
  })

  it('shows the finishing state after the final step is confirmed', async () => {
    await render(<IntroWizardSurface finishing={false} />)

    await page.getByRole('button', { name: 'Next' }).click()
    await page.getByRole('button', { name: 'Next' }).click()
    await page.getByRole('button', { name: 'Got it, start exploring' }).click()

    await expect
      .element(page.getByRole('button', { name: 'Opening…' }))
      .toBeInTheDocument()

    await capture('finishing')
    await cleanup()
  })
})
