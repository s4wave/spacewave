import { it, expect, vi, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { EmptyState } from './EmptyState.js'

afterEach(cleanup)

it('keeps empty content and exposes its supplied action', async () => {
  // Start with an empty list that has no available action.
  const user = userEvent.setup()
  const view = render(<EmptyState title="No results" />)
  expect(screen.getByRole('heading', { name: 'No results' })).toBeDefined()
  expect(screen.queryByRole('button')).toBeNull()

  // Show the supplied description, icon and creation action together.
  const create = vi.fn()
  view.rerender(
    <EmptyState
      title="Empty"
      description="Try adding something"
      icon={<span>Custom icon</span>}
      action={{ label: 'Add Item', onClick: create }}
    />,
  )
  expect(screen.getByText('Try adding something')).toBeDefined()
  expect(screen.getByText('Custom icon')).toBeDefined()
  await user.click(screen.getByRole('button', { name: 'Add Item' }))
  expect(create).toHaveBeenCalledOnce()

  // Removing optional content updates the compact presentation without a stale action.
  view.rerender(<EmptyState title="Compact" variant="compact" />)
  expect(screen.getByRole('heading', { name: 'Compact' })).toBeDefined()
  expect(screen.queryByText('Try adding something')).toBeNull()
  expect(screen.queryByText('Custom icon')).toBeNull()
  expect(screen.queryByRole('button')).toBeNull()
})
