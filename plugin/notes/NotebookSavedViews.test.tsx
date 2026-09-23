import { fireEvent, render, within } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'

import NotebookSavedViews from './NotebookSavedViews.js'
import type { SavedView } from './saved-views.js'

const shared = vi.hoisted(() => ({
  views: [] as SavedView[],
  save: vi.fn(),
  rename: vi.fn(),
  remove: vi.fn(),
}))

vi.mock('./useNotebookSavedViews.js', () => ({
  useNotebookSavedViews: () => ({
    ...shared,
    loading: false,
    pending: false,
    ready: true,
    error: null,
  }),
}))

const view: SavedView = {
  id: 'review',
  notebook: 'notes',
  name: 'Review',
  sourceRef: 'files/-/',
  path: '',
  filterTag: 'review',
  filterStatus: null,
  sort: 'title',
}

beforeEach(() => {
  vi.clearAllMocks()
  shared.views = [view]
})

it('keeps each viewer selection and filter draft independent of shared edits', () => {
  // Each mounted viewer owns its personal namespace and explicit load action.
  const aliceLoad = vi.fn(() => null)
  const bobLoad = vi.fn(() => null)
  const world = { value: null, loading: false, error: null, retry: vi.fn() }
  const pair = () => (
    <>
      <section aria-label="Alice">
        <NotebookSavedViews
          world={world}
          notebook="notes"
          handle={null}
          draft={{ ...view, filterTag: 'alice-draft' }}
          onLoad={aliceLoad}
        />
      </section>
      <section aria-label="Bob">
        <NotebookSavedViews
          world={world}
          notebook="notes"
          handle={null}
          draft={{ ...view, filterTag: 'bob-draft' }}
          onLoad={bobLoad}
        />
      </section>
    </>
  )
  const mounted = render(pair())
  const alice = within(mounted.getByRole('region', { name: 'Alice' }))
  const bob = within(mounted.getByRole('region', { name: 'Bob' }))
  fireEvent.click(alice.getByText('Shared views'))
  fireEvent.click(bob.getByText('Shared views'))
  fireEvent.change(alice.getByLabelText('Saved view'), {
    target: { value: view.id },
  })
  fireEvent.click(alice.getByRole('button', { name: 'Load view' }))
  expect(aliceLoad).toHaveBeenCalledWith(view)
  expect(bobLoad).not.toHaveBeenCalled()

  // Renaming and editing shared filters update both pickers without loading either draft.
  shared.views = [{ ...view, name: 'Weekly review', filterTag: 'shared-edit' }]
  mounted.rerender(pair())
  expect(alice.getByRole('option', { name: 'Weekly review' })).toBeTruthy()
  expect(bob.getByRole('option', { name: 'Weekly review' })).toBeTruthy()
  expect((alice.getByLabelText('Saved view') as HTMLSelectElement).value).toBe(
    view.id,
  )
  expect((bob.getByLabelText('Saved view') as HTMLSelectElement).value).toBe('')
  expect(aliceLoad).toHaveBeenCalledTimes(1)
  expect(bobLoad).not.toHaveBeenCalled()
  fireEvent.click(alice.getByRole('button', { name: 'Save changes' }))
  expect(shared.save).toHaveBeenCalledWith({
    ...view,
    name: 'Weekly review',
    filterTag: 'alice-draft',
  })

  // A remote deletion leaves a clear recovery action and no stale load button.
  shared.views = []
  mounted.rerender(pair())
  expect(alice.getByRole('status').textContent).toContain('no longer available')
  expect(
    (alice.getByRole('button', { name: 'Load view' }) as HTMLButtonElement)
      .disabled,
  ).toBe(true)
  mounted.unmount()
})
