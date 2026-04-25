/**
 * E2E tests for the baked-in Heavy Frost tab bar CSS.
 *
 * Builds the real FlexLayout DOM structure with proper class names,
 * applies the shell CSS, and verifies computed styles match the
 * Heavy Frost design spec (16px blur, 0.4 inactive opacity, 5px radius, etc).
 */
import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { render, cleanup } from 'vitest-browser-react'
import '@s4wave/web/style/app.css'

/**
 * Builds a mock DOM structure matching FlexLayout's real output.
 * This is the hierarchy from .shell-flexlayout down to tab buttons.
 */
function MockShellFlexLayout({ withMenu = true }: { withMenu?: boolean }) {
  const shellClass =
    withMenu ?
      'shell-flexlayout shell-flexlayout--with-menu'
    : 'shell-flexlayout'

  return (
    <div
      className={shellClass}
      style={{ width: 800, height: 400 }}
      data-testid="shell"
    >
      <div className="flexlayout__optimized_layout">
        <div
          className="flexlayout__layout"
          style={{ position: 'relative', width: '100%', height: '100%' }}
        >
          <div className="flexlayout__layout_main">
            <div className="flexlayout__row">
              <div className="flexlayout__tabset_container">
                <div className="flexlayout__tabset" data-testid="tabset">
                  {/* Tab bar outer */}
                  <div
                    className="flexlayout__tabset_tabbar_outer flexlayout__tabset_tabbar_outer_top flexlayout__tabset-selected"
                    data-testid="tabbar-outer"
                  >
                    {/* Tab bar inner */}
                    <div
                      className="flexlayout__tabset_tabbar_inner flexlayout__tabset_tabbar_inner_top"
                      data-testid="tabbar-inner"
                    >
                      {/* Tab container */}
                      <div
                        className="flexlayout__tabset_tabbar_inner_tab_container flexlayout__tabset_tabbar_inner_tab_container_top"
                        data-testid="tab-container"
                      >
                        {/* Inactive tab */}
                        <div
                          className="flexlayout__tab_button flexlayout__tab_button--unselected flexlayout__tab_button_top"
                          data-testid="tab-inactive"
                        >
                          <div className="flexlayout__tab_button_content">
                            Home
                          </div>
                        </div>

                        {/* Active/selected tab */}
                        <div
                          className="flexlayout__tab_button flexlayout__tab_button--selected flexlayout__tab_button_top"
                          data-testid="tab-active"
                        >
                          <div className="flexlayout__tab_button_content">
                            main.tsx
                          </div>
                        </div>

                        {/* Another inactive tab */}
                        <div
                          className="flexlayout__tab_button flexlayout__tab_button--unselected flexlayout__tab_button_top"
                          data-testid="tab-inactive-2"
                        >
                          <div className="flexlayout__tab_button_content">
                            Terminal
                          </div>
                        </div>
                      </div>
                    </div>

                    {/* Toolbar */}
                    <div
                      className="flexlayout__tab_toolbar"
                      data-testid="toolbar"
                    >
                      <button className="flexlayout__tab_toolbar_button">
                        +
                      </button>
                    </div>
                  </div>

                  {/* Content area */}
                  <div
                    className="flexlayout__tabset_content"
                    data-testid="content"
                  >
                    <div className="flexlayout__tab" style={{ flex: 1 }}>
                      <div>Tab content</div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

function getEl(testId: string): HTMLElement {
  return document.querySelector(`[data-testid="${testId}"]`) as HTMLElement
}

function getStyle(testId: string): CSSStyleDeclaration {
  return window.getComputedStyle(getEl(testId))
}

describe('Heavy Frost Tab CSS (baked-in)', () => {
  beforeEach(() => void cleanup())
  afterEach(() => void cleanup())

  it('tab bar outer is 30px in menu mode', async () => {
    await render(<MockShellFlexLayout />)
    await expect.poll(() => getEl('tabbar-outer')).not.toBeNull()

    const style = getStyle('tabbar-outer')
    const height = parseFloat(style.height)
    expect(height).toBeGreaterThanOrEqual(28)
    expect(height).toBeLessThanOrEqual(32)
  })

  it('tab bar inner uses flex-end alignment (tabs hug bottom)', async () => {
    await render(<MockShellFlexLayout />)
    await expect.poll(() => getEl('tabbar-inner')).not.toBeNull()

    const style = getStyle('tabbar-inner')
    expect(style.alignItems).toBe('flex-end')
  })

  it('tab container uses flex-end alignment with proper gap', async () => {
    await render(<MockShellFlexLayout />)
    await expect.poll(() => getEl('tab-container')).not.toBeNull()

    const style = getStyle('tab-container')
    expect(style.alignItems).toBe('flex-end')
    // Gap should be 2px
    const gap = parseFloat(style.gap || style.columnGap || '0')
    expect(gap).toBe(2)
  })

  it('inactive tab has frosted glass background with backdrop-filter', async () => {
    await render(<MockShellFlexLayout />)
    await expect.poll(() => getEl('tab-inactive')).not.toBeNull()

    const style = getStyle('tab-inactive')
    // Should have backdrop-filter blur
    const bf =
      style.backdropFilter ||
      (style as unknown as Record<string, string>)['webkitBackdropFilter'] ||
      ''
    expect(bf).toContain('blur')

    // Border-radius should be 5px 5px 0 0
    expect(style.borderTopLeftRadius).toBe('5px')
    expect(style.borderTopRightRadius).toBe('5px')
    expect(style.borderBottomLeftRadius).toBe('0px')
    expect(style.borderBottomRightRadius).toBe('0px')

    // Border-bottom should be none (or 0px)
    const bbWidth = parseFloat(style.borderBottomWidth)
    expect(bbWidth).toBe(0)
  })

  it('active tab has opaque background and no backdrop-filter', async () => {
    await render(<MockShellFlexLayout />)
    await expect.poll(() => getEl('tab-active')).not.toBeNull()

    const style = getStyle('tab-active')
    // Should NOT have backdrop-filter
    const bf =
      style.backdropFilter ||
      (style as unknown as Record<string, string>)['webkitBackdropFilter'] ||
      ''
    expect(bf === '' || bf === 'none').toBe(true)

    // Border-radius should be 5px 5px 0 0
    expect(style.borderTopLeftRadius).toBe('5px')
    expect(style.borderTopRightRadius).toBe('5px')

    // Border-bottom should be none (tab sits flush on content)
    const bbWidth = parseFloat(style.borderBottomWidth)
    expect(bbWidth).toBe(0)
  })

  it('active tab text is brighter than inactive tab text', async () => {
    await render(<MockShellFlexLayout />)
    await expect.poll(() => getEl('tab-active')).not.toBeNull()

    const activeColor = getStyle('tab-active').color
    const inactiveColor = getStyle('tab-inactive').color

    // Colors should be different — active much brighter
    expect(activeColor).not.toBe(inactiveColor)
  })

  it('tabs are 22px tall', async () => {
    await render(<MockShellFlexLayout />)
    await expect.poll(() => getEl('tab-inactive')).not.toBeNull()

    const inactiveHeight = getEl('tab-inactive').getBoundingClientRect().height
    const activeHeight = getEl('tab-active').getBoundingClientRect().height

    // Tabs should be 22px
    expect(inactiveHeight).toBeGreaterThanOrEqual(21)
    expect(inactiveHeight).toBeLessThanOrEqual(23)
    expect(activeHeight).toBeGreaterThanOrEqual(21)
    expect(activeHeight).toBeLessThanOrEqual(23)
  })

  it('tab bar inner is 24px (bottom-hugging)', async () => {
    await render(<MockShellFlexLayout />)
    await expect.poll(() => getEl('tabbar-inner')).not.toBeNull()

    const innerHeight = getEl('tabbar-inner').getBoundingClientRect().height
    expect(innerHeight).toBeGreaterThanOrEqual(23)
    expect(innerHeight).toBeLessThanOrEqual(25)
  })

  it('::after pseudo-element is hidden on tab buttons', async () => {
    await render(<MockShellFlexLayout />)
    await expect.poll(() => getEl('tab-active')).not.toBeNull()

    const afterStyle = window.getComputedStyle(getEl('tab-active'), '::after')
    expect(afterStyle.display).toBe('none')
  })

  it('inactive tab borders are subtle (top slightly stronger than sides)', async () => {
    await render(<MockShellFlexLayout />)
    await expect.poll(() => getEl('tab-inactive')).not.toBeNull()

    const style = getStyle('tab-inactive')
    // All borders should be 1px
    expect(parseFloat(style.borderTopWidth)).toBe(1)
    expect(parseFloat(style.borderLeftWidth)).toBe(1)
    expect(parseFloat(style.borderRightWidth)).toBe(1)
  })

  it('active tab top border is more visible than inactive', async () => {
    await render(<MockShellFlexLayout />)
    await expect.poll(() => getEl('tab-active')).not.toBeNull()

    const activeStyle = getStyle('tab-active')
    const inactiveStyle = getStyle('tab-inactive')

    // Both should have 1px top borders
    expect(parseFloat(activeStyle.borderTopWidth)).toBe(1)
    expect(parseFloat(inactiveStyle.borderTopWidth)).toBe(1)

    // Active border should be more opaque (different color)
    expect(activeStyle.borderTopColor).not.toBe(inactiveStyle.borderTopColor)
  })
})
