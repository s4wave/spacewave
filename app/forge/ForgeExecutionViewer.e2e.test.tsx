import { describe, expect, it, vi } from 'vitest'
import { page } from 'vitest/browser'
import { render } from 'vitest-browser-react'

import {
  Execution,
  State,
} from '@go/github.com/s4wave/spacewave/forge/execution/execution.pb.js'

import { ForgeExecutionViewer } from './ForgeExecutionViewer.js'

// block holds the Execution the mocked block hook returns, set per state.
const block = vi.hoisted(() => ({ value: null as unknown }))

vi.mock('@s4wave/web/forge/useForgeBlockData.js', () => ({
  useForgeBlock: () => ({
    value: block.value,
    loading: false,
    error: null,
    retry: () => {},
  }),
}))

const worker = 'forge/worker/forge'

const cardWait = {
  reason: {
    case: 'capacity' as const,
    value: {
      workerObjectKey: worker,
      request: { milliCpu: 4000n, backend: 'docker', cards: { count: 1 } },
    },
  },
}

const log = [{ level: 'info', message: 'pulling rocm/pytorch' }]

// STATES are the CardAdmission spec's Execution states, plus the plugin wait
// that shares the header line, each with the text the viewer must show.
const STATES: [string, Partial<Execution>, string][] = [
  ['starting', { executionState: State.ExecutionState_PENDING }, 'PENDING'],
  [
    'waiting-card',
    { executionState: State.ExecutionState_RUNNING, waiting: cardWait },
    `Waiting for a graphics card on ${worker}`,
  ],
  [
    'running',
    { executionState: State.ExecutionState_RUNNING, logEntries: log },
    'pulling rocm/pytorch',
  ],
  [
    'complete',
    {
      executionState: State.ExecutionState_COMPLETE,
      result: { success: true },
      logEntries: log,
    },
    'COMPLETE',
  ],
  [
    'refused',
    {
      executionState: State.ExecutionState_COMPLETE,
      result: {
        failError:
          'reserve docker capacity: requests 2 graphics cards, Worker offers 1: request exceeds worker capacity',
      },
    },
    'requests 2 graphics cards, Worker offers 1',
  ],
  [
    'waiting-plugin',
    {
      executionState: State.ExecutionState_RUNNING,
      waiting: { reason: { case: 'pluginId', value: 'forge-train' } },
    },
    'Waiting for plugin forge-train to load',
  ],
]

describe('ForgeExecutionViewer states', () => {
  it.each(STATES)('shows %s', async (name, execution, text) => {
    block.value = Execution.create({
      peerId: '12D3KooWExec',
      timestamp: new Date('2026-11-26T12:00:00Z'),
      ...execution,
    })
    render(
      <div className="h-120">
        <ForgeExecutionViewer
          objectInfo={{
            info: {
              case: 'worldObjectInfo',
              value: {
                objectKey: 'forge/execution/train',
                objectType: 'forge/execution',
              },
            },
          }}
          worldState={{
            value: {} as never,
            loading: false,
            error: null,
            retry: () => {},
          }}
          objectState={{} as never}
        />
      </div>,
    )
    await expect
      .element(page.getByText(text, { exact: false }).first())
      .toBeVisible()
    await page.screenshot({
      path: `__screenshots__/forge-execution-states/${name}.png`,
    })
  })
})
