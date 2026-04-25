import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, cleanup, fireEvent, act } from '@testing-library/react'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'

describe('CopyableField', () => {
  const writeText = vi.fn().mockResolvedValue(undefined)

  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  function setup() {
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      writable: true,
      configurable: true,
    })
    writeText.mockClear()
  }

  it('renders label text', () => {
    setup()
    render(<CopyableField label="Session ID" value="abc-123" />)
    expect(screen.getByText('Session ID')).toBeDefined()
  })

  it('renders value text', () => {
    setup()
    render(<CopyableField label="Session ID" value="abc-123" />)
    expect(screen.getByText('abc-123')).toBeDefined()
  })

  it('shows "Click to copy" aria-label initially', () => {
    setup()
    render(<CopyableField label="Peer ID" value="peer-456" />)
    const button = screen.getByRole('button')
    expect(button.getAttribute('aria-label')).toBe('Click to copy')
  })

  it('changes aria-label to "Copied!" after clicking the button', () => {
    setup()
    vi.useFakeTimers()
    render(<CopyableField label="Peer ID" value="peer-456" />)
    const button = screen.getByRole('button')
    fireEvent.click(button)
    expect(button.getAttribute('aria-label')).toBe('Copied!')
    vi.useRealTimers()
  })

  it('calls navigator.clipboard.writeText with the value when clicked', () => {
    setup()
    render(<CopyableField label="Account" value="acct-789" />)
    const button = screen.getByRole('button')
    fireEvent.click(button)
    expect(writeText).toHaveBeenCalledWith('acct-789')
  })

  it('displays value in a mono font span', () => {
    setup()
    render(<CopyableField label="ID" value="mono-value" />)
    const valueSpan = screen.getByText('mono-value')
    expect(valueSpan.classList.contains('font-mono')).toBe(true)
  })

  it('reverts aria-label back to "Click to copy" after timeout', () => {
    setup()
    vi.useFakeTimers()
    render(<CopyableField label="Field" value="val" />)
    const button = screen.getByRole('button')
    fireEvent.click(button)
    expect(button.getAttribute('aria-label')).toBe('Copied!')
    act(() => {
      vi.advanceTimersByTime(2000)
    })
    expect(button.getAttribute('aria-label')).toBe('Click to copy')
    vi.useRealTimers()
  })
})
