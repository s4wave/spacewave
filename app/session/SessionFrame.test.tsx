import React from 'react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, cleanup, fireEvent } from '@testing-library/react'
import { SessionFrame } from './SessionFrame.js'
import { BottomBarLevel } from '@s4wave/web/frame/bottom-bar-level.js'
import { BottomBarItem } from '@s4wave/web/frame/bottom-bar-item.js'
import { BottomBarRoot } from '@s4wave/web/frame/bottom-bar-root.js'

function TestWrapper({ children }: { children: React.ReactNode }) {
  return <>{children}</>
}

describe('SessionFrame', () => {
  beforeEach(() => {
    cleanup()
  })

  describe('Button Integration', () => {
    it('passes onClick handler to button render function', () => {
      const onClickCalls: (() => void)[] = []
      const setOpenMenu = vi.fn()

      const TestComponent = () => {
        return (
          <TestWrapper>
            <BottomBarRoot setOpenMenu={setOpenMenu} openMenu="">
              <BottomBarLevel
                id="test-item"
                button={(selected, onClick) => {
                  onClickCalls.push(onClick)
                  return <button data-testid="test-button">Test</button>
                }}
              >
                <SessionFrame>
                  <div>Content</div>
                </SessionFrame>
              </BottomBarLevel>
            </BottomBarRoot>
          </TestWrapper>
        )
      }

      render(
        <TestWrapper>
          <TestComponent />
        </TestWrapper>,
      )

      expect(onClickCalls.length).toBeGreaterThan(0)
      expect(typeof onClickCalls[0]).toBe('function')
    })

    it('toggles menu state when button is clicked multiple times', () => {
      const setOpenMenu = vi.fn()

      const TestComponent = () => {
        return (
          <BottomBarRoot setOpenMenu={setOpenMenu} openMenu="">
            <BottomBarLevel
              id="test-item"
              button={(selected, onClick) => (
                <button data-testid="test-button" onClick={onClick}>
                  Test
                </button>
              )}
            >
              <SessionFrame>
                <div>Content</div>
              </SessionFrame>
            </BottomBarLevel>
          </BottomBarRoot>
        )
      }

      const { getByTestId, rerender } = render(
        <TestWrapper>
          <TestComponent />
        </TestWrapper>,
      )
      const button = getByTestId('test-button')

      fireEvent.click(button)
      expect(setOpenMenu).toHaveBeenCalledWith('test-item')

      const TestComponentSelected = () => {
        return (
          <BottomBarRoot setOpenMenu={setOpenMenu} openMenu="test-item">
            <BottomBarLevel
              id="test-item"
              button={(selected, onClick) => (
                <button data-testid="test-button" onClick={onClick}>
                  Test
                </button>
              )}
            >
              <SessionFrame>
                <div>Content</div>
              </SessionFrame>
            </BottomBarLevel>
          </BottomBarRoot>
        )
      }

      rerender(
        <TestWrapper>
          <TestComponentSelected />
        </TestWrapper>,
      )
      fireEvent.click(getByTestId('test-button'))
      expect(setOpenMenu).toHaveBeenCalledWith('')
    })

    it('passes className to button render function when selected', () => {
      const classNames: (string | undefined)[] = []

      const TestComponent = () => {
        return (
          <BottomBarRoot setOpenMenu={() => {}} openMenu="test-item">
            <BottomBarLevel
              id="test-item"
              button={(selected, onClick, className) => {
                classNames.push(className)
                return <button data-testid="test-button">Test</button>
              }}
            >
              <SessionFrame>
                <div>Content</div>
              </SessionFrame>
            </BottomBarLevel>
          </BottomBarRoot>
        )
      }

      render(
        <TestWrapper>
          <TestComponent />
        </TestWrapper>,
      )

      const selectedClassName = classNames.find((cn) => cn && cn !== '')
      expect(selectedClassName).toContain('bg-bar-item-selected')
    })

    it('passes empty className to button render function when not selected', () => {
      const classNames: (string | undefined)[] = []

      const TestComponent = () => {
        return (
          <BottomBarRoot setOpenMenu={() => {}} openMenu="">
            <BottomBarLevel
              id="test-item"
              button={(selected, onClick, className) => {
                classNames.push(className)
                return <button data-testid="test-button">Test</button>
              }}
            >
              <SessionFrame>
                <div>Content</div>
              </SessionFrame>
            </BottomBarLevel>
          </BottomBarRoot>
        )
      }

      render(
        <TestWrapper>
          <TestComponent />
        </TestWrapper>,
      )

      const unselectedClassName = classNames[classNames.length - 1]
      expect(unselectedClassName).toBe('')
    })
  })

  describe('BottomBarItem with Space Key', () => {
    it('triggers setOpenMenu when Space is pressed on BottomBarItem', () => {
      const setOpenMenu = vi.fn()

      const TestComponent = () => {
        return (
          <BottomBarRoot setOpenMenu={setOpenMenu} openMenu="">
            <BottomBarLevel
              id="test-item"
              button={(selected, onClick) => (
                <BottomBarItem onClick={onClick} data-testid="test-item">
                  Test
                </BottomBarItem>
              )}
            >
              <SessionFrame>
                <div>Content</div>
              </SessionFrame>
            </BottomBarLevel>
          </BottomBarRoot>
        )
      }

      const { container } = render(
        <TestWrapper>
          <TestComponent />
        </TestWrapper>,
      )
      const item = container.querySelector('[data-testid="test-item"]')

      expect(item).toBeTruthy()
      fireEvent.keyDown(item!, { key: ' ' })

      expect(setOpenMenu).toHaveBeenCalledWith('test-item')
    })

    it('triggers setOpenMenu when Enter is pressed on BottomBarItem', () => {
      const setOpenMenu = vi.fn()

      const TestComponent = () => {
        return (
          <BottomBarRoot setOpenMenu={setOpenMenu} openMenu="">
            <BottomBarLevel
              id="test-item"
              button={(selected, onClick) => (
                <BottomBarItem onClick={onClick} data-testid="test-item">
                  Test
                </BottomBarItem>
              )}
            >
              <SessionFrame>
                <div>Content</div>
              </SessionFrame>
            </BottomBarLevel>
          </BottomBarRoot>
        )
      }

      const { container } = render(
        <TestWrapper>
          <TestComponent />
        </TestWrapper>,
      )
      const item = container.querySelector('[data-testid="test-item"]')

      expect(item).toBeTruthy()
      fireEvent.keyDown(item!, { key: 'Enter' })

      expect(setOpenMenu).toHaveBeenCalledWith('test-item')
    })

    it('BottomBarItem receives className for selected state styling', () => {
      const TestComponent = () => {
        return (
          <BottomBarRoot setOpenMenu={() => {}} openMenu="test-item">
            <BottomBarLevel
              id="test-item"
              button={(selected, onClick, className) => (
                <BottomBarItem
                  onClick={onClick}
                  className={className}
                  data-testid="test-item"
                >
                  Test
                </BottomBarItem>
              )}
            >
              <SessionFrame>
                <div>Content</div>
              </SessionFrame>
            </BottomBarLevel>
          </BottomBarRoot>
        )
      }

      const { container } = render(
        <TestWrapper>
          <TestComponent />
        </TestWrapper>,
      )
      const item = container.querySelector('[data-testid="test-item"]')

      expect(item?.className).toContain('bg-bar-item-selected')
    })
  })

  describe('Multiple Items', () => {
    it('handles multiple bottom bar items with keyboard navigation', () => {
      const setOpenMenu = vi.fn()

      const TestComponent = () => {
        return (
          <BottomBarRoot setOpenMenu={setOpenMenu} openMenu="">
            <BottomBarLevel
              id="item1"
              button={(selected, onClick) => (
                <BottomBarItem onClick={onClick} data-testid="item1">
                  Item 1
                </BottomBarItem>
              )}
            >
              <BottomBarLevel
                id="item2"
                button={(selected, onClick) => (
                  <BottomBarItem onClick={onClick} data-testid="item2">
                    Item 2
                  </BottomBarItem>
                )}
              >
                <SessionFrame>
                  <div>Content</div>
                </SessionFrame>
              </BottomBarLevel>
            </BottomBarLevel>
          </BottomBarRoot>
        )
      }

      const { container } = render(
        <TestWrapper>
          <TestComponent />
        </TestWrapper>,
      )
      const item1 = container.querySelector('[data-testid="item1"]')
      const item2 = container.querySelector('[data-testid="item2"]')

      fireEvent.keyDown(item1!, { key: ' ' })
      expect(setOpenMenu).toHaveBeenCalledWith('item1')

      fireEvent.keyDown(item2!, { key: 'Enter' })
      expect(setOpenMenu).toHaveBeenCalledWith('item2')
    })

    it('renders items registered inside SessionFrame children', () => {
      const setOpenMenu = vi.fn()

      const TestComponent = () => {
        return (
          <BottomBarRoot setOpenMenu={setOpenMenu} openMenu="">
            <BottomBarLevel
              id="parent"
              button={(selected, onClick) => (
                <BottomBarItem onClick={onClick} data-testid="parent">
                  Parent
                </BottomBarItem>
              )}
            >
              <SessionFrame>
                {/* This item is INSIDE SessionFrame - key test case! */}
                <BottomBarLevel
                  id="child"
                  button={(selected, onClick) => (
                    <BottomBarItem onClick={onClick} data-testid="child">
                      Child
                    </BottomBarItem>
                  )}
                >
                  <div>Content</div>
                </BottomBarLevel>
              </SessionFrame>
            </BottomBarLevel>
          </BottomBarRoot>
        )
      }

      const { container } = render(
        <TestWrapper>
          <TestComponent />
        </TestWrapper>,
      )

      // Both items should be rendered in the bottom bar
      const parent = container.querySelector('[data-testid="parent"]')
      const child = container.querySelector('[data-testid="child"]')

      expect(parent).toBeTruthy()
      expect(child).toBeTruthy()

      // Verify both can be clicked
      fireEvent.keyDown(parent!, { key: ' ' })
      expect(setOpenMenu).toHaveBeenCalledWith('parent')

      fireEvent.keyDown(child!, { key: 'Enter' })
      expect(setOpenMenu).toHaveBeenCalledWith('child')
    })
  })
})
