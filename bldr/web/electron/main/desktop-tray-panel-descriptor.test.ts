import { describe, expect, it } from 'vitest'

import {
  DesktopTrayActionKind,
  DesktopTrayEntryKind,
  DesktopTrayIconState,
  DesktopTraySeverity,
  type DesktopTrayEntry,
  type DesktopTrayState,
} from '@go/github.com/s4wave/spacewave/bldr/desktop/tray/tray.pb.js'
import type { DesktopRuntimeState } from '../desktop-runtime/desktop-runtime.pb.js'
import {
  buildDesktopTrayPanelDescriptor,
  canInvokeDesktopTrayEntry,
} from './desktop-tray-panel-descriptor.js'

const longSpaceLabel =
  'Project Alpha With An Extremely Long Name That Must Be Preserved For Truncation'

describe('buildDesktopTrayPanelDescriptor', () => {
  it('adapts the watched tray tree into stable panel sections and actions', () => {
    const state = panelTrayState()

    const descriptor = buildDesktopTrayPanelDescriptor({
      appName: 'Spacewave',
      state,
    })

    expect(descriptor.statusText).toBe('Running')
    expect(descriptor.icon.variant).toBe('healthy')
    expect(descriptor.tabs).toEqual([
      { id: 'overview', label: 'Overview', enabled: true, count: 0 },
      { id: 'sessions', label: 'Sessions', enabled: true, count: 1 },
      { id: 'spaces', label: 'Spaces', enabled: true, count: 1 },
    ])
    expect(descriptor.primaryActions.map((row) => row.id)).toEqual([
      'open',
      'new-window',
      'apply-update',
    ])
    expect(descriptor.sections.map((section) => section.title)).toEqual([
      'Overview',
      'Status',
      'Sessions',
      'Spaces',
      'Activity',
      'Quick Actions',
    ])
    expect(descriptor.sessionRows[0]).toMatchObject({
      id: 'navigation-session',
      label: 'coolguy@spacewave.app',
      detail: 'Cloud',
      statusText: 'Ready',
      active: true,
      enabled: true,
    })
    expect(descriptor.actionRows.map((row) => row.id)).toContain('apply-update')
  })

  it('keeps empty and sparse states renderable without schema extensions', () => {
    const descriptor = buildDesktopTrayPanelDescriptor({
      state: {
        statusText: 'Starting',
        iconState: DesktopTrayIconState.NORMAL,
        entries: [
          status('title', 'Spacewave: Starting'),
          action('open', 'Open Spacewave', DesktopTrayActionKind.OPEN_ROUTE),
          section('Sessions'),
          status('Sessions-empty', 'No sessions'),
          section('Spaces'),
          status('Spaces-empty', 'No spaces'),
        ],
      },
    })

    expect(descriptor.tabs[1]).toMatchObject({ enabled: false, count: 0 })
    expect(descriptor.tabs[2]).toMatchObject({ enabled: false, count: 0 })
    expect(descriptor.cards.map((card) => card.value)).toContain('None')
    expect(descriptor.primaryActions.map((row) => row.id)).toEqual(['open'])
  })

  it.each([
    [DesktopTrayIconState.ACTIVE, 'Active', 'active'],
    [DesktopTrayIconState.ATTENTION, 'Needs attention', 'attention'],
    [DesktopTrayIconState.DISCONNECTED, 'Disconnected', 'disconnected'],
    [DesktopTrayIconState.QUITTING, 'Quitting', 'quitting'],
  ] as const)(
    'derives %s icon and header state from the watched tray snapshot',
    (iconState, statusText, variant) => {
      const descriptor = buildDesktopTrayPanelDescriptor({
        state: {
          statusText,
          iconState,
          entries: [status('title', `Spacewave: ${statusText}`)],
        },
      })

      expect(descriptor.statusText).toBe(statusText)
      expect(descriptor.icon.variant).toBe(variant)
    },
  )

  it('uses runtime state only as presentation enrichment', () => {
    const runtimeState: DesktopRuntimeState = {
      statusText: 'Running',
      sessions: [{ id: 'a' }, { id: 'b' }],
      spaces: [{ id: 'drive' }],
      activity: [{ id: 'sync' }],
      attentionItems: [],
    }

    const descriptor = buildDesktopTrayPanelDescriptor({
      state: panelTrayState(),
      runtimeState,
    })

    expect(descriptor.tabs[1]).toMatchObject({ count: 2 })
    expect(descriptor.tabs[2]).toMatchObject({ count: 1 })
    expect(descriptor.actionRows.every((row) => row.action?.id)).toBe(true)
  })

  it('preserves long labels and update-ready visual action state', () => {
    const descriptor = buildDesktopTrayPanelDescriptor({
      state: panelTrayState(),
    })

    expect(descriptor.spaceRows[0]).toMatchObject({
      id: 'navigation-space',
      label: longSpaceLabel,
      detail: 'Shared',
      enabled: true,
    })
    expect(descriptor.cards.find((card) => card.id === 'spaces')).toMatchObject(
      {
        detail: `${longSpaceLabel} - Shared`,
      },
    )
    expect(descriptor.primaryActions.at(-1)).toMatchObject({
      id: 'apply-update',
      label: 'Install Update',
      enabled: true,
      severity: 'info',
      action: {
        id: 'apply-update',
        kind: DesktopTrayActionKind.ATTACHED_HANDLER,
        value: '1.2.3',
      },
    })
  })

  it('uses DesktopTrayEntry action eligibility for panel and native parity', () => {
    expect(
      canInvokeDesktopTrayEntry(
        action('copy-empty', 'Copy', DesktopTrayActionKind.COPY_TEXT, {
          value: '',
        }),
      ),
    ).toBe(false)
    expect(
      canInvokeDesktopTrayEntry(
        action('copy-value', 'Copy', DesktopTrayActionKind.COPY_TEXT, {
          value: 'diagnostics',
        }),
      ),
    ).toBe(true)
    expect(
      canInvokeDesktopTrayEntry(
        action('disabled', 'Disabled', DesktopTrayActionKind.OPEN_ROUTE, {
          enabled: false,
        }),
      ),
    ).toBe(false)
  })
})

