import { describe, expect, it, vi } from 'vitest'

import {
  BootMark,
  BootReport,
  BootReportState,
  BootValidationViolationKind,
} from './report.pb.js'

// The red-confirmed gate landed these durability tests before any store
// implementation existed. They load ./store.js through requireStoreModule so
// a missing implementation names the missing durability behavior instead of
// failing on an unresolved module import.
// The computed specifier keeps Vite from analyzing this import during
// transformation; resolution then fails inside the guarded call at test time.
const storeModuleSpecifier = './store.js'

async function requireStoreModule(
  behavior: string,
): Promise<any> {
  try {
    return await import(/* @vite-ignore */ storeModuleSpecifier)
  } catch (cause) {
    throw new Error(
      `missing BootReportStore durability behavior (${behavior}): ` +
        'bldr/web/boot/store.ts is not implemented yet',
      { cause },
    )
  }
}

// The fake IndexedDB implements the native request/transaction callback
// surface only: requests settle through onsuccess/onerror, transactions
// settle through oncomplete/onabort/onerror, and requests expose no
// promise interface. This forces the production wrappers to own the
// request-to-promise conversion and transaction-completion discipline.

interface FakeIDBEvent {
  target: unknown
}

type RequestCompletion = () => void

class FakeIDBRequest<T> {
  result!: T
  error: Error | null = null
  onsuccess: ((event: FakeIDBEvent) => unknown) | null = null
  onerror: ((event: FakeIDBEvent) => unknown) | null = null

  // _onSettled is invoked by the owning transaction once this request has
  // been executed, so the transaction can count down to completion.
  _onSettled: RequestCompletion | null = null

  constructor(
    private readonly transaction: FakeIDBTransaction,
    private readonly produce: () => T,
  ) {}

  _execute(): void {
    try {
      this.result = this.produce()
      this.onsuccess?.({ target: this })
    } catch (cause) {
      this.error = cause as Error
      this.onerror?.({ target: this })
    }
    this.transaction._requestSettled()
  }
}

export class FakeIDBOpenRequest {
  result!: FakeIDatabase
  error: Error | null = null
  onupgradeneeded: ((event: FakeIDBEvent) => unknown) | null = null
  onerror: ((event: FakeIDBEvent) => unknown) | null = null
  onsuccess: ((event: FakeIDBEvent) => unknown) | null = null
}

class FakeIDBObjectStore {
  readonly indexes = new Map<string, string>()

  constructor(
    private readonly database: FakeIDatabase,
    private readonly transaction: FakeIDBTransaction,
    readonly name: string,
    readonly keyPath: string,
    readonly records: Map<string, Record<string, unknown>>,
  ) {}

  createIndex(name: string, keyPath: string): void {
    this.indexes.set(name, keyPath)
  }

  index(name: string): {
    getAll: (value?: unknown) => FakeIDBRequest<Record<string, unknown>[]>
  } {
    const keyPath = this.indexes.get(name)
    if (keyPath === undefined) {
      throw new Error(`no such index ${name} on ${this.name}`)
    }
    return {
      getAll: (value?: unknown) =>
        this.transaction._track(() =>
          [...this.records.values()].filter(
            (record) => value === undefined || record[keyPath] === value,
          ),
        ),
    }
  }

  get(key: string): FakeIDBRequest<Record<string, unknown> | undefined> {
    return this.transaction._track(() => this.records.get(key))
  }

  getAll(): FakeIDBRequest<Record<string, unknown>[]> {
    return this.transaction._track(() => [...this.records.values()])
  }

  put(value: Record<string, unknown>): FakeIDBRequest<string> {
    const key = value[this.keyPath] as string
    return this.transaction._track(
      () => {
        if (this.database.factory.putFailure) throw this.database.factory.putFailure
        this.records.set(key, value)
        return key
      },
      { key },
    )
  }

  delete(key: string): FakeIDBRequest<undefined> {
    return this.transaction._track(
      () => {
        this.records.delete(key)
        return undefined
      },
      { key },
    )
  }
}

class FakeIDBTransaction {
  oncomplete: ((event: FakeIDBEvent) => unknown) | null = null
  onabort: ((event: FakeIDBEvent) => unknown) | null = null
  onerror: ((event: FakeIDBEvent) => unknown) | null = null

  private outstanding = 0
  private finished = false

  constructor(
    private readonly database: FakeIDatabase,
    readonly mode: 'readonly' | 'readwrite',
  ) {}

  objectStore(name: string): FakeIDBObjectStore {
    if (this.finished) throw new Error('transaction already finished')
    const store = this.database.stores.get(name)
    if (!store) throw new Error(`no such object store ${name}`)
    return new FakeIDBObjectStore(
      this.database,
      this,
      store.name,
      store.keyPath,
      store.records,
    )
  }

  // Abort marks the transaction so completion rejects through onabort even
  // after every individual request succeeded.
  abort(): void {
    this.database.commitFailure = true
  }

