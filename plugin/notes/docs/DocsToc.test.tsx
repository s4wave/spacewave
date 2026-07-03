import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import DocsToc from './DocsToc.js'

describe('DocsToc', () => {
  afterEach(() => cleanup())

  it('renders Org headings with TODO keywords and tags stripped', () => {
    render(
      <DocsToc
        content={'* TODO Overview :docs:\n** Details\n***** Ignored\n'}
        format="org"
      />,
    )

    expect(screen.getByRole('button', { name: 'Overview' })).toBeDefined()
    expect(screen.getByRole('button', { name: 'Details' })).toBeDefined()
    expect(screen.queryByRole('button', { name: 'Ignored' })).toBeNull()
  })

  it('keeps Markdown heading parsing for Markdown pages', () => {
    render(<DocsToc content={'# Overview\n## Details\n'} format="markdown" />)

    expect(screen.getByRole('button', { name: 'Overview' })).toBeDefined()
    expect(screen.getByRole('button', { name: 'Details' })).toBeDefined()
  })
})
