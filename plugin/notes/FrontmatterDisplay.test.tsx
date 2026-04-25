import { describe, it, expect, afterEach, vi } from 'vitest'
import { render, screen, cleanup, fireEvent } from '@testing-library/react'

import FrontmatterDisplay from './FrontmatterDisplay.js'

describe('FrontmatterDisplay', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders nothing when frontmatter is empty', () => {
    const { container } = render(<FrontmatterDisplay frontmatter={{}} />)
    expect(container.children.length).toBe(0)
  })

  it('renders tags', () => {
    render(<FrontmatterDisplay frontmatter={{ tags: ['alpha', 'beta'] }} />)
    expect(screen.getByText('alpha')).toBeDefined()
    expect(screen.getByText('beta')).toBeDefined()
  })

  it('renders status badge', () => {
    render(
      <FrontmatterDisplay frontmatter={{ status: 'in-progress' }} />,
    )
    expect(screen.getByText('in-progress')).toBeDefined()
  })

  it('calls onStatusClick when the status badge is clicked', () => {
    const onStatusClick = vi.fn()
    render(
      <FrontmatterDisplay
        frontmatter={{ status: 'in-progress' }}
        onStatusClick={onStatusClick}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: 'in-progress' }))
    expect(onStatusClick).toHaveBeenCalledWith('in-progress')
  })

  it('renders author', () => {
    render(
      <FrontmatterDisplay frontmatter={{ author: ['[[Kevin Kelly]]'] }} />,
    )
    expect(screen.getByText('Kevin Kelly')).toBeDefined()
  })

  it('renders created date', () => {
    render(
      <FrontmatterDisplay frontmatter={{ created: '2026-03-18' }} />,
    )
    expect(screen.getByText('2026-03-18')).toBeDefined()
  })

  it('renders URL as link', () => {
    render(
      <FrontmatterDisplay
        frontmatter={{ url: 'https://example.com' }}
      />,
    )
    const link = screen.getByText('source')
    expect(link).toBeDefined()
    expect(link.closest('a')?.getAttribute('href')).toBe(
      'https://example.com',
    )
  })

  it('deduplicates tags and topics', () => {
    render(
      <FrontmatterDisplay
        frontmatter={{ tags: ['alpha'], topics: ['alpha', 'gamma'] }}
      />,
    )
    const alphaElements = screen.getAllByText('alpha')
    expect(alphaElements.length).toBe(1)
    expect(screen.getByText('gamma')).toBeDefined()
  })

  it('strips wiki links from categories', () => {
    render(
      <FrontmatterDisplay
        frontmatter={{ categories: ['[[Clippings]]'] }}
      />,
    )
    expect(screen.getByText('Clippings')).toBeDefined()
  })
})
