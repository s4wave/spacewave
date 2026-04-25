import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { ForgeWorkerViewer } from './ForgeWorkerViewer.js'

const mockContents = {
  setProcessBinding: vi.fn().mockResolvedValue(undefined),
}

vi.mock('@aptre/bldr-sdk/hooks/useResource.js', () => ({
  useResourceValue: () => mockContents,
}))

vi.mock('@aptre/bldr-sdk/hooks/useStreamingResource.js', () => ({
  useStreamingResource: () => ({
    value: {
      processBindings: [
        {
          objectKey: 'forge/worker/main',
          typeId: 'forge/worker',
          approved: false,
        },
      ],
    },
    loading: false,
    error: null,
    retry: vi.fn(),
  }),
}))

vi.mock('@s4wave/web/contexts/contexts.js', () => ({
  SpaceContentsContext: {
    useContext: () => mockContents,
  },
}))

vi.mock('@s4wave/web/forge/ForgeViewerShell.js', () => ({
  ForgeViewerShell: ({
    tabs,
    actions,
  }: {
    tabs?: Array<{ id: string; content: React.ReactNode }>
    actions?: Array<{ label: string; onClick: () => void }>
  }) => (
    <div data-testid="forge-viewer-shell">
      {tabs?.map((tab) => (
        <div key={tab.id}>{tab.content}</div>
      ))}
      {actions?.map((action) => (
        <button key={action.label} type="button" onClick={action.onClick}>
          {action.label}
        </button>
      ))}
    </div>
  ),
}))

vi.mock('@s4wave/web/forge/useForgeBlockData.js', () => ({
  useForgeBlockData: () => ({
    name: 'session-worker',
  }),
}))

vi.mock('@s4wave/web/forge/useForgeLinkedEntities.js', () => ({
  useForgeLinkedEntities: () => ({
    entities: [
      {
        objectKey: 'forge/cluster/main',
        typeId: 'forge/cluster',
      },
    ],
    loading: false,
  }),
}))

vi.mock('./useForgeClusterSnapshot.js', () => ({
  useForgeClusterSnapshot: () => ({
    snapshot: {
      jobs: [],
      tasks: [],
      passes: [],
      executions: [
        {
          objectKey: 'forge/execution/active',
          clusterKey: 'forge/cluster/main',
          jobKey: 'forge/job/main',
          taskKey: 'forge/task/main',
          passKey: 'forge/pass/main',
          data: {
            peerId: '12D3KooWWorker',
            executionState: 2,
            timestamp: new Date('2026-04-17T12:05:00Z'),
          },
        },
        {
          objectKey: 'forge/execution/history',
          clusterKey: 'forge/cluster/main',
          jobKey: 'forge/job/main',
          taskKey: 'forge/task/main',
          passKey: 'forge/pass/prev',
          data: {
            peerId: '12D3KooWWorker',
            executionState: 3,
            timestamp: new Date('2026-04-17T12:00:00Z'),
            result: {
              success: true,
            },
          },
        },
      ],
      workers: [
        {
          objectKey: 'forge/worker/main',
          clusterKeys: ['forge/cluster/main'],
          keypairKeys: ['kp/12D3KooWWorker'],
          peerIds: ['12D3KooWWorker'],
          data: {
            name: 'session-worker',
          },
        },
      ],
    },
    loading: false,
  }),
}))

describe('ForgeWorkerViewer', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('renders assignments/history and toggles process binding', async () => {
    const user = userEvent.setup()

    render(
      <ForgeWorkerViewer
        objectInfo={{
          info: {
            case: 'worldObjectInfo',
            value: {
              objectKey: 'forge/worker/main',
              objectType: 'forge/worker',
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

    expect(screen.getByText('1/1')).toBeTruthy()
    expect(screen.getAllByText('12D3KooWWorker').length).toBeGreaterThan(0)
    expect(screen.getByText('forge/execution/active')).toBeTruthy()
    expect(screen.getByText('forge/execution/history')).toBeTruthy()

    await user.click(screen.getByRole('button', { name: /start worker/i }))

    expect(mockContents.setProcessBinding).toHaveBeenCalledWith(
      'forge/worker/main',
      'forge/worker',
      true,
    )
  })
})
