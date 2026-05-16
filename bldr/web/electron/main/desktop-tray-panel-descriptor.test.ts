import { describe, expect, it } from 'vitest'

import {
  DesktopTrayActionKind,
  DesktopTrayEntryKind,
  DesktopTrayIconState,
  DesktopTraySeverity,
  type DesktopTrayEntry,
  type DesktopTrayState,
} from '@go/github.com/s4wave/spacewave/bldr/desktop/tray/tray.pb.js'

import {
  buildTrayPanelDescriptor,
  canInvokeTrayPanelEntry,
} from './desktop-tray-panel-descriptor.js'

describe('buildTrayPanelDescriptor', () => {
  it('preserves watched row identity, ordering, grouping, and status hints', () => {
    const state: DesktopTrayState = {
      statusText: 'Needs attention',
      iconState: DesktopTrayIconState.ATTENTION,
      entries: [
        {
          id: 'title',
          kind: DesktopTrayEntryKind.STATUS,
          group: 'runtime',
          order: 0,
          label: 'Spacewave: Needs attention',
          statusText: 'attention',
          iconState: DesktopTrayIconState.ATTENTION,
          severity: DesktopTraySeverity.WARNING,
        },
        {
          id: 'open',
          kind: DesktopTrayEntryKind.ACTION,
          group: 'runtime',
          order: 1,
          label: 'Open Spacewave',
          enabled: true,
          action: {
            kind: DesktopTrayActionKind.OPEN_ROUTE,
            route: '/u/1/',
          },
        },
        {
          id: 'session-1',
          kind: DesktopTrayEntryKind.ACTION,
          path: ['Sessions'],
          group: 'navigation',
          order: 2,
          label: 'coolguy@spacewave.app',
          detail: 'Cloud',
          statusText: 'Ready',
          active: true,
          enabled: true,
          action: {
            kind: DesktopTrayActionKind.OPEN_ROUTE,
            route: '/u/1/',
          },
        },
      ],
    }

    const descriptor = buildTrayPanelDescriptor(state)

    expect(descriptor.statusText).toBe('Needs attention')
    expect(descriptor.iconState).toBe(DesktopTrayIconState.ATTENTION)
    expect(descriptor.rows.map((row) => row.id)).toEqual([
      'title',
      'open',
      'session-1',
    ])
    expect(descriptor.rows[0]).toMatchObject({
      position: 0,
      group: 'runtime',
      order: 0,
      statusText: 'attention',
      iconState: DesktopTrayIconState.ATTENTION,
      severity: DesktopTraySeverity.WARNING,
      actionEligible: false,
    })
    expect(descriptor.rows[1]).toMatchObject({
      position: 1,
      group: 'runtime',
      actionEligible: true,
      actionKind: DesktopTrayActionKind.OPEN_ROUTE,
    })
    expect(descriptor.rows[2]).toMatchObject({
      position: 2,
      path: ['Sessions'],
      group: 'navigation',
      active: true,
      enabled: true,
      detail: 'Cloud',
      statusText: 'Ready',
    })
    expect(
      descriptor.sections.map((section) => section.rows.map((row) => row.id)),
    ).toEqual([['title', 'open'], ['session-1']])
    expect(descriptor.sections[1]).toMatchObject({
      path: ['Sessions'],
      group: 'navigation',
    })
  })

  it('uses native tray action semantics for panel action eligibility', () => {
    expect(
      canInvokeTrayPanelEntry(
        actionEntry('open', DesktopTrayActionKind.OPEN_ROUTE),
      ),
    ).toBe(true)
    expect(
      canInvokeTrayPanelEntry(
        actionEntry('copy', DesktopTrayActionKind.COPY_TEXT, {
          value: 'diagnostics',
        }),
      ),
    ).toBe(true)
    expect(
      canInvokeTrayPanelEntry(
        actionEntry('copy-empty', DesktopTrayActionKind.COPY_TEXT),
      ),
    ).toBe(false)
    expect(
      canInvokeTrayPanelEntry({
        ...actionEntry('disabled', DesktopTrayActionKind.QUIT),
        enabled: false,
      }),
    ).toBe(false)
    expect(
      canInvokeTrayPanelEntry({
        id: 'status',
        kind: DesktopTrayEntryKind.STATUS,
        label: 'Runtime',
      }),
    ).toBe(false)
  })

  it('clones path and action data so descriptor rows are pure snapshots', () => {
    const entry: DesktopTrayEntry = {
      id: 'copy-diagnostics',
      kind: DesktopTrayEntryKind.ACTION,
      path: ['Quick Actions'],
      group: 'actions',
      label: 'Copy Diagnostics',
      enabled: true,
      action: {
        kind: DesktopTrayActionKind.COPY_TEXT,
        value: 'before',
      },
    }
    const descriptor = buildTrayPanelDescriptor({ entries: [entry] })

    entry.path?.push('Nested')
    if (entry.action) {
      entry.action.value = 'after'
    }

    expect(descriptor.rows[0]?.path).toEqual(['Quick Actions'])
    expect(descriptor.rows[0]?.action?.value).toBe('before')
  })
})

function actionEntry(
  id: string,
  kind: DesktopTrayActionKind,
  opts?: Pick<NonNullable<DesktopTrayEntry['action']>, 'value'>,
): DesktopTrayEntry {
  return {
    id,
    kind: DesktopTrayEntryKind.ACTION,
    label: id,
    enabled: true,
    action: {
      kind,
      value: opts?.value,
    },
  }
}
