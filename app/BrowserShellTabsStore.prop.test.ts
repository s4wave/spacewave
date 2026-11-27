import { afterEach, describe, it } from 'vitest'

import {
  BROWSER_SHELL_TABS_STORAGE_KEY,
  BrowserShellTabsStore,
  BrowserShellTabsStoreError,
  type BrowserShellTabRecord,
  type BrowserShellTabsSnapshot,
} from './BrowserShellTabsStore.js'
import { DEFAULT_HOME_TAB, getTabNameFromPath } from './shell-tab.js'

// This file drives random valid event sequences against two Shell Tab stores
// that share one localStorage, as two browser tabs do, and checks each step
// against a reference model. A failing sequence is shrunk by deleting events
// while the failure persists, and the seed and shrunk sequence are printed.

const SEQUENCES = Number(process.env.PROP_SEQUENCES ?? 300)
const STEPS = Number(process.env.PROP_STEPS ?? 40)
const BASE_SEED = Number(process.env.PROP_SEED ?? 0x5eed)

const IDS = ['t0', 't1', 't2', 't3', 't4']
const PATHS = ['/', '/community', '/docs/a', '/u/1/settings/x', '/zz']

type Event =
  | {
      kind: 'create'
      writer: number
      versioned: boolean
      id: string
      path: string
      name?: string
      customName?: string
    }
  | {
      kind: 'updatePath'
      writer: number
      versioned: boolean
      id: string
      path: string
    }
  | {
      kind: 'updateName'
      writer: number
      versioned: boolean
      id: string
      name: string
    }
  | {
      kind: 'rename'
      writer: number
      versioned: boolean
      id: string
      customName?: string
    }
  | { kind: 'close'; writer: number; versioned: boolean; id: string }
  | {
      kind: 'reset'
      writer: number
      versioned: boolean
      id?: string
      path?: string
      customName?: string
    }

interface Model {
  epoch: number
  revision: number
  records: BrowserShellTabRecord[]
}

