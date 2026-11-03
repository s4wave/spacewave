import type { MessageType } from '@aptre/protobuf-es-lite/message'

import type { ViewVersion } from './view.pb.js'

// Versioned is a view message that carries its version.
export interface Versioned {
  view?: ViewVersion
}

// ViewStore keeps encoded views across page loads.
export interface ViewStore {
  get(key: string): Promise<Uint8Array | undefined>
  put(key: string, data: Uint8Array): Promise<void>
}

// ApplyChanges applies a changes message to the view it follows, returning
// the whole view it makes.
export type ApplyChanges<T> = (held: T, changes: T) => T

// heldLimit is how many views HeldViews keeps in memory.
const heldLimit = 64

// HeldViews keeps the last whole view of each versioned watch by its watch
// key, past the stream and the connection that carried it. A watch reopens
// naming the version it holds; the backend answers an unchanged view with
// the version alone, and the reader receives the held view again.
// Construct one instance per authorization scope and discard it when access
// changes. Persistent stores must use a distinct account/Space namespace;
// only public views may use a namespace shared across sign-ins.
export class HeldViews {
  // views holds the latest views, oldest first.
  private readonly views = new Map<string, Versioned>()

  constructor(
    private readonly store?: ViewStore,
    private readonly limit = heldLimit,
  ) {}

  // resume follows the watch at key, starting it with the version held for
  // key. It yields each whole view, the held view for an unchanged answer,
  // and with apply the view a changes message makes from the held one. With
  // type, the view also persists in the store across loads.
  public async *resume<T extends Versioned>(
    key: string,
    start: (held: string, signal: AbortSignal) => AsyncIterable<T>,
    signal: AbortSignal,
    type?: MessageType<T>,
    apply?: ApplyChanges<T>,
  ): AsyncIterable<T> {
    let held = (this.views.get(key) ?? (await this.load(key, type))) as
      | T
      | undefined
    for await (const message of start(held?.view?.version ?? '', signal)) {
      // An unchanged answer names the held view at a later revision.
      let view = message
      if (message.view?.unchanged) {
        if (!held || held.view?.version !== message.view.version) {
          // The retry starts holding nothing.
          this.views.delete(key)
          throw new Error(`${key} answered unchanged for a view it never sent`)
        }
        const { unchanged: _, ...version } = message.view
        view = { ...held, view: version }
      } else if (message.view?.changes) {
        // Changes apply to the view the stream last delivered.
        if (!held || !apply) {
          this.views.delete(key)
          throw new Error(`${key} sent changes to a view it never sent`)
        }
        const { changes: _, ...version } = message.view
        view = { ...apply(held, message), view: version }
      }
      if (type && !message.view?.unchanged && view.view?.version)
        void this.save(key, view, type)
      if (view.view?.version) this.keep(key, view)
      held = view
      yield view
    }
  }

  // keep holds view for key, dropping the oldest past the limit.
  private keep(key: string, view: Versioned) {
    this.views.delete(key)
    this.views.set(key, view)
    if (this.views.size > this.limit)
      this.views.delete(this.views.keys().next().value!)
  }

  // load reads key's view from the store.
  private async load<T extends Versioned>(key: string, type?: MessageType<T>) {
    if (!type || !this.store) return undefined
    try {
      const data = await this.store.get(key)
      return data ? type.fromBinary(data) : undefined
    } catch {
      return undefined
    }
  }

  // save writes key's view to the store.
  private async save<T extends Versioned>(
    key: string,
    view: T,
    type: MessageType<T>,
  ) {
    try {
      await this.store?.put(key, type.toBinary(view))
    } catch {
      // A view the store cannot keep is fetched whole next load.
    }
  }
}

// indexedViews keeps views in the browser's IndexedDB, or nothing where it
// is unavailable.
export function indexedViews(name: string): ViewStore | undefined {
  // Open the database once, on first use.
  if (typeof indexedDB === 'undefined') return undefined
  let opened: Promise<IDBDatabase> | undefined
  const db = () =>
    (opened ??= new Promise((resolve, reject) => {
      const request = indexedDB.open(name, 1)
      request.onupgradeneeded = () => request.result.createObjectStore('views')
      request.onsuccess = () => resolve(request.result)
      request.onerror = () =>
        reject(request.error ?? new Error('IndexedDB request failed'))
    }))

  // run makes one request in its own transaction.
  const run = async <R>(
    mode: IDBTransactionMode,
    act: (views: IDBObjectStore) => IDBRequest<R>,
  ) => {
    const views = (await db()).transaction('views', mode).objectStore('views')
    return new Promise<R>((resolve, reject) => {
      const request = act(views)
      request.onsuccess = () => resolve(request.result)
      request.onerror = () =>
        reject(request.error ?? new Error('IndexedDB request failed'))
    })
  }

  // Each view is the encoded message under its watch key.
  return {
    get: async (key) => {
      const data = await run<unknown>('readonly', (views) => views.get(key))
      return data instanceof Uint8Array ? data : undefined
    },
    put: async (key, data) => {
      await run('readwrite', (views) => views.put(data, key))
    },
  }
}
