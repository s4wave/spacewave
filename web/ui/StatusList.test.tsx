import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { StatusList, type StatusListItem } from './StatusList.js'

describe('StatusList', () => {
  afterEach(cleanup)

  it('shows status, details and custom labels and calls onItemClick', async () => {
    // Start with the default and configured empty prompts.
    const { rerender } = render(<StatusList items={[]} />)
    expect(screen.getByText('No items')).toBeTruthy()
    rerender(<StatusList items={[]} emptyMessage="Nothing here" />)
    expect(screen.getByText('Nothing here')).toBeTruthy()

    // Display every status and its detail through the ordinary item contract.
    const items: StatusListItem[] = [
      { id: '1', label: 'Alpha', status: 'success', detail: '12ms' },
      { id: '2', label: 'Beta', status: 'error' },
      { id: '3', label: 'Gamma', status: 'pending' },
      { id: '4', label: 'Delta', status: 'none' },
    ]
    rerender(<StatusList items={items} />)
    expect(screen.getByText('PASS')).toBeTruthy()
    expect(screen.getByText('FAIL')).toBeTruthy()
    expect(screen.getByText('....')).toBeTruthy()
    expect(screen.getByText('----')).toBeTruthy()
    expect(screen.getByText('12ms')).toBeTruthy()

    // Keep custom labels and report the selected item unchanged.
    const user = userEvent.setup()
    const onItemClick = vi.fn()
    rerender(
      <StatusList
        items={items}
        statusLabels={{ success: 'OK', error: 'ERR' }}
        onItemClick={onItemClick}
      />,
    )
    expect(screen.getByText('OK')).toBeTruthy()
    expect(screen.getByText('ERR')).toBeTruthy()
    await user.click(screen.getByText('Alpha'))
    expect(onItemClick).toHaveBeenCalledExactlyOnceWith(items[0])
  })
})
