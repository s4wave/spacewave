import { castToError } from 'starpc'
import type { Client as ResourceClient } from '../resource/client.js'
import type { RegisterCleanup } from './useResource.js'

// sharedResourceGraceMs is how long a mount outlives its last lease, so a
// route change or tab switch that drops and re-takes a lease keeps the mount.
export const sharedResourceGraceMs = 5000

/** SharedParent is the live handle a shared mount is keyed under. */
export interface SharedParent {
  readonly id: number
  readonly released: boolean
}

/** SharedMount mounts the value of one shared entry. */
export type SharedMount<T> = (
  signal: AbortSignal,
  cleanup: RegisterCleanup,
) => Promise<T | null>

/** SharedResourceSnapshot is the published state of one shared entry. */
export interface SharedResourceSnapshot<T> {
  readonly value: T | null
  readonly error: Error | null
  readonly loading: boolean
}

// absentSnapshot is the snapshot of a key with no entry: a lease is about to
// start its mount.
const absentSnapshot: SharedResourceSnapshot<never> = {
  value: null,
  error: null,
  loading: true,
}

type Disposable = { [Symbol.dispose](): void }

// SharedRun is one in-flight mount and the handles it registered so far.
interface SharedRun {
  controller: AbortController
  cleanups: Disposable[]
}

// SharedEntry is the mount held under one key.
//
// The entry owns the handles its mount registered and disposes them when the
// value is replaced or the entry is deleted. Consumers borrow the value and
// never dispose it.
interface SharedEntry {
  key: string
  snapshot: SharedResourceSnapshot<unknown>
  leases: number
  generation: string
  mount: SharedMount<unknown> | null
  parent: SharedParent | null
  run: SharedRun | null
  owned: Disposable[]
  graceTimer: ReturnType<typeof setTimeout> | null
}

// isReleased reports whether value is a handle the server or client released.
function isReleased(value: unknown): boolean {
  return (
    typeof value === 'object' &&
    value !== null &&
    'released' in value &&
    value.released === true
  )
}

// disposeAll disposes handles child-before-parent: the reverse of the order
// a mount registered them.
function disposeAll(handles: Disposable[]): void {
  for (let i = handles.length - 1; i >= 0; i--) {
    handles[i][Symbol.dispose]()
  }
}

/**
 * sharedResourceKey returns the key of the entry named name under parent.
 * Keys chain through parent resource ids, so a remounted parent gives its
 * children new keys.
 */
export function sharedResourceKey(parent: SharedParent, name: string): string {
  return `${parent.id}/${name}`
}

/**
 * SharedResources holds mounts that several components use at once, such as
 * a Session open in two shell tabs. Each entry mounts once, stays while any
 * component leases it plus a short grace period, and remounts when the
 * server releases its value while leased.
 */
export class SharedResources {
  private readonly entries = new Map<string, SharedEntry>()
  // listenersByKey holds subscribers by key, so a subscription made before
  // its entry exists still hears the entry's changes.
  private readonly listenersByKey = new Map<string, Set<() => void>>()
  private unsubscribeReleased: (() => void) | null = null

  constructor(private readonly client: ResourceClient | null) {}

  /** subscribe calls cb whenever the snapshot of key changes. */
  subscribe(key: string, cb: () => void): () => void {
    let listeners = this.listenersByKey.get(key)
    if (!listeners) {
      listeners = new Set()
      this.listenersByKey.set(key, listeners)
    }
    listeners.add(cb)
    return () => {
      listeners.delete(cb)
      if (listeners.size === 0) {
        this.listenersByKey.delete(key)
      }
    }
  }

  /** getSnapshot returns the current snapshot of key. */
  getSnapshot<T>(key: string): SharedResourceSnapshot<T> {
    return (this.entries.get(key)?.snapshot ??
      absentSnapshot) as SharedResourceSnapshot<T>
  }

