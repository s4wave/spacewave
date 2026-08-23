import { BootReport, BootReportState } from './report.pb.js'

// The database contract is frozen at version 1: one `reports` object store
// keyed reportId with state and sealedAt indexes. First deployment creates
// version 1. Opening a database at an unknown or newer version fails closed:
// the store refuses reads and writes and never deletes or resets anything.
// Future shape changes bump the IndexedDB version and land an explicit
// versionchange migration here; destructive reset is prohibited.
export const bootReportDatabaseName = 'bldr-boot-reports'
export const bootReportDatabaseVersion = 1
export const bootReportObjectName = 'reports'

// Retention resolves OQ-1: keep the last retentionSealedLimit sealed reports
// within retentionBodyBudgetBytes of encoded BootReport body bytes. RECORDING
// reports and the newest FAILED report are never eviction candidates, but the
// newest FAILED report counts toward both limits. v1 stores no attachments
// (attachment budget 0); a later attachment phase must version-migrate.
export const retentionSealedLimit = 100
export const retentionBodyBudgetBytes = 10 << 20

interface IDBFactoryLike {
  open(name: string, version?: number): IDBOpenRequestLike
}

interface IDBOpenRequestLike {
  onupgradeneeded: ((event: unknown) => unknown) | null
  onerror: ((event: unknown) => unknown) | null
  onsuccess: ((event: unknown) => unknown) | null
}

interface IDBRequestLike<T> {
  result: T
  error: Error | null
  onsuccess: ((event: unknown) => unknown) | null
  onerror: ((event: unknown) => unknown) | null
}

interface IDBTransactionLike {
  objectStore(name: string): {
    get(key: string): IDBRequestLike<unknown>
    getAll(): IDBRequestLike<unknown[]>
    put(value: Record<string, unknown>): IDBRequestLike<unknown>
    delete(key: string): IDBRequestLike<undefined>
  }
  oncomplete: ((event: unknown) => unknown) | null
  onabort: ((event: unknown) => unknown) | null
  onerror: ((event: unknown) => unknown) | null
}

interface IDBDatabaseLike {
  // Transaction mirrors the native IDBDatabase signature: the object store
  // name comes first, then the mode. The mode-only call asks the browser for
  // an object store literally named "readwrite" and fails there.
  transaction(
    storeName: string,
    mode: 'readonly' | 'readwrite',
  ): IDBTransactionLike
}

type LockManagerLike =
  | {
      request(
        name: string,
        options: { ifAvailable: boolean },
        callback: (lock: unknown) => unknown,
      ): Promise<unknown>
    }
  | undefined
  | null

export interface OpenBootReportStoreOptions {
  // IdbFactory overrides the global IndexedDB factory for tests.
  idbFactory?: IDBFactoryLike | null
  // Locks overrides the global Web Locks manager for tests. Null disables
  // locking and selects the conservative non-destructive recovery default.
  locks?: LockManagerLike
}

export interface BootReportStore {
  // Put journals one report write through the serialized writer and resolves
  // only after the IndexedDB transaction commits. It rejects on storage,
  // quota, or abort failure instead of dropping the write.
  put(report: BootReport): Promise<void>
  // Get returns one persisted report by id.
  get(reportId: string): Promise<BootReport | undefined>
  // List returns every persisted report.
  list(): Promise<BootReport[]>
  // Delete removes one report by id.
  delete(reportId: string): Promise<void>
  // RecoverOnStartup aborts stale RECORDING reports whose crash released
  // their Web Lock and leaves live concurrent boots untouched.
  recoverOnStartup(): Promise<void>
  // HoldBootLock takes this tab's unique recording lock for one report and
  // keeps it until sealReport releases it after the terminal commit. It
  // resolves true only after the manager granted the lock to this document,
  // and false when the lock is held elsewhere, locking is unavailable, or
  // the request fails.
  holdBootLock(reportId: string): Promise<boolean>
  // SealReport writes the complete sealed record in one atomic put and
  // releases the report's boot lock after the terminal commit.
  sealReport(
    report: BootReport,
    state: BootReportState,
    terminalErrorCode?: string,
  ): Promise<BootReport>
  // ApplyRetention evicts sealed reports beyond the accepted limits after
  // seal and returns the eviction count with reasons. Defaults are the
  // accepted OQ-1 recommendation; overrides exist for focused tests.
  applyRetention(overrides?: {
    sealedLimit?: number
    bodyBudgetBytes?: number
  }): Promise<{ evicted: Array<{ reportId: string; reason: string }> }>
}

