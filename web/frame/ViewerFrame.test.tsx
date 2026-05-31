import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { BottomBarLevel } from './bottom-bar-level.js'
import { BottomBarRoot } from './bottom-bar-root.js'
import { BottomBarItem } from './bottom-bar-item.js'
import { ViewerFrame } from './ViewerFrame.js'

function button(label: string) {
  return (selected: boolean, onClick: () => void, className?: string) => (
    <BottomBarItem selected={selected} onClick={onClick} className={className}>
      {label}
    </BottomBarItem>
  )
}

describe('ViewerFrame', () => {
  afterEach(() => {
    cleanup()
    vi.useRealTimers()
  })

  it('collapses intermediate left bottom-bar items into a menu', async () => {
    const user = userEvent.setup()
    const setOpenMenu = vi.fn()

    render(
      <BottomBarRoot openMenu="" setOpenMenu={setOpenMenu}>
        <BottomBarLevel id="first" menuLabel="First" button={button('First')}>
          <BottomBarLevel
            id="middle"
            menuLabel="Middle"
            button={button('Middle')}
          >
            <BottomBarLevel id="last" menuLabel="Last" button={button('Last')}>
              <ViewerFrame>
                <div>Content</div>
              </ViewerFrame>
            </BottomBarLevel>
          </BottomBarLevel>
        </BottomBarLevel>
      </BottomBarRoot>,
    )

    expect(screen.getByText('First')).toBeTruthy()
    expect(screen.getByText('Last')).toBeTruthy()
    expect(screen.queryByText('Middle')).toBeNull()

    await user.click(screen.getByLabelText('Open hidden bottom bar items'))

    const item = screen.getByText('Middle')
    expect(item).toBeTruthy()

    await user.click(item)
    expect(setOpenMenu).toHaveBeenCalledWith('middle')
  })

  it('opens a collapsed item context menu without selecting the hidden item', async () => {
    const user = userEvent.setup()
    const setOpenMenu = vi.fn()
    const onSelect = vi.fn()

    render(
      <BottomBarRoot openMenu="" setOpenMenu={setOpenMenu}>
        <BottomBarLevel id="first" menuLabel="First" button={button('First')}>
          <BottomBarLevel
            id="middle"
            menuLabel="Middle"
            button={button('Middle')}
            contextMenuItems={[
              {
                type: 'action',
                id: 'inspect',
                label: 'Inspect Middle',
                onSelect,
              },
            ]}
          >
            <BottomBarLevel id="last" menuLabel="Last" button={button('Last')}>
              <ViewerFrame>
                <div>Content</div>
              </ViewerFrame>
            </BottomBarLevel>
          </BottomBarLevel>
        </BottomBarLevel>
      </BottomBarRoot>,
    )

    await user.click(screen.getByLabelText('Open hidden bottom bar items'))
    fireEvent.contextMenu(screen.getByText('Middle'), {
      clientX: 64,
      clientY: 10,
    })

    expect(setOpenMenu).not.toHaveBeenCalled()
    await user.click(await screen.findByText('Inspect Middle'))
    expect(onSelect).toHaveBeenCalledWith(
      expect.objectContaining({ itemId: 'middle', openKind: 'mouse' }),
    )
  })

  it('opens a collapsed item context menu from the keyboard', async () => {
    const user = userEvent.setup()
    const onSelect = vi.fn()

    render(
      <BottomBarRoot openMenu="" setOpenMenu={() => {}}>
        <BottomBarLevel id="first" menuLabel="First" button={button('First')}>
          <BottomBarLevel
            id="middle"
            menuLabel="Middle"
            button={button('Middle')}
            contextMenuItems={[
              {
                type: 'action',
                id: 'inspect',
                label: 'Inspect Middle',
                onSelect,
              },
            ]}
          >
            <BottomBarLevel id="last" menuLabel="Last" button={button('Last')}>
              <ViewerFrame>
                <div>Content</div>
              </ViewerFrame>
            </BottomBarLevel>
          </BottomBarLevel>
        </BottomBarLevel>
      </BottomBarRoot>,
    )

    await user.click(screen.getByLabelText('Open hidden bottom bar items'))
    const collapsedItem = screen.getByText('Middle')
    collapsedItem.focus()
    await user.keyboard('{ContextMenu}')

    await user.click(await screen.findByText('Inspect Middle'))
    expect(onSelect).toHaveBeenCalledWith(
      expect.objectContaining({ itemId: 'middle', openKind: 'keyboard' }),
    )
  })

  it('keeps two left bottom-bar items visible without a collapse menu', () => {
    render(
      <BottomBarRoot openMenu="" setOpenMenu={() => {}}>
        <BottomBarLevel id="first" menuLabel="First" button={button('First')}>
          <BottomBarLevel id="last" menuLabel="Last" button={button('Last')}>
            <ViewerFrame>
              <div>Content</div>
            </ViewerFrame>
          </BottomBarLevel>
        </BottomBarLevel>
      </BottomBarRoot>,
    )

    expect(screen.getByText('First')).toBeTruthy()
    expect(screen.getByText('Last')).toBeTruthy()
    expect(screen.queryByLabelText('Open hidden bottom bar items')).toBeNull()
  })

  it('opens a visible item context menu on right-click without toggling the primary overlay', async () => {
    const user = userEvent.setup()
    const setOpenMenu = vi.fn()
    const onSelect = vi.fn((context) => context.openPrimaryOverlay())

    render(
      <BottomBarRoot openMenu="" setOpenMenu={setOpenMenu}>
        <BottomBarLevel
          id="first"
          menuLabel="First"
          button={button('First')}
          overlay={<div>First overlay</div>}
          contextMenuLabel="First actions"
          contextMenuItems={[
            {
              type: 'action',
              id: 'open-details',
              label: 'Open Details',
              shortcut: 'Enter',
              onSelect,
            },
          ]}
        >
          <ViewerFrame>
            <div>Content</div>
          </ViewerFrame>
        </BottomBarLevel>
      </BottomBarRoot>,
    )

    fireEvent.contextMenu(screen.getByText('First'), {
      clientX: 40,
      clientY: 8,
    })

    expect(screen.queryByText('First overlay')).toBeNull()
    expect(setOpenMenu).not.toHaveBeenCalled()
    expect(await screen.findByText('Open Details')).toBeTruthy()
    expect(screen.getByText('Enter')).toBeTruthy()

    const content = document.body.querySelector(
      '[data-slot="dropdown-menu-content"]',
    )
    expect(content?.getAttribute('data-side')).toBe('top')

    await user.click(screen.getByText('Open Details'))
    expect(onSelect).toHaveBeenCalledWith(
      expect.objectContaining({ itemId: 'first', openKind: 'mouse' }),
    )
    expect(setOpenMenu).toHaveBeenCalledWith('first')
  })

  it('opens a right-position item context menu through the same renderer', async () => {
    const onSelect = vi.fn()

    render(
      <BottomBarRoot openMenu="" setOpenMenu={() => {}}>
        <ViewerFrame>
          <BottomBarLevel
            id="status"
            position="right"
            menuLabel="Status"
            button={button('Status')}
            contextMenuItems={[
              {
                type: 'action',
                id: 'inspect',
                label: 'Inspect Status',
                onSelect,
              },
            ]}
          >
            <div>Content</div>
          </BottomBarLevel>
        </ViewerFrame>
      </BottomBarRoot>,
    )

    fireEvent.contextMenu(screen.getByText('Status'), {
      clientX: 140,
      clientY: 8,
    })

    expect(await screen.findByText('Inspect Status')).toBeTruthy()
  })

  it('reports context menu action failures without destabilizing the frame', async () => {
    const user = userEvent.setup()
    const err = new Error('failed action')
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})

    render(
      <BottomBarRoot openMenu="" setOpenMenu={() => {}}>
        <BottomBarLevel
          id="first"
          menuLabel="First"
          button={button('First')}
          contextMenuItems={[
            {
              type: 'action',
              id: 'switch',
              label: 'Switch Object Here',
              onSelect: () => Promise.reject(err),
            },
          ]}
        >
          <ViewerFrame>
            <div>Content</div>
          </ViewerFrame>
        </BottomBarLevel>
      </BottomBarRoot>,
    )

    fireEvent.contextMenu(screen.getByText('First'), {
      clientX: 40,
      clientY: 8,
    })
    await user.click(await screen.findByText('Switch Object Here'))

    await waitFor(() => {
      expect(warn).toHaveBeenCalledWith(
        'bottom bar context menu action failed',
        err,
      )
    })
    warn.mockRestore()
  })

  it('opens a context menu from keyboard and returns focus on dismissal', async () => {
    const user = userEvent.setup()

    render(
      <BottomBarRoot openMenu="" setOpenMenu={() => {}}>
        <BottomBarLevel
          id="first"
          menuLabel="First"
          button={button('First')}
          contextMenuItems={[
            {
              type: 'action',
              id: 'open-details',
              label: 'Open Details',
              onSelect: () => {},
            },
          ]}
        >
          <ViewerFrame>
            <div>Content</div>
          </ViewerFrame>
        </BottomBarLevel>
      </BottomBarRoot>,
    )

    const trigger = screen.getByText('First')
    trigger.focus()
    await user.keyboard('{ContextMenu}')

    expect(await screen.findByText('Open Details')).toBeTruthy()
    expect(trigger.getAttribute('aria-expanded')).toBe('true')

    await user.keyboard('{Escape}')
    await waitFor(() => {
      expect(screen.queryByText('Open Details')).toBeNull()
      expect(document.activeElement).toBe(trigger)
    })
  })

  it('opens a context menu from long-press and suppresses the primary overlay click', async () => {
    vi.useFakeTimers()
    const setOpenMenu = vi.fn()

    render(
      <BottomBarRoot openMenu="" setOpenMenu={setOpenMenu}>
        <BottomBarLevel
          id="first"
          menuLabel="First"
          button={button('First')}
          contextMenuItems={[
            {
              type: 'action',
              id: 'open-details',
              label: 'Open Details',
              onSelect: () => {},
            },
          ]}
        >
          <ViewerFrame>
            <div>Content</div>
          </ViewerFrame>
        </BottomBarLevel>
      </BottomBarRoot>,
    )

    const trigger = screen.getByText('First')
    fireEvent.pointerDown(trigger, {
      pointerId: 1,
      pointerType: 'touch',
      clientX: 40,
      clientY: 8,
    })
    act(() => {
      vi.advanceTimersByTime(550)
    })
    fireEvent.click(trigger)

    expect(screen.getByText('Open Details')).toBeTruthy()
    expect(setOpenMenu).not.toHaveBeenCalled()
  })
})