function panelTrayState(): DesktopTrayState {
  return {
    statusText: 'Running',
    iconState: DesktopTrayIconState.NORMAL,
    entries: [
      status('title', 'Spacewave: Running'),
      action('open', 'Open Spacewave', DesktopTrayActionKind.OPEN_ROUTE),
      action('new-window', 'New Window', DesktopTrayActionKind.NEW_WINDOW),
      section('Status'),
      status('status-runtime', 'CLI reachable', {
        detail: '1 CLI client connected',
      }),
      section('Sessions'),
      action(
        'navigation-session',
        'coolguy@spacewave.app',
        DesktopTrayActionKind.OPEN_ROUTE,
        {
          detail: 'Cloud',
          statusText: 'Ready',
          route: '/u/1/',
          active: true,
        },
      ),
      section('Spaces'),
      action(
        'navigation-space',
        longSpaceLabel,
        DesktopTrayActionKind.OPEN_ROUTE,
        {
          detail: 'Shared',
          route: '/u/1/so/project-alpha',
        },
      ),
      section('Activity'),
      status('activity-sync', 'Uploading changes', { detail: '2 sync items' }),
      section('Quick Actions'),
      action(
        'apply-update',
        'Install Update',
        DesktopTrayActionKind.ATTACHED_HANDLER,
        {
          value: '1.2.3',
          severity: DesktopTraySeverity.INFO,
        },
      ),
    ],
  }
}

function section(label: string): DesktopTrayEntry {
  return {
    id: `${label}-section`,
    kind: DesktopTrayEntryKind.SECTION,
    label,
  }
}

function status(
  id: string,
  label: string,
  opts?: Partial<DesktopTrayEntry>,
): DesktopTrayEntry {
  return {
    id,
    kind: DesktopTrayEntryKind.STATUS,
    label,
    ...opts,
  }
}

function action(
  id: string,
  label: string,
  kind: DesktopTrayActionKind,
  opts?: Partial<DesktopTrayEntry> & {
    route?: string
    value?: string
  },
): DesktopTrayEntry {
  return {
    id,
    kind: DesktopTrayEntryKind.ACTION,
    label,
    detail: opts?.detail,
    statusText: opts?.statusText,
    active: opts?.active,
    enabled: opts?.enabled ?? true,
    severity: opts?.severity,
    action: {
      kind,
      route: opts?.route,
      value: opts?.value,
    },
  }
}
