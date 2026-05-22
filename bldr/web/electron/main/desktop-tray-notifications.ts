import {
  DesktopTrayEntryKind,
  DesktopTrayIconState,
  DesktopTraySeverity,
  type DesktopTrayEntry,
  type DesktopTrayState,
} from '@go/github.com/s4wave/spacewave/bldr/desktop/tray/tray.pb.js'
import { canInvokeDesktopTrayEntry } from './desktop-tray-panel-descriptor.js'

export interface DesktopTrayNotificationDecision {
  key: string
  title: string
  body: string
}

export function buildDesktopTrayNotificationDecision(
  previous: DesktopTrayState | undefined,
  next: DesktopTrayState,
): DesktopTrayNotificationDecision | undefined {
  const update = findEntry(next, 'apply-update')
  const previousUpdate =
    previous ? findEntry(previous, 'apply-update') : undefined
  if (
    update &&
    canInvokeDesktopTrayEntry(update) &&
    (!previousUpdate || !canInvokeDesktopTrayEntry(previousUpdate))
  ) {
    return {
      key: `update:${update.action?.value || update.label || 'ready'}`,
      title: 'Spacewave update ready',
      body:
        update.action?.value ?
          `Version ${update.action.value}`
        : 'Ready to install',
    }
  }

  const critical = firstCriticalAttention(next)
  const previousCritical =
    previous ? firstCriticalAttention(previous) : undefined
  if (
    critical &&
    previous?.iconState !== DesktopTrayIconState.ATTENTION &&
    critical.id !== previousCritical?.id
  ) {
    return {
      key: `attention:${critical.id || critical.label}`,
      title: critical.label || 'Spacewave needs attention',
      body:
        critical.detail ||
        critical.statusText ||
        next.statusText ||
        'Open Spacewave',
    }
  }

  return undefined
}

function findEntry(
  state: DesktopTrayState,
  id: string,
): DesktopTrayEntry | undefined {
  return (state.entries ?? []).find((entry) => entry.id === id)
}

function firstCriticalAttention(
  state: DesktopTrayState,
): DesktopTrayEntry | undefined {
  return (state.entries ?? []).find(
    (entry) =>
      entry.kind !== DesktopTrayEntryKind.SEPARATOR &&
      entry.severity === DesktopTraySeverity.CRITICAL,
  )
}
