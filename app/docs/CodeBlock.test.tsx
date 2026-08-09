import { render, screen, waitFor } from '@testing-library/react'
import { expect, it, vi } from 'vitest'

const highlighter = vi.hoisted(() => {
  let resolve!: (value: unknown) => void
  const promise = new Promise((done) => {
    resolve = done
  })
  return { promise, resolve }
})

vi.mock('shiki', () => ({
  createHighlighter: vi.fn(() => highlighter.promise),
}))

import { CodeBlock } from './CodeBlock.js'

it('publishes only the latest highlight when inputs overlap', async () => {
  const view = render(<CodeBlock lang="typescript" code="first" />)
  view.rerender(<CodeBlock lang="typescript" code="second" />)

  highlighter.resolve({
    getLoadedLanguages: () => ['typescript'],
    codeToHtml: (code: string) => `<pre data-highlighted="true">${code}</pre>`,
  })

  await waitFor(() =>
    expect(
      screen
        .getByText('second')
        .closest('pre')
        ?.getAttribute('data-highlighted'),
    ).toBe('true'),
  )
  expect(screen.queryByText('first')).toBeNull()
})
