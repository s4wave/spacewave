import { afterEach, describe, expect, it } from 'vitest'
import { page } from 'vitest/browser'
import { cleanup, render } from 'vitest-browser-react'

import {
  EXPERIMENTAL_CREATORS_STORAGE_KEY,
  setExperimentalCreatorsEnabled,
  useExperimentalCreatorsEnabled,
} from './creator-visibility.js'
import { getVisibleQuickstartOptions } from './quickstart/options.js'
import { normalizeObjectWizards } from './space/object-wizards.js'

function BrowserInventoryProbe() {
  const experimentalCreatorsEnabled = useExperimentalCreatorsEnabled(false)
  const quickstarts = getVisibleQuickstartOptions(experimentalCreatorsEnabled)
  const wizards = normalizeObjectWizards(
    [
      {
        typeId: 'git/repo',
        displayName: 'Git Repository',
        createOpId: 'spacewave/git/repo/create',
        keyPrefix: 'git/repo/',
      },
      {
        typeId: 'forge/task',
        displayName: 'Forge Task',
        createOpId: 'spacewave/forge/task/create',
        keyPrefix: 'forge/task/',
        experimental: true,
      },
    ],
    experimentalCreatorsEnabled,
  )

  return (
    <div>
      <section aria-label="quickstarts">
        {quickstarts.map((option) => (
          <div key={option.id}>{option.name}</div>
        ))}
      </section>
      <section aria-label="object wizards">
        {wizards.map((wizard) => (
          <div key={wizard.typeId}>{wizard.displayName}</div>
        ))}
      </section>
    </div>
  )
}

afterEach(() => {
  void cleanup()
  localStorage.removeItem(EXPERIMENTAL_CREATORS_STORAGE_KEY)
})

describe('experimental creator browser visibility', () => {
  it('hides release experimental creators until the browser preference is enabled', async () => {
    await render(<BrowserInventoryProbe />)

    await expect.element(page.getByText('Create a Drive')).toBeInTheDocument()
    await expect
      .element(page.getByText('Git Repository', { exact: true }))
      .toBeInTheDocument()
    await expect.element(page.getByText('Add a Device')).not.toBeInTheDocument()
    await expect.element(page.getByText('Forge Task')).not.toBeInTheDocument()

    setExperimentalCreatorsEnabled(true)

    await expect.element(page.getByText('Add a Device')).toBeInTheDocument()
    await expect.element(page.getByText('Forge Task')).toBeInTheDocument()
  })

  it('honors a console-set preference on first render', async () => {
    localStorage.setItem(EXPERIMENTAL_CREATORS_STORAGE_KEY, '1')

    await render(<BrowserInventoryProbe />)

    await expect.element(page.getByText('Add a Device')).toBeInTheDocument()
    await expect.element(page.getByText('Forge Task')).toBeInTheDocument()
  })
})
