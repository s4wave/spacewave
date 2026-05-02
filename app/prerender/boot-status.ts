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
