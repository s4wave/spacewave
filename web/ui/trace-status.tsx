import {
  LuCircle,
  LuCircleCheck,
  LuCircleDot,
  LuCircleX,
} from '@s4wave/web/images/icons.js'

import type { TraceStatus } from '@s4wave/web/ui/trace.js'

const statusTitles: Record<TraceStatus, string> = {
  running: 'Running',
  done: 'Ran',
  error: 'Failed to run',
  pending: 'Pending',
}

const statusLabels: Record<TraceStatus, string> = {
  running: 'Running',
  done: 'Done',
  error: 'Failed',
  pending: 'Pending',
}

/** statusTitle names a run from its status and name. */
export function statusTitle(status: TraceStatus, name: string) {
  return `${statusTitles[status]} ${name}`
}

/** statusLabel is the screen-reader name of a trace status. */
export function statusLabel(status: TraceStatus) {
  return statusLabels[status]
}

/** stepIcon is the status mark for a plan step or a queue item. */
export function stepIcon(status: TraceStatus) {
  if (status === 'done') {
    return <LuCircleCheck />
  }
  if (status === 'running') {
    return <LuCircleDot />
  }
  if (status === 'error') {
    return <LuCircleX />
  }
  return <LuCircle />
}
