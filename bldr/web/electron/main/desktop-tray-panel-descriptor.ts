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
  buildDesktopTrayIconModel,
  type DesktopTrayIconModel,
} from './desktop-tray-icon.js'

export type DesktopTrayPanelRowKind =
  | 'action'
  | 'section'
  | 'separator'
  | 'status'
  | 'submenu'
  | 'unknown'

export type DesktopTrayPanelSeverity = 'critical' | 'info' | 'none' | 'warning'

export interface DesktopTrayPanelAction {
  id: string
  kind: DesktopTrayActionKind
  route?: string
  value?: string
}

export interface DesktopTrayPanelRow {
  id: string
  kind: DesktopTrayPanelRowKind
  label: string
  detail: string
  statusText: string
  path: string[]
  active: boolean
  enabled: boolean
  empty: boolean
  severity: DesktopTrayPanelSeverity
  action?: DesktopTrayPanelAction
}

export interface DesktopTrayPanelSection {
  id: string
  title: string
  rows: DesktopTrayPanelRow[]
}

export interface DesktopTrayPanelCard {
  id: string
  label: string
  value: string
  detail: string
  severity: DesktopTrayPanelSeverity
}

export interface DesktopTrayPanelTab {
  id: 'overview' | 'sessions' | 'spaces'
  label: string
  enabled: boolean
  count: number
}

export interface DesktopTrayPanelDescriptor {
  appName: string
  statusText: string
  title: string
  subtitle: string
  icon: DesktopTrayIconModel
  severity: DesktopTrayPanelSeverity
  tabs: DesktopTrayPanelTab[]
  cards: DesktopTrayPanelCard[]
  primaryActions: DesktopTrayPanelRow[]
  attentionRows: DesktopTrayPanelRow[]
  sessionRows: DesktopTrayPanelRow[]
  spaceRows: DesktopTrayPanelRow[]
  activityRows: DesktopTrayPanelRow[]
  actionRows: DesktopTrayPanelRow[]
  sections: DesktopTrayPanelSection[]
}

export interface BuildDesktopTrayPanelDescriptorOpts {
  appName?: string
  state: DesktopTrayState
  runtimeState?: DesktopRuntimeState
  dynamicIconEnabled?: boolean
}

export function buildDesktopTrayPanelDescriptor(
  opts: BuildDesktopTrayPanelDescriptorOpts,
): DesktopTrayPanelDescriptor {
  const appName = opts.appName || 'Spacewave'
  const state = opts.state
  const rows = (state.entries ?? []).map((entry) => toPanelRow(entry))
  const displayRows = rows.filter((row) => row.id !== 'title')
  const sections = buildSections(displayRows)
  const statusText =
    state.statusText || opts.runtimeState?.statusText || 'Running'
  const attentionRows = displayRows.filter(
    (row) =>
      row.id.startsWith('attention-') ||
      row.severity === 'critical' ||
      row.severity === 'warning',
  )
  const sessionRows = sectionRows(sections, 'Sessions')
  const spaceRows = sectionRows(sections, 'Spaces')
  const activityRows = sectionRows(sections, 'Activity')
  const actionRows = displayRows.filter((row) => row.action)
  const primaryActions = selectPrimaryActions(actionRows)
  const severity = panelSeverity(attentionRows, state.iconState)

  return {
    appName,
    statusText,
    title: appName,
    subtitle: statusText,
    icon: buildDesktopTrayIconModel({
      appName,
      state,
      dynamicIconEnabled: opts.dynamicIconEnabled ?? false,
    }),
    severity,
    tabs: [
      { id: 'overview', label: 'Overview', enabled: true, count: 0 },
      {
        id: 'sessions',
        label: 'Sessions',
        enabled: countRows(sessionRows, 'No sessions') !== 0,
        count:
          opts.runtimeState?.sessions?.length ??
          countRows(sessionRows, 'No sessions'),
      },
      {
        id: 'spaces',
        label: 'Spaces',
        enabled: countRows(spaceRows, 'No spaces') !== 0,
        count:
          opts.runtimeState?.spaces?.length ??
          countRows(spaceRows, 'No spaces'),
      },
    ],
    cards: buildCards({
      statusText,
      severity,
      state,
      sessionRows,
      spaceRows,
      activityRows,
      attentionRows,
      runtimeState: opts.runtimeState,
    }),
    primaryActions,
    attentionRows,
    sessionRows,
    spaceRows,
    activityRows,
    actionRows,
    sections,
  }
}

