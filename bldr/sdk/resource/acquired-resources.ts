import '../dispose-symbol.js'

import type { ResourceReleaseReason } from './client.js'
import { ownsResources, type ResourceOwnership } from './reference-lifetime.js'

/** AcquiredResources composes cleanup and retirement for one factory invocation. */
export class AcquiredResources implements ResourceOwnership {
  private readonly resources: { [Symbol.dispose](): void }[] = []
  private disposed = false

  /** add retains a factory's acquisition, or disposes a stale arrival. */
  add<T extends { [Symbol.dispose](): void } | null | undefined>(
    resource: T,
  ): T {
    if (!resource) return resource
    if (this.disposed) {
      queueMicrotask(() => resource[Symbol.dispose]())
      return resource
    }
    this.resources.push(resource)
    return resource
  }

  /** onReleased subscribes to every constituent without inspecting the result value. */
  onReleased(callback: (reason: ResourceReleaseReason) => void): () => void {
    const subscriptions = this.resources.flatMap((resource) =>
      ownsResources(resource) ? [resource.onReleased(callback)] : [],
    )
    return () => subscriptions.forEach((unsubscribe) => unsubscribe())
  }

  /** hasResources reports whether this run acquired a cleanup obligation. */
  get hasResources(): boolean {
    return this.resources.length !== 0
  }

  /** dispose releases every acquisition once, including an abandoned result. */
  [Symbol.dispose](): void {
    if (this.disposed) return
    this.disposed = true
    this.resources.forEach((resource) => resource[Symbol.dispose]())
  }
}
