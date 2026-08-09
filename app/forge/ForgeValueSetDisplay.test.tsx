import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ForgeValueSetDisplay } from './ForgeValueSetDisplay.js'

describe('ForgeValueSetDisplay', () => {
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('renders unnamed world object snapshots with bigint revisions', () => {
    render(
      <ForgeValueSetDisplay
        title="Inputs"
        emptyLabel="No inputs"
        values={[
          {
            name: '',
            worldObjectSnapshot: {
              key: 'forge/task/one',
              rev: 9_007_199_254_740_993n,
            },
          },
        ]}
      />,
    )

    expect(screen.getByText('value-1')).toBeTruthy()
    expect(screen.getByText('world object @ rev 9007199254740993')).toBeTruthy()
  })

  it('keeps value nodes stable when values reorder', () => {
    const first = {
      name: 'shared',
      worldObjectSnapshot: { key: 'forge/task/one', rev: 1n },
    }
    const second = {
      name: 'shared',
      worldObjectSnapshot: { key: 'forge/task/two', rev: 2n },
    }
    const { rerender } = render(
      <ForgeValueSetDisplay
        title="Inputs"
        emptyLabel="No inputs"
        values={[first, second]}
      />,
    )
    const firstNode = screen.getByText('world object @ rev 1').parentElement
    const secondNode = screen.getByText('world object @ rev 2').parentElement

    rerender(
      <ForgeValueSetDisplay
        title="Inputs"
        emptyLabel="No inputs"
        values={[second, first]}
      />,
    )

    expect(screen.getByText('world object @ rev 1').parentElement).toBe(
      firstNode,
    )
    expect(screen.getByText('world object @ rev 2').parentElement).toBe(
      secondNode,
    )
  })

  it('keeps distinct binary encodings stable when they reorder', () => {
    const first = {
      name: 'binary',
      blockRef: { hash: { hash: Uint8Array.from([1, 23]) } },
    }
    const second = {
      name: 'binary',
      blockRef: { hash: { hash: Uint8Array.from([12, 3]) } },
    }
    const { rerender } = render(
      <ForgeValueSetDisplay
        title="Inputs"
        emptyLabel="No inputs"
        values={[first, second]}
      />,
    )
    const [firstNode, secondNode] = screen
      .getAllByText('block ref')
      .map((node) => node.parentElement)

    rerender(
      <ForgeValueSetDisplay
        title="Inputs"
        emptyLabel="No inputs"
        values={[second, first]}
      />,
    )

    const [reorderedSecondNode, reorderedFirstNode] = screen
      .getAllByText('block ref')
      .map((node) => node.parentElement)
    expect(reorderedFirstNode).toBe(firstNode)
    expect(reorderedSecondNode).toBe(secondNode)
  })

  it('renders exact duplicate values without duplicate React keys', () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    const duplicate = {
      name: 'duplicate',
      worldObjectSnapshot: { key: 'forge/task/one', rev: 1n },
    }
    render(
      <ForgeValueSetDisplay
        title="Inputs"
        emptyLabel="No inputs"
        values={[duplicate, duplicate]}
      />,
    )

    expect(screen.getAllByText('duplicate')).toHaveLength(2)
    expect(consoleError).not.toHaveBeenCalled()
  })

  it('distinguishes semantic values with the same display name', () => {
    render(
      <ForgeValueSetDisplay
        title="Inputs"
        emptyLabel="No inputs"
        values={[
          {
            name: 'artifact',
            worldObjectSnapshot: { key: 'forge/task/one', rev: 1n },
          },
          {
            name: 'artifact',
            worldObjectSnapshot: { key: 'forge/task/one', rev: 2n },
          },
        ]}
      />,
    )

    expect(screen.getByText('world object @ rev 1')).toBeTruthy()
    expect(screen.getByText('world object @ rev 2')).toBeTruthy()
  })
})
