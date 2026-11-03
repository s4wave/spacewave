import { describe, expect, it } from 'vitest'
import { page } from 'vitest/browser'
import { render } from 'vitest-browser-react'
import { useState } from 'react'

import { BottomBarItem } from './bottom-bar-item.js'
import { BottomBarLevel } from './bottom-bar-level.js'
import { BottomBarRoot } from './bottom-bar-root.js'
import { ViewerFrame } from './ViewerFrame.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    overflow: 'hidden',
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
    height: `calc(${themeLayout['--spacing']} * 150)`,
    width: `calc(${themeLayout['--spacing']} * 225)`,
  },
  rule2: {
    flex: 1,
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
    height: '100%',
    width: '100%',
  },
})

function ObjectDetailsButton(selected: boolean, onClick: () => void) {
  return (
    <BottomBarItem
      selected={selected}
      onClick={onClick}
      aria-label="Object details"
    >
      getting-started.md
    </BottomBarItem>
  )
}

function EmbeddedViewer() {
  const [openMenu, setOpenMenu] = useState('')

  return (
    <div
      sx={stylexStyles.rule1}
      className="space-flexlayout"
      data-testid="object-layout"
    >
      <div sx={stylexStyles.rule2}>
        <div className={'flexlayout__optimized_layout'}>
          <div
            className={'flexlayout__optimized_layout_tab_container'}
            data-layout-path="/tab-container"
          >
            <div
              className={'flexlayout__tab'}
              role="tabpanel"
              data-tab-id="markdown"
            >
              <BottomBarRoot openMenu={openMenu} setOpenMenu={setOpenMenu}>
                <BottomBarLevel
                  id="tab-markdown"
                  menuLabel="getting-started.md"
                  button={ObjectDetailsButton}
                  overlay={
                    <div data-testid="object-details-overlay">
                      Object details
                    </div>
                  }
                >
                  <ViewerFrame>
                    <div>getting-started.md</div>
                  </ViewerFrame>
                </BottomBarLevel>
              </BottomBarRoot>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

describe('ObjectLayout embedded object details overlay', () => {
  it('mounts and hides the overlay when the bottom-left toggle is clicked', async () => {
    await render(<EmbeddedViewer />)

    const toggle = page.getByRole('button', { name: 'Object details' })
    await toggle.click()
    await expect
      .element(page.getByTestId('object-details-overlay'))
      .toBeVisible()
    await expect.element(toggle).toHaveAttribute('aria-pressed', 'true')

    await toggle.click()
    expect(page.getByTestId('object-details-overlay').query()).toBeNull()
    await expect.element(toggle).toHaveAttribute('aria-pressed', 'false')
  })
})