  _track<T>(produce: () => T, meta?: { key?: string }): FakeIDBRequest<T> {
    const request = new FakeIDBRequest<T>(this, produce)
    if (this.database.factory.holdPredicate?.(meta)) {
      // Held requests stay unsettled until the factory releases them, which
      // lets tests discriminate serialized execution from concurrent
      // execution.
      this.database.factory.held.push({
        execute: () => request._execute(),
        settle: () => {
          this.outstanding += 1
          queueMicrotask(() => request._execute())
        },
      })
      return request
    }
    this.outstanding += 1
    queueMicrotask(() => request._execute())
    return request
  }

  _requestSettled(): void {
    this.outstanding -= 1
    if (this.outstanding <= 0 && !this.finished) {
      this.finished = true
      queueMicrotask(() => this._settle())
    }
  }

  _settle(): void {
    if (this.database.factory.commitFailure) {
      const error = new Error('fake indexeddb transaction aborted')
      this.onabort?.({ target: this })
      this.onerror?.({ target: this })
      void error
      return
    }
    this.oncomplete?.({ target: this })
  }
}

class FakeIDatabase {
  readonly stores = new Map<
    string,
    { name: string; keyPath: string; records: Map<string, Record<string, unknown>> }
  >()
  putFailure: Error | null = null
  commitFailure = false

  constructor(
    readonly name: string,
    public version: number,
    readonly factory: FakeIDBFactory,
  ) {}

  createObjectStore(
    name: string,
    options: { keyPath: string },
  ): {
    name: string
    keyPath: string
    records: Map<string, Record<string, unknown>>
    indexes: Map<string, string>
    createIndex(indexName: string, indexKeyPath: string): void
  } {
    const store = {
      name,
      keyPath: options.keyPath,
      records: new Map<string, Record<string, unknown>>(),
      indexes: new Map<string, string>(),
      createIndex(indexName: string, indexKeyPath: string): void {
        store.indexes.set(indexName, indexKeyPath)
      },
    }
    this.stores.set(name, store)
    return store
  }

  // Transaction mirrors the native IDBDatabase signature and rejects an
  // unknown store name so a mode-only caller cannot regress silently.
  transactionCalls: Array<{
    storeName: string
    mode: 'readonly' | 'readwrite'
  }> = []

  transaction(
    storeName: string,
    mode: 'readonly' | 'readwrite',
  ): FakeIDBTransaction {
    if (!this.stores.has(storeName)) {
      throw new Error(`no such object store ${storeName}`)
    }
    this.transactionCalls.push({ storeName, mode })
    return new FakeIDBTransaction(this, mode)
  }
}

class FakeIDBFactory {
  readonly databases = new Map<string, FakeIDatabase>()
  putFailure: Error | null = null
  commitFailure = false
  holdPredicate:
    | ((meta?: { key?: string }) => boolean)
    | null = null
  readonly held: Array<{
    execute: () => void
    settle: () => void
  }> = []

  open(name: string, version?: number): FakeIDBOpenRequest {
    const requestedVersion = version ?? 1
    const request = new FakeIDBOpenRequest()
    queueMicrotask(() => {
      let database = this.databases.get(name)
      if (database && database.version > requestedVersion) {
        const failure = new Error(
          `The requested version (${requestedVersion}) is less than ` +
            `the existing version (${database.version}).`,
        )
        failure.name = 'VersionError'
        request.error = failure
        request.onerror?.({ target: request })
        return
      }
      const upgrading = !database || database.version < requestedVersion
      if (!database) {
        database = new FakeIDatabase(name, requestedVersion, this)
        this.databases.set(name, database)
      } else {
        database.version = requestedVersion
      }
      if (upgrading) {
        request.result = database
        request.onupgradeneeded?.({ target: request })
      }
      request.result = database
      request.onsuccess?.({ target: request })
    })
    return request
  }

  seedDatabaseAtVersion(
    name: string,
    version: number,
    configure: (database: FakeIDatabase) => void,
  ): FakeIDatabase {
    const database = new FakeIDatabase(name, version, this)
    this.databases.set(name, database)
    configure(database)
    return database
  }

  // Release every held request in FIFO order.
  settleHeld(): void {
    const held = [...this.held]
    this.held.length = 0
    for (const entry of held) entry.execute()
    for (const entry of held) entry.settle()
  }
}

function lockManagerAcquirable(): LockManager {
  const manager = {
    // With ifAvailable an acquirable lock hands the lock object to the
    // callback, proving the previous holder died.
    request: async (
      name: string,
      _options: unknown,
      callback: (lock: unknown) => unknown,
    ) => callback({ name, mode: 'exclusive' }),
  }
  return manager as unknown as LockManager
}

function lockManagerHeldElsewhere(): LockManager {
  const manager = {
    // With ifAvailable a held lock hands null to the callback, proving
    // another live tab owns the boot.
    request: async (
      _name: string,
      _options: unknown,
      callback: (lock: unknown) => unknown,
    ) => callback(null),
  }
  return manager as unknown as LockManager
}

