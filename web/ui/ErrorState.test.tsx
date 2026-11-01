import { it, expect, vi, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ErrorState } from './ErrorState.js'

afterEach(cleanup)

it('keeps error content and exposes retry only while it is available', async () => {
  // Show the default failure without a retry action.
  const user = userEvent.setup()
  const view = render(
    <ErrorState message="Something went wrong">Details</ErrorState>,
  )
  expect(screen.getByRole('heading', { name: 'Error' })).toBeDefined()
  expect(screen.getByText('Something went wrong')).toBeDefined()
  expect(screen.getByText('Details')).toBeDefined()
  expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull()

  // Follow the supplied retry action and retain the chosen title and message.
  const retry = vi.fn()
  view.rerender(
    <ErrorState
      title="Connection Failed"
      message="Cannot connect"
      onRetry={retry}
    />,
  )
  expect(
    screen.getByRole('heading', { name: 'Connection Failed' }),
  ).toBeDefined()
  expect(screen.getByText('Cannot connect')).toBeDefined()
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  expect(retry).toHaveBeenCalledOnce()

  // Removing the action removes its control in the inline presentation too.
  view.rerender(<ErrorState message="Offline" variant="inline" />)
  expect(screen.getByText('Offline')).toBeDefined()
  expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull()
})
