import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { BlogReadingView } from './BlogReadingView.js'
import type { BlogPostData } from './types.js'

vi.mock('../CodeBlock.js', () => ({
  useMarkdownCodeOverrides: () => ({}),
}))

const published: BlogPostData = {
  name: 'published.md',
  title: 'Published Post',
  date: '2026-04-17',
  summary: '',
  tags: [],
  body: 'Published body',
}

const draft: BlogPostData = {
  name: 'untitled.md',
  title: 'untitled',
  date: '',
  summary: '',
  tags: [],
  body: 'Draft body',
}

describe('BlogReadingView', () => {
  afterEach(() => {
    cleanup()
  })

  it('does not render a selected non-published note as a reading post', () => {
    render(
      <BlogReadingView
        posts={[published, draft]}
        selectedPost={draft}
        onSelectPost={vi.fn()}
        authorRegistry={{}}
      />,
    )

    expect(screen.queryByText('untitled')).toBeNull()
    expect(screen.getByText('Published Post')).toBeDefined()
  })
})
