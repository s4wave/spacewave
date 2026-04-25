import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { StatusList, type StatusListItem } from './StatusList.js'

describe('StatusList', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders empty state with default message', () => {
    render(<StatusList items={[]} />)
    expect(screen.getByText('No items')).toBeDefined()
  })

  it('renders custom empty message', () => {
    render(<StatusList items={[]} emptyMessage="Nothing here" />)
    expect(screen.getByText('Nothing here')).toBeDefined()
  })

  it('renders items with labels', () => {
    const items: StatusListItem[] = [
      { id: '1', label: 'Test Alpha', status: 'success' },
      { id: '2', label: 'Test Beta', status: 'error' },
    ]
    render(<StatusList items={items} />)
    expect(screen.getByText('Test Alpha')).toBeDefined()
    expect(screen.getByText('Test Beta')).toBeDefined()
  })

  it('shows correct default status labels', () => {
    const items: StatusListItem[] = [
      { id: '1', label: 'A', status: 'success' },
      { id: '2', label: 'B', status: 'error' },
      { id: '3', label: 'C', status: 'pending' },
      { id: '4', label: 'D', status: 'none' },
    ]
    render(<StatusList items={items} />)
    expect(screen.getByText('PASS')).toBeDefined()
    expect(screen.getByText('FAIL')).toBeDefined()
    expect(screen.getByText('....')).toBeDefined()
    expect(screen.getByText('----')).toBeDefined()
  })

  it('supports custom status labels', () => {
    const items: StatusListItem[] = [
      { id: '1', label: 'A', status: 'success' },
      { id: '2', label: 'B', status: 'error' },
    ]
    render(
      <StatusList
        items={items}
        statusLabels={{ success: 'OK', error: 'ERR' }}
      />,
    )
    expect(screen.getByText('OK')).toBeDefined()
    expect(screen.getByText('ERR')).toBeDefined()
  })

  it('shows detail text when provided', () => {
    const items: StatusListItem[] = [
      { id: '1', label: 'A', status: 'success', detail: '12ms' },
    ]
    render(<StatusList items={items} />)
    expect(screen.getByText('12ms')).toBeDefined()
  })

  it('calls onItemClick when an item is clicked', async () => {
    const user = userEvent.setup()
    const handleClick = vi.fn()
    const items: StatusListItem[] = [
      { id: '1', label: 'Clickable Item', status: 'success' },
    ]
    render(<StatusList items={items} onItemClick={handleClick} />)

    await user.click(screen.getByText('Clickable Item'))
    expect(handleClick).toHaveBeenCalledOnce()
    expect(handleClick).toHaveBeenCalledWith(items[0])
  })

  it('applies custom className', () => {
    const { container } = render(
      <StatusList items={[]} className="my-custom-class" />,
    )
    expect(
      container.firstElementChild?.classList.contains('my-custom-class'),
    ).toBe(true)
  })
})