type GlobalsWithBootStore = typeof globalThis & {
  indexedDB?: IDBFactoryLike
  navigator?: { locks?: LockManagerLike }
  __swStartupMarkOverflows?: number
}

function lockNameFor(reportId: string): string {
  return `boot-report-lock:${reportId}`
}

// requestToPromise settles with the request result and rejects with the
// request error through the native onsuccess/onerror callbacks only.
function requestToPromise<T>(request: IDBRequestLike<T>): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    request.onsuccess = () => resolve(request.result)
    request.onerror = () =>
      reject(request.error ?? new Error('indexeddb request failed'))
  })
}

// transactionDone resolves when the transaction commits and rejects on abort
// or error, so a request success followed by a transaction abort still
// rejects the operation.
function transactionDone(transaction: IDBTransactionLike): Promise<void> {
  return new Promise<void>((resolve, reject) => {
    transaction.oncomplete = () => resolve()
    transaction.onabort = () =>
      reject(transaction.error ?? new Error('indexeddb transaction aborted'))
    transaction.onerror = () =>
      reject(new Error('indexeddb transaction failed'))
  })
}

// ProbeLock distinguishes a dead holder (the lock grants to us, so the
// previous tab crashed or closed) from a live holder (the lock stays
// elsewhere, so another tab still owns the boot). An absent manager reports
// unavailable and every caller takes the conservative non-destructive path.
async function probeLock(
  locks: LockManagerLike,
  name: string,
): Promise<'acquired' | 'held-elsewhere' | 'unavailable'> {
  if (!locks || typeof locks.request !== 'function') return 'unavailable'
  let outcome: 'acquired' | 'held-elsewhere' = 'held-elsewhere'
  await locks.request(name, { ifAvailable: true }, (lock) => {
    outcome = lock ? 'acquired' : 'held-elsewhere'
  })
  return outcome
}

function encodedBodyBytes(report: BootReport): number {
  try {
    return BootReport.toBinary(report).byteLength
  } catch {
    return Number.MAX_SAFE_INTEGER
  }
}

// heldBootLocks maps report ids of live recordings to the release function
// for this document's own Web Lock hold. sealReport releases the hold after
// the terminal commit.
const heldBootLocks = new Map<string, () => void>()

// externalHolders tracks report ids whose lock probe answered held-elsewhere
// inside this document realm. A live foreign tab registered here is never
// aborted by recovery; real cross-tab liveness is proven by the Web Lock
// itself when the realm differs.
const externalHolders = new Set<string>()

// SerializedWriter executes enqueued operations strictly one at a time in
// schedule order and never lets one failure break the chain.
class SerializedWriter {
  private tail: Promise<unknown> = Promise.resolve()

  run<T>(operation: () => Promise<T>): Promise<T> {
    const next = this.tail.then(operation, operation)
    this.tail = next.catch(() => undefined)
    return next
  }
}

