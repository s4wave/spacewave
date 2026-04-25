import { describe, it, expect, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import { Badge } from './badge.js'

describe('Badge', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders children text', () => {
    render(<Badge>New</Badge>)
    expect(screen.getByText('New')).toBeDefined()
  })

  it('applies data-slot="badge"', () => {
    render(<Badge>Tag</Badge>)
    const badge = screen.getByText('Tag')
    expect(badge.getAttribute('data-slot')).toBe('badge')
  })

  it('default variant classes include bg-primary', () => {
    render(<Badge>Default</Badge>)
    const badge = screen.getByText('Default')
    expect(badge.className).toContain('bg-primary')
  })

  it('secondary variant classes include bg-secondary', () => {
    render(<Badge variant="secondary">Secondary</Badge>)
    const badge = screen.getByText('Secondary')
    expect(badge.className).toContain('bg-secondary')
  })

  it('destructive variant classes include bg-destructive', () => {
    render(<Badge variant="destructive">Destructive</Badge>)
    const badge = screen.getByText('Destructive')
    expect(badge.className).toContain('bg-destructive')
  })

  it('outline variant classes include text-foreground', () => {
    render(<Badge variant="outline">Outline</Badge>)
    const badge = screen.getByText('Outline')
    expect(badge.className).toContain('text-foreground')
  })

  it('applies custom className', () => {
    render(<Badge className="my-custom">Custom</Badge>)
    const badge = screen.getByText('Custom')
    expect(badge.className).toContain('my-custom')
  })

  it('renders as span by default', () => {
    render(<Badge>Span</Badge>)
    const badge = screen.getByText('Span')
    expect(badge.tagName).toBe('SPAN')
  })

  it('asChild renders as child element type', () => {
    render(
      <Badge asChild>
        <a href="/test">Link Badge</a>
      </Badge>,
    )
    const badge = screen.getByText('Link Badge')
    expect(badge.tagName).toBe('A')
    expect(badge.getAttribute('data-slot')).toBe('badge')
  })
})
