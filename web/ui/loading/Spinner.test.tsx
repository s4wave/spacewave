import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, render } from '@testing-library/react'

import { Spinner } from './Spinner.js'

describe('Spinner', () => {
  afterEach(() => {
    cleanup()
  })

  it.each([
    ['sm', 'h-3.5', 'w-3.5'],
    ['md', 'h-4', 'w-4'],
    ['lg', 'h-6', 'w-6'],
    ['xl', 'h-8', 'w-8'],
  ] as const)('applies %s size classes', (size, h, w) => {
    const { container } = render(<Spinner size={size} />)
    const svg = container.querySelector('svg')
    expect(svg?.classList.contains(h)).toBe(true)
    expect(svg?.classList.contains(w)).toBe(true)
    expect(svg?.classList.contains('animate-spin')).toBe(true)
  })

  it('defaults to md when size is omitted', () => {
    const { container } = render(<Spinner />)
    const svg = container.querySelector('svg')
    expect(svg?.classList.contains('h-4')).toBe(true)
    expect(svg?.classList.contains('w-4')).toBe(true)
  })

  it('forwards a custom className', () => {
    const { container } = render(<Spinner className="text-brand" />)
    const svg = container.querySelector('svg')
    expect(svg?.classList.contains('text-brand')).toBe(true)
  })
})
