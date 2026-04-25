import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'

import { PhaseChecklist } from './PhaseChecklist.js'

describe('PhaseChecklist', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders one row per phase with the correct label', () => {
    render(
      <PhaseChecklist
        phases={[
          { label: 'Create', done: true },
          { label: 'Mount', done: false, active: true },
          { label: 'Populate', done: false },
        ]}
      />,
    )
    expect(screen.getByText('Create')).toBeDefined()
    expect(screen.getByText('Mount')).toBeDefined()
    expect(screen.getByText('Populate')).toBeDefined()
  })

  it('applies the done text color only to completed rows', () => {
    render(
      <PhaseChecklist
        phases={[
          { label: 'Create', done: true },
          { label: 'Mount', done: false, active: true },
          { label: 'Populate', done: false },
        ]}
      />,
    )
    expect(screen.getByText('Create').className).toContain('text-foreground')
    expect(screen.getByText('Create').className).not.toContain(
      'text-foreground-alt',
    )
    expect(screen.getByText('Mount').className).toContain('text-foreground-alt')
    expect(screen.getByText('Populate').className).toContain(
      'text-foreground-alt',
    )
  })

  it('renders a spinner for the active row and a pending dot for pending rows', () => {
    const { container } = render(
      <PhaseChecklist
        phases={[
          { label: 'Create', done: true },
          { label: 'Mount', done: false, active: true },
          { label: 'Populate', done: false },
        ]}
      />,
    )
    const rows = container.querySelectorAll(':scope > div > div')
    expect(rows.length).toBe(3)

    const doneIcon = rows[0].querySelector('svg')
    const activeIcon = rows[1].querySelector('svg')
    const pendingIcon = rows[2].querySelector('svg')

    expect(doneIcon).not.toBeNull()
    expect(doneIcon?.getAttribute('class') ?? '').not.toContain('animate-spin')

    expect(activeIcon).not.toBeNull()
    expect(activeIcon?.getAttribute('class') ?? '').toContain('animate-spin')

    expect(pendingIcon).toBeNull()
    const pendingDot = rows[2].querySelector('div.rounded-full')
    expect(pendingDot).not.toBeNull()
  })
})
