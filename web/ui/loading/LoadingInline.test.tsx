import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'

import { LoadingInline } from './LoadingInline.js'

describe('LoadingInline', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders the label next to a spinner', () => {
    const { container } = render(<LoadingInline label="Loading messages..." />)
    expect(screen.getByText('Loading messages...')).toBeTruthy()
    expect(container.querySelector('svg.animate-spin')).toBeTruthy()
  })

  it.each([
    ['brand', 'text-brand'],
    ['muted', 'text-foreground-alt'],
    ['destructive', 'text-destructive'],
  ] as const)('applies the %s tone', (tone, expected) => {
    const { container } = render(<LoadingInline label="Loading" tone={tone} />)
    const span = container.querySelector('span.inline-flex')
    expect(span?.classList.contains(expected)).toBe(true)
  })
})
