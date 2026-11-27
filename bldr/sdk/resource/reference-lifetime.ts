import type { ResourceReleaseReason } from './client.js'

/** ResourceOwnership composes disposal with the lifetimes a caller acquired. */
export interface ResourceOwnership {
  [Symbol.dispose](): void
  onReleased(callback: (reason: ResourceReleaseReason) => void): () => void
}

/** ReferenceLifetime commits one terminal cause and notifies each holder once. */
export class ReferenceLifetime {
  /** reason is null until the first terminal transition. */
  private cause: ResourceReleaseReason | null = null

  constructor(signal?: AbortSignal) {
    if (!signal) return
    const ended = () => this.retire('connection-lost')
    signal.addEventListener('abort', ended, { once: true })
    this.onReleased(() => signal.removeEventListener('abort', ended))
    if (signal.aborted) ended()
  }

  /** reason returns the immutable terminal cause, or null while live. */
  get reason(): ResourceReleaseReason | null {
    return this.cause
  }
  private readonly listeners = new Set<
    (reason: ResourceReleaseReason) => void
  >()

  /** onReleased also reports retirement to a subscriber arriving after it. */
  onReleased(callback: (reason: ResourceReleaseReason) => void): () => void {
    if (this.reason !== null) {
      this.notify(callback, this.reason)
      return () => {}
    }
    this.listeners.add(callback)
    return () => this.listeners.delete(callback)
  }

  /** retire records the first cause before notifying subscribers. */
  retire(reason: ResourceReleaseReason): void {
    if (this.reason !== null) return
    this.cause = reason
    for (const callback of this.listeners) this.notify(callback, reason)
    this.listeners.clear()
  }

  /** notify isolates observer failures from ownership and other observers. */
  private notify(
    callback: (reason: ResourceReleaseReason) => void,
    reason: ResourceReleaseReason,
  ): void {
    try {
      callback(reason)
    } catch (error) {
      console.error('Error in resource release listener:', error)
    }
  }
}

/** ownsResources recognizes the shared ownership contract across bundle copies. */
export function ownsResources(value: object): value is ResourceOwnership {
  return 'onReleased' in value && typeof value.onReleased === 'function'
}
