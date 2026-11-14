import { LuActivity, LuListTodo, LuPlay } from 'react-icons/lu'

import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  forgeBlockStatus,
  ForgeViewerShell,
  type ForgeViewerTab,
} from '@s4wave/web/forge/ForgeViewerShell.js'
import { useParams } from '@s4wave/web/router/router.js'
import { FactRow } from '@s4wave/web/ui/FactList.js'
import { FigureStrip } from '@s4wave/web/ui/FigureStrip.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'

import { ExecutionLog } from '@s4wave/app/forge/ForgeExecutionViewer.js'

// noop stands in for a Retry press.
const noop = () => {}

// blocks are a task block in each read state that hides the tabs; the
// viewer's own forgeBlockStatus turns each into what the shell shows.
const blocks: Record<string, Resource<unknown>> = {
  waiting: { value: null, loading: true, error: null, retry: noop },
  failed: {
    value: null,
    loading: false,
    error: new Error('block not found'),
    retry: noop,
  },
  empty: { value: null, loading: false, error: null, retry: noop },
}

// taskStateLabels name the task states the fixture shows.
const taskStateLabels: Record<number, string> = { 2: 'RUNNING' }

// facts are the task identity Details lists.
const facts = (
  <>
    <FactRow kind="text" label="Name" value="Publish artifact" />
    <FactRow label="Assigned To" value="12D3KooWQf8bJ3rXk2p9mVdE7sTn4yHc6L" />
    <FactRow kind="text" label="Created" value="2026-11-14T09:41:00.000Z" />
    <FactRow label="Object Key" value="forge/task/publish" />
  </>
)

// taskTabs are a Task viewer's tabs with representative content.
const taskTabs: ForgeViewerTab[] = [
  {
    id: 'overview',
    label: 'Overview',
    content: (
      <div className="space-y-3">
        <FigureStrip
          figures={[
            { label: 'Passes', value: 3, detail: 'Current nonce 3' },
            { label: 'Task outputs', value: 1, detail: 'Success' },
          ]}
        />
        <InfoCard
          icon={
            <LuActivity
              aria-hidden
              className="text-foreground-alt/60 size-3.5"
            />
          }
          title="Current Execution"
        >
          <div className="text-foreground text-sm font-medium">Pass #3</div>
        </InfoCard>
      </div>
    ),
  },
  { id: 'passes', label: 'Pass History', content: null },
  { id: 'outputs', label: 'Outputs', content: null },
]

// logEntries are an execution's log, long enough to scroll.
const logEntries = Array.from({ length: 60 }, (_, i) =>
  i % 9 === 4
    ? {
        timestamp: new Date(2026, 10, 14, 9, 41, i),
        level: 'warn',
        message: `retrying fetch of layer ${i}`,
      }
    : {
        timestamp: new Date(2026, 10, 14, 9, 41, i),
        level: 'info',
        message: `step ${i}: compiled module ${i} in 12ms`,
      },
)

// logTabs are an Execution viewer's tabs, its log leading as a stream.
const logTabs = (empty: boolean): ForgeViewerTab[] => [
  {
    id: 'logs',
    label: 'Logs',
    stream: true,
    content: <ExecutionLog logEntries={empty ? [] : logEntries} />,
  },
  { id: 'inputs', label: 'Inputs', content: null },
  { id: 'outputs', label: 'Outputs', content: null },
  { id: 'summary', label: 'Summary', content: null },
]

/**
 * ForgeViewerStatesDebug renders the Forge viewer shell in one fixture state,
 * with the viewport as the pane, for the viewer sweep: overview (a task's
 * tabs), details (its Details open), waiting, failed and empty (the block
 * not yet readable), log (an execution's log as a stream) and log-empty
 * (an execution with no output yet). An unknown or missing state shows
 * overview.
 */
export function ForgeViewerStatesDebug() {
  // Read the fixture state from the route.
  const { state = 'overview' } = useParams()

  // The execution's log fills the pane under its tabs.
  if (state === 'log' || state === 'log-empty') {
    return (
      <div className="viewer-pane h-full w-full">
        <ForgeViewerShell
          stateKey="debug/forge/log"
          icon={<LuPlay className="size-4" />}
          title="Execution"
          state={2}
          stateLabels={{ 2: 'RUNNING' }}
          details={{ label: 'Execution', facts }}
          tabs={logTabs(state === 'log-empty')}
        />
      </div>
    )
  }

  // A task in a block state, or readable with its tabs.
  const block = blocks[state]
  const status = block ? forgeBlockStatus('the task', block) : null
  return (
    <div className="viewer-pane h-full w-full">
      <ForgeViewerShell
        stateKey="debug/forge/task"
        icon={<LuListTodo className="size-4" />}
        title={status ? 'Task' : 'Publish artifact'}
        state={status ? undefined : 2}
        stateLabels={taskStateLabels}
        details={{ label: 'Task', facts, defaultOpen: state === 'details' }}
        status={status}
        tabs={taskTabs}
      />
    </div>
  )
}
