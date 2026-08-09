import React from 'react'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Button } from './button.js'

describe('Button', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders children', () => {
    render(<Button>Click me</Button>)
    expect(screen.getByText('Click me')).toBeDefined()
  })

  it('renders as button element by default', () => {
    render(<Button>Save changes</Button>)
    const button = screen.getByRole('button', { name: 'Save changes' })
    expect(button.tagName).toBe('BUTTON')
  })

  it('onClick handler fires', async () => {
    const user = userEvent.setup()
    const handleButtonPress = vi.fn()
    render(<Button onClick={handleButtonPress}>Press</Button>)

    await user.click(screen.getByRole('button', { name: 'Press' }))
    expect(handleButtonPress).toHaveBeenCalledOnce()
  })

  it('forwards the disabled state', () => {
    render(<Button disabled>Disabled</Button>)
    const button = screen.getByRole('button', { name: 'Disabled' })
    expect((button as HTMLButtonElement).disabled).toBe(true)
  })

  it('applies custom className', () => {
    render(<Button className="extra-class">Styled</Button>)
    const button = screen.getByRole('button', { name: 'Styled' })
    expect(button.className).toContain('extra-class')
  })

  it('asChild renders as child element type', () => {
    render(
      <Button asChild>
        <a href="/link">Link Button</a>
      </Button>,
    )
    const link = screen.getByText('Link Button')
    expect(link.tagName).toBe('A')
    expect(link.getAttribute('href')).toBe('/link')
  })

  it('forwards ref', () => {
    const ref = React.createRef<HTMLButtonElement>()
    render(<Button ref={ref}>Ref Button</Button>)
    expect(ref.current).toBeTruthy()
    expect(ref.current?.tagName).toBe('BUTTON')
  })
})
