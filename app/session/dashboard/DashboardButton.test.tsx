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
    const handleClick = vi.fn()
    render(
      <DashboardButton icon={<span>ic</span>} onClick={handleClick}>
        Press
      </DashboardButton>,
    )
    await user.click(screen.getByRole('button'))
    expect(handleClick).toHaveBeenCalledOnce()
  })

  it('applies custom className', () => {
    render(
      <DashboardButton icon={<span>ic</span>} className="my-custom-class">
        Styled
      </DashboardButton>,
    )
    const button = screen.getByRole('button')
    expect(button.classList.contains('my-custom-class')).toBe(true)
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
