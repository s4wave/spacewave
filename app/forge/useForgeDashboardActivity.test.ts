import { describe, expect, it, vi } from 'vitest'

import {
  Job,
  State as JobState,
} from '@go/github.com/s4wave/spacewave/forge/job/job.pb.js'
import { ForgeDashboard } from '@s4wave/core/forge/dashboard/dashboard.pb.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import { loadForgeDashboardActivityEntries } from './useForgeDashboardActivity.js'

function disposable<T extends object>(value: T): T & Disposable {
  return Object.assign(value, {
    [Symbol.dispose]() {},
  })
}

describe('loadForgeDashboardActivityEntries', () => {
  it('does not let dormant AgentInstance or old Evidence history block first paint activity', async () => {
    const signal = new AbortController().signal
    const dashboard: ForgeDashboard = {
      name: 'Workfront',
      createdAt: new Date('2026-05-11T12:00:00Z'),
    }
    const jobData = Job.toBinary({
      jobState: JobState.JobState_RUNNING,
      timestamp: new Date('2026-05-11T12:01:00Z'),
    })
    const world = {
      getObject: vi.fn((objectKey: string) => {
        if (objectKey !== 'forge/job/active') {
          throw new Error(`unexpected hydration for ${objectKey}`)
        }
        return Promise.resolve(
          disposable({
            accessWorldState: vi.fn(() =>
              Promise.resolve(
                disposable({
                  unmarshal: vi.fn(() =>
                    Promise.resolve({ found: true, data: jobData }),
                  ),
                }),
              ),
            ),
          }),
        )
      }),
    } as unknown as IWorldState

    const entries = await loadForgeDashboardActivityEntries(
      world,
      dashboard,
      [
        {
          objectKey: 'glados/dogfood/workfront/job/task/agent-instance/dormant',
          typeId: 'glados/agent-instance',
        },
        {
          objectKey: 'glados/dogfood/workfront/evidence/old-history',
          typeId: 'glados/evidence',
        },
        {
          objectKey: 'forge/job/active',
          typeId: 'forge/job',
        },
      ],
      signal,
    )

    expect(world.getObject).toHaveBeenCalledTimes(1)
    expect(world.getObject).toHaveBeenCalledWith('forge/job/active', signal)
    expect(entries.map((entry) => entry.id)).toEqual([
      'forge/job/active:snapshot',
      'dashboard-created',
    ])
  })
})