// lockManagerRejecting models a manager whose request promise rejects,
// exercising holdBootLock's failure cleanup without granting anything.
function lockManagerRejecting(): LockManager {
  const manager = {
    request: (
      _name: string,
      _options: unknown,
      _callback: (lock: unknown) => unknown,
    ) => Promise.reject(new Error('lock manager unavailable')),
  }
  return manager as unknown as LockManager
}

// lockManagerDenied denies every request asynchronously the way a native
// manager reports held-elsewhere.
function lockManagerDenied(): LockManager {
  const manager = {
    request: (
      _name: string,
      _options: unknown,
      callback: (lock: unknown) => unknown,
    ) => Promise.resolve().then(() => callback(null)),
  }
  return manager as unknown as LockManager
}

// lockManagerShared is one minimal exclusive Web Locks stand-in shared by
// every store in a test: a name stays held until the holder's release promise
// settles, and contending requests observe null asynchronously exactly like
// the native manager.
function lockManagerShared(): LockManager {
  const held = new Map<string, boolean>()
  const manager = {
    request: (
      name: string,
      _options: unknown,
      callback: (lock: unknown) => unknown,
    ) =>
      Promise.resolve().then(() => {
        if (held.has(name)) {
          callback(null)
          return undefined
        }
        held.set(name, true)
        return Promise.resolve(callback({ name, mode: 'exclusive' })).then(
          (result) => {
            held.delete(name)
            return result
          },
        )
      }),
  }
  return manager as unknown as LockManager
}

function recordingReport(reportId: string, labels: string[]): BootReport {
  return BootReport.create({
    schemaVersion: 1,
    reportId,
    startedUnixMicros: 1_000_000n,
    entrypointId: 'drive',
    usableMark: 'boot-status.app',
    state: BootReportState.RECORDING,
    marks: labels.map((label, index): BootMark => ({
      sequence: BigInt(index + 1),
      label,
    })),
  })
}

interface InlineShellGlobals {
  __swStartupMarks?:
    | {
        name: string
        label: string
        sequence: number
        detail: Record<string, unknown>
      }[]
  __swStartupMarkOverflows?: number
  __swBootReport?: unknown
}

async function openStore(options: {
  idbFactory: IDBFactory
  locks?: LockManager | null
}): Promise<any> {
  const storeModule = await requireStoreModule('openBootReportStore')
  return storeModule.openBootReportStore(options)
}

// waitForStoreRecord polls the store until the collector's asynchronous
// attach/journal path has persisted at least one record.
async function waitForStoreRecord(
  store: { list(): Promise<unknown[]> },
): Promise<unknown[]> {
  for (let attempt = 0; attempt < 50; attempt++) {
    const listed = await store.list()
    if (listed.length > 0) return listed
    await new Promise((resolve) => setTimeout(resolve, 10))
  }
  return []
}

