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
  // storage is the document storage that holds this document's saved Shell
  // state. Defaults to sessionStorage.
  storage?: Storage
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
  const restored =
    options.sameEntry === true ||
    options.persisted === true ||
    navigationType === 'reload' ||
    navigationType === 'back_forward'
  // A restored document continues only the Shell state it saved itself. One
  // that saved none, such as a page that failed before its Shell mounted and
  // then reloaded, has no tab to continue and enters fresh: continuing would
  // adopt a record another document owns.
  const saved = restored ? readShellDocumentState(options.storage) : null

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
    kind: saved ? 'continuation' : 'fresh',
    path: navigation.path,
    params: navigation.params,
    incarnation:
      options.incarnation ?? saved?.incarnation ?? createIncarnation(),
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

// documentEntries holds the entry each document's first Shell mount
// classified, keyed by the document storage that holds its state.
const documentEntries = new WeakMap<Storage, ShellDocumentEntry>()

// resolveShellDocumentEntry returns the entry for the document that owns
// storage. The first mount classifies it. A later mount in the same document,
// such as after the runtime root reloads behind the loading screen, continues
// that entry: classifying it again would read the unchanged URL as a fresh
// navigation and create another Shell record. The entry is only continued
// while storage still holds its incarnation's state.
export function resolveShellDocumentEntry(
  storage: Storage | undefined,
  classify: () => ShellDocumentEntry,
): ShellDocumentEntry {
  if (!storage) return classify()
  const first = documentEntries.get(storage)
  if (
    first &&
    readShellDocumentState(storage)?.incarnation === first.incarnation
  ) {
    return {
      kind: 'continuation',
      path: first.path,
      params: first.params,
      incarnation: first.incarnation,
    }
  }
  const entry = classify()
  documentEntries.set(storage, entry)
  return entry
}