export function canInvokeDesktopTrayEntry(entry: DesktopTrayEntry): boolean {
  if (
    entry.kind !== DesktopTrayEntryKind.ACTION ||
    !(entry.enabled ?? false) ||
    !entry.action
  ) {
    return false
  }
  switch (entry.action.kind) {
    case DesktopTrayActionKind.OPEN_ROUTE:
    case DesktopTrayActionKind.NEW_WINDOW:
    case DesktopTrayActionKind.QUIT:
    case DesktopTrayActionKind.ATTACHED_HANDLER:
      return true
    case DesktopTrayActionKind.COPY_TEXT:
    case DesktopTrayActionKind.REVEAL_PATH:
      return !!entry.action.value
    default:
      return false
  }
}

function toPanelRow(entry: DesktopTrayEntry): DesktopTrayPanelRow {
  const invokable = canInvokeDesktopTrayEntry(entry)
  return {
    id: entry.id || stableRowID(entry),
    kind: panelRowKind(entry.kind),
    label: entry.label || '',
    detail: entry.detail || '',
    statusText: entry.statusText || actionStatusText(entry),
    path: [...(entry.path ?? [])].filter((part) => part !== ''),
    active: entry.active ?? false,
    enabled: invokable,
    empty: emptyRow(entry),
    severity: panelRowSeverity(entry.severity),
    action:
      invokable && entry.action ?
        {
          id: entry.id || stableRowID(entry),
          kind: entry.action.kind ?? DesktopTrayActionKind.UNSPECIFIED,
          route: entry.action.route,
          value: entry.action.value,
        }
      : undefined,
  }
}

function buildSections(rows: DesktopTrayPanelRow[]): DesktopTrayPanelSection[] {
  const sections: DesktopTrayPanelSection[] = []
  let current = ensureSection(sections, 'overview', 'Overview')

  for (const row of rows) {
    if (row.kind === 'separator') {
      continue
    }
    if (row.kind === 'section' || row.kind === 'submenu') {
      current = ensureSection(sections, sectionID(row.label), row.label)
      continue
    }
    if (row.path.length) {
      current = ensureSection(
        sections,
        sectionID(row.path.join('-')),
        row.path.join(' / '),
      )
    }
    current.rows.push(row)
  }

  return sections.filter((section) => section.rows.length !== 0)
}

function ensureSection(
  sections: DesktopTrayPanelSection[],
  id: string,
  title: string,
): DesktopTrayPanelSection {
  let section = sections.find((item) => item.id === id)
  if (!section) {
    section = { id, title, rows: [] }
    sections.push(section)
  }
  return section
}

function sectionRows(
  sections: DesktopTrayPanelSection[],
  title: string,
): DesktopTrayPanelRow[] {
  return sections.find((section) => section.title === title)?.rows ?? []
}

function selectPrimaryActions(
  rows: DesktopTrayPanelRow[],
): DesktopTrayPanelRow[] {
  const byID = new Map(rows.map((row) => [row.id, row]))
  return ['open', 'new-window', 'apply-update']
    .map((id) => byID.get(id))
    .filter((row): row is DesktopTrayPanelRow => !!row)
    .slice(0, 3)
}

function buildCards(opts: {
  statusText: string
  severity: DesktopTrayPanelSeverity
  state: DesktopTrayState
  sessionRows: DesktopTrayPanelRow[]
  spaceRows: DesktopTrayPanelRow[]
  activityRows: DesktopTrayPanelRow[]
  attentionRows: DesktopTrayPanelRow[]
  runtimeState?: DesktopRuntimeState
}): DesktopTrayPanelCard[] {
  const sessionCount =
    opts.runtimeState?.sessions?.length ??
    countRows(opts.sessionRows, 'No sessions')
  const spaceCount =
    opts.runtimeState?.spaces?.length ?? countRows(opts.spaceRows, 'No spaces')
  const activityCount =
    opts.runtimeState?.activity?.length ?? countRows(opts.activityRows, '')
  const attentionCount =
    opts.runtimeState?.attentionItems?.length ?? opts.attentionRows.length

  return [
    {
      id: 'health',
      label: 'Runtime',
      value: iconStateLabel(opts.state.iconState),
      detail: opts.statusText,
      severity: opts.severity,
    },
    {
      id: 'sessions',
      label: 'Sessions',
      value: countLabel(sessionCount),
      detail: firstRowLabel(opts.sessionRows, 'No active sessions'),
      severity: 'none',
    },
    {
      id: 'spaces',
      label: 'Spaces',
      value: countLabel(spaceCount),
      detail: firstRowLabel(opts.spaceRows, 'No recent spaces'),
      severity: 'none',
    },
    {
      id: attentionCount ? 'attention' : 'activity',
      label: attentionCount ? 'Attention' : 'Activity',
      value:
        attentionCount ? countLabel(attentionCount) : countLabel(activityCount),
      detail:
        attentionCount ?
          firstRowLabel(opts.attentionRows, 'Needs attention')
        : firstRowLabel(opts.activityRows, 'Quiet'),
      severity: attentionCount ? opts.severity : 'none',
    },
  ]
}

