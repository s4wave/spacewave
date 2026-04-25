import React from 'react'
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'

import { ForgeTaskViewer } from './ForgeTaskViewer.js'

vi.mock('@s4wave/web/forge/ForgeViewerShell.js', () => ({
  ForgeViewerShell: ({
    tabs,
  }: {
    tabs?: Array<{ id: string; content: React.ReactNode }>
  }) => (
    <div data-testid="forge-viewer-shell">
      {tabs?.map((tab) => (
        <div key={tab.id}>{tab.content}</div>
      ))}
    </div>
  ),
}))

vi.mock('@s4wave/web/forge/useForgeBlockData.js', () => ({
  useForgeBlockData: () => ({
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
  }),
}))

vi.mock('@s4wave/web/forge/useForgeLinkedEntities.js', () => ({
  useForgeLinkedEntities: () => ({
    entities: [
      {
        objectKey: 'forge/pass/2',
        typeId: 'forge/pass',
      },
      {
        objectKey: 'forge/pass/1',
        typeId: 'forge/pass',
      },
    ],
    loading: false,
  }),
}))

vi.mock('./useForgeDecodedLinkedEntities.js', () => ({
  useForgeDecodedLinkedEntities: () => ({
    items: [
      {
        entity: {
          objectKey: 'forge/pass/2',
          typeId: 'forge/pass',
        },
        data: {
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
      },
      {
        entity: {
          objectKey: 'forge/pass/1',
          typeId: 'forge/pass',
        },
        data: {
          passNonce: 1n,
          passState: 4,
          replicas: 1,
          timestamp: new Date('2026-04-17T12:02:00Z'),
          execStates: [],
          result: {
            failError: 'cache miss',
          },
        },
      },
    ],
    loading: false,
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
    expect(screen.getByText('Task Outputs')).toBeTruthy()
    expect(screen.getByText('bundle')).toBeTruthy()
    expect(screen.getAllByText('Success').length).toBeGreaterThan(0)
    expect(screen.getByText('cache miss')).toBeTruthy()
  })
})
