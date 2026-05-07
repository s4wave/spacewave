import {
  DesktopTrayActionKind,
  DesktopTrayEntryKind,
  DesktopTrayIconState,
  DesktopTraySeverity,
  type DesktopTrayAction,
  type DesktopTrayEntry,
} from '@go/github.com/s4wave/spacewave/bldr/desktop/tray/tray.pb.js'
import {
  DesktopRuntimeActionKind,
  DesktopRuntimeHealth,
  DesktopRuntimeSeverity,
  type DesktopRuntimeActionItem,
  type DesktopRuntimeActivityItem,
  type DesktopRuntimeAttentionItem,
  type DesktopRuntimeNavigationItem,
  type DesktopRuntimeState,
} from '../desktop-runtime/desktop-runtime.pb.js'

export function buildDesktopTrayEntriesFromRuntimeState(
  state: DesktopRuntimeState,
): DesktopTrayEntry[] {
  if (state.attentionItems?.length) {
    return buildAttentionEntries(state)
  }
  return buildHealthyEntries(state)
}

function buildHealthyEntries(state: DesktopRuntimeState): DesktopTrayEntry[] {
  return [
    statusEntry('title', `Spacewave: ${state.statusText || 'Running'}`),
    separatorEntry('open-separator'),
    actionEntry('open', 'Open Spacewave', DesktopTrayActionKind.OPEN_ROUTE),
    actionEntry('new-window', 'New Window', DesktopTrayActionKind.NEW_WINDOW),
    separatorEntry('status-separator'),
    ...buildStatusSection(state),
    ...buildNavigationSection('Sessions', state.sessions, 'No sessions'),
    ...buildNavigationSection('Spaces', state.spaces, 'No spaces'),
    ...buildActivitySection(state.activity),
    ...buildActionSection(state),
    separatorEntry('app-separator'),
    actionEntry('settings', 'Settings...', DesktopTrayActionKind.OPEN_ROUTE, {
      route: '/settings',
    }),
    actionEntry('about', 'About Spacewave', DesktopTrayActionKind.OPEN_ROUTE, {
      route: '/about',
    }),
    separatorEntry('quit-separator'),
    actionEntry('quit', 'Quit', DesktopTrayActionKind.QUIT),
  ]
}

function buildAttentionEntries(state: DesktopRuntimeState): DesktopTrayEntry[] {
  const item = selectPrimaryAttentionItem(state.attentionItems)
  return [
    statusEntry(
      'title',
      `Spacewave: ${state.statusText || 'Needs attention'}`,
      { iconState: iconStateForRuntimeHealth(state.health) },
    ),
    statusEntry('attention-primary', item?.label || 'Needs attention', {
      severity: severityFromRuntimeSeverity(item?.severity),
    }),
    ...(item?.detail ? [statusEntry('attention-detail', item.detail)] : []),
    separatorEntry('open-separator'),
    actionEntry('open', 'Open Spacewave', DesktopTrayActionKind.OPEN_ROUTE),
    separatorEntry('quit-separator'),
    actionEntry('quit', 'Quit', DesktopTrayActionKind.QUIT),
  ]
}

function buildStatusSection(state: DesktopRuntimeState): DesktopTrayEntry[] {
  const listener = state.listener
  const rows = [
    sectionEntry('status-section', 'Status'),
    statusEntry(
      'status-runtime',
      compactLabel([
        listener?.label || 'Runtime',
        listener?.detail || state.statusText || 'Running',
      ]),
    ),
  ]
  if (state.update?.label) {
    rows.push(
      statusEntry(
        'status-update',
        compactLabel(['Update', state.update.label, state.update.detail]),
      ),
    )
  }
  return rows
}

function buildNavigationSection(
  title: string,
  items: DesktopRuntimeNavigationItem[] | undefined,
  emptyLabel: string,
): DesktopTrayEntry[] {
  return [
    separatorEntry(`${title}-separator`),
    sectionEntry(`${title}-section`, title),
    ...nonEmpty(items).map((item) => buildNavigationItem(item)),
    ...(items?.length ? [] : [statusEntry(`${title}-empty`, emptyLabel)]),
  ]
}

function buildActivitySection(
  items: DesktopRuntimeActivityItem[] | undefined,
): DesktopTrayEntry[] {
  return [
    separatorEntry('activity-separator'),
    sectionEntry('activity-section', 'Activity'),
    ...nonEmpty(items).map((item) =>
      statusEntry(
        `activity-${item.id || compactLabel([item.label, item.detail])}`,
        compactLabel([item.label, item.detail]),
      ),
    ),
    ...(items?.length ?
      []
    : [statusEntry('activity-empty', 'No recent activity')]),
  ]
}

function buildActionSection(state: DesktopRuntimeState): DesktopTrayEntry[] {
  const items = [...buildSyntheticActions(state), ...nonEmpty(state.actions)]
  return [
    separatorEntry('quick-actions-separator'),
    sectionEntry('quick-actions-section', 'Quick Actions'),
    ...items.map((item) => buildActionItem(item)),
    ...(items.length ?
      []
    : [statusEntry('quick-actions-empty', 'No quick actions')]),
  ]
}

function buildNavigationItem(
  item: DesktopRuntimeNavigationItem,
): DesktopTrayEntry {
  const label = compactLabel([item.label, item.detail, item.statusText])
  if (!item.route) {
    return statusEntry(`navigation-${item.id || label}`, label, {
      active: item.active,
    })
  }
  return actionEntry(
    `navigation-${item.id || label}`,
    label,
    DesktopTrayActionKind.OPEN_ROUTE,
    { active: item.active, route: item.route },
  )
}