  /**
   * acquire leases key and returns the function that drops the lease. The
   * first lease starts mount. A non-empty generation that differs from the
   * entry's generation remounts the entry once, so every lessee that observed
   * the same change shares one remount.
   */
  acquire<T>(
    key: string,
    mount: SharedMount<T>,
    parent: SharedParent | null,
    generation = '',
  ): () => void {
    // Lease the entry, creating it on first use.
    let entry = this.entries.get(key)
    if (!entry) {
      entry = this.createEntry(key, generation)
    }
    entry.mount = mount as SharedMount<unknown>
    entry.parent = parent
    entry.leases++
    this.cancelGrace(entry)

    // Mount when nothing is mounted, the value was released, or the
    // generation moved on.
    const stale = generation !== '' && generation !== entry.generation
    entry.generation = generation || entry.generation
    const unmounted = !entry.run && entry.snapshot === absentSnapshot
    if (stale || unmounted || isReleased(entry.snapshot.value)) {
      this.start(entry)
    }

    // Drop the lease once, however often the caller releases it.
    let released = false
    return () => {
      if (released) return
      released = true
      this.releaseLease(entry)
    }
  }

  /** remount discards the value of key and mounts it again. */
  remount(key: string): void {
    const entry = this.entries.get(key)
    if (entry?.mount) {
      this.start(entry)
    }
  }

  /**
   * adopt moves a value mounted elsewhere into key, so the first lease uses it
   * instead of mounting. The entry owns handles from then on and disposes
   * them after the grace period unless a lease arrives. A key that is leased
   * or holds a live value keeps its entry and disposes the adopted handles,
   * since replacing a leased entry would drop its lessees' leases.
   */
  adopt(key: string, value: unknown, handles: Disposable[]): void {
    // Keep an entry that is leased or holds a live value.
    const existing = this.entries.get(key)
    const live = existing?.snapshot.value != null && !existing.run
    if (existing && (existing.leases > 0 || live)) {
      disposeAll(handles)
      return
    }

    // Replace an idle entry with the adopted value until a lease arrives.
    if (existing) {
      this.deleteEntry(existing)
    }
    const entry = this.createEntry(key, '')
    entry.owned = handles
    this.publish(entry, { value, error: null, loading: false })
    this.scheduleGrace(entry)
  }

  /** clear disposes every entry. The registry stays usable. */
  clear(): void {
    for (const entry of this.entries.values()) {
      this.deleteEntry(entry)
    }
  }

  // createEntry adds an empty entry for key.
  private createEntry(key: string, generation: string): SharedEntry {
    const entry: SharedEntry = {
      key,
      snapshot: absentSnapshot,
      leases: 0,
      generation,
      mount: null,
      parent: null,
      run: null,
      owned: [],
      graceTimer: null,
    }
    this.entries.set(key, entry)
    this.watchReleased()
    return entry
  }

  // start mounts entry, replacing any in-flight mount. The current value
  // stays visible with loading set until the new value replaces it.
  private start(entry: SharedEntry): void {
    // Cancel the mount this one replaces.
    const mount = entry.mount
    if (!mount) return
    this.abortRun(entry)

    // Collect the run's handles. A replaced run disposes late arrivals.
    const run: SharedRun = { controller: new AbortController(), cleanups: [] }
    entry.run = run
    const cleanup: RegisterCleanup = (handle) => {
      if (!handle) return handle
      if (entry.run !== run) {
        queueMicrotask(() => handle[Symbol.dispose]())
        return handle
      }
      run.cleanups.push(handle)
      return handle
    }

    // Show the current value as loading until the run settles.
    this.publish(entry, {
      value: entry.snapshot.value,
      error: null,
      loading: true,
    })

    // Run the mount.
    mount(run.controller.signal, cleanup).then(
      (value) => this.settle(entry, run, value, null),
      (err: unknown) => this.settle(entry, run, null, castToError(err)),
    )
  }

