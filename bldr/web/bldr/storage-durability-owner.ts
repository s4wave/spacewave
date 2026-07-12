// PersistenceStatus is the latest observed browser eviction-protection state.
export type PersistenceStatus = 'unknown' | 'persisted' | 'not-persisted'

// StorageManagerLike is the subset of navigator.storage this owner requires.
export interface StorageManagerLike {
  persisted(): Promise<boolean>
  persist(): Promise<boolean>
}

// PersistenceStatusListener observes the latest recorded persistence status.
export type PersistenceStatusListener = (status: PersistenceStatus) => void

// StorageDurabilityOwner requests browser eviction protection once, on the
// first user-authored durable write, and records the observed status.
//
// It never requests persistence before a meaningful write, deduplicates
// concurrent first writes into a single persisted()/persist() sequence, and
// isolates every persistence error so a query or request failure can never fail
// the user write that triggered it. Denial is silent: only the recorded status
// changes, with no toast, modal, retry, or engagement prompt. The dedup guard is
// in-memory, so a new document (a new app start) re-checks on its first
// meaningful write, with the browser remaining the source of truth.
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

  // noteMeaningfulWrite requests eviction protection on the first call and is a
  // no-op afterward. It never throws and never blocks the caller.
  public noteMeaningfulWrite(): void {
    if (this.requested) {
      return
    }
    this.requested = true
    this.pending = this.requestPersistence()
  }

  // whenSettled resolves once the in-flight persistence request completes.
  public whenSettled(): Promise<void> {
    return this.pending ?? Promise.resolve()
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
