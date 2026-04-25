import { describe, it, expect, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import { OutputPanel } from './OutputPanel.js'

describe('OutputPanel', () => {
  afterEach(() => {
    cleanup()
  })

  it('shows default placeholder when lines is empty and no error', () => {
    render(<OutputPanel lines={[]} />)
    expect(screen.getByText('No output')).toBeDefined()
  })

  it('shows custom placeholder', () => {
    render(<OutputPanel lines={[]} placeholder="Waiting for input..." />)
    expect(screen.getByText('Waiting for input...')).toBeDefined()
  })

  it('shows error message when error is set', () => {
    render(<OutputPanel lines={[]} error="Connection lost" />)
    expect(screen.getByText('Error: Connection lost')).toBeDefined()
  })

  it('shows lines when provided', () => {
    render(<OutputPanel lines={['line one', 'line two', 'line three']} />)
    expect(screen.getByText('line one')).toBeDefined()
    expect(screen.getByText('line two')).toBeDefined()
    expect(screen.getByText('line three')).toBeDefined()
  })

  it('applies default coloring rules', () => {
    render(
      <OutputPanel
        lines={[
          'PASS test_a',
          'FAIL test_b',
          '[stderr] warning msg',
          'Running: build',
          'Error: bad input',
        ]}
      />,
    )
    expect(
      screen.getByText('PASS test_a').classList.contains('text-success'),
    ).toBe(true)
    expect(
      screen.getByText('FAIL test_b').classList.contains('text-error'),
    ).toBe(true)
    expect(
      screen
        .getByText('[stderr] warning msg')
        .classList.contains('text-warning'),
    ).toBe(true)
    expect(
      screen.getByText('Running: build').classList.contains('font-semibold'),
    ).toBe(true)
    expect(
      screen.getByText('Error: bad input').classList.contains('text-error'),
    ).toBe(true)
  })

  it('applies custom rules', () => {
    const customRules = [
      { match: 'INFO', className: 'text-info' },
      {
        match: (line: string) => line.includes('WARN'),
        className: 'text-yellow',
      },
    ]
    render(
      <OutputPanel
        lines={['INFO starting up', 'some WARN here']}
        rules={customRules}
      />,
    )
    expect(
      screen.getByText('INFO starting up').classList.contains('text-info'),
    ).toBe(true)
    expect(
      screen.getByText('some WARN here').classList.contains('text-yellow'),
    ).toBe(true)
  })

  it('applies testId attribute', () => {
    render(<OutputPanel lines={[]} testId="my-output" />)
    expect(screen.getByTestId('my-output')).toBeDefined()
  })

  it('applies custom className', () => {
    const { container } = render(
      <OutputPanel lines={[]} className="my-output-panel" />,
    )
    expect(
      container.firstElementChild?.classList.contains('my-output-panel'),
    ).toBe(true)
  })

  it('shows both error and lines when both are present', () => {
    render(<OutputPanel lines={['output line']} error="Something failed" />)
    expect(screen.getByText('Error: Something failed')).toBeDefined()
    expect(screen.getByText('output line')).toBeDefined()
  })

  it('does not show placeholder when error is set but lines is empty', () => {
    render(<OutputPanel lines={[]} error="Broken" />)
    expect(screen.queryByText('No output')).toBeNull()
    expect(screen.getByText('Error: Broken')).toBeDefined()
  })
})