// mulberry32 is a small seeded PRNG, so a seed replays its sequence exactly.
function mulberry32(seed: number): () => number {
  let a = seed >>> 0
  return () => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = a
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

function pick<T>(rand: () => number, items: readonly T[]): T {
  return items[Math.floor(rand() * items.length)]
}

function genEvent(rand: () => number): Event {
  const writer = rand() < 0.5 ? 0 : 1
  const versioned = rand() < 0.3
  const id = pick(rand, IDS)
  const r = rand()
  if (r < 0.3) {
    return {
      kind: 'create',
      writer,
      versioned,
      id,
      path: pick(rand, PATHS),
      name: pick(rand, [undefined, '', 'Named']),
      customName: pick(rand, [undefined, '', 'Custom']),
    }
  }
  if (r < 0.45) {
    return {
      kind: 'updatePath',
      writer,
      versioned,
      id,
      path: pick(rand, PATHS),
    }
  }
  if (r < 0.55) {
    return {
      kind: 'updateName',
      writer,
      versioned,
      id,
      name: pick(rand, ['', 'Title']),
    }
  }
  if (r < 0.65) {
    return {
      kind: 'rename',
      writer,
      versioned,
      id,
      customName: pick(rand, [undefined, '', 'Renamed']),
    }
  }
  if (r < 0.93) {
    return { kind: 'close', writer, versioned, id }
  }
  return {
    kind: 'reset',
    writer,
    versioned,
    id: pick(rand, [undefined, ...IDS]),
    path: pick(rand, [undefined, ...PATHS]),
    customName: pick(rand, [undefined, '', 'Custom']),
  }
}

function genSequence(seed: number, steps: number): Event[] {
  const rand = mulberry32(seed)
  return Array.from({ length: steps }, () => genEvent(rand))
}

// applyModel returns the error code the store must raise, or applies the event
// to the model. freshId is the id the store chose for a replacement record,
// an id the model cannot predict.
function applyModel(
  model: Model,
  event: Event,
  stale: boolean,
  now: number,
): string | null {
  if (stale) return 'stale-version'
  const find = (id: string) => model.records.findIndex((r) => r.id === id)
  switch (event.kind) {
    case 'create': {
      if (find(event.id) >= 0) return 'id-collision'
      const seq = Math.max(0, ...model.records.map((r) => r.creationSequence))
      model.records = [
        ...model.records,
        {
          id: event.id,
          path: event.path,
          name: event.name || getTabNameFromPath(event.path),
          customName: event.customName || undefined,
          creationSequence: seq + 1,
        },
      ]
      model.revision += 1
      return null
    }
    case 'updatePath':
    case 'updateName':
    case 'rename': {
      const i = find(event.id)
      if (i < 0) return 'record-not-found'
      const r = { ...model.records[i] }
      if (event.kind === 'updatePath') {
        r.path = event.path
        r.name = getTabNameFromPath(event.path)
      } else if (event.kind === 'updateName') {
        r.name = event.name || getTabNameFromPath(r.path)
      } else {
        r.customName = event.customName || undefined
      }
      model.records = model.records.map((c, j) => (j === i ? r : c))
      model.revision += 1
      return null
    }
    case 'close': {
      const i = find(event.id)
      if (i < 0) return 'record-not-found'
      model.records =
        model.records.length === 1
          ? [
              {
                id: '*fresh*',
                path: DEFAULT_HOME_TAB.path,
                name: DEFAULT_HOME_TAB.name,
                creationSequence: 1,
              },
            ]
          : model.records.filter((_, j) => j !== i)
      model.revision += 1
      return null
    }
    case 'reset': {
      const path = event.path ?? DEFAULT_HOME_TAB.path
      model.records = [
        {
          id: event.id ?? `${DEFAULT_HOME_TAB.id}-${now}`,
          path,
          name: getTabNameFromPath(path),
          customName: event.customName || undefined,
          creationSequence: 1,
        },
      ]
      model.epoch += 1
      model.revision = 1
      return null
    }
  }
}

// outcomes counts each event kind and result the sequences reached, so a run
// shows that the rare paths (stale writes, closing the last tab) ran.
const outcomes = new Map<string, number>()

class SerialLocks {
  private tail = Promise.resolve()

  request<T>(
    _name: string,
    _options: LockOptions,
    callback: (lock: Lock) => T | PromiseLike<T>,
  ): Promise<Awaited<T>> {
    const run = this.tail.then(() => callback({} as Lock))
    this.tail = run.then(
      () => undefined,
      () => undefined,
    )
    return run as Promise<Awaited<T>>
  }
}

// canon drops undefined fields and sorts keys, so two values compare by
// content and not by key order.
function canon(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canon)
  if (value && typeof value === 'object') {
    return Object.fromEntries(
      Object.entries(value)
        .filter(([, v]) => v !== undefined)
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([k, v]) => [k, canon(v)]),
    )
  }
  return value
}

function same(a: unknown, b: unknown): boolean {
  return JSON.stringify(canon(a)) === JSON.stringify(canon(b))
}

function dispatch(
  store: BrowserShellTabsStore,
  event: Event,
): Promise<unknown> {
  const snap = store.getSnapshot()
  const version = event.versioned
    ? { epoch: snap.epoch, revision: snap.revision }
    : undefined
  switch (event.kind) {
    case 'create':
      return store.create(
        {
          id: event.id,
          path: event.path,
          name: event.name,
          customName: event.customName,
        },
        version,
      )
    case 'updatePath':
      return store.updatePath(event.id, event.path, version)
    case 'updateName':
      return store.updateName(event.id, event.name, version)
    case 'rename':
      return store.rename(event.id, event.customName, version)
    case 'close':
      return store.close(event.id, version)
    case 'reset':
      return store.reset(
        { id: event.id, path: event.path, customName: event.customName },
        version,
      )
  }
}

