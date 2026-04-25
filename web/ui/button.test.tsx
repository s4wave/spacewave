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
    render(<Button>Submit</Button>)
    const button = screen.getByRole('button', { name: 'Submit' })
    expect(button.tagName).toBe('BUTTON')
  })

  it('onClick handler fires', async () => {
    const user = userEvent.setup()
    const handleClick = vi.fn()
    render(<Button onClick={handleClick}>Press</Button>)

    await user.click(screen.getByRole('button', { name: 'Press' }))
    expect(handleClick).toHaveBeenCalledOnce()
  })

  it('disabled state applies opacity and pointer-events', () => {
    render(<Button disabled>Disabled</Button>)
    const button = screen.getByRole('button', { name: 'Disabled' })
    expect(button.className).toContain('disabled:opacity-50')
    expect(button.className).toContain('disabled:pointer-events-none')
  })

  it('default variant includes bg-primary', () => {
    render(<Button>Default</Button>)
    const button = screen.getByRole('button', { name: 'Default' })
    expect(button.className).toContain('bg-primary')
  })

  it('ghost variant includes hover:bg-accent', () => {
    render(<Button variant="ghost">Ghost</Button>)
    const button = screen.getByRole('button', { name: 'Ghost' })
    expect(button.className).toContain('hover:bg-accent')
  })

  it('size sm includes h-8', () => {
    render(<Button size="sm">Small</Button>)
    const button = screen.getByRole('button', { name: 'Small' })
    expect(button.className).toContain('h-8')
  })

  it('size icon includes h-9 w-9', () => {
    render(<Button size="icon">Icon</Button>)
    const button = screen.getByRole('button', { name: 'Icon' })
    expect(button.className).toContain('h-9')
    expect(button.className).toContain('w-9')
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
