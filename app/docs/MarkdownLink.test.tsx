import { afterEach, describe, it, expect } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'

import { StaticProvider } from '@s4wave/app/prerender/StaticContext.js'

import { MarkdownLink } from './MarkdownLink.js'

describe('MarkdownLink', () => {
  afterEach(() => {
    cleanup()
  })

  it('rewrites app-local links to hash routes outside static mode', () => {
    render(<MarkdownLink href="/community">Community</MarkdownLink>)

    expect(screen.getByRole('link').getAttribute('href')).toBe('#/community')
  })

  it('keeps crawlable paths in static mode', () => {
    render(
      <StaticProvider>
        <MarkdownLink href="/community">Community</MarkdownLink>
      </StaticProvider>,
    )

    expect(screen.getByRole('link').getAttribute('href')).toBe('/community')
  })

  it('leaves external links unchanged', () => {
    render(
      <MarkdownLink href="https://github.com/s4wave/spacewave">
        GitHub
      </MarkdownLink>,
    )

    expect(screen.getByRole('link').getAttribute('href')).toBe(
      'https://github.com/s4wave/spacewave',
    )
  })
})
