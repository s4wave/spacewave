// BrowserReleaseSyncRequestMessage asks the SW to refresh the release manifest.
export interface BrowserReleaseSyncRequestMessage {
  bldrSyncManifest: true
}

// postBrowserReleaseSyncRequest asks the controlling ServiceWorker to sync.
export function postBrowserReleaseSyncRequest(): void {
  navigator.serviceWorker.controller?.postMessage({
    bldrSyncManifest: true,
  } satisfies BrowserReleaseSyncRequestMessage)
}

declare global {
  var __swGenerationId: string | undefined
}

// initBrowserReleaseUpdates refreshes the offline cache without interrupting active tabs.
export function initBrowserReleaseUpdates(): void {
  if (!('serviceWorker' in navigator)) {
    return
  }

  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') {
      postBrowserReleaseSyncRequest()
    }
  })
  window.addEventListener('focus', () => {
    postBrowserReleaseSyncRequest()
  })
  window.addEventListener('online', () => {
    postBrowserReleaseSyncRequest()
  })
}
