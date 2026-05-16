import {
  DesktopTrayActionKind,
  DesktopTrayEntryKind,
  DesktopTrayIconState,
  DesktopTraySeverity,
  type DesktopTrayAction,
  type DesktopTrayEntry,
  type DesktopTrayState,
} from '@go/github.com/s4wave/spacewave/bldr/desktop/tray/tray.pb.js'

const pathSeparator = '\u0000'
const groupSeparator = '\u0001'

export interface TrayPanelDescriptor {
  statusText: string
  iconState: DesktopTrayIconState
  rows: TrayPanelRow[]
  sections: TrayPanelSection[]
}

export interface TrayPanelSection {
  key: string
  path: string[]
  group: string
  rows: TrayPanelRow[]
}

export interface TrayPanelRow {
  id: string
  position: number
  kind: DesktopTrayEntryKind
  path: string[]
  group: string
  order: number
  label: string
  detail: string
  statusText: string
  iconName: string
  iconState: DesktopTrayIconState
  severity: DesktopTraySeverity
  active: boolean
  enabled: boolean
  actionEligible: boolean
  actionKind: DesktopTrayActionKind
  action?: DesktopTrayAction
}

export function buildTrayPanelDescriptor(
  state: DesktopTrayState,
): TrayPanelDescriptor {
  const rows = (state.entries ?? []).map((entry, index) =>
    buildTrayPanelRow(entry, index),
  )
  return {
    statusText: state.statusText ?? '',
    iconState: state.iconState ?? DesktopTrayIconState.UNSPECIFIED,
    rows,
    sections: buildTrayPanelSections(rows),
  }
}

export function buildTrayPanelRow(
  entry: DesktopTrayEntry,
  position: number,
): TrayPanelRow {
  const action = entry.action ? { ...entry.action } : undefined
  return {
    id: entry.id ?? '',
    position,
    kind: entry.kind ?? DesktopTrayEntryKind.UNSPECIFIED,
    path: [...(entry.path ?? [])],
    group: entry.group ?? '',
    order: entry.order ?? 0,
    label: entry.label ?? '',
    detail: entry.detail ?? '',
    statusText: entry.statusText ?? '',
    iconName: entry.iconName ?? '',
    iconState: entry.iconState ?? DesktopTrayIconState.UNSPECIFIED,
    severity: entry.severity ?? DesktopTraySeverity.UNSPECIFIED,
    active: entry.active ?? false,
    enabled: entry.enabled ?? false,
    actionEligible: canInvokeTrayPanelEntry(entry),
    actionKind: action?.kind ?? DesktopTrayActionKind.UNSPECIFIED,
    action,
  }
}

export function canInvokeTrayPanelEntry(entry: DesktopTrayEntry): boolean {
  if (
    entry.kind !== DesktopTrayEntryKind.ACTION ||
    !(entry.enabled ?? false) ||
    !entry.action
  ) {
    return false
  }
  return canInvokeTrayPanelAction(entry.action)
}

function canInvokeTrayPanelAction(action: DesktopTrayAction): boolean {
  switch (action.kind) {
    case DesktopTrayActionKind.OPEN_ROUTE:
    case DesktopTrayActionKind.NEW_WINDOW:
    case DesktopTrayActionKind.QUIT:
    case DesktopTrayActionKind.ATTACHED_HANDLER:
      return true
    case DesktopTrayActionKind.COPY_TEXT:
    case DesktopTrayActionKind.REVEAL_PATH:
      return !!action.value
    default:
      return false
  }
}

function buildTrayPanelSections(rows: TrayPanelRow[]): TrayPanelSection[] {
  const sections: TrayPanelSection[] = []
  for (const row of rows) {
    const current = sections.at(-1)
    if (current?.key === rowSectionKey(row)) {
      current.rows.push(row)
      continue
    }
    sections.push({
      key: rowSectionKey(row),
      path: [...row.path],
      group: row.group,
      rows: [row],
    })
  }
  return sections
}

function rowSectionKey(row: TrayPanelRow): string {
  return `${row.path.join(pathSeparator)}${groupSeparator}${row.group}`
}
