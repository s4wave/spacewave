import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'

import { FactList, FactRow } from './FactList.js'
import { hiddenScheme, splitTail } from './MiddleEllipsis.js'

const peerId = '12D3KooWQx7Hc4Lr8VbZ3TfPa9WkEe5Yd2Hs7GuLN2mXq9'

describe('FactRow', () => {
  afterEach(() => {
    cleanup()
    vi.useRealTimers()
  })

  it('keeps the whole id in the text, title and copy', async () => {
    // Stub a clipboard that accepts the copy.
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })

    // Show the id and read its whole value from the text and title.
    render(
      <FactList>
        <FactRow label="Peer ID" value={peerId} />
      </FactList>,
    )
    const mid = screen.getByTitle(peerId)
    expect(mid.textContent).toBe(peerId)

    // Copy it and see the press confirm the whole value.
    const press = screen.getByRole('button', { name: 'Copy Peer ID' })
    await act(async () => {
      fireEvent.click(press)
      await Promise.resolve()
    })
    expect(writeText).toHaveBeenCalledWith(peerId)
    expect(press.getAttribute('aria-label')).toBe('Copied')
  })

  it('says when the clipboard refuses the copy', async () => {
    // Stub a clipboard that refuses the copy.
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: vi.fn().mockRejectedValue(new Error('denied')) },
      configurable: true,
    })

    // Copy and see the press report the failure.
    render(<FactRow label="Peer ID" value={peerId} />)
    const press = screen.getByRole('button', { name: 'Copy Peer ID' })
    await act(async () => {
      fireEvent.click(press)
      await Promise.resolve()
    })
    expect(press.getAttribute('aria-label')).toBe('Copy failed')
  })

  it('draws a URL from its host and copies it whole', async () => {
    // Stub a clipboard that accepts the copy.
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })

    // Keep the whole URL in the text, drawing it from the host.
    const url = 'https://github.com/aperturerobotics/spacewave/'
    render(<FactRow label="Remote" value={url} />)
    const mid = screen.getByTitle(url)
    expect(mid.textContent).toBe(url)
    expect(mid.querySelector('.sr-only')?.textContent).toBe('https://')
    expect(mid.querySelector('[data-mid-head]')?.textContent).toBe(
      'github.com/aperturerobotics',
    )

    // Copy it and see the whole URL on the clipboard.
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Copy Remote' }))
      await Promise.resolve()
    })
    expect(writeText).toHaveBeenCalledWith(url)
  })

  it('gives an empty value no copy press', () => {
    render(<FactRow label="Object key" value="" />)
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('draws text facts without the mono shape and without a press when asked', () => {
    render(<FactRow kind="text" label="Rev" value="42" control={null} />)
    expect(screen.getByTitle('42').classList.contains('truncate')).toBe(true)
    expect(screen.queryByRole('button')).toBeNull()
  })
})

describe('hiddenScheme', () => {
  it('hides only https:// and keeps every other scheme drawn', () => {
    expect(hiddenScheme('https://github.com/a')).toBe('https://')
    for (const v of [
      'http://github.com/a',
      's3://bucket/a',
      'file:///a',
      'ssh://host/a',
      'git://host/a',
    ]) {
      expect(hiddenScheme(v)).toBe('')
    }
  })
})

describe('splitTail', () => {
  it('keeps the last path segment, the last six id characters, or a short value whole', () => {
    // A URL draws from its host and keeps its last real segment.
    const url = 'github.com/aperturerobotics/spacewave'
    expect(url.slice(splitTail(url))).toBe('/spacewave')
    expect(url.slice(0, splitTail(url))).toBe('github.com/aperturerobotics')
    const dir = 'github.com/aperturerobotics/spacewave/'
    expect(dir.slice(splitTail(dir))).toBe('/spacewave/')
    expect(peerId.slice(splitTail(peerId))).toBe('N2mXq9')
    expect('abc/'.slice(splitTail('abc/'))).toBe('abc/')
  })
})
