import {
  DEFAULT_HOME_TAB,
  generateTabId,
  getTabNameFromPath,
  type ShellTab,
} from '@s4wave/app/shell-tab.js'

export const BROWSER_SHELL_TABS_STORAGE_KEY = 'browser-shell-tabs'
export const BROWSER_SHELL_TABS_SCHEMA_VERSION = 1
export const BROWSER_SHELL_TABS_LOCK_NAME = 'spacewave-shell-tabs'

export interface BrowserShellTabRecord extends ShellTab {
  creationSequence: number
}

export interface BrowserShellTabsSnapshot {
  schemaVersion: number
  epoch: number
  revision: number
  records: BrowserShellTabRecord[]
}

export type BrowserShellTabsStoreErrorCode =
  | 'web-lock-unavailable'
  | 'stale-version'
  | 'invalid-snapshot'
  | 'storage-read'
  | 'storage-write'
  | 'quota'
  | 'id-collision'
  | 'record-not-found'

export class BrowserShellTabsStoreError extends Error {
  readonly code: BrowserShellTabsStoreErrorCode

  constructor(code: BrowserShellTabsStoreErrorCode, message: string) {
    super(message)
    this.name = 'BrowserShellTabsStoreError'
    this.code = code
  }
}

export interface BrowserShellTabsStoreOptions {
  key?: string
  storage?: Storage
  locks?: Pick<LockManager, 'request'>
  lockName?: string
  now?: () => number
}

export interface BrowserShellTabsMutationVersion {
  schemaVersion?: number
  epoch?: number
  revision?: number
}

export interface CreateBrowserShellTabInput {
  id: string
  path: string
  name?: string
  customName?: string
}

export interface ResetBrowserShellTabsOptions {
  id?: string
  path?: string
  name?: string
  customName?: string
}

const EMPTY_SNAPSHOT: BrowserShellTabsSnapshot = Object.freeze({
  schemaVersion: BROWSER_SHELL_TABS_SCHEMA_VERSION,
  epoch: 0,
  revision: 0,
  records: Object.freeze([]) as unknown as BrowserShellTabRecord[],
})

function cloneRecord(record: BrowserShellTabRecord): BrowserShellTabRecord {
  return { ...record }
}

function cloneSnapshot(
  snapshot: BrowserShellTabsSnapshot,
): BrowserShellTabsSnapshot {
  return {
    schemaVersion: snapshot.schemaVersion,
    epoch: snapshot.epoch,
    revision: snapshot.revision,
    records: snapshot.records.map(cloneRecord),
  }
}

function createFreshDefaultRecord(
  records: BrowserShellTabRecord[],
): BrowserShellTabRecord {
  const ids = new Set(records.map((record) => record.id))
  let id = generateTabId()
  let collision = 0
  while (ids.has(id)) {
    collision += 1
    id = `${generateTabId()}-${collision}`
  }
  return {
    id,
    path: DEFAULT_HOME_TAB.path,
    name: DEFAULT_HOME_TAB.name,
    creationSequence: 1,
  }
}

function emptySnapshot(): BrowserShellTabsSnapshot {
  return cloneSnapshot(EMPTY_SNAPSHOT)
}

function isRecord(value: unknown): value is BrowserShellTabRecord {
  if (!value || typeof value !== 'object') return false
  const record = value as Partial<BrowserShellTabRecord>
  return (
    typeof record.id === 'string' &&
    record.id.length > 0 &&
    typeof record.path === 'string' &&
    typeof record.name === 'string' &&
    record.name.length > 0 &&
    Number.isSafeInteger(record.creationSequence) &&
    (record.creationSequence as number) > 0 &&
    (record.customName === undefined || typeof record.customName === 'string')
  )
}

