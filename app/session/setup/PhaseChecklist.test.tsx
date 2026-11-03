import { afterEach, expect, it } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'

import { PhaseChecklist } from './PhaseChecklist.js'

afterEach(cleanup)

it('keeps every phase visible as the current step advances to completion', () => {
  // The active row is exposed independently of its check, spinner or pending-dot skin.
  const { container, rerender } = render(
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
  expect(container.querySelector('[aria-current="step"]')?.textContent).toBe(
    'Mount',
  )

  // Completing the current step moves the marker without losing prior phase labels.
  rerender(
    <PhaseChecklist
      phases={[
        { label: 'Create', done: true },
        { label: 'Mount', done: true },
        { label: 'Populate', done: false, active: true },
      ]}
    />,
  )
  expect(container.querySelectorAll('[aria-current="step"]')).toHaveLength(1)
  expect(container.querySelector('[aria-current="step"]')?.textContent).toBe(
    'Populate',
  )
  rerender(
    <PhaseChecklist
      phases={[
        { label: 'Create', done: true },
        { label: 'Mount', done: true },
        { label: 'Populate', done: true },
      ]}
    />,
  )
  expect(container.querySelector('[aria-current="step"]')).toBeNull()
  expect(screen.getByText('Create')).toBeDefined()
  expect(screen.getByText('Populate')).toBeDefined()
})
