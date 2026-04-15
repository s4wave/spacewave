// ServiceWorkerFetchTracker tracks proxied fetches by ServiceWorker client ID.
// It allows the ServiceWorker to abort all outstanding proxied fetches for a
// page when that page sends a close/goodbye signal.
export class ServiceWorkerFetchTracker {
  private readonly fetchesByClient = new Map<string, Set<AbortController>>()

  // trackFetch registers a fetch under the given client ID and returns an
  // AbortController plus a release callback for cleanup.
  public trackFetch(clientId: string): {
    abortController: AbortController
    release: () => void
  } {
    const abortController = new AbortController()
    const fetches = this.fetchesByClient.get(clientId) ?? new Set()
    fetches.add(abortController)
    this.fetchesByClient.set(clientId, fetches)

    let released = false
    const release = () => {
      if (released) {
        return
      }
      released = true

      const active = this.fetchesByClient.get(clientId)
      if (!active) {
        return
      }
      active.delete(abortController)
      if (!active.size) {
        this.fetchesByClient.delete(clientId)
      }
    }

    return { abortController, release }
  }

  // abortClient aborts all outstanding proxied fetches for the given client.
  public abortClient(clientId: string, reason?: unknown) {
    const fetches = this.fetchesByClient.get(clientId)
    if (!fetches) {
      return
    }
    this.fetchesByClient.delete(clientId)
    for (const abortController of fetches) {
      abortController.abort(reason)
    }
  }

  // getActiveFetchCount returns the number of outstanding proxied fetches for
  // the client. Intended for tests and diagnostics.
  public getActiveFetchCount(clientId: string): number {
    return this.fetchesByClient.get(clientId)?.size ?? 0
  }
}
