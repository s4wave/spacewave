import React, { useMemo } from 'react'

import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'

import { useBlogPosts } from './useBlogPosts.js'

function buildResource<T>(value: T) {
  return {
    value,
    loading: false,
    error: null,
    retry: vi.fn(),
  }
}

function makeRootHandle(files: Record<string, string>) {
  return {
    lookup: vi.fn(async (name: string) => ({
      readAt: vi.fn(async () => ({
        data: new TextEncoder().encode(files[name] ?? ''),
        eof: true,
      })),
      release: vi.fn(),
    })),
  }
}

function HookHarness({
  files,
  entries,
}: {
  files: Record<string, string>
  entries: Array<{ name: string; isDir: boolean }>
}) {
  const rootHandle = useMemo(
    () => buildResource(makeRootHandle(files) as never),
    [files],
  )
  const entryResource = useMemo(
    () => buildResource(entries as never),
    [entries],
  )
  const posts = useBlogPosts(rootHandle, entryResource)

  if (posts.loading) {
    return React.createElement('div', null, 'loading')
  }

  return React.createElement(
    'div',
    { 'data-testid': 'posts' },
    (posts.value ?? []).map((post) => `${post.name}:${post.date}`).join('|'),
  )
}

describe('useBlogPosts', () => {
  afterEach(() => {
    cleanup()
  })

  it('includes dated markdown posts while hiding notebook-only markdown files', async () => {
    render(
      React.createElement(HookHarness, {
        entries: [
          { name: 'shared-post.md', isDir: false },
          { name: 'work-note.md', isDir: false },
          { name: 'image.png', isDir: false },
        ],
        files: {
          'shared-post.md':
            '---\n' +
            'title: Shared Post\n' +
            'date: 2026-04-16\n' +
            'summary: Shared across notebook and blog\n' +
            'tags: [shared]\n' +
            '---\n\n' +
            '# Shared Post\n',
          'work-note.md':
            '---\n' +
            'status: in-progress\n' +
            'tags: [internal]\n' +
            '---\n\n' +
            '# Work Note\n',
        },
      }),
    )

    await waitFor(() => {
      const text = screen.getByTestId('posts').textContent ?? ''
      expect(text).toContain('shared-post.md:2026-04-16')
      expect(text).not.toContain('work-note.md')
    })
  })
})
