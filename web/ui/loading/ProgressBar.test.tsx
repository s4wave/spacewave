import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'

import { ProgressBar } from './ProgressBar.js'

describe('ProgressBar', () => {
  afterEach(cleanup)

  it('clamps determinate labels and switches between percentages, rates and indeterminate progress', () => {
    // Show bounded percentages for determinate progress.
    const { rerender } = render(<ProgressBar value={42} />)
    expect(screen.getByText('42%')).toBeTruthy()
    rerender(<ProgressBar value={175} />)
    expect(screen.getByText('100%')).toBeTruthy()
    rerender(<ProgressBar value={-5} />)
    expect(screen.getByText('0%')).toBeTruthy()

    // Prefer the supplied rate, including while the total is unknown.
    rerender(<ProgressBar value={62} rate="1.5 MiB/s" />)
    expect(screen.getByText('1.5 MiB/s')).toBeTruthy()
    expect(screen.queryByText(/%$/)).toBeNull()
    rerender(<ProgressBar indeterminate rate="Uploading" />)
    expect(screen.getByText('Uploading')).toBeTruthy()
    rerender(<ProgressBar indeterminate />)
    expect(screen.queryByText(/%$/)).toBeNull()
    expect(screen.queryByText('Uploading')).toBeNull()
  })
})
