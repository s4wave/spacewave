// ShellTab represents a single tab in the editor shell.
export interface ShellTab {
  id: string
  name: string
  path: string
  // customName overrides the auto-derived name when set.
  // Clearing it (setting to empty string or undefined) reverts to the default.
  customName?: string
}

// getTabDisplayName returns the display name for a tab.
// Uses customName if set, otherwise falls back to auto-derived name.
export function getTabDisplayName(tab: ShellTab): string {
  if (tab.customName) return tab.customName
  return tab.name
}

// getTabNameFromPath derives a tab name from a URL path.
export function getTabNameFromPath(path: string): string {
  const normalized = path.replace(/^\/+/, '')

  if (!normalized || normalized === '/') {
    return 'Home'
  }

  if (normalized === 'community') {
    return 'Community'
  }

  if (normalized === 'login') {
    return 'Login'
  }

  if (normalized.startsWith('quickstart')) {
    return 'Quickstart'
  }

  if (normalized.startsWith('blog')) {
    return 'Blog'
  }

  if (normalized.startsWith('changelog')) {
    return 'Changelog'
  }

  if (normalized.startsWith('u/')) {
    const parts = normalized.split('/')
    if (parts.length >= 4 && parts[2] === 'so') {
      // /u/{idx}/so/{id} or deeper -- derive from sub-path if present
      if (parts.length >= 6 && parts[4] === '-') {
        // /u/{idx}/so/{id}/-/{objectKey}/... -- use object key
        const objectKey = parts[5]
        if (objectKey === 'object-layout') return 'Layout'
        if (objectKey === 'unixfs') return 'Files'
        if (objectKey === 'canvas') return 'Canvas'
        if (objectKey === 'notes') return 'Notes'
        if (objectKey === 'chat') return 'Chat'
        return 'Space'
      }
      return 'Space'
    }
    if (parts.length >= 4 && parts[2] === 'devices') {
      return 'Devices'
    }
    if (parts.length >= 4 && parts[2] === 'settings') {
      return 'Settings'
    }
    return 'Session'
  }

  return 'Tab'
}

// generateTabId generates a unique tab ID.
export function generateTabId(): string {
  return `tab-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`
}

// DEFAULT_HOME_TAB is the default tab shown when no tabs exist.
export const DEFAULT_HOME_TAB: ShellTab = {
  id: 'home',
  name: 'Home',
  path: '/',
}

// getSessionPathFromPath extracts the session base path from a URL path.
// Returns `/u/{sessionIndex}` if the path is within a session, otherwise null.
export function getSessionPathFromPath(path: string): string | null {
  const match = path.match(/^\/u\/(\d+)/)
  if (match) {
    return `/u/${match[1]}`
  }
  return null
}