// runSequence runs one event sequence from empty storage and returns the first
// broken invariant, or null when every step holds.
async function runSequence(events: Event[]): Promise<string | null> {
  localStorage.clear()
  const now = 1000
  // stores[0] and stores[1] write, but stores[2] only reads, so each writer
  // keeps the view it last saw and a versioned write can be stale.
  const stores = [0, 1, 2].map(
    () =>
      new BrowserShellTabsStore({
        locks: new SerialLocks() as unknown as Pick<LockManager, 'request'>,
        storageEvents: false,
        now: () => now,
      }),
  )
  const model: Model = { epoch: 0, revision: 0, records: [] }

  for (let step = 0; step < events.length; step++) {
    const event = events[step]
    const writer = stores[event.writer]
    const seen = writer.getSnapshot()
    const before = localStorage.getItem(BROWSER_SHELL_TABS_STORAGE_KEY)
    const stale =
      event.versioned &&
      (seen.epoch !== model.epoch || seen.revision !== model.revision)
    const want = applyModel(model, event, stale, now)
    let got: string | null = null
    try {
      await dispatch(writer, event)
    } catch (error) {
      got =
        error instanceof BrowserShellTabsStoreError
          ? error.code
          : `unexpected ${String(error)}`
    }
    const outcome =
      event.kind === 'close' &&
      want === null &&
      model.records[0]?.id === '*fresh*'
        ? 'close-last'
        : `${event.kind}:${want ?? 'ok'}`
    outcomes.set(outcome, (outcomes.get(outcome) ?? 0) + 1)
    const at = `step ${step} ${JSON.stringify(event)}`
    if (got !== want) return `${at}: error ${got}, model expects ${want}`
    if (want !== null) {
      if (localStorage.getItem(BROWSER_SHELL_TABS_STORAGE_KEY) !== before) {
        return `${at}: rejected mutation changed storage`
      }
      continue
    }

    const raw = localStorage.getItem(BROWSER_SHELL_TABS_STORAGE_KEY)
    if (!raw) return `${at}: nothing stored`
    const stored = JSON.parse(raw) as Record<string, unknown>
    const published = writer.getSnapshot()
    if (!same(stored, published)) {
      return `${at}: stored keys ${Object.keys(stored).join(',')} differ from published ${Object.keys(published).join(',')}`
    }

    // Adopt the store's id for a replacement record the model cannot predict.
    const fresh = model.records.find((r) => r.id === '*fresh*')
    if (fresh) fresh.id = published.records[0]?.id ?? fresh.id

    const views: [string, BrowserShellTabsSnapshot][] = [
      ['writer', published],
      ['observer', stores[2].read()],
    ]
    for (const [i, snap] of views) {
      if (snap.epoch !== model.epoch || snap.revision !== model.revision) {
        return `${at}: store ${i} at ${snap.epoch}/${snap.revision}, model at ${model.epoch}/${model.revision}`
      }
      if (!same(snap.records, model.records)) {
        return `${at}: store ${i} records ${JSON.stringify(snap.records)} differ from model ${JSON.stringify(model.records)}`
      }
      if (snap.records.length === 0) return `${at}: store ${i} has no tab`
      const ids = new Set(snap.records.map((r) => r.id))
      const seqs = new Set(snap.records.map((r) => r.creationSequence))
      if (ids.size !== snap.records.length) return `${at}: duplicate id`
      if (seqs.size !== snap.records.length) {
        return `${at}: duplicate creationSequence`
      }
    }
  }
  return null
}

// shrink deletes chunks, then single events, while the sequence still fails.
async function shrink(events: Event[]): Promise<Event[]> {
  let current = events
  for (let chunk = Math.floor(current.length / 2); chunk >= 1;) {
    let removed = false
    for (let i = 0; i + chunk <= current.length;) {
      const candidate = [...current.slice(0, i), ...current.slice(i + chunk)]
      if (await runSequence(candidate)) {
        current = candidate
        removed = true
      } else {
        i += chunk
      }
    }
    if (!removed) chunk = Math.floor(chunk / 2)
  }
  return current
}

async function checkProperty(): Promise<void> {
  outcomes.clear()
  const start = performance.now()
  let steps = 0
  for (let n = 0; n < SEQUENCES; n++) {
    const seed = BASE_SEED + n
    const events = genSequence(seed, STEPS)
    const failure = await runSequence(events)
    steps += events.length
    if (failure) {
      const shrunk = await shrink(events)
      const last = await runSequence(shrunk)
      throw new Error(
        `seed ${seed} failed after ${n} passing sequences: ${failure}\n` +
          `shrunk to ${shrunk.length} events: ${JSON.stringify(shrunk)}\n` +
          `shrunk failure: ${last}`,
      )
    }
  }
  const ms = performance.now() - start
  console.log(
    `property: ${SEQUENCES} sequences, ${steps} events, ${ms.toFixed(0)} ms, seeds ${BASE_SEED}..${BASE_SEED + SEQUENCES - 1}\n` +
      JSON.stringify(Object.fromEntries([...outcomes].sort())),
  )
}

describe('BrowserShellTabsStore properties', () => {
  afterEach(() => {
    localStorage.clear()
  })

  // The stored JSON must equal the published snapshot, so other tabs receive
  // the snapshot and nothing beside it.
  it('agrees with the model and stores what it publishes', async () => {
    await checkProperty()
  }, 120_000)
})
