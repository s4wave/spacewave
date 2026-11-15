import React from 'react'
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'

import { resourceOf } from '@aptre/bldr-sdk/hooks/index.js'

import { ForgeTaskViewer } from './ForgeTaskViewer.js'

vi.mock('@s4wave/web/forge/ForgeViewerShell.js', () => ({
  forgeBlockStatus: () => null,
  ForgeViewerShell: ({
    details,
    tabs,
  }: {
    details?: { facts: React.ReactNode }
    tabs?: Array<{ id: string; content: React.ReactNode }>
  }) => (
    <div data-testid="forge-viewer-shell">
      {details?.facts}
      {tabs?.map((tab) => (
        <div key={tab.id}>{tab.content}</div>
      ))}
    </div>
  ),
}))

vi.mock('@s4wave/web/forge/useForgeBlockData.js', () => ({
  useForgeBlock: () => ({
    value: {
      name: 'Publish artifact',
      peerId: '12D3KooWTask',
      passNonce: 2n,
      taskState: 2,
      timestamp: new Date('2026-04-17T12:00:00Z'),
      valueSet: {
        outputs: [
          {
            name: 'bundle',
          },
        ],
      },
      result: {
        success: true,
      },
    },
    loading: false,
    error: null,
    retry: () => {},
  }),
}))

// The pass set lists the older pass first, so the viewer's newest-first
// order is its own.
vi.mock('@s4wave/web/hooks/useEntitySet.js', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useEntitySet: () =>
    resourceOf({
      value: {
        seqno: 1n,
        keys: ['forge/pass/1', 'forge/pass/2'],
        rows: new Map<string, object>(
          (
            [
              [
                'forge/pass/1',
                {
                  passNonce: 1n,
                  passState: 4,
                  replicas: 1,
                  timestamp: new Date('2026-04-17T12:02:00Z'),
                  execStates: [],
                  result: { failError: 'cache miss' },
                },
              ],
              [
                'forge/pass/2',
                {
                  passNonce: 2n,
                  passState: 2,
                  replicas: 1,
                  timestamp: new Date('2026-04-17T12:05:00Z'),
                  execStates: [
                    {
                      objectKey: 'forge/execution/current',
                      executionState: 2,
                      peerId: '12D3KooWExec',
                      timestamp: new Date('2026-04-17T12:05:05Z'),
                    },
                  ],
                },
              ],
            ] as [string, object][]
          ).map(([key, value]) => [key, { value }]),
        ),
        complete: true,
      },
      loading: false,
      error: null,
      retry: () => {},
    }),
}))

describe('ForgeTaskViewer', () => {
  it('renders current execution detail and task outputs', () => {
    render(
      <ForgeTaskViewer
        objectInfo={{
          info: {
            case: 'worldObjectInfo',
            value: {
              objectKey: 'forge/task/main',
              objectType: 'forge/task',
            },
          },
        }}
        worldState={{
          value: {} as never,
          loading: false,
          error: null,
          retry: vi.fn(),
        }}
        objectState={{} as never}
      />,
    )

    expect(screen.getByText('Current Execution')).toBeTruthy()
    expect(screen.getByText('forge/execution/current')).toBeTruthy()
    expect(screen.getByText('12D3KooWExec')).toBeTruthy()
    expect(screen.getByText('Task outputs')).toBeTruthy()
    expect(screen.getByText('bundle')).toBeTruthy()
    expect(screen.getAllByText('Success').length).toBeGreaterThan(0)
    expect(screen.getByText('cache miss')).toBeTruthy()
  })
})
