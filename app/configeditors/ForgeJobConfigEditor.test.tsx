import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'

const clusterResource: {
  value: Array<{ key: string; name: string }>
  loading: boolean
  error: Error | null
  retry: ReturnType<typeof vi.fn>
} = {
  value: [{ key: 'cluster-1', name: 'Primary build cluster' }],
  loading: false,
  error: null,
  retry: vi.fn(),
}

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResource: () => clusterResource,
}))

vi.mock('@s4wave/web/contexts/SpaceContainerContext.js', () => ({
  SpaceContainerContext: {
    useContext: () => ({ spaceWorldResource: {} }),
  },
}))

import { ForgeJobCreateOp } from '@s4wave/core/forge/job/job.pb.js'
import { Cluster } from '@go/github.com/s4wave/spacewave/forge/cluster/cluster.pb.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import {
  ForgeJobConfigEditor,
  loadForgeClusterOptions,
} from './ForgeJobConfigEditor.js'

describe('loadForgeClusterOptions', () => {
  it('settles a one-cluster read with its display name and stable key', async () => {
    const release = vi.fn()
    const close = vi.fn()
    const world = {
      listObjectsWithType: vi.fn().mockResolvedValue(['cluster-1']),
      getObject: vi.fn().mockResolvedValue({
        accessWorldState: vi.fn().mockResolvedValue({
          unmarshal: vi.fn().mockResolvedValue({
            found: true,
            data: Cluster.toBinary({ name: 'Primary build cluster' }),
          }),
          [Symbol.dispose]: close,
        }),
        release,
      }),
    } as unknown as IWorldState

    await expect(
      loadForgeClusterOptions(world, new AbortController().signal),
    ).resolves.toEqual([{ key: 'cluster-1', name: 'Primary build cluster' }])
    expect(close).toHaveBeenCalledOnce()
    expect(release).toHaveBeenCalledOnce()
  })
})

describe('ForgeJobConfigEditor', () => {
  afterEach(() => {
    cleanup()
    clusterResource.value = [
      {
        key: 'cluster-1',
        name: 'Primary build cluster',
      },
    ]
    clusterResource.loading = false
    clusterResource.error = null
    clusterResource.retry.mockClear()
  })
  it('shows a loaded Cluster context and accepts a named initial task', async () => {
    const user = userEvent.setup()
    const onValueChange = vi.fn()
    function Harness() {
      const [value, setValue] = React.useState<ForgeJobCreateOp>({
        clusterKey: '',
        taskDefs: [],
      })
      return React.createElement(ForgeJobConfigEditor, {
        value,
        onValueChange: (next) => {
          onValueChange(next)
          setValue(next)
        },
      })
    }
    render(React.createElement(Harness))

    expect(screen.queryByText('Loading clusters…')).toBeNull()
    expect(screen.getByText('Primary build cluster')).toBeTruthy()
    expect(screen.getByText('cluster-1')).toBeTruthy()

    await user.click(
      screen.getByRole('button', { name: /primary build cluster/i }),
    )
    expect(onValueChange).toHaveBeenCalledWith({
      clusterKey: 'cluster-1',
      taskDefs: [],
    })

    await user.type(screen.getByLabelText('Task 1 name'), 'compile')
    expect(onValueChange).toHaveBeenLastCalledWith({
      clusterKey: 'cluster-1',
      taskDefs: [{ name: 'compile' }],
    })
  })

  it('distinguishes cluster loading and failure and retries the failure', async () => {
    const user = userEvent.setup()
    clusterResource.value = []
    clusterResource.loading = true
    const { rerender } = render(
      <ForgeJobConfigEditor
        value={{ clusterKey: '', taskDefs: [] }}
        onValueChange={vi.fn()}
      />,
    )

    expect(screen.getByText('Loading clusters…')).toBeTruthy()

    clusterResource.loading = false
    clusterResource.error = new Error('cluster graph unavailable')
    rerender(
      <ForgeJobConfigEditor
        value={{ clusterKey: '', taskDefs: [] }}
        onValueChange={vi.fn()}
      />,
    )
    expect(screen.getByText('Clusters unavailable')).toBeTruthy()
    expect(screen.queryByText(/No Clusters are available/)).toBeNull()

    await user.click(screen.getByRole('button', { name: /retry/i }))
    expect(clusterResource.retry).toHaveBeenCalledOnce()
  })

  it('preserves task row identity through proto round trips and first, middle, and last edits', async () => {
    const user = userEvent.setup()
    function ProtoHarness() {
      const [value, setValue] = React.useState<ForgeJobCreateOp>({
        clusterKey: 'cluster-1',
        taskDefs: [{ name: 'one' }, { name: 'two' }, { name: 'three' }],
      })
      return (
        <ForgeJobConfigEditor
          value={value}
          onValueChange={(next) => {
            const decoded = ForgeJobCreateOp.fromBinary(
              ForgeJobCreateOp.toBinary(next),
            )
            setValue(decoded)
          }}
        />
      )
    }
    render(<ProtoHarness />)

    const two = screen.getByLabelText('Task 2 name')
    const three = screen.getByLabelText('Task 3 name')
    three.focus()
    fireEvent.click(screen.getByRole('button', { name: 'Remove task 1' }))
    await waitFor(() => expect(screen.getByLabelText('Task 1 name')).toBe(two))
    expect(screen.getByLabelText('Task 2 name')).toBe(three)
    expect(document.activeElement).toBe(three)

    await user.click(screen.getByRole('button', { name: 'Add Task' }))
    await waitFor(() => expect(screen.getByLabelText('Task 1 name')).toBe(two))
    expect(screen.getByLabelText('Task 2 name')).toBe(three)
    const added = screen.getByLabelText('Task 3 name')

    two.focus()
    fireEvent.click(screen.getByRole('button', { name: 'Remove task 2' }))
    await waitFor(() => expect(screen.getByLabelText('Task 1 name')).toBe(two))
    expect(screen.getByLabelText('Task 2 name')).toBe(added)
    expect(document.activeElement).toBe(two)

    await user.click(screen.getByRole('button', { name: 'Add Task' }))
    const last = screen.getByLabelText('Task 3 name')
    two.focus()
    fireEvent.click(screen.getByRole('button', { name: 'Remove task 3' }))
    await waitFor(() =>
      expect(screen.queryByLabelText('Task 3 name')).toBeNull(),
    )
    expect(screen.getByLabelText('Task 1 name')).toBe(two)
    expect(document.activeElement).toBe(two)
    expect(last.isConnected).toBe(false)
  })
})