  // settle publishes the result of run unless a later mount replaced it.
  private settle(
    entry: SharedEntry,
    run: SharedRun,
    value: unknown,
    error: Error | null,
  ): void {
    // Discard the result of a replaced run.
    if (entry.run !== run) {
      disposeAll(run.cleanups)
      return
    }

    // Take ownership of a value's handles. An error keeps no handles.
    entry.run = null
    const previous = entry.owned
    entry.owned = error ? [] : run.cleanups
    if (error) {
      disposeAll(run.cleanups)
    }

    // Publish the result before disposing the value it replaces, so no
    // consumer renders a disposed value.
    this.publish(entry, { value: error ? null : value, error, loading: false })
    disposeAll(previous)

    // Release a result that nobody leases anymore.
    if (entry.leases === 0) {
      this.releaseIdle(entry)
    }
  }

  // releaseLease drops one lease and releases the entry when it was the last.
  private releaseLease(entry: SharedEntry): void {
    entry.leases--
    if (entry.leases === 0 && this.entries.get(entry.key) === entry) {
      this.releaseIdle(entry)
    }
  }

  // releaseIdle handles an entry with no leases: a value or an in-flight
  // mount waits out the grace period, but an empty or failed result goes
  // at once.
  private releaseIdle(entry: SharedEntry): void {
    if (entry.run || entry.snapshot.value !== null) {
      this.scheduleGrace(entry)
      return
    }
    this.deleteEntry(entry)
  }

  // scheduleGrace deletes entry after the grace period.
  private scheduleGrace(entry: SharedEntry): void {
    this.cancelGrace(entry)
    entry.graceTimer = setTimeout(() => {
      entry.graceTimer = null
      if (entry.leases === 0) {
        this.deleteEntry(entry)
      }
    }, sharedResourceGraceMs)
  }

  // cancelGrace stops a pending grace deletion.
  private cancelGrace(entry: SharedEntry): void {
    if (entry.graceTimer) {
      clearTimeout(entry.graceTimer)
      entry.graceTimer = null
    }
  }

  // abortRun cancels the in-flight mount of entry.
  private abortRun(entry: SharedEntry): void {
    // Detach the run, then abort it and dispose what it registered.
    const run = entry.run
    if (!run) return
    entry.run = null
    run.controller.abort()
    disposeAll(run.cleanups)
  }

  // deleteEntry disposes entry and removes it.
  private deleteEntry(entry: SharedEntry): void {
    // Stop the entry's timer and run.
    if (this.entries.get(entry.key) !== entry) return
    this.cancelGrace(entry)
    this.abortRun(entry)

    // Remove the entry and dispose its value after consumers see it go.
    this.entries.delete(entry.key)
    const owned = entry.owned
    entry.owned = []
    this.publish(entry, absentSnapshot)
    disposeAll(owned)

    // Stop following releases with no entries left.
    if (this.entries.size === 0) {
      this.unsubscribeReleased?.()
      this.unsubscribeReleased = null
    }
  }

  // publish replaces the snapshot of entry and notifies its subscribers.
  private publish(
    entry: SharedEntry,
    snapshot: SharedResourceSnapshot<unknown>,
  ): void {
    entry.snapshot = snapshot
    for (const listener of this.listenersByKey.get(entry.key) ?? []) {
      listener()
    }
  }

  // watchReleased follows resource releases while any entry exists.
  private watchReleased(): void {
    if (this.unsubscribeReleased || !this.client) return
    this.unsubscribeReleased = this.client.onResourceReleased(() => {
      this.onReleased()
    })
  }

  // onReleased remounts each leased entry whose value was released, and drops
  // unleased ones. An entry whose parent was released waits for its lessees
  // to move to the remounted parent's key.
  private onReleased(): void {
    for (const entry of this.entries.values()) {
      if (entry.run || !isReleased(entry.snapshot.value)) continue
      if (entry.leases === 0 || !entry.mount) {
        this.deleteEntry(entry)
        continue
      }
      if (entry.parent?.released) {
        this.publish(entry, { ...entry.snapshot, loading: true })
        continue
      }
      this.start(entry)
    }
  }
}