export async function openBootReportStore(
  options: OpenBootReportStoreOptions = {},
): Promise<BootReportStore> {
  const globals = globalThis as GlobalsWithBootStore
  const factory = options.idbFactory ?? globals.indexedDB
  if (!factory) {
    throw new Error('BootReportStore requires an IndexedDB factory')
  }
  const database = await openDatabase(factory)

  function openDatabase(factory: IDBFactoryLike): Promise<IDBDatabaseLike> {
    return new Promise((resolve, reject) => {
      const request = factory.open(
        bootReportDatabaseName,
        bootReportDatabaseVersion,
      )
      request.onupgradeneeded = (event) => {
        try {
          const target = (event as { target: unknown }).target as {
            result: {
              createObjectStore(
                name: string,
                options: { keyPath: string },
              ): {
                createIndex(name: string, keyPath: string): void
              }
            }
          }
          const store = target.result.createObjectStore(bootReportObjectName, {
            keyPath: 'reportId',
          })
          store.createIndex('state', 'state')
          store.createIndex('sealedAt', 'sealedAt')
        } catch (cause) {
          // An upgrade failure must reject the open instead of escaping as an
          // uncaught exception while the open request stays pending.
          reject(cause as Error)
        }
      }
      request.onerror = () => {
        const error = (request as unknown as { error?: Error }).error
        reject(error ?? new Error('BootReportStore open failed'))
      }
      request.onsuccess = () => {
        resolve((request as unknown as { result: IDBDatabaseLike }).result)
      }
    })
  }

  const writer = new SerializedWriter()

  const store: BootReportStore = {
    async put(report: BootReport): Promise<void> {
      return writer.run(async () => {
        if (
          report.state === BootReportState.RECORDING &&
          !externalHolders.has(report.reportId ?? '')
        ) {
          // A RECORDING report written while our own probe answers
          // held-elsewhere belongs to a live foreign tab: register it so a
          // same-realm recovery pass never aborts an active boot.
          const probe = await probeLock(
            options.locks,
            lockNameFor(report.reportId ?? ''),
          )
          if (probe === 'held-elsewhere') {
            externalHolders.add(report.reportId ?? '')
          }
        }
        const transaction = database.transaction(
          bootReportObjectName,
          'readwrite',
        )
        const done = transactionDone(transaction)
        await requestToPromise(
          writeObjectStore(transaction).put(
            report as unknown as Record<string, unknown>,
          ),
        )
        await done
      })
    },

    async get(reportId: string): Promise<BootReport | undefined> {
      return writer.run(async () => {
        const transaction = database.transaction(
          bootReportObjectName,
          'readonly',
        )
        const done = transactionDone(transaction)
        const stored = await requestToPromise(
          transaction.objectStore(bootReportObjectName).get(reportId),
        )
        await done
        return stored == null ? undefined : (stored as BootReport)
      })
    },

    async list(): Promise<BootReport[]> {
      return writer.run(async () => {
        const transaction = database.transaction(
          bootReportObjectName,
          'readonly',
        )
        const done = transactionDone(transaction)
        const all = await requestToPromise(
          transaction.objectStore(bootReportObjectName).getAll(),
        )
        await done
        return (all as unknown[]).filter(Boolean) as BootReport[]
      })
    },

    async delete(reportId: string): Promise<void> {
      return writer.run(async () => {
        const transaction = database.transaction(
          bootReportObjectName,
          'readwrite',
        )
        const done = transactionDone(transaction)
        await requestToPromise(
          transaction.objectStore(bootReportObjectName).delete(reportId),
        )
        await done
      })
    },

    async recoverOnStartup(): Promise<void> {
      const reports = await store.list()
      for (const report of reports) {
        if (report.state !== BootReportState.RECORDING) continue
        const reportId = report.reportId ?? ''
        if (heldBootLocks.has(reportId)) continue
        if (externalHolders.has(reportId)) continue
        const probe = await probeLock(options.locks, lockNameFor(reportId))
        if (probe !== 'acquired') continue
        await store.put({ ...report, state: BootReportState.ABORTED })
      }
    },

    holdBootLock(reportId: string): Promise<boolean> {
      const locks =
        options.locks ?? (globals.navigator?.locks as LockManagerLike)
      if (!locks || typeof locks.request !== 'function') {
        console.warn(
          'bootreport: Web Locks unavailable; running without a cross-tab ' +
            'recording lease',
        )
        return Promise.resolve(false)
      }
      let release!: () => void
      const released = new Promise<void>((resolve) => {
        release = resolve
      })
      // The grant settles only inside the manager callback, so the result
      // resolves there: concurrent contenders all observe the manager's own
      // answer instead of an optimistic local registration.
      return new Promise<boolean>((resolveHold) => {
        try {
          void locks
            .request(
              lockNameFor(reportId),
              { ifAvailable: true },
              (lock) => {
                if (!lock) {
                  resolveHold(false)
                  return undefined
                }
                // Granted: install the hold now and keep the lock until
                // sealReport releases it after the terminal commit or the
                // tab dies, whichever comes first.
                heldBootLocks.set(reportId, release)
                resolveHold(true)
                return released
              },
            )
            ?.catch((cause: unknown) => {
              // Drop the registration only when this request installed it;
              // a failed contender must never release another holder's lock.
              if (heldBootLocks.get(reportId) === release) {
                heldBootLocks.delete(reportId)
              }
              console.warn('bootreport: lock hold failed', cause)
              resolveHold(false)
            })
        } catch (cause) {
          console.warn('bootreport: lock hold failed', cause)
          resolveHold(false)
        }
      })
    },

    async sealReport(report, state, terminalErrorCode) {
      const terminal = (report.marks ?? []).at(-1)
      const sealed: BootReport = {
        ...report,
        state,
        terminalMark: terminal?.label,
        terminalErrorCode,
        sealedAt: BigInt(Date.now()) * 1000n,
      } as BootReport
      await this.put(sealed)
      const release = heldBootLocks.get(sealed.reportId ?? '')
      if (release) {
        heldBootLocks.delete(sealed.reportId ?? '')
        release()
      }
      return sealed
    },

    async applyRetention(overrides) {
      const evicted: Array<{ reportId: string; reason: string }> = []
      const sealedLimit =
        overrides?.sealedLimit ?? retentionSealedLimit
      const bodyBudget =
        overrides?.bodyBudgetBytes ?? retentionBodyBudgetBytes
      const reports = await store.list()
      const terminals = reports.filter((report) => {
        return (
          report.state === BootReportState.READY ||
          report.state === BootReportState.FAILED ||
          report.state === BootReportState.ABORTED
        )
      })
      const recency = (
        a: BootReport,
        b: BootReport,
      ): number => {
        const aAt = a.sealedAt ?? 0n
        const bAt = b.sealedAt ?? 0n
        // BigInt-safe comparison: microsecond stamps may exceed the safe
        // integer range in future schemas.
        if (aAt === bAt) return 0
        return aAt > bAt ? -1 : 1
      }
      const newestFailed = terminals
        .filter((report) => report.state === BootReportState.FAILED)
        .sort(recency)
        .at(0)
      const ordered = terminals.sort(recency)
      let kept = 0
      let budget = bodyBudget
      for (const report of ordered) {
        const reportId = report.reportId ?? ''
        if (newestFailed && reportId === newestFailed.reportId) {
          // The newest FAILED report is never evicted, but it still consumes
          // both the count and byte budgets.
          kept += 1
          budget -= encodedBodyBytes(report)
          continue
        }
        const size = encodedBodyBytes(report)
        if (kept >= sealedLimit) {
          evicted.push({ reportId, reason: 'sealed-limit' })
          await store.delete(reportId)
          continue
        }
        if (size > budget) {
          evicted.push({ reportId, reason: 'body-budget' })
          await store.delete(reportId)
          continue
        }
        budget -= size
        kept += 1
      }
      return { evicted }
    },
  }

  function writeObjectStore(transaction: IDBTransactionLike) {
    return transaction.objectStore(bootReportObjectName)
  }

  return store
}
