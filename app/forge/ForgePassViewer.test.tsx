import React from 'react'
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'

import { ForgePassViewer } from './ForgePassViewer.js'

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
    peerId: '12D3KooWPass',
    passNonce: 4n,
    replicas: 2,
    passState: 3,
    timestamp: new Date('2026-04-17T12:10:00Z'),
    valueSet: {
      inputs: [
        {
          name: 'source',
        },
      ],
      outputs: [
        {
          name: 'artifact',
        },
      ],
    },
    execStates: [{}, {}],
    result: {
      failError: 'validation mismatch',
    },
  }),
}))

vi.mock('@s4wave/web/forge/useForgeLinkedEntities.js', () => ({
  useForgeLinkedEntities: () => ({
    entities: [
      {
        objectKey: 'forge/execution/1',
        typeId: 'forge/execution',
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
          objectKey: 'forge/execution/1',
          typeId: 'forge/execution',
        },
        data: {
          peerId: '12D3KooWExec',
          executionState: 2,
          timestamp: new Date('2026-04-17T12:11:00Z'),
          result: {
            success: true,
          },
        },
      },
    ],
    loading: false,
  }),
}))

describe('ForgePassViewer', () => {
  it('renders pass details and decoded execution entries', () => {
    render(
      <ForgePassViewer
        objectInfo={{
          info: {
            case: 'worldObjectInfo',
            value: {
              objectKey: 'forge/pass/main',
              objectType: 'forge/pass',
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

    expect(screen.getByText('Pass Nonce')).toBeTruthy()
    expect(screen.getAllByText('validation mismatch').length).toBeGreaterThan(0)
    expect(screen.getByText('forge/execution/1')).toBeTruthy()
    expect(screen.getByText('12D3KooWExec')).toBeTruthy()
    expect(screen.getByText('source')).toBeTruthy()
    expect(screen.getByText('artifact')).toBeTruthy()
  })
})
