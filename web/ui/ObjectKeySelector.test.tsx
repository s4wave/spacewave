import React, { useEffect, type DependencyList } from 'react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, cleanup, fireEvent, screen } from '@testing-library/react'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type { WorldQuery } from '@s4wave/sdk/world/world-query.js'
import { listingWorld } from '@s4wave/web/test/world-query.js'

vi.mock('@s4wave/web/ui/Popover.js', () => ({
  Popover: ({
    children,
    onOpenChange,
  }: {
    children: React.ReactNode
    onOpenChange: (open: boolean) => void
  }) => {
    useEffect(() => onOpenChange(true), [onOpenChange])
    return <div>{children}</div>
  },
  PopoverTrigger: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  PopoverContent: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}))

vi.mock('@s4wave/web/hooks/useWorldQuery.js', async () => {
  const { useFakeWorldQuery } = await import('@s4wave/web/test/world-query.js')
  return {
    useWorldQuery: <T,>(
      world: Resource<IWorldState>,
      query: WorldQuery<T>,
      deps: DependencyList,
    ) => useFakeWorldQuery(world.value!, query, deps),
  }
})

import { ObjectKeySelector } from './ObjectKeySelector.js'

const testWorld: Resource<IWorldState> = {
  value: listingWorld([
    { objectKey: 'dir/file1', objectType: 'unixfs/fs-node' },
    { objectKey: 'dir/file2', objectType: 'canvas' },
    { objectKey: 'toplevel', objectType: 'canvas' },
  ]),
  loading: false,
  error: null,
  retry: () => {},
}

beforeEach(() => {
  cleanup()
})

describe('ObjectKeySelector', () => {
  it('renders trigger button with value prop text', () => {
    const onChange = vi.fn()
    render(
      <ObjectKeySelector
        world={testWorld}
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
        world={testWorld}
        value=""
        onChange={onChange}
        placeholder="Pick one..."
      />,
    )
    const button = screen.getByRole('button', { name: 'Pick one...' })
    expect(button).toBeDefined()
  })

  it('renders both top-level nodes', async () => {
    render(<ObjectKeySelector world={testWorld} value="" onChange={vi.fn()} />)
    expect(await screen.findByText('dir')).toBeDefined()
    expect(screen.getByText('toplevel')).toBeDefined()
  })

  it('drills into folder children on click', async () => {
    render(<ObjectKeySelector world={testWorld} value="" onChange={vi.fn()} />)
    fireEvent.click(await screen.findByText('dir'))
    expect(await screen.findByText('file1')).toBeDefined()
    expect(screen.getByText('file2')).toBeDefined()
    expect(screen.getByText('dir/')).toBeDefined()
  })

  it('returns to top level on back click', async () => {
    render(<ObjectKeySelector world={testWorld} value="" onChange={vi.fn()} />)
    fireEvent.click(await screen.findByText('dir'))
    fireEvent.click(await screen.findByText('dir/'))
    expect(await screen.findByText('toplevel')).toBeDefined()
    expect(screen.getByText('dir')).toBeDefined()
  })

  it('selects a leaf without calling onChange', async () => {
    const onChange = vi.fn()
    render(<ObjectKeySelector world={testWorld} value="" onChange={onChange} />)
    fireEvent.click(await screen.findByText('toplevel'))
    expect(onChange).not.toHaveBeenCalled()
  })

  it('calls onChange with the key after selecting a leaf and clicking Select', async () => {
    const onChange = vi.fn()
    render(<ObjectKeySelector world={testWorld} value="" onChange={onChange} />)
    fireEvent.click(await screen.findByText('toplevel'))
    fireEvent.click(screen.getByText('Select'))
    expect(onChange).toHaveBeenCalledWith('toplevel')
  })

  it('calls onChange with nested key after drill-in select and confirm', async () => {
    const onChange = vi.fn()
    render(<ObjectKeySelector world={testWorld} value="" onChange={onChange} />)
    fireEvent.click(await screen.findByText('dir'))
    fireEvent.click(await screen.findByText('file1'))
    fireEvent.click(screen.getByText('Select'))
    expect(onChange).toHaveBeenCalledWith('dir/file1')
  })

  it('disables Select button when nothing is selected', () => {
    render(<ObjectKeySelector world={testWorld} value="" onChange={vi.fn()} />)
    const selectBtn = screen.getByText('Select')
    expect((selectBtn as HTMLButtonElement).disabled).toBe(true)
  })

  it('disables the trigger button when disabled prop is true', () => {
    const onChange = vi.fn()
    render(
      <ObjectKeySelector
        world={testWorld}
        value="test"
        onChange={onChange}
        disabled
      />,
    )
    const trigger = screen.getByRole('button', { name: 'test' })
    expect((trigger as HTMLButtonElement).disabled).toBe(true)
  })
})
