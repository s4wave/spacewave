import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import DocsToc from './DocsToc.js'

describe('DocsToc', () => {
  afterEach(() => cleanup())

  it('strips Org heading metadata and scrolls within its own viewer', () => {
    // Render independent documentation viewers with overlapping headings.
    render(
      <>
        <div data-docs-viewer>
          <div contentEditable suppressContentEditableWarning>
            <h1>Overview</h1>
          </div>
        </div>
        <div data-docs-viewer>
          <div contentEditable suppressContentEditableWarning>
            <h1>Overview</h1>
            <h2>Details</h2>
          </div>
          <DocsToc
            content={'* TODO Overview :docs:\n** Details\n***** Ignored\n'}
            format="org"
          />
        </div>
      </>,
    )

    // Navigation labels retain only supported heading text.
    expect(screen.getByRole('button', { name: 'Overview' })).toBeDefined()
    expect(screen.getByRole('button', { name: 'Details' })).toBeDefined()
    expect(screen.queryByRole('button', { name: 'Ignored' })).toBeNull()

    // A same-named heading in another viewer must not receive the navigation.
    const headings = screen.getAllByRole('heading', { name: 'Overview' })
    const otherScroll = vi.fn()
    const currentScroll = vi.fn()
    Object.defineProperty(headings[0], 'scrollIntoView', { value: otherScroll })
    Object.defineProperty(headings[1], 'scrollIntoView', {
      value: currentScroll,
    })
    fireEvent.click(screen.getByRole('button', { name: 'Overview' }))
    expect(currentScroll).toHaveBeenCalledWith({
      behavior: 'smooth',
      block: 'start',
    })
    expect(otherScroll).not.toHaveBeenCalled()
  })

  it('keeps Markdown heading parsing for Markdown pages', () => {
    render(<DocsToc content={'# Overview\n## Details\n'} format="markdown" />)

    expect(screen.getByRole('button', { name: 'Overview' })).toBeDefined()
    expect(screen.getByRole('button', { name: 'Details' })).toBeDefined()
  })
})