function countRows(rows: DesktopTrayPanelRow[], emptyLabel: string): number {
  return rows.filter((row) => !row.empty && row.label !== emptyLabel).length
}

function countLabel(count: number): string {
  return count === 0 ? 'None' : String(count)
}

function firstRowLabel(rows: DesktopTrayPanelRow[], fallback: string): string {
  const row = rows.find((item) => !item.empty)
  if (!row) return fallback
  return [row.label, row.detail || row.statusText].filter(Boolean).join(' - ')
}

function panelSeverity(
  attentionRows: DesktopTrayPanelRow[],
  iconState: DesktopTrayIconState | undefined,
): DesktopTrayPanelSeverity {
  if (attentionRows.some((row) => row.severity === 'critical')) {
    return 'critical'
  }
  if (iconState === DesktopTrayIconState.ATTENTION) {
    return 'warning'
  }
  if (attentionRows.some((row) => row.severity === 'warning')) {
    return 'warning'
  }
  if (attentionRows.some((row) => row.severity === 'info')) {
    return 'info'
  }
  return 'none'
}

function panelRowKind(
  kind: DesktopTrayEntryKind | undefined,
): DesktopTrayPanelRowKind {
  switch (kind) {
    case DesktopTrayEntryKind.ACTION:
      return 'action'
    case DesktopTrayEntryKind.SECTION:
      return 'section'
    case DesktopTrayEntryKind.SEPARATOR:
      return 'separator'
    case DesktopTrayEntryKind.STATUS:
      return 'status'
    case DesktopTrayEntryKind.SUBMENU:
      return 'submenu'
    default:
      return 'unknown'
  }
}

function panelRowSeverity(
  severity: DesktopTraySeverity | undefined,
): DesktopTrayPanelSeverity {
  switch (severity) {
    case DesktopTraySeverity.CRITICAL:
      return 'critical'
    case DesktopTraySeverity.WARNING:
      return 'warning'
    case DesktopTraySeverity.INFO:
      return 'info'
    default:
      return 'none'
  }
}

function actionStatusText(entry: DesktopTrayEntry): string {
  if (entry.statusText) {
    return entry.statusText
  }
  switch (entry.action?.kind) {
    case DesktopTrayActionKind.COPY_TEXT:
      return 'Copy'
    case DesktopTrayActionKind.REVEAL_PATH:
      return 'Reveal'
    case DesktopTrayActionKind.NEW_WINDOW:
      return 'New'
    default:
      return ''
  }
}

function emptyRow(entry: DesktopTrayEntry): boolean {
  return (
    entry.kind === DesktopTrayEntryKind.STATUS &&
    (entry.label === 'No sessions' || entry.label === 'No spaces')
  )
}

function iconStateLabel(iconState: DesktopTrayIconState | undefined): string {
  switch (iconState) {
    case DesktopTrayIconState.ACTIVE:
      return 'Active'
    case DesktopTrayIconState.ATTENTION:
      return 'Attention'
    case DesktopTrayIconState.DISCONNECTED:
      return 'Offline'
    case DesktopTrayIconState.QUITTING:
      return 'Quitting'
    default:
      return 'Healthy'
  }
}

function sectionID(label: string): string {
  return label
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

function stableRowID(entry: DesktopTrayEntry): string {
  return [entry.path?.join('-'), entry.label, entry.order?.toString()]
    .filter(Boolean)
    .join('-')
}
