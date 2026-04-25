import React from 'react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, cleanup, fireEvent, screen } from '@testing-library/react'
import type { TreeNode } from '@s4wave/web/ui/tree/TreeNode.js'
import type { ObjectTreeNode } from '@s4wave/web/space/object-tree.js'

vi.mock('@s4wave/web/ui/Popover.js', () => ({
  Popover: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  PopoverTrigger: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  PopoverContent: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}))

import { ObjectKeySelector } from './ObjectKeySelector.js'

const testNodes: TreeNode<ObjectTreeNode>[] = [
  {
    id: 'dir',
    name: 'dir',
    icon: null,
    children: [
      {
        id: 'dir/file1',
        name: 'file1',
        icon: null,
        data: {
          objectKey: 'dir/file1',
          objectType: 'unixfs/fs-node',
          isVirtual: false,
        },
      },
      {
        id: 'dir/file2',
        name: 'file2',
        icon: null,
        data: {
          objectKey: 'dir/file2',
          objectType: 'canvas',
          isVirtual: false,
        },
      },
    ],
    data: { objectKey: 'dir', objectType: '', isVirtual: true },
  },
  {
    id: 'toplevel',
    name: 'toplevel',
    icon: null,
    data: { objectKey: 'toplevel', objectType: 'canvas', isVirtual: false },
  },
]

beforeEach(() => {
  cleanup()
})

describe('ObjectKeySelector', () => {
  it('renders trigger button with value prop text', () => {
    const onChange = vi.fn()
    render(
      <ObjectKeySelector
        nodes={testNodes}
        value="my-object"
        onChange={onChange}
      />,
    )
    const button = screen.getByRole('button', { name: 'my-object' })
    expect(button).toBeDefined()
  })

  it('renders trigger button with placeholder when value is empty', () => {
    const onChange = vi.fn()
    render(
      <ObjectKeySelector
        nodes={testNodes}
        value=""
        onChange={onChange}
        placeholder="Pick one..."
      />,
    )
    const button = screen.getByRole('button', { name: 'Pick one...' })
    expect(button).toBeDefined()
  })

  it('renders both top-level nodes', () => {
    const onChange = vi.fn()
    render(<ObjectKeySelector nodes={testNodes} value="" onChange={onChange} />)
    expect(screen.getByText('dir')).toBeDefined()
    expect(screen.getByText('toplevel')).toBeDefined()
  })

  it('renders folder items in the node list', () => {
    const onChange = vi.fn()
    render(<ObjectKeySelector nodes={testNodes} value="" onChange={onChange} />)
    expect(screen.getByText('dir')).toBeDefined()
  })

  it('renders leaf items in the node list', () => {
    const onChange = vi.fn()
    render(<ObjectKeySelector nodes={testNodes} value="" onChange={onChange} />)
    expect(screen.getByText('toplevel')).toBeDefined()
  })

  it('drills into folder children on click', () => {
    const onChange = vi.fn()
    render(<ObjectKeySelector nodes={testNodes} value="" onChange={onChange} />)
    fireEvent.click(screen.getByText('dir'))
    expect(screen.getByText('file1')).toBeDefined()
    expect(screen.getByText('file2')).toBeDefined()
  })

  it('shows back button after drilling in', () => {
    const onChange = vi.fn()
    render(<ObjectKeySelector nodes={testNodes} value="" onChange={onChange} />)
    fireEvent.click(screen.getByText('dir'))
    expect(screen.getByText('dir/')).toBeDefined()
  })

  it('returns to top level on back click', () => {
    const onChange = vi.fn()
    render(<ObjectKeySelector nodes={testNodes} value="" onChange={onChange} />)
    fireEvent.click(screen.getByText('dir'))
    expect(screen.getByText('file1')).toBeDefined()
    fireEvent.click(screen.getByText('dir/'))
    expect(screen.getByText('dir')).toBeDefined()
    expect(screen.getByText('toplevel')).toBeDefined()
  })

  it('selects a leaf without calling onChange', () => {
    const onChange = vi.fn()
    render(<ObjectKeySelector nodes={testNodes} value="" onChange={onChange} />)
    fireEvent.click(screen.getByText('toplevel'))
    expect(onChange).not.toHaveBeenCalled()
  })

  it('calls onChange with the key after selecting a leaf and clicking Select', () => {
    const onChange = vi.fn()
    render(<ObjectKeySelector nodes={testNodes} value="" onChange={onChange} />)
    fireEvent.click(screen.getByText('toplevel'))
    fireEvent.click(screen.getByText('Select'))
    expect(onChange).toHaveBeenCalledWith('toplevel')
  })

  it('calls onChange with nested key after drill-in select and confirm', () => {
    const onChange = vi.fn()
    render(<ObjectKeySelector nodes={testNodes} value="" onChange={onChange} />)
    fireEvent.click(screen.getByText('dir'))
    fireEvent.click(screen.getByText('file1'))
    fireEvent.click(screen.getByText('Select'))
    expect(onChange).toHaveBeenCalledWith('dir/file1')
  })

  it('disables Select button when nothing is selected', () => {
    const onChange = vi.fn()
    render(<ObjectKeySelector nodes={testNodes} value="" onChange={onChange} />)
    const selectBtn = screen.getByText('Select')
    expect((selectBtn as HTMLButtonElement).disabled).toBe(true)
  })

  it('disables the trigger button when disabled prop is true', () => {
    const onChange = vi.fn()
    render(
      <ObjectKeySelector
        nodes={testNodes}
        value="test"
        onChange={onChange}
        disabled
      />,
    )
    const trigger = screen.getByRole('button', { name: 'test' })
    expect((trigger as HTMLButtonElement).disabled).toBe(true)
  })
})
