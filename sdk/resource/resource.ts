import { ItState } from '@aptre/bldr'

import type { ClientResourceRef } from './client.js'

// ResourceDebugInfo contains debug information for a resource.
export interface ResourceDebugInfo {
  // label is a short label shown in the resource list.
  label?: string
  // details contains key-value pairs for the details panel.
  details?: Record<string, string | number | boolean | null>
}

// WatchableDebugInfo is a helper for resources with dynamic debug info.
export type WatchableDebugInfo = ReturnType<typeof createWatchableDebugInfo>

// createWatchableDebugInfo creates a helper for resources with dynamic debug info.
export function createWatchableDebugInfo(initial: ResourceDebugInfo = {}) {
  let current = initial
  const state = new ItState<ResourceDebugInfo>(() => Promise.resolve(current))

  return {
    get: () => current,
    set: (info: ResourceDebugInfo) => {
      current = info
      state.pushChangeEvent(info)
    },
    watch: () => state.getIterable(),
  }
}

// Resource is the base abstract class for a Resource type.
export abstract class Resource {
  constructor(public readonly resourceRef: ClientResourceRef) {}

  // Get the resource ID
  public get id() {
    return this.resourceRef.resourceId
  }

  // Get the underlying SRPC client for making RPC calls
  public get client() {
    return this.resourceRef.client
  }

  // Check if the resource has been released
  public get released() {
    return this.resourceRef.released
  }

  // release releases the resource.
  public release() {
    this.resourceRef.release()
  }

  // dispose releases the resource.
  // usage: using myResource = new MyResource()
  // the resource will be disposed when the scope is disposed.
  [Symbol.dispose]() {
    this.release()
  }

  // getDebugInfo returns debug information for devtools.
  // Override in subclasses to provide resource-specific debug info.
  public getDebugInfo?(): ResourceDebugInfo

  // watchDebugInfo returns an async iterable of debug info updates.
  // Override in subclasses that have dynamic debug info.
  public watchDebugInfo?(): AsyncIterable<ResourceDebugInfo>
}
