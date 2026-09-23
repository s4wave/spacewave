import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import type { Space } from '@s4wave/sdk/space/space.js'
import {
  Execution,
  State,
} from '@go/github.com/s4wave/spacewave/forge/execution/execution.pb.js'

const mocks = vi.hoisted(() => ({
  context: null as unknown,
}))
vi.mock('@s4wave/web/contexts/SpaceContainerContext.js', () => ({
  SpaceContainerContext: { useContextSafe: () => mocks.context },
}))
vi.mock('./SpacePluginObjects.js', () => ({
  SpacePluginObjects: () => null,
}))

import { SpacePluginBuild } from './SpacePluginBuild.js'

afterEach(cleanup)

it('installs only a successful build and releases observation after completion', async () => {
  let complete: (() => void) | undefined
  let current: Execution = { executionState: State.ExecutionState_RUNNING }
  const releaseObject = vi.fn()
  const releaseCursor = vi.fn()
  const buildSpacePlugin = vi
    .fn()
    .mockResolvedValue({ executionKey: 'builds/one' })
  const addSpacePlugin = vi.fn().mockResolvedValue(undefined)
  const navigateToObjects = vi.fn()
  mocks.context = {
    navigateToObjects,
    spaceState: {
      worldContents: {
        objects: [
          { objectKey: 'projects/colors', objectType: 'unixfs/fs-node' },
          { objectKey: 'devices/local', objectType: 'spacewave/device' },
        ],
      },
    },
    spaceWorldResource: {
      value: {
        getObject: async () => ({
          [Symbol.dispose]: releaseObject,
          getRootRef: async () => ({ rev: 1n, rootRef: {} }),
          accessWorldState: async () => ({
            [Symbol.dispose]: releaseCursor,
            unmarshal: async () => ({
              found: true,
              data: Execution.toBinary(current),
            }),
          }),
          waitRev: (
            _revision: bigint,
            _ignoreMissing: boolean,
            signal: AbortSignal,
          ) =>
            new Promise<void>((resolve, reject) => {
              complete = resolve
              signal.addEventListener('abort', () => reject(signal.reason), {
                once: true,
              })
            }),
        }),
      },
      loading: false,
      error: null,
    },
  }
  const space = { buildSpacePlugin, addSpacePlugin } as unknown as Space
  const rendered = render(<SpacePluginBuild space={space} />)
  fireEvent.click(screen.getByText('Build a TypeScript plugin'))
  fireEvent.change(screen.getByLabelText('Source folder'), {
    target: { value: 'projects/colors' },
  })
  fireEvent.change(screen.getByLabelText('Build device'), {
    target: { value: 'devices/local' },
  })
  fireEvent.change(screen.getByLabelText('Plugin manifest ID'), {
    target: { value: 'space-colors' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Build' }))
  await waitFor(() =>
    expect(buildSpacePlugin).toHaveBeenCalledWith({
      sourceKey: 'projects/colors',
      deviceKey: 'devices/local',
      manifestId: 'space-colors',
    }),
  )
  await waitFor(() => expect(complete).toBeDefined())
  expect(
    screen.queryByRole('button', { name: 'Install in this Space' }),
  ).toBeNull()

  await act(async () => {
    current = {
      executionState: State.ExecutionState_COMPLETE,
      result: { success: true },
      valueSet: {
        outputs: [
          {
            name: 'manifest',
            worldObjectSnapshot: { key: 'plugins/colors/v1' },
          },
        ],
      },
    }
    complete?.()
  })
  fireEvent.click(
    await screen.findByRole('button', { name: 'Install in this Space' }),
  )
  await waitFor(() =>
    expect(addSpacePlugin).toHaveBeenCalledWith(
      'space-colors',
      'plugins/colors/v1',
    ),
  )
  expect(
    await screen.findByText(
      'Plugin added. Its status appears in the installed list.',
    ),
  ).toBeDefined()
  rendered.unmount()
  expect(releaseObject).toHaveBeenCalledOnce()
  expect(releaseCursor).toHaveBeenCalledTimes(2)
})
