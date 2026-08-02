// PersistenceStatus is the latest observed browser eviction-protection state.
export type PersistenceStatus = 'unknown' | 'persisted' | 'not-persisted'

// StorageManagerLike is the subset of navigator.storage that
// StorageDurabilityOwner reads.
export interface StorageManagerLike {
  persisted(): Promise<boolean>
  persist(): Promise<boolean>
}

// PersistenceStatusListener observes the latest recorded persistence status.
export type PersistenceStatusListener = (status: PersistenceStatus) => void

// StorageDurabilityOwner requests browser eviction protection once after an
// explicit user request or the first user-authored durable write.
//
// It deduplicates explicit requests and concurrent first writes into a single
// persisted()/persist() sequence, and isolates every persistence error so a
// query or request failure can never fail the user write that triggered it.
// Denial is silent: only the recorded status changes, with no toast, modal,
// retry, or engagement prompt. The dedup guard is in-memory, so a new document
// re-checks on its first request or meaningful write, with the browser
// remaining the source of truth.
export class StorageDurabilityOwner {
  private statusValue: PersistenceStatus = 'unknown'
  private requested = false
  private pending?: Promise<void>

  constructor(
    private readonly storage: StorageManagerLike | null,
    private readonly onStatus?: PersistenceStatusListener,
  ) {}

  // getStatus returns the latest observed persistence status.
  public getStatus(): PersistenceStatus {
    return this.statusValue
  }

  // requestProtection requests eviction protection once and never throws.
  public requestProtection(): Promise<void> {
    if (this.requested) {
      return this.pending ?? Promise.resolve()
    }
    this.requested = true
    this.pending = this.requestPersistence()
    return this.pending
  }

  // noteMeaningfulWrite requests protection without blocking the user write.
  public noteMeaningfulWrite(): void {
    void this.requestProtection()
  }

  // whenSettled resolves once the in-flight persistence request completes.
  public whenSettled(): Promise<void> {
    return this.pending ?? Promise.resolve()
  }

  // readStatus refreshes the recorded status without requesting protection.
  public async readStatus(): Promise<PersistenceStatus> {
    if (!this.storage) {
      return this.statusValue
    }
    try {
      this.setStatus(
        (await this.storage.persisted()) ? 'persisted' : 'not-persisted',
      )
    } catch {
      // A status query failure leaves the last known status unchanged.
    }
    return this.statusValue
  }

  private async requestPersistence(): Promise<void> {
    if (!this.storage) {
      return
    }
    try {
      let persisted = await this.storage.persisted()
      if (!persisted) {
        persisted = await this.storage.persist()
      }
      this.setStatus(persisted ? 'persisted' : 'not-persisted')
    } catch (err) {
      // A persistence query or request failure must never fail the user write;
      // record a single bounded diagnostic and leave the status unchanged.
      console.warn('WebDocument: storage persistence request failed', err)
    }
  }

  private setStatus(status: PersistenceStatus): void {
    this.statusValue = status
    this.onStatus?.(status)
  }
}
