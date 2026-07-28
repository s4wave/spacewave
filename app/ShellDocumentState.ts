// ShellDocumentState persists one browser document's local Shell selection.
export interface ShellDocumentState {
  incarnation: string
  activeTabId: string
  pendingCreatedTabId?: string
}

const SHELL_DOCUMENT_STATE_STORAGE_KEY = 'shell-document-state'
const SHELL_TAB_STATE_PREFIX = 'shell-tab-state:'
const OBSOLETE_SHELL_TABS_STATE_STORAGE_KEY = 'shell-tabs-state'
const OBSOLETE_SHELL_TAB_STATE_PREFIX = 'tab-state-'

export function clearObsoleteShellTabsState(): void {
  const storage = getSessionStorage()
  if (!storage) return
  try {
    storage.removeItem(OBSOLETE_SHELL_TABS_STATE_STORAGE_KEY)
  } catch {
    // Obsolete cleanup is best effort and never imports old state.
  }
}
function getLocalStorage(): Storage | null {
  try {
    if (typeof localStorage === 'undefined') return null
    return localStorage
  } catch {
    return null
  }
}
function getSessionStorage(): Storage | null {
  try {
    if (typeof sessionStorage === 'undefined') return null
    return sessionStorage
  } catch {
    return null
  }
}

function isDocumentState(value: unknown): value is ShellDocumentState {
  if (!value || typeof value !== 'object') return false
  const state = value as Partial<ShellDocumentState>
  return (
    typeof state.incarnation === 'string' &&
    state.incarnation.length > 0 &&
    typeof state.activeTabId === 'string' &&
    (state.pendingCreatedTabId === undefined ||
      (typeof state.pendingCreatedTabId === 'string' &&
        state.pendingCreatedTabId.length > 0))
  )
}

export function readShellDocumentState(): ShellDocumentState | null {
  const storage = getSessionStorage()
  if (!storage) return null
  try {
    const raw = storage.getItem(SHELL_DOCUMENT_STATE_STORAGE_KEY)
    if (!raw) return null
    const parsed: unknown = JSON.parse(raw)
    return isDocumentState(parsed) ? parsed : null
  } catch {
    return null
  }
}

export function writeShellDocumentState(state: ShellDocumentState): void {
  const storage = getSessionStorage()
  if (!storage) return
  try {
    storage.setItem(SHELL_DOCUMENT_STATE_STORAGE_KEY, JSON.stringify(state))
  } catch {
    // Document-local continuity is best effort; shared mutations remain fail-closed.
  }
}

export function shellTabStateStorageKey(
  incarnation: string,
  tabId: string,
): string {
  return `${SHELL_TAB_STATE_PREFIX}${incarnation}:${tabId}`
}

export function removeShellTabDocumentState(
  incarnation: string,
  tabId: string,
): void {
  const storage = getSessionStorage()
  if (!storage) return
  try {
    storage.removeItem(shellTabStateStorageKey(incarnation, tabId))
  } catch {
    // Explicit Shell close/reset cleanup is best effort after shared commit.
  }
}

export function removeObsoleteShellTabState(tabId: string): void {
  const storage = getLocalStorage()
  if (!storage) return
  try {
    storage.removeItem(`${OBSOLETE_SHELL_TAB_STATE_PREFIX}${tabId}`)
  } catch {
    // Obsolete per-tab cleanup is best effort after shared commit.
  }
}