function buildSyntheticActions(
  state: DesktopRuntimeState,
): DesktopRuntimeActionItem[] {
  const socketPath = state.listener?.socketPath
  if (!socketPath) {
    return []
  }
  return [
    {
      id: 'copy-cli-socket',
      kind: DesktopRuntimeActionKind.COPY_TEXT,
      label: 'Copy CLI Socket',
      value: socketPath,
      enabled: true,
    },
    {
      id: 'copy-diagnostics',
      kind: DesktopRuntimeActionKind.COPY_TEXT,
      label: 'Copy Diagnostics',
      value: buildDiagnosticText(state),
      enabled: true,
    },
  ]
}

function buildActionItem(item: DesktopRuntimeActionItem): DesktopTrayEntry {
  const label = compactLabel([item.label, item.detail])
  return actionEntry(
    `action-${item.id || label}`,
    label,
    actionKindFromRuntimeActionKind(item.kind),
    {
      enabled: item.enabled,
      route: item.route,
      value: item.value,
    },
  )
}

function statusEntry(
  id: string,
  label: string,
  opts?: Partial<Pick<DesktopTrayEntry, 'active' | 'iconState' | 'severity'>>,
): DesktopTrayEntry {
  return {
    id,
    kind: DesktopTrayEntryKind.STATUS,
    label,
    active: opts?.active ?? false,
    iconState: opts?.iconState ?? DesktopTrayIconState.UNSPECIFIED,
    severity: opts?.severity ?? DesktopTraySeverity.UNSPECIFIED,
  }
}

function sectionEntry(id: string, label: string): DesktopTrayEntry {
  return {
    id,
    kind: DesktopTrayEntryKind.SECTION,
    label,
  }
}

function separatorEntry(id: string): DesktopTrayEntry {
  return {
    id,
    kind: DesktopTrayEntryKind.SEPARATOR,
  }
}

function actionEntry(
  id: string,
  label: string,
  kind: DesktopTrayActionKind,
  opts?: Partial<Pick<DesktopTrayEntry, 'active' | 'enabled'>> &
    Partial<Pick<DesktopTrayAction, 'route' | 'value'>>,
): DesktopTrayEntry {
  return {
    id,
    kind: DesktopTrayEntryKind.ACTION,
    label,
    active: opts?.active ?? false,
    enabled: opts?.enabled ?? true,
    action: {
      kind,
      route: opts?.route,
      value: opts?.value,
    },
  }
}

function compactLabel(parts: Array<string | undefined>): string {
  return parts.filter((part) => part && part.trim() !== '').join(' - ')
}

function nonEmpty<T>(items: T[] | undefined): T[] {
  return items ?? []
}

function selectPrimaryAttentionItem(
  items: DesktopRuntimeAttentionItem[] | undefined,
): DesktopRuntimeAttentionItem | undefined {
  return [...nonEmpty(items)].sort((a, b) => {
    const severity = severityPriority(b.severity) - severityPriority(a.severity)
    if (severity !== 0) {
      return severity
    }
    return (a.label ?? '').localeCompare(b.label ?? '')
  })[0]
}

function severityPriority(
  severity: DesktopRuntimeSeverity | undefined,
): number {
  return severity ?? DesktopRuntimeSeverity.INFO
}

function severityFromRuntimeSeverity(
  severity: DesktopRuntimeSeverity | undefined,
): DesktopTraySeverity {
  switch (severity) {
    case DesktopRuntimeSeverity.CRITICAL:
      return DesktopTraySeverity.CRITICAL
    case DesktopRuntimeSeverity.WARNING:
      return DesktopTraySeverity.WARNING
    case DesktopRuntimeSeverity.INFO:
      return DesktopTraySeverity.INFO
    default:
      return DesktopTraySeverity.UNSPECIFIED
  }
}

export function iconStateForRuntimeHealth(
  health: DesktopRuntimeHealth | undefined,
): DesktopTrayIconState {
  switch (health) {
    case DesktopRuntimeHealth.ACTIVE:
      return DesktopTrayIconState.ACTIVE
    case DesktopRuntimeHealth.NEEDS_ATTENTION:
      return DesktopTrayIconState.ATTENTION
    case DesktopRuntimeHealth.DISCONNECTED:
      return DesktopTrayIconState.DISCONNECTED
    case DesktopRuntimeHealth.QUITTING:
      return DesktopTrayIconState.QUITTING
    default:
      return DesktopTrayIconState.NORMAL
  }
}

function actionKindFromRuntimeActionKind(
  kind: DesktopRuntimeActionKind | undefined,
): DesktopTrayActionKind {
  switch (kind) {
    case DesktopRuntimeActionKind.OPEN_ROUTE:
      return DesktopTrayActionKind.OPEN_ROUTE
    case DesktopRuntimeActionKind.NEW_WINDOW:
      return DesktopTrayActionKind.NEW_WINDOW
    case DesktopRuntimeActionKind.COPY_TEXT:
      return DesktopTrayActionKind.COPY_TEXT
    case DesktopRuntimeActionKind.REVEAL_PATH:
      return DesktopTrayActionKind.REVEAL_PATH
    case DesktopRuntimeActionKind.QUIT:
      return DesktopTrayActionKind.QUIT
    default:
      return DesktopTrayActionKind.UNSPECIFIED
  }
}

function buildDiagnosticText(state: DesktopRuntimeState): string {
  return [
    `Spacewave: ${state.statusText || 'Running'}`,
    compactLabel([state.listener?.label || 'Runtime', state.listener?.detail]),
    state.listener?.socketPath ? `Socket: ${state.listener.socketPath}` : '',
  ]
    .filter((line) => line !== '')
    .join('\n')
}
