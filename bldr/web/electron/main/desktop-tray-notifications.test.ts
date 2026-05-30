import { describe, expect, it } from 'vitest'

import {
  DesktopTrayActionKind,
  DesktopTrayEntryKind,
  DesktopTrayIconState,
  DesktopTraySeverity,
  type DesktopTrayState,
} from '@go/github.com/s4wave/spacewave/bldr/desktop/tray/tray.pb.js'
import { buildDesktopTrayNotificationDecision } from './desktop-tray-notifications.js'

describe('desktop tray notification policy', () => {
  it('emits update-ready only on the transition into an invokable update', () => {
    const previous = state('Running', DesktopTrayIconState.NORMAL)
    const next = state('Update ready', DesktopTrayIconState.ATTENTION, true)

    expect(buildDesktopTrayNotificationDecision(previous, next)).toEqual({
      key: 'update:1.2.3',
      title: 'Spacewave update ready',
      body: 'Version 1.2.3',
    })
    expect(buildDesktopTrayNotificationDecision(next, next)).toBeUndefined()
  })

  it('emits critical attention after healthy state', () => {
    const previous = state('Running', DesktopTrayIconState.NORMAL)
    const next = state('Needs attention', DesktopTrayIconState.ATTENTION)
    next.entries?.push({
      id: 'attention-auth',
      kind: DesktopTrayEntryKind.STATUS,
      label: 'Sign in required',
      detail: 'coolguy@spacewave.app',
      severity: DesktopTraySeverity.CRITICAL,
    })

    expect(buildDesktopTrayNotificationDecision(previous, next)).toEqual({
      key: 'attention:attention-auth',
      title: 'Sign in required',
      body: 'coolguy@spacewave.app',
    })
  })

  it('stays quiet for disconnected and completion-only changes', () => {
    const previous = state('Running', DesktopTrayIconState.NORMAL)
    const disconnected = state(
      'Disconnected',
      DesktopTrayIconState.DISCONNECTED,
    )
    const completed = state('Sync complete', DesktopTrayIconState.NORMAL)
    completed.entries?.push({
      id: 'activity-done',
      kind: DesktopTrayEntryKind.STATUS,
      label: 'Sync complete',
    })

    expect(
      buildDesktopTrayNotificationDecision(previous, disconnected),
    ).toBeUndefined()
    expect(
      buildDesktopTrayNotificationDecision(previous, completed),
    ).toBeUndefined()
  })
})

function state(
  statusText: string,
  iconState: DesktopTrayIconState,
  updateReady = false,
): DesktopTrayState {
  return {
    statusText,
    iconState,
    entries: [
      {
        id: 'title',
        kind: DesktopTrayEntryKind.STATUS,
        label: `Spacewave: ${statusText}`,
      },
      ...(updateReady ?
        [
          {
            id: 'apply-update',
            kind: DesktopTrayEntryKind.ACTION,
            label: 'Install Update',
            enabled: true,
            action: {
              kind: DesktopTrayActionKind.ATTACHED_HANDLER,
              value: '1.2.3',
            },
          },
        ]
      : []),
    ],
  }
}
