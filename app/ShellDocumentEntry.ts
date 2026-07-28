import { getAppNavigation } from '@s4wave/web/router/app-path.js'
import { parseHashNavigation } from '@s4wave/web/router/hash.js'
import { readShellDocumentState } from './ShellDocumentState.js'

export type ShellDocumentEntryKind = 'fresh' | 'continuation' | 'handoff'

export interface ShellDocumentEntry {
  kind: ShellDocumentEntryKind
  path: string
  params: Record<string, string>
  tabId?: string
  incarnation: string
}

export interface ShellDocumentEntryOptions {
  hash?: string
  path?: string
  params?: Record<string, string>
  navigationType?: 'navigate' | 'reload' | 'back_forward'
  sameEntry?: boolean
  persisted?: boolean
  incarnation?: string
}

function createIncarnation(): string {
  if (
    typeof crypto !== 'undefined' &&
    typeof crypto.randomUUID === 'function'
  ) {
    return crypto.randomUUID()
  }
  return `shell-document-${Date.now()}-${Math.random().toString(36).slice(2)}`
}

function inferredNavigationType(): ShellDocumentEntryOptions['navigationType'] {
  if (typeof performance === 'undefined') return 'navigate'
  const entry = performance.getEntriesByType('navigation').at(0) as
    | PerformanceNavigationTiming
    | undefined
  if (entry?.type === 'reload') return 'reload'
  if (entry?.type === 'back_forward') return 'back_forward'
  return 'navigate'
}

function isSyntacticallyValidTabId(tabId: string): boolean {
  return /^[A-Za-z0-9][A-Za-z0-9._:-]*$/.test(tabId)
}

export function classifyShellDocumentEntry(
  options: ShellDocumentEntryOptions = {},
): ShellDocumentEntry {
  const navigation =
    options.hash !== undefined
      ? parseHashNavigation(options.hash)
      : options.path !== undefined || options.params !== undefined
        ? { path: options.path ?? '/', params: options.params ?? {} }
        : typeof window === 'undefined'
          ? { path: '/', params: {} }
          : getAppNavigation()
  const tabId = navigation.params.shellTabId
  const hasValidExplicitId =
    tabId !== undefined && isSyntacticallyValidTabId(tabId)
  const navigationType = options.navigationType ?? inferredNavigationType()
  const isContinuation =
    options.sameEntry === true ||
    options.persisted === true ||
    navigationType === 'reload' ||
    navigationType === 'back_forward'

  if (hasValidExplicitId) {
    return {
      kind: 'handoff',
      path: navigation.path,
      params: navigation.params,
      tabId,
      incarnation: options.incarnation ?? createIncarnation(),
    }
  }
  return {
    kind: isContinuation ? 'continuation' : 'fresh',
    path: navigation.path,
    params: navigation.params,
    incarnation:
      options.incarnation ??
      (isContinuation ? readShellDocumentState()?.incarnation : undefined) ??
      createIncarnation(),
  }
}

export function isShellDocumentContinuation(
  entry: ShellDocumentEntry,
): boolean {
  return entry.kind === 'continuation'
}

export function isShellDocumentHandoff(entry: ShellDocumentEntry): boolean {
  return entry.kind === 'handoff' && entry.tabId !== undefined
}
