import { useState } from 'react'
import * as stylex from '@stylexjs/stylex'
import { expect, it } from 'vitest'
import { page } from 'vitest/browser'
import { render } from 'vitest-browser-react'

import { VmErrorBanner } from '../../app/vm/VmErrorBanner.js'
import { TagChip } from '../../plugin/notes/blog/TagChip.js'
import { InfoCard } from '../ui/InfoCard.js'
import { OutputPanel } from '../ui/OutputPanel.js'
import { CopyableField } from '../ui/CopyableField.js'
import { RadioOption } from '../ui/RadioOption.js'

const styles = stylex.create({
  override: { padding: 19, borderRadius: 9 },
  firstRule: { color: 'rgb(17, 34, 51)' },
  secondRule: { color: 'rgb(68, 85, 102)' },
})

// PilotJourney exercises shared composition together with real app and plugin consumers.
function PilotJourney() {
  const [selected, setSelected] = useState('None')
  const [radioSelected, setRadioSelected] = useState(false)
  return (
    <main>
      <InfoCard compact title="Details" sx={styles.override}>
        <VmErrorBanner title="Unavailable" message="Try again" />
        <TagChip tag="native-css" onSelectTag={setSelected} />
        <output aria-label="Selected tag">{selected}</output>
      </InfoCard>
      <section aria-label="Output rule precedence">
        <OutputPanel
          lines={['INFO WARN overlap', 'WARN predicate', 'Plain fallback']}
          rules={[
            { match: 'INFO', sx: styles.firstRule },
            { match: (line) => line.includes('WARN'), sx: styles.secondRule },
          ]}
        />
      </section>
      <CopyableField label="Identifier" value="shared-value" />
      <RadioOption
        label="Style choice"
        selected={radioSelected}
        onSelect={() => setRadioSelected(true)}
      />
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

  // Prefix matching wins the overlap, while the next line reaches the predicate rule.
  const lines = document.querySelectorAll(
    'section[aria-label="Output rule precedence"] > div > div',
  )
  expect(lines).toHaveLength(3)
  expect(getComputedStyle(lines[0]!).color).toBe('rgb(17, 34, 51)')
  expect(getComputedStyle(lines[1]!).color).toBe('rgb(68, 85, 102)')

  // Resolve the fallback color through the browser's actual theme variables.
  const reference = document.createElement('span')
  reference.style.color = 'var(--color-text-secondary)'
  document.body.append(reference)
  expect(getComputedStyle(lines[2]!).color).toBe(
    getComputedStyle(reference).color,
  )
  reference.remove()

  // The copy icon follows the actual hover state of its marked button.
  const copyIcon = document.querySelector(
    'button[aria-label="Click to copy"] svg',
  )!
  expect(getComputedStyle(copyIcon).opacity).toBe('0')
  await page.getByRole('button', { name: 'Click to copy', exact: true }).hover()
  await expect.poll(() => getComputedStyle(copyIcon).opacity).toBe('1')

  // A real hover and selection change reach distinct radio border states.
  const radio = document.querySelector('[role="radio"]')!
  const idleBorder = getComputedStyle(radio).borderColor
  await page.getByRole('radio', { name: 'Style choice', exact: true }).hover()
  await expect
    .poll(() => getComputedStyle(radio).borderColor)
    .not.toBe(idleBorder)
  await page.getByRole('radio', { name: 'Style choice', exact: true }).click()
  expect(radio.getAttribute('aria-checked')).toBe('true')
})
