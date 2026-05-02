import { useSyncExternalStore } from 'react'

export const bootStatusEvent = 'spacewave:boot-status'

export interface BrowserBootStatus {
  phase: string
  detail: string
  state: 'loading' | 'error'
}

const defaultStatus: BrowserBootStatus = {
  phase: 'loading',
  detail: 'Loading application...',
  state: 'loading',
}

declare global {
  var __swBootStatus: BrowserBootStatus | undefined
}

export function readBrowserBootStatus(): BrowserBootStatus {
  return globalThis.__swBootStatus ?? defaultStatus
}

// canMutateBrowserBootStatusTarget returns true when boot scripts may update a
// status node without racing React hydration.
export function canMutateBrowserBootStatusTarget(
  target: Element | null,
): target is Element {
  if (!target) return false
  const root = target.closest('#bldr-root[data-prerendered]')
  if (!root) return true
  return !!target.closest('#sw-loading')
}

export function subscribeBrowserBootStatus(callback: () => void): () => void {
  window.addEventListener(bootStatusEvent, callback)
  return () => window.removeEventListener(bootStatusEvent, callback)
}

export function useBrowserBootStatus(): BrowserBootStatus {
  return useSyncExternalStore(
    subscribeBrowserBootStatus,
    readBrowserBootStatus,
    () => defaultStatus,
  )
}
