// BrowserReleaseUpdateMessage is the ServiceWorker shell-update broadcast shape.
export interface BrowserReleaseUpdateMessage {
  bldrPromotedGenerationId?: string
}

// shouldReloadForPromotedGeneration checks if the tab should reload.
export function shouldReloadForPromotedGeneration(
  currentGenerationId: string | undefined,
  promotedGenerationId: string | undefined,
): boolean {
  if (!promotedGenerationId) {
    return false
  }
  if (!currentGenerationId) {
    return true
  }
  return currentGenerationId !== promotedGenerationId
}

declare global {
  var __swGenerationId: string | undefined
}

// initBrowserReleaseAutoReload reloads the tab when a newer promoted shell arrives.
export function initBrowserReleaseAutoReload(): void {
  if (!('serviceWorker' in navigator)) {
    return
  }

  navigator.serviceWorker.addEventListener('message', (ev: MessageEvent) => {
    const data = ev.data as BrowserReleaseUpdateMessage
    if (!data || typeof data !== 'object') {
      return
    }
    if (
      shouldReloadForPromotedGeneration(
        globalThis.__swGenerationId,
        data.bldrPromotedGenerationId,
      )
    ) {
      window.location.reload()
    }
  })
}
