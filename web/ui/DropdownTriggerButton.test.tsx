import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'

import { DropdownTriggerButton } from './DropdownTriggerButton.js'

describe('DropdownTriggerButton', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders the label and icon', () => {
    render(
      <DropdownTriggerButton icon={<span data-testid="lead">ic</span>}>
        Assign to...
      </DropdownTriggerButton>,
    )
    expect(screen.getByText('Assign to...')).toBeDefined()
    expect(screen.getByTestId('lead')).toBeDefined()
  })

  it('renders the chevron by default', () => {
    render(<DropdownTriggerButton>Open</DropdownTriggerButton>)
    const button = screen.getByRole('button')
    expect(button.querySelector('svg')).toBeTruthy()
  })

  it('applies ghost trigger styling', () => {
    render(
      <DropdownTriggerButton triggerStyle="ghost">
        Change billing account
      </DropdownTriggerButton>,
    )
    const button = screen.getByRole('button')
    expect(button.classList.contains('px-0')).toBe(true)
    expect(button.classList.contains('shadow-none')).toBe(true)
  })
})