function parseSnapshot(value: string | null): BrowserShellTabsSnapshot {
  if (value === null) return emptySnapshot()

  let parsed: unknown
  try {
    parsed = JSON.parse(value)
  } catch {
    throw new BrowserShellTabsStoreError(
      'invalid-snapshot',
      'The browser Shell Tabs snapshot is not valid JSON.',
    )
  }

  if (!parsed || typeof parsed !== 'object') {
    throw new BrowserShellTabsStoreError(
      'invalid-snapshot',
      'The browser Shell Tabs snapshot is not an object.',
    )
  }
  const snapshot = parsed as Partial<BrowserShellTabsSnapshot>
  if (
    snapshot.schemaVersion !== BROWSER_SHELL_TABS_SCHEMA_VERSION ||
    !Number.isSafeInteger(snapshot.epoch) ||
    (snapshot.epoch as number) < 0 ||
    !Number.isSafeInteger(snapshot.revision) ||
    (snapshot.revision as number) < 0 ||
    !Array.isArray(snapshot.records)
  ) {
    throw new BrowserShellTabsStoreError(
      'stale-version',
      'The browser Shell Tabs snapshot has an incompatible schema or version.',
    )
  }

  const ids = new Set<string>()
  const creationSequences = new Set<number>()
  const records: BrowserShellTabRecord[] = []
  for (const record of snapshot.records) {
    if (!isRecord(record)) {
      throw new BrowserShellTabsStoreError(
        'invalid-snapshot',
        'The browser Shell Tabs snapshot contains an invalid record.',
      )
    }
    records.push(cloneRecord(record))
  }
  for (const record of records) {
    if (ids.has(record.id) || creationSequences.has(record.creationSequence)) {
      throw new BrowserShellTabsStoreError(
        'invalid-snapshot',
        'The browser Shell Tabs snapshot contains a duplicate record.',
      )
    }
    ids.add(record.id)
    creationSequences.add(record.creationSequence)
  }

  return {
    schemaVersion: snapshot.schemaVersion,
    epoch: snapshot.epoch as number,
    revision: snapshot.revision as number,
    records,
  }
}

function versionMatches(
  snapshot: BrowserShellTabsSnapshot,
  version: BrowserShellTabsMutationVersion | undefined,
): boolean {
  if (!version) return true
  return (
    (version.schemaVersion === undefined ||
      version.schemaVersion === snapshot.schemaVersion) &&
    (version.epoch === undefined || version.epoch === snapshot.epoch) &&
    (version.revision === undefined || version.revision === snapshot.revision)
  )
}

function isQuotaError(error: unknown): boolean {
  if (!error || typeof error !== 'object') return false
  const candidate = error as { name?: string; code?: number }
  return candidate.name === 'QuotaExceededError' || candidate.code === 22
}

function browserStorage(): Storage | undefined {
  try {
    return typeof localStorage === 'undefined' ? undefined : localStorage
  } catch {
    return undefined
  }
}

function browserLocks(): Pick<LockManager, 'request'> | undefined {
  try {
    return typeof navigator === 'undefined' ? undefined : navigator.locks
  } catch {
    return undefined
  }
}

export class BrowserShellTabsStore {
  readonly key: string
  readonly lockName: string
  private readonly storage: Storage | undefined
  private readonly locks: Pick<LockManager, 'request'> | undefined
  private readonly now: () => number
  private snapshot: BrowserShellTabsSnapshot
  private readonly listeners = new Set<() => void>()
  private storageListenerAttached = false

  constructor(options: BrowserShellTabsStoreOptions = {}) {
    this.key = options.key ?? BROWSER_SHELL_TABS_STORAGE_KEY
    this.lockName = options.lockName ?? BROWSER_SHELL_TABS_LOCK_NAME
    this.storage = options.storage ?? browserStorage()
    this.locks = options.locks ?? browserLocks()
    this.now = options.now ?? Date.now
    this.snapshot = this.readSnapshotForView()
  }