describe('BootReportStore durability', () => {
  it('opens every transaction on the reports store with the native signature', async () => {
    const factory = new FakeIDBFactory()
    const store = await openStore({
      idbFactory: factory as unknown as IDBFactory,
    })
    const report = recordingReport('boot-report-txn', ['boot.started'])
    await store.put(report)
    await store.get('boot-report-txn')
    await store.list()
    await store.delete('boot-report-txn')

    // Native IDBDatabase.transaction takes (storeNames, mode); a mode-only
    // call asks the browser for a store literally named "readwrite".
    expect(
      factory.databases.get('bldr-boot-reports')!.transactionCalls,
    ).toEqual([
      { storeName: 'reports', mode: 'readwrite' },
      { storeName: 'reports', mode: 'readonly' },
      { storeName: 'reports', mode: 'readonly' },
      { storeName: 'reports', mode: 'readwrite' },
    ])
  })

  it('fails closed against a newer schema version without resetting data', async () => {
    const storeModule = await requireStoreModule(
      'unknown/newer schema version fails closed without destructive reset',
    )

    const factory = new FakeIDBFactory()
    factory.seedDatabaseAtVersion(
      'bldr-boot-reports',
      99,
      (database) => {
        const store = database.createObjectStore('reports', {
          keyPath: 'reportId',
        })
        store.createIndex('state', 'state')
        store.createIndex('sealedAt', 'sealedAt')
        const future = recordingReport('boot-report-future', ['boot.started'])
        store.records.set(
          'boot-report-future',
          future as unknown as Record<string, unknown>,
        )
      },
    )

    await expect(
      storeModule.openBootReportStore({
        idbFactory: factory as unknown as IDBFactory,
      }),
    ).rejects.toThrow(/version/i)

    const surviving = factory.databases.get('bldr-boot-reports')
    expect(surviving?.version).toBe(99)
    expect(surviving?.stores.get('reports')?.records.size).toBe(1)
  })

  it('marks stale RECORDING aborted when the crash released its web lock', async () => {
    const storeModule = await requireStoreModule(
      'crash-released web lock recovers stale RECORDING to ABORTED',
    )

    const factory = new FakeIDBFactory()
    const writer = await openStore({
      idbFactory: factory as unknown as IDBFactory,
      locks: lockManagerAcquirable(),
    })
    const prior = recordingReport('boot-report-deadtab', ['boot.started'])
    await writer.put(prior)

    const recovery = await storeModule.openBootReportStore({
      idbFactory: factory as unknown as IDBFactory,
      locks: lockManagerAcquirable(),
    })
    await recovery.recoverOnStartup()

    const recovered = await recovery.get('boot-report-deadtab')
    expect(recovered?.state).toBe(BootReportState.ABORTED)
    // The exact prior partial timeline survives for recovery reading.
    expect(recovered?.marks.map((mark: BootMark) => mark.label)).toEqual([
      'boot.started',
    ])
  })

  it('keeps a live tab recording and preserves its partial timeline across reload', async () => {
    const storeModule = await requireStoreModule(
      'live tab stays RECORDING untouched while its web lock is held',
    )

    const factory = new FakeIDBFactory()
    const liveTab = await openStore({
      idbFactory: factory as unknown as IDBFactory,
      locks: lockManagerHeldElsewhere(),
    })
    const partial = recordingReport('boot-report-livetab', [
      'boot.started',
      'content-ready',
    ])
    await liveTab.put(partial)

    const recovery = await storeModule.openBootReportStore({
      idbFactory: factory as unknown as IDBFactory,
      locks: lockManagerAcquirable(),
    })
    await recovery.recoverOnStartup()

    const recovered = await recovery.get('boot-report-livetab')
    expect(recovered?.state).toBe(BootReportState.RECORDING)

    // A reload reopens the same database and reads the exact prior partial
    // timeline back without mutation.
    const reopened = await storeModule.openBootReportStore({
      idbFactory: factory as unknown as IDBFactory,
      locks: lockManagerAcquirable(),
    })
    const readback = await reopened.get('boot-report-livetab')
    expect(readback?.state).toBe(BootReportState.RECORDING)
    expect(readback?.marks.map((mark: BootMark) => mark.label)).toEqual([
      'boot.started',
      'content-ready',
    ])
  })

  it('surfaces quota failures instead of dropping writes', async () => {
    await requireStoreModule(
      'store/quota errors surface instead of silently dropping writes',
    )

    const factory = new FakeIDBFactory()
    factory.putFailure = Object.assign(new Error('write failed'), {
      name: 'QuotaExceededError',
    })

    const store = await openStore({
      idbFactory: factory as unknown as IDBFactory,
    })
    await expect(
      store.put(recordingReport('boot-report-quota', ['boot.started'])),
    ).rejects.toMatchObject({ name: 'QuotaExceededError' })
  })

  it('rejects a write when the transaction aborts after request success', async () => {
    await requireStoreModule(
      'transaction abort after request success rejects the operation',
    )
    const factory = new FakeIDBFactory()
    factory.commitFailure = true
    const store = await openStore({
      idbFactory: factory as unknown as IDBFactory,
    })
    await expect(
      store.put(recordingReport('boot-report-abort', ['boot.started'])),
    ).rejects.toThrow(/abort/i)
  })

  it('serializes concurrent writes in schedule order', async () => {
    await requireStoreModule(
      'serialized writer applies writes one at a time in schedule order',
    )
    const factory = new FakeIDBFactory()
    // Hold only the first scheduled write: a concurrent (non-serialized)
    // implementation would land writes two and three first and the record
    // order would come out reversed. The serialized writer must not issue
    // the later operations until the held one commits.
    const ids = ['boot-report-s1', 'boot-report-s2', 'boot-report-s3']
    factory.holdPredicate = (meta) => meta?.key === 'boot-report-s1' 
    const store = await openStore({
      idbFactory: factory as unknown as IDBFactory,
    })
    const pending = ids.map((reportId) =>
      store.put(recordingReport(reportId, ['boot.started'])),
    )
    // Let the serialized writer schedule the first operation only.
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(factory.held.length).toBe(1)
    factory.settleHeld()
    await Promise.all(pending)

    const keys = [
      ...factory.databases.get('bldr-boot-reports')!.stores
        .get('reports')!.records.keys(),
    ]
    expect(keys).toEqual(ids)
  })

  it('seals a report atomically in one terminal write', async () => {
    await requireStoreModule(
      'seal atomicity writes one complete terminal record',
    )
    const factory = new FakeIDBFactory()
    const store = await openStore({
      idbFactory: factory as unknown as IDBFactory,
    })
    await store.put(recordingReport('boot-report-seal', ['boot.started']))

    const sealed = await store.sealReport(
      recordingReport('boot-report-seal', ['boot.started']),
      BootReportState.READY,
    )
    expect(sealed.state).toBe(BootReportState.READY)
    expect(sealed.sealedAt).toBeDefined()

    const records = factory.databases
      .get('bldr-boot-reports')
      ?.stores.get('reports')?.records
    expect(records?.size).toBe(1)
    const readback = await store.get('boot-report-seal')
    expect(readback?.state).toBe(BootReportState.READY)
    expect(readback?.sealedAt).toBeDefined()
  })

  it('applies retention limits without evicting RECORDING or newest FAILED', async () => {
    await requireStoreModule(
      'retention keeps the newest sealed reports and protects live boots and the newest FAILED',
    )
    const factory = new FakeIDBFactory()
    const store = await openStore({
      idbFactory: factory as unknown as IDBFactory,
    })
    for (let index = 1; index <= 101; index += 1) {
      const ready = recordingReport(`boot-report-ready-${index}`, [
        'boot.started',
      ])
      ready.state = BootReportState.READY
      ;(ready as { sealedAt?: bigint }).sealedAt = BigInt(1000 + index)
      await store.put(ready)
    }
    const failedOld = recordingReport('boot-report-failed-old', ['boot.started'])
    failedOld.state = BootReportState.FAILED
    ;(failedOld as { sealedAt?: bigint }).sealedAt = 0n
    await store.put(failedOld)
    const failedNew = recordingReport('boot-report-failed-new', ['boot.started'])
    failedNew.state = BootReportState.FAILED
    ;(failedNew as { sealedAt?: bigint }).sealedAt = 5000n
    await store.put(failedNew)
    const live = recordingReport('boot-report-recording', ['boot.started'])
    await store.put(live)

    const result = await store.applyRetention()

    const survivors = await store.list()
    expect(survivors.some((r: BootReport) => r.reportId === 'boot-report-recording')).toBe(
      true,
    )
    expect(survivors.some((r: BootReport) => r.reportId === 'boot-report-failed-new')).toBe(
      true,
    )
    expect(survivors.some((r: BootReport) => r.reportId === 'boot-report-failed-old')).toBe(
      false,
    )
    expect(survivors.some((r: BootReport) => r.reportId === 'boot-report-ready-1')).toBe(
      false,
    )
    const terminals = survivors.filter((r: BootReport) => r.state !== BootReportState.RECORDING)
    expect(terminals.length).toBeLessThanOrEqual(100)
    expect(result.evicted.length).toBeGreaterThanOrEqual(2)
    const evictions = result.evicted as Array<{
      reportId: string
      reason: string
    }>
    for (const eviction of evictions) {
      expect(eviction.reason.length).toBeGreaterThan(0)
    }
  })

  it('uses encoded byte length for the body budget across multibyte content', async () => {
    await requireStoreModule(
      'body budget counts encoded bytes, not characters',
    )
    const factory = new FakeIDBFactory()
    const store = await openStore({
      idbFactory: factory as unknown as IDBFactory,
    })

    // A multibyte-heavy label encodes to more bytes than its character
    // count suggests; the budget must measure the encoded body.
    const wide = recordingReport('boot-report-wide', [
      '\u{1F600}'.repeat(24),
    ])
    wide.state = BootReportState.READY
    ;(wide as { sealedAt?: bigint }).sealedAt = 10n
    await store.put(wide)
    const small = recordingReport('boot-report-small', ['ok'])
    small.state = BootReportState.READY
    ;(small as { sealedAt?: bigint }).sealedAt = 11n
    await store.put(small)

    const wideBytes = (
      await import(/* @vite-ignore */ './report.pb.js')
    ).BootReport.toBinary(wide).byteLength
    const smallBytes = (
      await import(/* @vite-ignore */ './report.pb.js')
    ).BootReport.toBinary(small).byteLength
    expect(wideBytes).toBeGreaterThan(smallBytes)

    const result = await store.applyRetention({
      sealedLimit: 2,
      bodyBudgetBytes: Math.floor((wideBytes + smallBytes) / 2),
    })
    const survivors = await store.list()
    expect(survivors.some((r: BootReport) => r.reportId === 'boot-report-small')).toBe(
      true,
    )
    const evictedIds = result.evicted.map((eviction: { reportId: string; reason: string }) => eviction.reportId)
    expect(evictedIds).toContain('boot-report-wide')
    const evictions = result.evicted as Array<{
      reportId: string
      reason: string
    }>
    for (const eviction of evictions) {
      expect(eviction.reason).toBe('body-budget')
    }
  })

  it('never evicts the newest FAILED even when it exceeds the whole body budget', async () => {
    await requireStoreModule(
      'newest FAILED over budget is protected while other sealed reports are evicted',
    )
    const factory = new FakeIDBFactory()
    const store = await openStore({
      idbFactory: factory as unknown as IDBFactory,
    })

    const hugeFailed = recordingReport('boot-report-failed-huge', [
      'x'.repeat(512),
    ])
    hugeFailed.state = BootReportState.FAILED
    ;(hugeFailed as { sealedAt?: bigint }).sealedAt = 9007199254740995n
    await store.put(hugeFailed)

    const oldReady = recordingReport('boot-report-old-ready', ['old'])
    oldReady.state = BootReportState.READY
    ;(oldReady as { sealedAt?: bigint }).sealedAt = 9007199254740993n
    await store.put(oldReady)

    const result = await store.applyRetention({
      sealedLimit: 1,
      bodyBudgetBytes: 64,
    })

    const survivors = await store.list()
    expect(survivors.map((r: BootReport) => r.reportId)).toEqual([
      'boot-report-failed-huge',
    ])
    expect(result.evicted.map((eviction: { reportId: string; reason: string }) => eviction.reportId)).toEqual([
      'boot-report-old-ready',
    ])
    expect(result.evicted[0]?.reason).toBe('sealed-limit')
  })

  it('orders retention recency with BigInt-safe comparisons', async () => {
    await requireStoreModule(
      'retention recency compares sealed timestamps beyond the safe integer range',
    )
    const factory = new FakeIDBFactory()
    const store = await openStore({
      idbFactory: factory as unknown as IDBFactory,
    })

    const bigBase = 9007199254740990n
    const older = recordingReport('boot-report-big-old', ['older'])
    older.state = BootReportState.READY
    ;(older as { sealedAt?: bigint }).sealedAt = bigBase + 1n
    await store.put(older)
    const newer = recordingReport('boot-report-big-new', ['newer'])
    newer.state = BootReportState.READY
    ;(newer as { sealedAt?: bigint }).sealedAt = bigBase + 2n
    await store.put(newer)

    const result = await store.applyRetention({ sealedLimit: 1 })
    const survivors = await store.list()

    // Number() coercion would collapse both stamps to the same double and
    // could evict the wrong side; BigInt comparison keeps the newer report.
    expect(survivors.map((r: BootReport) => r.reportId)).toEqual(['boot-report-big-new'])
    expect(result.evicted.map((eviction: { reportId: string; reason: string }) => eviction.reportId)).toEqual([
      'boot-report-big-old',
    ])
  })

  it('hands the inline shell buffer and first mark to the durable store through the collector', async () => {
    await requireStoreModule(
      'inline input-buffer and first-mark handoff journaled into IndexedDB',
    )
    const collectorModule = await import(
      /* @vite-ignore */ './collector.js'
    )
    const globals = globalThis as InlineShellGlobals
    globals.__swStartupMarks = [
      {
        name: 'spacewave.startup.boot.started',
        label: 'boot.started',
        sequence: 1,
        detail: { source: 'browser' },
      },
      {
        name: 'spacewave.startup.content-ready',
        label: 'content-ready',
        sequence: 2,
        detail: {},
      },
    ]

    const factory = new FakeIDBFactory()
    // Durable RECORDING journaling requires a held boot lease, so the store
    // opens with a granting lock manager.
    const store = await openStore({
      idbFactory: factory as unknown as IDBFactory,
      locks: lockManagerShared(),
    })
    const collector = collectorModule.initBootReportCollector(
      { entrypointId: 'drive', usableMark: 'webview.revealed' },
      { store },
    )
    if (!collector) throw new Error('boot report collector missing')
    try {
      const persisted = await waitForStoreRecord(store)
      expect(persisted.length).toBeGreaterThanOrEqual(1)
      const labels = (persisted[0] as BootReport).marks?.map(
        (mark: BootMark) => mark.label,
      )
      expect(labels).toEqual(['boot.started', 'content-ready'])
      expect(collector.isSealed()).toBe(false)
    } finally {
      // Sealing clears the module-global live collector so later tests start
      // from a clean document state.
      collector.seal(BootReportState.ABORTED, 'test-cleanup')
      collector.stop()
      delete globals.__swStartupMarks
    }
  })

  it('persists inline buffer overflow as a REPORT_CONTRACT validation failure at seal', async () => {
    await requireStoreModule(
      'inline overflow becomes a persisted validation failure',
    )
    const collectorModule = await import(
      /* @vite-ignore */ './collector.js'
    )
    const globals = globalThis as InlineShellGlobals
    delete globals.__swBootReport
    globals.__swStartupMarks = [
      {
        name: 'spacewave.startup.boot.started',
        label: 'boot.started',
        sequence: 1,
        detail: { source: 'browser' },
      },
    ]
    globals.__swStartupMarkOverflows = 3

    const factory = new FakeIDBFactory()
    const store = await openStore({
      idbFactory: factory as unknown as IDBFactory,
    })
    const collector = collectorModule.initBootReportCollector(
      { entrypointId: 'drive', usableMark: 'webview.revealed' },
      { store },
    )
    if (!collector) throw new Error('boot report collector missing')
    try {
      window.dispatchEvent(
        new CustomEvent('spacewave-startup-mark', {
          detail: {
            detail: {
              name: 'spacewave.startup.webview.revealed',
              label: 'webview.revealed',
              sequence: 2,
              detail: {},
            },
          },
        }),
      )

      const sealed = await vi.waitFor(() => {
        const report = collectorModule.readSealedBootReport() as BootReport
        expect(report?.validation?.violations?.length ?? 0).toBeGreaterThan(0)
        return report
      })
      expect(sealed.validation?.pass).toBe(false)
      expect(sealed.validation?.violations?.at(-1)?.kind).toBe(
        BootValidationViolationKind.REPORT_CONTRACT,
      )

      const persisted = await waitForStoreRecord(store)
      const sealedPersisted = persisted.find(
        (report) => (report as BootReport).state === BootReportState.READY,
      ) as BootReport
      expect(sealedPersisted?.validation?.pass).toBe(false)
    } finally {
      collector.stop()
      delete globals.__swBootReport
      delete globals.__swStartupMarks
      delete globals.__swStartupMarkOverflows
    }
  })
})

