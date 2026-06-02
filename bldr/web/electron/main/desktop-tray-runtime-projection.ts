import {
  DesktopTrayActionKind,
  DesktopTrayEntryKind,
  DesktopTrayIconState,
  DesktopTraySeverity,
  type DesktopTrayAction,
  type DesktopTrayEntry,
} from '@go/github.com/s4wave/spacewave/bldr/desktop/tray/tray.pb.js'
import {
  DesktopCLIInstallStatus,
  DesktopRuntimeActionKind,
  DesktopRuntimeHealth,
  DesktopRuntimeSeverity,
  type DesktopRuntimeActionItem,
  type DesktopRuntimeActivityItem,
  type DesktopRuntimeAttentionItem,
  type DesktopRuntimeNavigationItem,
  type DesktopRuntimeState,
  type DesktopRuntimeCLIInstallSummary,
} from '../desktop-runtime/desktop-runtime.pb.js'

export function buildDesktopTrayCLIInstallEntries(
  summary: DesktopRuntimeCLIInstallSummary | undefined,
): DesktopTrayEntry[] {
  if (!hasCLIInstallSummary(summary)) return []
  return [
    separatorEntry('cli-install-separator'),
    sectionEntry('cli-install-section', 'Command Line'),
    statusEntry(
      'cli-install-status',
      compactLabel(['Command line', summary?.label, summary?.detail]),
      { severity: cliInstallSeverity(summary?.status) },
    ),
    actionEntry(
      'cli-install-settings',
      'Command Line Settings',
      DesktopTrayActionKind.OPEN_ROUTE,
      {
        route: summary?.route || '/',
        enabled: true,
      },
    ),
  ]
}

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
    statusEntry('title', `Spacewave: ${state.statusText || 'Running'}`, {
      iconState: iconStateForRuntimeHealth(state.health),
    }),
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
      route: settingsRoute(state),
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
    ...(state.update?.ready
      ? [
          separatorEntry('quick-actions-separator'),
          sectionEntry('quick-actions-section', 'Quick Actions'),
          applyUpdateEntry(state.update),
        ]
      : []),
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
  if (hasCLIInstallSummary(state.cliInstall)) {
    rows.push(
      statusEntry(
        'status-cli-install',
        compactLabel([
          'Command line',
          state.cliInstall?.label,
          state.cliInstall?.detail,
        ]),
        { severity: cliInstallSeverity(state.cliInstall?.status) },
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
  if (!items?.length) {
    return []
  }
  return [
    separatorEntry('activity-separator'),
    sectionEntry('activity-section', 'Activity'),
    ...nonEmpty(items).map((item) =>
      statusEntry(
        `activity-${item.id || compactLabel([item.label, item.detail])}`,
        compactLabel([item.label, item.detail]),
      ),
    ),
  ]
}

function buildActionSection(state: DesktopRuntimeState): DesktopTrayEntry[] {
  const entries = [
    ...(state.update?.ready ? [applyUpdateEntry(state.update)] : []),
    ...buildSyntheticActions(state).map((item) => buildActionItem(item)),
    ...nonEmpty(state.actions).map((item) => buildActionItem(item)),
  ]
  if (!entries.length) {
    return []
  }
  return [
    separatorEntry('quick-actions-separator'),
    sectionEntry('quick-actions-section', 'Quick Actions'),
    ...entries,
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

export function desktopRuntimeCLISettingsRoute(
  state: Pick<DesktopRuntimeState, 'sessions'> | undefined,
): string {
  const session = selectSettingsSession(state?.sessions)
  if (!session?.route) {
    return ''
  }
  return `${session.route.replace(/\/+$/, '')}/settings/cli`
}

function settingsRoute(state: DesktopRuntimeState): string {
  return desktopRuntimeCLISettingsRoute(state) || '/'
}

function selectSettingsSession(
  items: DesktopRuntimeNavigationItem[] | undefined,
): DesktopRuntimeNavigationItem | undefined {
  let fallback: DesktopRuntimeNavigationItem | undefined
  for (const item of items ?? []) {
    if (!item.route) {
      continue
    }
    fallback ??= item
    if (item.active) {
      return item
    }
  }
  return fallback
}

function buildSyntheticActions(
  state: DesktopRuntimeState,
): DesktopRuntimeActionItem[] {
  const socketPath = state.listener?.socketPath
  const actions: DesktopRuntimeActionItem[] = []
  if (socketPath) {
    actions.push(
      {
        id: 'copy-cli-socket',
        kind: DesktopRuntimeActionKind.COPY_TEXT,
        label: 'Copy Socket Path',
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
    )
  }
  if (hasCLIInstallSummary(state.cliInstall)) {
    actions.push({
      id: 'open-cli-settings',
      kind: DesktopRuntimeActionKind.OPEN_ROUTE,
      label: 'Command Line Settings',
      detail: state.cliInstall?.label,
      route: state.cliInstall?.route || settingsRoute(state),
      enabled: true,
    })
  }
  return actions
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

function applyUpdateEntry(
  update: NonNullable<DesktopRuntimeState['update']>,
): DesktopTrayEntry {
  return actionEntry(
    'apply-update',
    'Install Update',
    DesktopTrayActionKind.ATTACHED_HANDLER,
    {
      enabled: update.ready,
      value: update.version,
      severity: DesktopTraySeverity.INFO,
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
    Partial<Pick<DesktopTrayEntry, 'severity'>> &
    Partial<Pick<DesktopTrayAction, 'route' | 'value'>>,
): DesktopTrayEntry {
  return {
    id,
    kind: DesktopTrayEntryKind.ACTION,
    label,
    active: opts?.active ?? false,
    enabled: opts?.enabled ?? true,
    severity: opts?.severity ?? DesktopTraySeverity.UNSPECIFIED,
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
  return nonEmpty(items).toSorted((a, b) => {
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

function hasCLIInstallSummary(
  summary: DesktopRuntimeCLIInstallSummary | undefined,
): boolean {
  const status =
    summary?.status ??
    DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_UNSPECIFIED
  return !!(
    summary?.label ||
    summary?.detail ||
    status !== DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_UNSPECIFIED
  )
}

function cliInstallSeverity(
  status: DesktopCLIInstallStatus | undefined,
): DesktopTraySeverity {
  switch (status) {
    case DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_INSTALLED:
      return DesktopTraySeverity.INFO
    case DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_UPDATE_AVAILABLE:
    case DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_CONFLICT:
    case DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_MISSING:
      return DesktopTraySeverity.WARNING
    case DesktopCLIInstallStatus.DESKTOP_CLI_INSTALL_STATUS_ERROR:
      return DesktopTraySeverity.CRITICAL
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
