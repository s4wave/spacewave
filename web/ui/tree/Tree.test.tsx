import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, waitFor, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Tree, TreeNode } from './index.js'

interface TestData {
  value: number
}

const mockNodes: TreeNode<TestData>[] = [
  {
    id: 'root',
    name: 'Root',
    data: { value: 1 },
    children: [
      {
        id: 'child1',
        name: 'Child 1',
        data: { value: 2 },
        children: [
          { id: 'grandchild1', name: 'Grandchild 1', data: { value: 3 } },
          { id: 'grandchild2', name: 'Grandchild 2', data: { value: 4 } },
        ],
      },
      { id: 'child2', name: 'Child 2', data: { value: 5 } },
    ],
  },
  { id: 'sibling', name: 'Sibling', data: { value: 6 } },
]

describe('Tree', () => {
  afterEach(() => {
    cleanup()
  })

  it('should render the tree container', () => {
    render(<Tree nodes={mockNodes} />)
    expect(screen.getByRole('tree')).toBeTruthy()
  })

  it('should render tree nodes', async () => {
    render(<Tree nodes={mockNodes} />)

    await waitFor(() => {
      expect(screen.getByText('Root')).toBeTruthy()
    })

    expect(screen.getByText('Sibling')).toBeTruthy()
  })

  it('should show placeholder when no nodes', () => {
    render(<Tree nodes={[]} placeholder="No items available" />)
    expect(screen.getByText('No items available')).toBeTruthy()
  })

  it('should show default placeholder when empty', () => {
    render(<Tree nodes={[]} />)
    expect(screen.getByText('No items')).toBeTruthy()
  })

  it('should expand nodes when defaultExpandedIds is set', async () => {
    render(<Tree nodes={mockNodes} defaultExpandedIds={new Set(['root'])} />)

    await waitFor(() => {
      expect(screen.getByText('Child 1')).toBeTruthy()
      expect(screen.getByText('Child 2')).toBeTruthy()
    })
  })

  it('should not show children when node is collapsed', async () => {
    render(<Tree nodes={mockNodes} />)

    await waitFor(() => {
      expect(screen.getByText('Root')).toBeTruthy()
    })

    expect(screen.queryByText('Child 1')).toBeNull()
    expect(screen.queryByText('Child 2')).toBeNull()
  })

  it('should handle single click selection', async () => {
    const user = userEvent.setup()
    render(<Tree nodes={mockNodes} />)

    await waitFor(() => {
      expect(screen.getByText('Root')).toBeTruthy()
    })

    const rootNode = screen.getByText('Root').closest('[role="treeitem"]')
    expect(rootNode).toBeTruthy()

    await user.click(rootNode!)

    await waitFor(() => {
      expect(rootNode?.getAttribute('aria-selected')).toBe('true')
    })
  })

  it('should handle double click to trigger default action', async () => {
    const user = userEvent.setup()
    const onAction = vi.fn()
    render(<Tree nodes={mockNodes} onRowDefaultAction={onAction} />)

    await waitFor(() => {
      expect(screen.getByText('Sibling')).toBeTruthy()
    })

    const siblingNode = screen.getByText('Sibling').closest('[role="treeitem"]')
    await user.dblClick(siblingNode!)

    await waitFor(() => {
      expect(onAction).toHaveBeenCalledOnce()
      expect(onAction.mock.calls[0]?.[0]).toHaveLength(1)
      expect(
        (onAction.mock.calls[0]?.[0] as { name: string }[])?.[0]?.name,
      ).toBe('Sibling')
    })
  })

  it('should expand/collapse on chevron click', async () => {
    const user = userEvent.setup()
    render(<Tree nodes={mockNodes} />)

    await waitFor(() => {
      expect(screen.getByText('Root')).toBeTruthy()
    })

    expect(screen.queryByText('Child 1')).toBeNull()

    const expandButton = screen.getByLabelText('Expand Root')
    await user.click(expandButton)

    await waitFor(() => {
      expect(screen.getByText('Child 1')).toBeTruthy()
      expect(screen.getByText('Child 2')).toBeTruthy()
    })

    const collapseButton = screen.getByLabelText('Collapse Root')
    await user.click(collapseButton)

    await waitFor(() => {
      expect(screen.queryByText('Child 1')).toBeNull()
    })
  })

  it('should handle keyboard navigation with ArrowDown', async () => {
    const user = userEvent.setup()
    render(<Tree nodes={mockNodes} defaultExpandedIds={new Set(['root'])} />)

    await waitFor(() => {
      expect(screen.getByText('Root')).toBeTruthy()
    })

    const tree = screen.getByRole('tree')
    tree.focus()

    await user.keyboard('{ArrowDown}')

    await waitFor(() => {
      const items = screen.getAllByRole('treeitem')
      const hasSelection = items.some(
        (item) => item.getAttribute('aria-selected') === 'true',
      )
      expect(hasSelection).toBe(true)
    })
  })

  it('should handle keyboard navigation with j/k', async () => {
    const user = userEvent.setup()
    render(<Tree nodes={mockNodes} defaultExpandedIds={new Set(['root'])} />)

    await waitFor(() => {
      expect(screen.getByText('Root')).toBeTruthy()
    })

    const tree = screen.getByRole('tree')
    tree.focus()

    // j moves down from Root (initially focused) to Child 1
    await user.keyboard('j')

    await waitFor(() => {
      const child1Item = screen
        .getByText('Child 1')
        .closest('[role="treeitem"]')
      expect(child1Item?.getAttribute('aria-selected')).toBe('true')
    })

    // Second j moves to Child 2
    await user.keyboard('j')

    await waitFor(() => {
      const child2Item = screen
        .getByText('Child 2')
        .closest('[role="treeitem"]')
      expect(child2Item?.getAttribute('aria-selected')).toBe('true')
    })

    // k moves back up to Child 1
    await user.keyboard('k')

    await waitFor(() => {
      const child1Item = screen
        .getByText('Child 1')
        .closest('[role="treeitem"]')
      expect(child1Item?.getAttribute('aria-selected')).toBe('true')
    })
  })

  it('should handle expand with ArrowRight', async () => {
    const user = userEvent.setup()
    render(<Tree nodes={mockNodes} />)

    await waitFor(() => {
      expect(screen.getByText('Root')).toBeTruthy()
    })

    const rootItem = screen.getByText('Root').closest('[role="treeitem"]')
    await user.click(rootItem!)

    await waitFor(() => {
      expect(rootItem?.getAttribute('aria-selected')).toBe('true')
    })

    await user.keyboard('{ArrowRight}')

    await waitFor(() => {
      expect(screen.getByText('Child 1')).toBeTruthy()
    })
  })

  it('should handle collapse with ArrowLeft', async () => {
    const user = userEvent.setup()
    render(<Tree nodes={mockNodes} defaultExpandedIds={new Set(['root'])} />)

    await waitFor(() => {
      expect(screen.getByText('Child 1')).toBeTruthy()
    })

    const rootItem = screen.getByText('Root').closest('[role="treeitem"]')
    await user.click(rootItem!)

    await user.keyboard('{ArrowLeft}')

    await waitFor(() => {
      expect(screen.queryByText('Child 1')).toBeNull()
    })
  })

  it('should navigate to parent with ArrowLeft when collapsed', async () => {
    const user = userEvent.setup()
    render(<Tree nodes={mockNodes} defaultExpandedIds={new Set(['root'])} />)

    await waitFor(() => {
      expect(screen.getByText('Child 1')).toBeTruthy()
    })

    const child1Item = screen.getByText('Child 1').closest('[role="treeitem"]')
    await user.click(child1Item!)

    await waitFor(() => {
      expect(child1Item?.getAttribute('aria-selected')).toBe('true')
    })

    await user.keyboard('{ArrowLeft}')

    await waitFor(() => {
      const rootItem = screen.getByText('Root').closest('[role="treeitem"]')
      expect(rootItem?.getAttribute('aria-selected')).toBe('true')
    })
  })

  it('should handle range selection with Shift+Arrow', async () => {
    const user = userEvent.setup()
    render(<Tree nodes={mockNodes} defaultExpandedIds={new Set(['root'])} />)

    await waitFor(() => {
      expect(screen.getByText('Root')).toBeTruthy()
    })

    const rootItem = screen.getByText('Root').closest('[role="treeitem"]')
    await user.click(rootItem!)

    await waitFor(() => {
      expect(rootItem?.getAttribute('aria-selected')).toBe('true')
    })

    await user.keyboard('{Shift>}{ArrowDown}{ArrowDown}{/Shift}')

    await waitFor(() => {
      const selectedItems = screen
        .getAllByRole('treeitem')
        .filter((item) => item.getAttribute('aria-selected') === 'true')
      expect(selectedItems.length).toBeGreaterThan(1)
    })
  })

  it('should handle toggle selection with Ctrl+click', async () => {
    const user = userEvent.setup()
    render(<Tree nodes={mockNodes} defaultExpandedIds={new Set(['root'])} />)

    await waitFor(() => {
      expect(screen.getByText('Root')).toBeTruthy()
    })

    const rootItem = screen.getByText('Root').closest('[role="treeitem"]')
    await user.click(rootItem!)

    await waitFor(() => {
      expect(rootItem?.getAttribute('aria-selected')).toBe('true')
    })

    const child1Item = screen.getByText('Child 1').closest('[role="treeitem"]')
    await user.keyboard('{Control>}')
    await user.click(child1Item!)
    await user.keyboard('{/Control}')

    await waitFor(() => {
      expect(rootItem?.getAttribute('aria-selected')).toBe('true')
      expect(child1Item?.getAttribute('aria-selected')).toBe('true')
    })
  })

  it('should handle Space to toggle expand on parent node', async () => {
    const user = userEvent.setup()
    render(<Tree nodes={mockNodes} />)

    await waitFor(() => {
      expect(screen.getByText('Root')).toBeTruthy()
    })

    const rootItem = screen.getByText('Root').closest('[role="treeitem"]')
    await user.click(rootItem!)

    await user.keyboard(' ')

    await waitFor(() => {
      expect(screen.getByText('Child 1')).toBeTruthy()
    })
  })

  it('should handle Enter to trigger default action', async () => {
    const user = userEvent.setup()
    const onAction = vi.fn()
    render(
      <Tree
        nodes={mockNodes}
        onRowDefaultAction={onAction}
        defaultExpandedIds={new Set(['root'])}
      />,
    )

    await waitFor(() => {
      expect(screen.getByText('Child 2')).toBeTruthy()
    })

    const child2Item = screen.getByText('Child 2').closest('[role="treeitem"]')
    await user.click(child2Item!)

    await user.keyboard('{Enter}')

    await waitFor(() => {
      expect(onAction).toHaveBeenCalledOnce()
      expect(
        (onAction.mock.calls[0]?.[0] as { name: string }[])?.[0]?.name,
      ).toBe('Child 2')
    })
  })

  it('should have proper ARIA attributes', async () => {
    render(<Tree nodes={mockNodes} defaultExpandedIds={new Set(['root'])} />)

    await waitFor(() => {
      expect(screen.getByText('Root')).toBeTruthy()
    })

    const tree = screen.getByRole('tree')
    expect(tree.getAttribute('aria-multiselectable')).toBe('true')
    expect(tree.getAttribute('aria-orientation')).toBe('vertical')

    const rootItem = screen.getByText('Root').closest('[role="treeitem"]')
    expect(rootItem?.getAttribute('aria-expanded')).toBe('true')
    expect(rootItem?.getAttribute('aria-level')).toBe('1')

    const child1Item = screen.getByText('Child 1').closest('[role="treeitem"]')
    expect(child1Item?.getAttribute('aria-expanded')).toBe('false')
    expect(child1Item?.getAttribute('aria-level')).toBe('2')
  })
})
