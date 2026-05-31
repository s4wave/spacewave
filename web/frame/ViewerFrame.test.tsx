import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
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
})
