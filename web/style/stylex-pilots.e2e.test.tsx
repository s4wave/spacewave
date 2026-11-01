import { useState } from 'react'
import * as stylex from '@stylexjs/stylex'
import { expect, it } from 'vitest'
import { page } from 'vitest/browser'
import { render } from 'vitest-browser-react'

import { VmErrorBanner } from '../../app/vm/VmErrorBanner.js'
import { TagChip } from '../../plugin/notes/blog/TagChip.js'
import { InfoCard } from '../ui/InfoCard.js'

const styles = stylex.create({ override: { padding: 19, borderRadius: 9 } })

// PilotJourney exercises shared composition together with real app and plugin consumers.
function PilotJourney() {
  const [selected, setSelected] = useState('None')
  return (
    <main>
      <InfoCard compact title="Details" sx={styles.override}>
        <VmErrorBanner title="Unavailable" message="Try again" />
        <TagChip tag="native-css" onSelectTag={setSelected} />
        <output aria-label="Selected tag">{selected}</output>
      </InfoCard>
    </main>
  )
}

it('composes shared, app and plugin styles through the browser build', async () => {
  // Render production components through Vitest's normal React and native Vite CSS pipeline.
  await render(<PilotJourney />)
  await expect
    .poll(() => document.querySelector('[role=alert]')?.textContent)
    .toBe('UnavailableTry again')
  const card = document.querySelector('main > div')!
  const style = getComputedStyle(card)
  expect(style.padding).toBe('19px')
  expect(style.borderRadius).toBe('9px')

  // A plugin interaction retains the same shared card and its caller's final override.
  await page.getByRole('button', { name: 'native-css', exact: true }).click()
  await expect
    .poll(() => document.querySelector('output')?.textContent)
    .toBe('native-css')
  expect(document.querySelector('main > div')).toBe(card)
  expect(getComputedStyle(card).padding).toBe('19px')
})