  getSnapshot = (): BrowserShellTabsSnapshot => this.snapshot

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener)
    this.attachStorageListener()
    return () => this.listeners.delete(listener)
  }

  read(): BrowserShellTabsSnapshot {
    this.snapshot = this.readSnapshot()
    return cloneSnapshot(this.snapshot)
  }

  getRecord(id: string): BrowserShellTabRecord | undefined {
    return this.snapshot.records.find((record) => record.id === id)
  }

  async create(
    input: CreateBrowserShellTabInput,
    version?: BrowserShellTabsMutationVersion,
  ): Promise<BrowserShellTabRecord> {
    return this.mutate(version, (current) => {
      if (current.records.some((record) => record.id === input.id)) {
        throw new BrowserShellTabsStoreError(
          'id-collision',
          `Shell Tab ID already exists: ${input.id}`,
        )
      }
      const nextSequence =
        current.records.reduce(
          (max, record) => Math.max(max, record.creationSequence),
          0,
        ) + 1
      const record: BrowserShellTabRecord = {
        id: input.id,
        path: input.path,
        name: input.name || getTabNameFromPath(input.path),
        customName: input.customName || undefined,
        creationSequence: nextSequence,
      }
      return {
        records: [...current.records, record],
        result: record,
      }
    })
  }

  async updatePath(
    id: string,
    path: string,
    version?: BrowserShellTabsMutationVersion,
  ): Promise<BrowserShellTabRecord> {
    return this.mutate(version, (current) => {
      const record = current.records.find((candidate) => candidate.id === id)
      if (!record) {
        throw new BrowserShellTabsStoreError(
          'record-not-found',
          `Shell Tab does not exist: ${id}`,
        )
      }
      const updated = {
        ...record,
        path,
        name: getTabNameFromPath(path),
      }
      return {
        records: current.records.map((candidate) =>
          candidate.id === id ? updated : candidate,
        ),
        result: updated,
      }
    })
  }

  async updateName(
    id: string,
    name: string,
    version?: BrowserShellTabsMutationVersion,
  ): Promise<BrowserShellTabRecord> {
    return this.mutate(version, (current) => {
      const record = current.records.find((candidate) => candidate.id === id)
      if (!record) {
        throw new BrowserShellTabsStoreError(
          'record-not-found',
          `Shell Tab does not exist: ${id}`,
        )
      }
      const updated = {
        ...record,
        name: name || getTabNameFromPath(record.path),
      }
      return {
        records: current.records.map((candidate) =>
          candidate.id === id ? updated : candidate,
        ),
        result: updated,
      }
    })
  }

  async rename(
    id: string,
    customName: string | undefined,
    version?: BrowserShellTabsMutationVersion,
  ): Promise<BrowserShellTabRecord> {
    return this.mutate(version, (current) => {
      const record = current.records.find((candidate) => candidate.id === id)
      if (!record) {
        throw new BrowserShellTabsStoreError(
          'record-not-found',
          `Shell Tab does not exist: ${id}`,
        )
      }
      const updated = { ...record, customName: customName || undefined }
      return {
        records: current.records.map((candidate) =>
          candidate.id === id ? updated : candidate,
        ),
        result: updated,
      }
    })
  }

  async close(
    id: string,
    version?: BrowserShellTabsMutationVersion,
  ): Promise<void> {
    await this.mutate(version, (current) => {
      if (!current.records.some((record) => record.id === id)) {
        throw new BrowserShellTabsStoreError(
          'record-not-found',
          `Shell Tab does not exist: ${id}`,
        )
      }
      const records =
        current.records.length === 1
          ? [createFreshDefaultRecord(current.records)]
          : current.records.filter((record) => record.id !== id)
      return {
        records,
        result: undefined,
      }
    })
  }

  async reset(
    options: ResetBrowserShellTabsOptions = {},
    version?: BrowserShellTabsMutationVersion,
  ): Promise<BrowserShellTabRecord> {
    return this.runMutation(version, (current) => {
      const id = options.id ?? `${DEFAULT_HOME_TAB.id}-${this.now()}`
      const record: BrowserShellTabRecord = {
        id,
        path: options.path ?? DEFAULT_HOME_TAB.path,
        name:
          options.name ??
          getTabNameFromPath(options.path ?? DEFAULT_HOME_TAB.path),
        customName: options.customName || undefined,
        creationSequence: 1,
      }
      return {
        schemaVersion: current.schemaVersion,
        epoch: current.epoch + 1,
        revision: 1,
        records: [record],
        result: record,
      }
    })
  }

  private async mutate<T>(
    version: BrowserShellTabsMutationVersion | undefined,
    transition: (current: BrowserShellTabsSnapshot) => {
      records: BrowserShellTabRecord[]
      result: T
    },
  ): Promise<T> {
    return this.runMutation(version, (current) => {
      const next = transition(current)
      return {
        schemaVersion: current.schemaVersion,
        epoch: current.epoch,
        revision: current.revision + 1,
        records: next.records,
        result: next.result,
      }
    })
  }

  private async runMutation<T>(
    version: BrowserShellTabsMutationVersion | undefined,
    transition: (current: BrowserShellTabsSnapshot) => {
      schemaVersion: number
      epoch: number
      revision: number
      records: BrowserShellTabRecord[]
      result: T
    },
  ): Promise<T> {
    const locks = this.locks ?? browserLocks()
    if (!locks || typeof locks.request !== 'function') {
      throw new BrowserShellTabsStoreError(
        'web-lock-unavailable',
        'Web Locks are required for Shell Tab mutations.',
      )
    }

    return locks.request(this.lockName, { mode: 'exclusive' }, (lock) => {
      if (!lock) {
        throw new BrowserShellTabsStoreError(
          'web-lock-unavailable',
          'The Shell Tab Web Lock is unavailable.',
        )
      }
      const current = this.readSnapshot()
      if (!versionMatches(current, version)) {
        throw new BrowserShellTabsStoreError(
          'stale-version',
          'The Shell Tab snapshot changed before this mutation committed.',
        )
      }
      const next = transition(current)
      const serialized = JSON.stringify(next)
      try {
        if (!this.storage) throw new Error('localStorage is unavailable')
        this.storage.setItem(this.key, serialized)
      } catch (error) {
        throw new BrowserShellTabsStoreError(
          isQuotaError(error) ? 'quota' : 'storage-write',
          isQuotaError(error)
            ? 'The browser Shell Tab storage quota was exceeded.'
            : 'The browser Shell Tab snapshot could not be written.',
        )
      }
      this.snapshot = cloneSnapshot(next)
      this.publish()
      return next.result
    })
  }

  private readSnapshotForView(): BrowserShellTabsSnapshot {
    try {
      return this.readSnapshot()
    } catch {
      return emptySnapshot()
    }
  }

  private readSnapshot(): BrowserShellTabsSnapshot {
    if (!this.storage) {
      throw new BrowserShellTabsStoreError(
        'storage-read',
        'Browser Shell Tab storage is unavailable.',
      )
    }
    let raw: string | null
    try {
      raw = this.storage.getItem(this.key)
    } catch {
      throw new BrowserShellTabsStoreError(
        'storage-read',
        'The browser Shell Tab snapshot could not be read.',
      )
    }
    return parseSnapshot(raw)
  }

  private attachStorageListener(): void {
    if (this.storageListenerAttached || typeof window === 'undefined') return
    this.storageListenerAttached = true
    window.addEventListener('storage', this.handleStorageEvent)
  }

  private readonly handleStorageEvent = (event: StorageEvent): void => {
    if (event.key !== this.key) return
    let next: BrowserShellTabsSnapshot
    try {
      next = parseSnapshot(event.newValue)
    } catch {
      return
    }
    const current = this.snapshot
    if (
      next.schemaVersion !== current.schemaVersion ||
      next.epoch < current.epoch ||
      (next.epoch === current.epoch && next.revision <= current.revision)
    ) {
      return
    }
    this.snapshot = next
    this.publish()
  }

  private publish(): void {
    for (const listener of this.listeners) listener()
  }
}

let defaultStore: BrowserShellTabsStore | undefined

export function getBrowserShellTabsStore(): BrowserShellTabsStore {
  return (defaultStore ??= new BrowserShellTabsStore())
}

export function resetBrowserShellTabsStoreForTests(): void {
  defaultStore = undefined
}
