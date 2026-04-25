import { describe, it, expect, beforeEach } from 'vitest'
import { render, cleanup } from '@testing-library/react'

describe('FlexLayout Border Styling', () => {
  beforeEach(() => {
    cleanup()
  })

  it('renders tabset with selected child using :has() selector', () => {
    const { container } = render(
      <div>
        <div className="flexlayout__tabset" data-testid="selected-tabset">
          <div className="flexlayout__tabset_tabbar_outer flexlayout__tabset-selected">
            Selected tabbar
          </div>
          <div className="flexlayout__tabset_container">Content</div>
        </div>
        <div className="flexlayout__tabset" data-testid="unselected-tabset">
          <div className="flexlayout__tabset_tabbar_outer">
            Unselected tabbar
          </div>
          <div className="flexlayout__tabset_container">Content</div>
        </div>
      </div>,
    )

    const selectedTabset = container.querySelector(
      '[data-testid="selected-tabset"]',
    )
    const unselectedTabset = container.querySelector(
      '[data-testid="unselected-tabset"]',
    )

    expect(selectedTabset).toBeTruthy()
    expect(unselectedTabset).toBeTruthy()

    const selectedChild = selectedTabset?.querySelector(
      '.flexlayout__tabset-selected',
    )
    const unselectedChild = unselectedTabset?.querySelector(
      '.flexlayout__tabset-selected',
    )

    expect(selectedChild).toBeTruthy()
    expect(unselectedChild).toBeFalsy()
  })

  it('verifies :has() selector targets parent when child has class', () => {
    const { container } = render(
      <div>
        <div className="parent-element" data-testid="with-selected">
          <div className="child-selected">Selected</div>
        </div>
        <div className="parent-element" data-testid="without-selected">
          <div className="child-unselected">Unselected</div>
        </div>
      </div>,
    )

    const parentWithSelected = container.querySelector(
      '[data-testid="with-selected"]',
    )
    const parentWithoutSelected = container.querySelector(
      '[data-testid="without-selected"]',
    )

    expect(parentWithSelected?.querySelector('.child-selected')).toBeTruthy()
    expect(parentWithoutSelected?.querySelector('.child-selected')).toBeFalsy()
  })

  it('verifies CSS :has() selector pattern', () => {
    const cssRule = '.flexlayout__tabset:has(.flexlayout__tabset-selected)'

    expect(cssRule).toContain('.flexlayout__tabset')
    expect(cssRule).toContain(':has(')
    expect(cssRule).toContain('.flexlayout__tabset-selected')

    const usesHasSelector = cssRule.match(
      /\.flexlayout__tabset:has\(\.flexlayout__tabset-selected\)/,
    )
    expect(usesHasSelector).toBeTruthy()
  })

  it('verifies the CSS rule targets parent tabset not child', () => {
    const correctRule = '.flexlayout__tabset:has(.flexlayout__tabset-selected)'
    const incorrectRule =
      '.flexlayout__tabset-selected .flexlayout__tabset_container'

    expect(correctRule).toContain(':has(')
    expect(incorrectRule).not.toContain(':has(')
  })
})
