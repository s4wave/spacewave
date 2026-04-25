import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'

import { ForgeExecutionViewer } from './ForgeExecutionViewer.js'

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
    peerId: '12D3KooWExec',
    executionState: 2,
    timestamp: new Date('2026-04-17T12:00:00Z'),
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
    logEntries: [
      {
        level: 'info',
        message: 'started noop execution',
      },
    ],
  }),
}))

describe('ForgeExecutionViewer', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('renders inputs, outputs, logs, and a live duration', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-04-17T12:01:05Z'))

    render(
      <ForgeExecutionViewer
        objectInfo={{
          info: {
            case: 'worldObjectInfo',
            value: {
              objectKey: 'forge/execution/main',
              objectType: 'forge/execution',
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

    expect(screen.getByText('started noop execution')).toBeTruthy()
    expect(screen.getByText('source')).toBeTruthy()
    expect(screen.getByText('artifact')).toBeTruthy()
    expect(screen.getByText('Live duration')).toBeTruthy()
    expect(screen.getByText('1m 5s')).toBeTruthy()
    expect(screen.getByText('12D3KooWExec')).toBeTruthy()
  })
})
