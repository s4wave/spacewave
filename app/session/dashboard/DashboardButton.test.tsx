import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'

describe('DashboardButton', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders children text', () => {
    render(<DashboardButton icon={<span>ic</span>}>Click Me</DashboardButton>)
    expect(screen.getByText('Click Me')).toBeDefined()
  })

  it('renders icon', () => {
    render(
      <DashboardButton icon={<span data-testid="btn-icon">ic</span>}>
        Label
      </DashboardButton>,
    )
    expect(screen.getByTestId('btn-icon')).toBeDefined()
  })

  it('fires onClick handler', async () => {
    const user = userEvent.setup()
    const handleDashboardAction = vi.fn()
    render(
      <DashboardButton icon={<span>ic</span>} onClick={handleDashboardAction}>
        Press
      </DashboardButton>,
    )
    await user.click(screen.getByRole('button'))
    expect(handleDashboardAction).toHaveBeenCalledOnce()
  })

  it('applies custom className', () => {
    render(
      <DashboardButton icon={<span>ic</span>} className="mt-1">
        Styled
      </DashboardButton>,
    )
    const button = screen.getByRole('button')
    expect(button.classList.contains('mt-1')).toBe(true)
  })

  it('supports the disabled prop', () => {
    render(
      <DashboardButton icon={<span>ic</span>} disabled>
        Disabled
      </DashboardButton>,
    )
    const button = screen.getByRole('button')
    expect(button.hasAttribute('disabled')).toBe(true)
  })
})