describe('BootReport lock and lazy-attach races', () => {
  it('grants one boot lock across stores through web locks contention', async () => {
    const factory = new FakeIDBFactory()
    const locks = lockManagerShared()
    const first = await openStore({
      idbFactory: factory as unknown as IDBFactory,
      locks,
    })
    const second = await openStore({
      idbFactory: factory as unknown as IDBFactory,
      locks,
    })

    await expect(first.holdBootLock('boot-report-contend')).resolves.toBe(true)
    // The contender learns its denial through the asynchronous Web Locks
    // callback; an optimistic synchronous read would answer true here.
    await expect(second.holdBootLock('boot-report-contend')).resolves.toBe(
      false,
    )

    // sealReport releases the hold after the terminal commit, so a later
    // store can acquire the lock again.
    await first.sealReport(
      recordingReport('boot-report-contend', ['boot.started']),
      BootReportState.READY,
    )
    const third = await openStore({
      idbFactory: factory as unknown as IDBFactory,
      locks,
    })
    await expect(third.holdBootLock('boot-report-contend')).resolves.toBe(true)
  })

  it('keeps the held boot lock releasable when a contender request fails', async () => {
    const factory = new FakeIDBFactory()
    const locks = lockManagerShared()
    const first = await openStore({
      idbFactory: factory as unknown as IDBFactory,
      locks,
    })
    await expect(first.holdBootLock('boot-report-fail')).resolves.toBe(true)

    // A contender whose manager rejects must not disturb the live hold's
    // registration; only its own failed request cleans up.
    const failing = await openStore({
      idbFactory: factory as unknown as IDBFactory,
      locks: lockManagerRejecting(),
    })
    await expect(failing.holdBootLock('boot-report-fail')).resolves.toBe(false)

    // The first store still owns the release handle, so sealReport releases
    // the lock and a later store can acquire it.
    await first.sealReport(
      recordingReport('boot-report-fail', ['boot.started']),
      BootReportState.READY,
    )
    const third = await openStore({
      idbFactory: factory as unknown as IDBFactory,
      locks,
    })
    await expect(third.holdBootLock('boot-report-fail')).resolves.toBe(true)
  })

  it('persists the sealed snapshot when the boot seals before the durable store opens', async () => {
    const collectorModule = await import(
      /* @vite-ignore */ './collector.js'
    )
    const globals = globalThis as InlineShellGlobals & {
      indexedDB?: IDBFactory
    }
    delete globals.__swBootReport
    globals.__swStartupMarks = [
      {
        name: 'spacewave.startup.boot.started',
        label: 'boot.started',
        sequence: 1,
        detail: {},
      },
      {
        name: 'spacewave.startup.boot-status.app',
        label: 'boot-status.app',
        sequence: 2,
        detail: {},
      },
    ]
    const factory = new FakeIDBFactory()
    const previousIndexedDB = globals.indexedDB
    globals.indexedDB = factory as unknown as IDBFactory
    let collector: ReturnType<typeof collectorModule.initBootReportCollector>
    try {
      collector = collectorModule.initBootReportCollector({
        entrypointId: 'drive',
        usableMark: 'boot-status.app',
      })
      expect(collector?.isSealed()).toBe(true)
      const sealedReport = (globalThis as { __swBootReport?: BootReport })
        .__swBootReport
      const reportId = sealedReport?.reportId ?? ''
      expect(reportId).toMatch(/^boot-report-/)

      // The lazy durable attach must land even though the boot sealed while
      // the store was still opening.
      await vi.waitFor(() => {
        const stored = factory.databases
          .get('bldr-boot-reports')
          ?.stores.get('reports')
          ?.records.get(reportId) as BootReport | undefined
        expect(stored?.state).toBe(BootReportState.READY)
      })
    } finally {
      collector?.stop()
      delete globals.__swBootReport
      delete globals.__swStartupMarks
      globals.indexedDB = previousIndexedDB
    }
  })

  it('recovers stale boots before taking the lease and writing recording', async () => {
    // Fresh module registry keeps the document-global collector state
    // hermetic against earlier construction-time seals.
    vi.resetModules()
    const collectorModule = await import(
      /* @vite-ignore */ './collector.js'
    )
    const globals = globalThis as InlineShellGlobals & { __swBootReport?: BootReport }
    delete globals.__swBootReport
    const factory = new FakeIDBFactory()
    // One shared exclusive manager across every store in this boot, so the
    // lease contention is real rather than per-instance.
    const locks = lockManagerShared()
    // A crashed tab left a stale RECORDING row whose web lock died with it.
    const seeded = await openStore({
      idbFactory: factory as unknown as IDBFactory,
      locks,
    })
    await seeded.put(recordingReport('boot-report-dead', ['boot.started']))

    const store = await openStore({
      idbFactory: factory as unknown as IDBFactory,
      locks,
    })
    const collector = collectorModule.initBootReportCollector(
      { entrypointId: 'drive', usableMark: 'webview.revealed' },
      { store },
    )
    if (!collector) throw new Error('boot report collector missing')
    try {
      window.dispatchEvent(
        new CustomEvent('spacewave-startup-mark', {
          detail: {
            detail: {
              name: 'spacewave.startup.runtime.started',
              label: 'runtime.started',
              sequence: 1,
              detail: {},
            },
          },
        }),
      )

      // Attach order is recover -> lease -> first durable write: the stale
      // row aborts, this boot's own row persists RECORDING under its held
      // lease, and a contender is denied that same lease.
      const published = (globalThis as { __swBootReport?: BootReport })
        .__swBootReport
      const reportId = published?.reportId ?? ''
      await vi.waitFor(async () => {
        const dead = await store.get('boot-report-dead')
        expect(dead?.state).toBe(BootReportState.ABORTED)
        const mine = await store.get(reportId)
        expect(mine?.state).toBe(BootReportState.RECORDING)
      })
      const contender = await openStore({
        idbFactory: factory as unknown as IDBFactory,
        locks,
      })
      await expect(contender.holdBootLock(reportId)).resolves.toBe(false)
    } finally {
      collector.seal(BootReportState.ABORTED, 'test-cleanup')
      collector.stop()
      delete globals.__swBootReport
    }
  })

  it('keeps a lease-denied boot memory-only until its terminal seal', async () => {
    // Fresh module registry keeps the document-global collector state
    // hermetic against earlier construction-time seals.
    vi.resetModules()
    const collectorModule = await import(
      /* @vite-ignore */ './collector.js'
    )
    const globals = globalThis as InlineShellGlobals & { __swBootReport?: BootReport }
    delete globals.__swBootReport
    const factory = new FakeIDBFactory()
    const store = await openStore({
      idbFactory: factory as unknown as IDBFactory,
      locks: lockManagerDenied(),
    })
    const collector = collectorModule.initBootReportCollector(
      { entrypointId: 'drive', usableMark: 'webview.revealed' },
      { store },
    )
    if (!collector) throw new Error('boot report collector missing')
    try {
      // Live marks keep collecting in memory while the lease is denied.
      window.dispatchEvent(
        new CustomEvent('spacewave-startup-mark', {
          detail: {
            detail: {
              name: 'spacewave.startup.runtime.started',
              label: 'runtime.started',
              sequence: 1,
              detail: {},
            },
          },
        }),
      )
      await new Promise((resolve) => setTimeout(resolve, 20))
      expect(await store.list()).toEqual([])

      // The terminal seal persists without a lease: the report key is a
      // device-local random id minted once per boot, so the write cannot
      // overwrite or race a live recorder's row.
      window.dispatchEvent(
        new CustomEvent('spacewave-startup-mark', {
          detail: {
            detail: {
              name: 'spacewave.startup.webview.revealed',
              label: 'webview.revealed',
              sequence: 2,
              detail: {},
            },
          },
        }),
      )
      await vi.waitFor(async () => {
        const rows = await store.list()
        expect(rows.length).toBe(1)
        expect(rows[0]?.state).toBe(BootReportState.READY)
      })
    } finally {
      collector.seal(BootReportState.ABORTED, 'test-cleanup')
      collector.stop()
      delete globals.__swBootReport
    }
  })

  it('accumulates zero durable RECORDING rows across repeated lockless boots', async () => {
    const globals = globalThis as InlineShellGlobals & {
      indexedDB?: IDBFactory
    }
    const factory = new FakeIDBFactory()
    const previousIndexedDB = globals.indexedDB
    globals.indexedDB = factory as unknown as IDBFactory
    try {
      for (let boot = 1; boot <= 2; boot += 1) {
        // Fresh module registry per boot mirrors separate document loads;
        // happy-dom exposes no navigator.locks, so both boots run lockless.
        vi.resetModules()
        const collectorModule = await import(
          /* @vite-ignore */ './collector.js'
        )
        delete globals.__swBootReport
        globals.__swStartupMarks = [
          {
            name: 'spacewave.startup.boot.started',
            label: 'boot.started',
            sequence: 1,
            detail: {},
          },
          {
            name: 'spacewave.startup.webview.revealed',
            label: 'webview.revealed',
            sequence: 2,
            detail: {},
          },
        ]
        const collector = collectorModule.initBootReportCollector({
          entrypointId: 'drive',
          usableMark: 'webview.revealed',
        })
        expect(collector?.isSealed()).toBe(true)
        collector?.stop()
        const expectedRows = boot
        await vi.waitFor(() => {
          const records = [
            ...factory.databases
              .get('bldr-boot-reports')!
              .stores.get('reports')!.records.values(),
          ]
          expect(records.length).toBe(expectedRows)
        })
        delete globals.__swBootReport
        delete globals.__swStartupMarks
      }

      const records = [
        ...factory.databases
          .get('bldr-boot-reports')!
          .stores.get('reports')!.records.values(),
      ] as BootReport[]
      expect(records.length).toBe(2)
      for (const record of records) {
        expect(record.state).toBe(BootReportState.READY)
      }
    } finally {
      delete globals.__swStartupMarks
      delete globals.__swBootReport
      globals.indexedDB = previousIndexedDB
    }
  })
})
