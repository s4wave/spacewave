import React, { useCallback, useMemo, useState } from 'react'
import { describe, it, expect, beforeEach } from 'vitest'
import { render, cleanup, fireEvent, screen } from '@testing-library/react'
import { useBottomBarItems, BottomBarItem } from './bottom-bar-context.js'
import { BottomBarLevel } from './bottom-bar-level.js'
import { BottomBarRoot } from './bottom-bar-root.js'
import { ViewerFrame } from './ViewerFrame.js'

describe('BottomBarContext', () => {
  beforeEach(() => {
    cleanup()
  })

  describe('BottomBarLevel', () => {
    it('registers a single item', () => {
      const TestComponent = () => {
        const items = useBottomBarItems()
        return <div data-testid="items">{JSON.stringify(items.length)}</div>
      }

      const { getByTestId } = render(
        <BottomBarRoot>
          <BottomBarLevel
            id="item1"
            button={(selected) => (
              <button>Item 1 {selected ? 'on' : 'off'}</button>
            )}
          >
            <TestComponent />
          </BottomBarLevel>
        </BottomBarRoot>,
      )

      expect(getByTestId('items').textContent).toBe('1')
    })

    it('maintains item order from shallow to deep', () => {
      const TestComponent = () => {
        const items = useBottomBarItems()
        return (
          <div data-testid="ids">
            {JSON.stringify(items.map((item) => item.id))}
          </div>
        )
      }

      const { getByTestId } = render(
        <BottomBarRoot>
          <BottomBarLevel id="outer" button={() => <button>Outer</button>}>
            <BottomBarLevel id="middle" button={() => <button>Middle</button>}>
              <BottomBarLevel id="inner" button={() => <button>Inner</button>}>
                <TestComponent />
              </BottomBarLevel>
            </BottomBarLevel>
          </BottomBarLevel>
        </BottomBarRoot>,
      )

      expect(JSON.parse(getByTestId('ids').textContent ?? '')).toEqual([
        'outer',
        'middle',
        'inner',
      ])
    })

    it('preserves button render functions', () => {
      const TestComponent = () => {
        const items = useBottomBarItems()
        const button = items[0]?.button
        return (
          <div>
            <div data-testid="unselected">{button?.(false, () => {}, '')}</div>
            <div data-testid="selected">{button?.(true, () => {}, '')}</div>
          </div>
        )
      }

      const { getByTestId } = render(
        <BottomBarRoot>
          <BottomBarLevel
            id="item"
            button={(selected) => (
              <span>{selected ? 'Selected' : 'Unselected'}</span>
            )}
          >
            <TestComponent />
          </BottomBarLevel>
        </BottomBarRoot>,
      )

      expect(getByTestId('unselected').textContent).toBe('Unselected')
      expect(getByTestId('selected').textContent).toBe('Selected')
    })

    it('handles overlays', () => {
      const TestComponent = () => {
        const items = useBottomBarItems()
        const overlay = items[0]?.overlay?.()
        return <div data-testid="overlay">{overlay}</div>
      }

      const overlayContent = <div>Test Overlay</div>

      const { getByTestId } = render(
        <BottomBarRoot>
          <BottomBarLevel
            id="item"
            button={() => <button>Item</button>}
            overlay={overlayContent}
          >
            <TestComponent />
          </BottomBarLevel>
        </BottomBarRoot>,
      )

      expect(getByTestId('overlay').textContent).toBe('Test Overlay')
    })

    it('updates the active overlay when child state changes overlay content', () => {
      function TestRoot() {
        const [openMenu, setOpenMenu] = useState('')
        return (
          <BottomBarRoot openMenu={openMenu} setOpenMenu={setOpenMenu}>
            <ViewerFrame>
              <TestComponent />
            </ViewerFrame>
          </BottomBarRoot>
        )
      }

      function TestComponent() {
        const [count, setCount] = useState(0)
        return (
          <BottomBarLevel
            id="item"
            button={(selected, onClick) => (
              <button onClick={onClick}>
                Item {selected ? 'selected' : 'idle'}
              </button>
            )}
            overlay={<div>Overlay {count}</div>}
            overlayKey={count}
          >
            <button onClick={() => setCount((n) => n + 1)}>Increment</button>
          </BottomBarLevel>
        )
      }

      render(<TestRoot />)

      fireEvent.click(screen.getByText('Item idle'))
      expect(screen.getByText('Overlay 0')).toBeDefined()

      fireEvent.click(screen.getByText('Increment'))
      expect(screen.getByText('Overlay 1')).toBeDefined()
    })

    it('updates keyed button and overlay content without unregistering between item revisions', () => {
      const lengths: number[] = []

      function RegistryProbe() {
        lengths.push(useBottomBarItems().length)
        return null
      }

      function RegisteredContentProbe() {
        const item = useBottomBarItems()[0]
        return (
          <div>
            <div data-testid="registered-button">
              {item?.button(false, () => {}, '')}
            </div>
            <div data-testid="registered-overlay">{item?.overlay?.()}</div>
          </div>
        )
      }

      function TestItem() {
        const [count, setCount] = useState(0)
        const button = useCallback(
          (_selected: boolean, onClick: () => void) => (
            <button onClick={onClick}>Item {count}</button>
          ),
          [count],
        )
        const overlay = useMemo(() => <div>Overlay {count}</div>, [count])

        return (
          <BottomBarLevel
            id="item"
            button={button}
            buttonKey={count}
            overlay={overlay}
            overlayKey={count}
          >
            <button onClick={() => setCount((n) => n + 1)}>Update</button>
          </BottomBarLevel>
        )
      }

      function TestRoot() {
        const [openMenu, setOpenMenu] = useState('')
        return (
          <BottomBarRoot openMenu={openMenu} setOpenMenu={setOpenMenu}>
            <RegistryProbe />
            <RegisteredContentProbe />
            <ViewerFrame>
              <TestItem />
            </ViewerFrame>
          </BottomBarRoot>
        )
      }

      render(<TestRoot />)

      expect(screen.getByTestId('registered-button').textContent).toBe('Item 0')
      expect(screen.getByTestId('registered-overlay').textContent).toBe(
        'Overlay 0',
      )
      lengths.length = 0

      fireEvent.click(screen.getByText('Update'))
      expect(screen.getByTestId('registered-button').textContent).toBe('Item 1')
      expect(screen.getByTestId('registered-overlay').textContent).toBe(
        'Overlay 1',
      )
      expect(lengths).not.toContain(0)
    })

    it('does not update the registry for unchanged inline overlay content', () => {
      const renders = { count: 0 }

      function TestItem() {
        const items = useBottomBarItems()
        renders.count += 1
        if (renders.count > 10) {
          throw new Error('bottom bar registry update loop')
        }

        return (
          <>
            <div data-testid="item-count">{items.length}</div>
            <BottomBarLevel
              id="item"
              button={() => <button>Item</button>}
              overlay={<div>Overlay</div>}
            >
              <div>Child</div>
            </BottomBarLevel>
          </>
        )
      }

      render(
        <BottomBarRoot>
          <TestItem />
        </BottomBarRoot>,
      )

      expect(screen.getByTestId('item-count').textContent).toBe('1')
      expect(renders.count).toBeLessThan(10)
    })

    it('handles missing overlay', () => {
      const TestComponent = () => {
        const items = useBottomBarItems()
        const overlayFn = items[0]?.overlay
        return <div data-testid="has-overlay">{overlayFn ? 'yes' : 'no'}</div>
      }

      const { getByTestId } = render(
        <BottomBarRoot>
          <BottomBarLevel id="item" button={() => <button>Item</button>}>
            <TestComponent />
          </BottomBarLevel>
        </BottomBarRoot>,
      )

      expect(getByTestId('has-overlay').textContent).toBe('no')
    })

    it('handles onBreadcrumbClick handler', () => {
      const clicks: string[] = []
      const handler = () => clicks.push('clicked')

      const TestComponent = () => {
        const items = useBottomBarItems()
        const onBreadcrumbClick = items[0]?.onBreadcrumbClick
        return (
          <div>
            <div data-testid="has-handler">
              {onBreadcrumbClick ? 'yes' : 'no'}
            </div>
            <button data-testid="trigger" onClick={() => onBreadcrumbClick?.()}>
              Trigger
            </button>
          </div>
        )
      }

      const { getByTestId } = render(
        <BottomBarRoot>
          <BottomBarLevel
            id="item"
            button={() => <button>Item</button>}
            onBreadcrumbClick={handler}
          >
            <TestComponent />
          </BottomBarLevel>
        </BottomBarRoot>,
      )

      expect(getByTestId('has-handler').textContent).toBe('yes')
      getByTestId('trigger').click()
      expect(clicks).toEqual(['clicked'])
    })

    it('handles missing onBreadcrumbClick handler', () => {
      const TestComponent = () => {
        const items = useBottomBarItems()
        const onBreadcrumbClick = items[0]?.onBreadcrumbClick
        return (
          <div data-testid="has-handler">
            {onBreadcrumbClick ? 'yes' : 'no'}
          </div>
        )
      }

      const { getByTestId } = render(
        <BottomBarRoot>
          <BottomBarLevel id="item" button={() => <button>Item</button>}>
            <TestComponent />
          </BottomBarLevel>
        </BottomBarRoot>,
      )

      expect(getByTestId('has-handler').textContent).toBe('no')
    })

    it('returns empty array when used outside BottomBarRoot', () => {
      const TestComponent = () => {
        const items = useBottomBarItems()
        return <div data-testid="count">{items.length}</div>
      }

      const { getByTestId } = render(<TestComponent />)

      expect(getByTestId('count').textContent).toBe('0')
    })

    it('creates independent context trees', () => {
      const TestComponent = ({ testId }: { testId: string }) => {
        const items = useBottomBarItems()
        return (
          <div data-testid={testId}>
            {JSON.stringify(items.map((item: BottomBarItem) => item.id))}
          </div>
        )
      }

      const { getByTestId } = render(
        <div>
          <BottomBarRoot>
            <BottomBarLevel id="tree1" button={() => <button>Tree 1</button>}>
              <TestComponent testId="tree1-result" />
            </BottomBarLevel>
          </BottomBarRoot>
          <BottomBarRoot>
            <BottomBarLevel id="tree2" button={() => <button>Tree 2</button>}>
              <TestComponent testId="tree2-result" />
            </BottomBarLevel>
          </BottomBarRoot>
        </div>,
      )

      expect(JSON.parse(getByTestId('tree1-result').textContent ?? '')).toEqual(
        ['tree1'],
      )
      expect(JSON.parse(getByTestId('tree2-result').textContent ?? '')).toEqual(
        ['tree2'],
      )
    })

    it('button render function receives correct selected state', () => {
      const states: boolean[] = []

      const TestComponent = () => {
        const items = useBottomBarItems()
        const button = items[0]?.button

        const _node1 = button?.(false, () => {}, '')
        const _node2 = button?.(true, () => {}, '')
        const _node3 = button?.(false, () => {}, '')

        return null
      }

      render(
        <BottomBarRoot>
          <BottomBarLevel
            id="item"
            button={(selected) => {
              states.push(selected)
              return <button>{selected ? 'on' : 'off'}</button>
            }}
          >
            <TestComponent />
          </BottomBarLevel>
        </BottomBarRoot>,
      )

      expect(states).toEqual([false, true, false])
    })

    it('handles complex nested structures', () => {
      const TestComponent = ({ level }: { level: string }) => {
        const items = useBottomBarItems()
        return (
          <div data-testid={`level-${level}`}>
            {JSON.stringify({
              count: items.length,
              ids: items.map((item: BottomBarItem) => item.id),
            })}
          </div>
        )
      }

      const { getByTestId } = render(
        <BottomBarRoot>
          <BottomBarLevel id="root" button={() => <button>Root</button>}>
            <TestComponent level="root" />
            <BottomBarLevel id="child1" button={() => <button>Child 1</button>}>
              <TestComponent level="child1" />
              <BottomBarLevel
                id="grandchild"
                button={() => <button>Grandchild</button>}
              >
                <TestComponent level="grandchild" />
              </BottomBarLevel>
            </BottomBarLevel>
            <BottomBarLevel id="child2" button={() => <button>Child 2</button>}>
              <TestComponent level="child2" />
            </BottomBarLevel>
          </BottomBarLevel>
        </BottomBarRoot>,
      )

      // With imperative registration, all items are visible everywhere
      // All TestComponents see the same set of registered items
      const expectedItems = {
        count: 4,
        ids: ['root', 'child1', 'child2', 'grandchild'],
      }

      expect(JSON.parse(getByTestId('level-root').textContent ?? '')).toEqual(
        expectedItems,
      )
      expect(JSON.parse(getByTestId('level-child1').textContent ?? '')).toEqual(
        expectedItems,
      )
      expect(
        JSON.parse(getByTestId('level-grandchild').textContent ?? ''),
      ).toEqual(expectedItems)
      expect(JSON.parse(getByTestId('level-child2').textContent ?? '')).toEqual(
        expectedItems,
      )
    })
  })

  describe('Context Memoization', () => {
    it('context value changes when items change', () => {
      const TestComponent = () => {
        const items = useBottomBarItems()
        return <div data-testid="item-id">{items[0]?.id}</div>
      }

      const buttonFn = () => <button>Item</button>
      const overlayContent = <div>Overlay</div>

      const { getByTestId, rerender } = render(
        <BottomBarRoot>
          <BottomBarLevel id="item" button={buttonFn} overlay={overlayContent}>
            <TestComponent />
          </BottomBarLevel>
        </BottomBarRoot>,
      )

      expect(getByTestId('item-id').textContent).toBe('item')

      rerender(
        <BottomBarRoot>
          <BottomBarLevel id="item" button={buttonFn} overlay={overlayContent}>
            <TestComponent />
          </BottomBarLevel>
        </BottomBarRoot>,
      )

      expect(getByTestId('item-id').textContent).toBe('item')
    })

    it('creates new items array when id changes', () => {
      const TestComponent = () => {
        const items = useBottomBarItems()
        return <div data-testid="item-id">{items[0]?.id}</div>
      }

      const buttonFn = () => <button>Item</button>

      const { getByTestId, rerender } = render(
        <BottomBarRoot>
          <BottomBarLevel id="item1" button={buttonFn}>
            <TestComponent />
          </BottomBarLevel>
        </BottomBarRoot>,
      )

      expect(getByTestId('item-id').textContent).toBe('item1')

      rerender(
        <BottomBarRoot>
          <BottomBarLevel id="item2" button={buttonFn}>
            <TestComponent />
          </BottomBarLevel>
        </BottomBarRoot>,
      )

      expect(getByTestId('item-id').textContent).toBe('item2')
    })

    it('creates new items array when button changes', () => {
      const TestComponent = () => {
        const items = useBottomBarItems()
        return <div data-testid="has-item">{items[0] ? 'yes' : 'no'}</div>
      }

      const buttonFn1 = () => <button>Item 1</button>
      const buttonFn2 = () => <button>Item 2</button>

      const { getByTestId, rerender } = render(
        <BottomBarRoot>
          <BottomBarLevel id="item" button={buttonFn1}>
            <TestComponent />
          </BottomBarLevel>
        </BottomBarRoot>,
      )

      expect(getByTestId('has-item').textContent).toBe('yes')

      rerender(
        <BottomBarRoot>
          <BottomBarLevel id="item" button={buttonFn2}>
            <TestComponent />
          </BottomBarLevel>
        </BottomBarRoot>,
      )

      expect(getByTestId('has-item').textContent).toBe('yes')
    })
  })
})
