import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'

import { ProgressBar } from './ProgressBar.js'

describe('ProgressBar', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders determinate fill at the supplied percentage', () => {
    const { container } = render(<ProgressBar value={42} />)
    const fill = container.querySelector('div.bg-brand') as HTMLElement | null
    expect(fill).toBeTruthy()
    expect(fill?.style.width).toBe('42%')
    expect(screen.getByText('42%')).toBeTruthy()
  })

  it('clamps determinate values outside 0..100', () => {
    const { container } = render(<ProgressBar value={175} />)
    const fill = container.querySelector('div.bg-brand') as HTMLElement | null
    expect(fill?.style.width).toBe('100%')
  })

  it('renders an animated indeterminate bar with no percent label', () => {
    const { container } = render(<ProgressBar indeterminate />)
    const sweep = container.querySelector('div.animate-progress-indeterminate')
    expect(sweep).toBeTruthy()
    expect(screen.queryByText(/%$/)).toBeNull()
  })

  it('shows a rate label in place of the percent when provided', () => {
    render(<ProgressBar value={62} rate="1.5 MiB/s" />)
    expect(screen.getByText('1.5 MiB/s')).toBeTruthy()
    expect(screen.queryByText('62%')).toBeNull()
  })

  it('shows a rate label on indeterminate variants', () => {
    render(<ProgressBar indeterminate rate="Uploading" />)
    expect(screen.getByText('Uploading')).toBeTruthy()
  })
})
