import React from 'react'
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'

import { resourceOf } from '@aptre/bldr-sdk/hooks/index.js'

import { ForgePassViewer } from './ForgePassViewer.js'

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
    },
    loading: false,
    error: null,
    retry: () => {},
  }),
}))

vi.mock('@s4wave/web/hooks/useEntitySet.js', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useEntitySet: () =>
    resourceOf({
      value: {
        seqno: 1n,
        keys: ['forge/execution/1'],
        rows: new Map<string, object>(
          (
            [
              [
                'forge/execution/1',
                {
                  peerId: '12D3KooWExec',
                  executionState: 2,
                  timestamp: new Date('2026-04-17T12:11:00Z'),
                  result: { success: true },
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
