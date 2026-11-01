import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { OutputPanel } from './OutputPanel.js'

describe('OutputPanel', () => {
  afterEach(cleanup)

  it('shows empty output, lines and errors as the log changes', () => {
    // Keep an empty log distinct from a failed operation.
    const { rerender } = render(<OutputPanel lines={[]} />)
    expect(screen.getByText('No output')).toBeTruthy()
    rerender(<OutputPanel lines={[]} placeholder="Waiting for input..." />)
    expect(screen.getByText('Waiting for input...')).toBeTruthy()
    rerender(<OutputPanel lines={[]} error="Connection lost" />)
    expect(screen.getByText('Error: Connection lost')).toBeTruthy()
    expect(screen.queryByText('No output')).toBeNull()

    // Retain collected lines when an operation also reports an error.
    rerender(
      <OutputPanel
        lines={['PASS first', 'FAIL second', 'plain line']}
        error="Build failed"
      />,
    )
    expect(screen.getByText('PASS first')).toBeTruthy()
    expect(screen.getByText('FAIL second')).toBeTruthy()
    expect(screen.getByText('plain line')).toBeTruthy()
    expect(screen.getByText('Error: Build failed')).toBeTruthy()
  })
})
