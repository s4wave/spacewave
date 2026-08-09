import { useCallback, useSyncExternalStore } from 'react'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'

import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import type {
  CreateCheckoutSessionRequest,
  CreateCheckoutSessionResponse,
  ListManagedBillingAccountsResponse,
  ReactivateSubscriptionResponse,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

export interface ManagedBillingAccountsSnapshot {
  data: ListManagedBillingAccountsResponse | null
  loading: boolean
  stale: boolean
  error: Error | null
}

const EMPTY_SNAPSHOT: ManagedBillingAccountsSnapshot = {
  data: null,
  loading: false,
  stale: false,
  error: null,
}

// Mutations are cancelable until their operation resolves. The refresh phase
// cannot turn acknowledged durable work into a rejected caller result.
type MutationPhase = 'queued' | 'operation' | 'refresh'

interface MutationItem {
  abort: VoidFunction
  abortSignal?: AbortSignal
  controller: AbortController
  operation: (signal: AbortSignal) => Promise<unknown>
  phase: MutationPhase
  reject: (reason?: unknown) => void
  resolve: (value: unknown) => void
  settled: boolean
}

export class ManagedBillingAccountsStore {
  private snapshot: ManagedBillingAccountsSnapshot = {
    data: null,
    loading: true,
    stale: true,
    error: null,
  }
  private readonly listeners = new Set<VoidFunction>()
  private readonly mutationQueue: MutationItem[] = []
  private runningMutation: MutationItem | null = null
  private loadController: AbortController | null = null
  private generation = 0

  public constructor(private readonly session: Session) {}

  public getSnapshot = (): ManagedBillingAccountsSnapshot => this.snapshot

  public subscribe = (listener: VoidFunction): VoidFunction => {
    this.listeners.add(listener)
    if (this.snapshot.stale && !this.loadController) void this.refresh()
    return () => {
      this.listeners.delete(listener)
      if (this.listeners.size === 0) this.abort()
    }
  }

  public async refresh(): Promise<void> {
    try {
      await this.refreshSnapshot()
    } catch {
      // The snapshot carries refresh errors for subscribers.
    }
  }

  public create = (displayName: string, abortSignal?: AbortSignal) =>
    this.enqueueMutation(
      (signal) =>
        this.session.spacewave.createBillingAccount(displayName, signal),
      abortSignal,
    )

  public createCheckoutSession = (
    request: CreateCheckoutSessionRequest,
    abortSignal?: AbortSignal,
  ): Promise<CreateCheckoutSessionResponse> =>
    this.enqueueMutation(
      (signal) => this.session.spacewave.createCheckoutSession(request, signal),
      abortSignal,
    )

  public rename = (
    billingAccountId: string,
    displayName: string,
    abortSignal?: AbortSignal,
  ) =>
    this.enqueueMutation(
      (signal) =>
        this.session.spacewave.renameBillingAccount(
          billingAccountId,
          displayName,
          signal,
        ),
      abortSignal,
    )

  public assign = (
    billingAccountId: string,
    ownerType: 'account' | 'organization',
    ownerId: string,
    abortSignal?: AbortSignal,
  ) =>
    this.enqueueMutation(
      (signal) =>
        this.session.spacewave.assignBillingAccount(
          billingAccountId,
          ownerType,
          ownerId,
          signal,
        ),
      abortSignal,
    )

  public detach = (
    ownerType: 'account' | 'organization',
    ownerId: string,
    abortSignal?: AbortSignal,
  ) =>
    this.enqueueMutation(
      (signal) =>
        this.session.spacewave.detachBillingAccount(ownerType, ownerId, signal),
      abortSignal,
    )

  public cancel = (billingAccountId: string, abortSignal?: AbortSignal) =>
    this.enqueueMutation(
      (signal) =>
        this.session.spacewave.cancelSubscription(billingAccountId, signal),
      abortSignal,
    )

  public reactivate = (
    billingAccountId: string,
    abortSignal?: AbortSignal,
  ): Promise<ReactivateSubscriptionResponse> =>
    this.enqueueMutation(
      (signal) =>
        this.session.spacewave.reactivateSubscription(billingAccountId, signal),
      abortSignal,
    )

  private enqueueMutation<T>(
    operation: (signal: AbortSignal) => Promise<T>,
    abortSignal?: AbortSignal,
  ): Promise<T> {
    return new Promise<T>((resolve, reject) => {
      const controller = new AbortController()
      const item: MutationItem = {
        abort: () => {},
        abortSignal,
        controller,
        operation,
        phase: 'queued',
        reject,
        resolve: (value) => resolve(value as T),
        settled: false,
      }
      item.abort = () => {
        controller.abort(abortSignal?.reason)
        if (item.phase === 'refresh') return
        if (item.phase === 'queued') {
          const index = this.mutationQueue.indexOf(item)
          if (index !== -1) this.mutationQueue.splice(index, 1)
          this.finishMutation(item)
        }
        this.rejectMutation(item, controller.signal.reason)
      }
      abortSignal?.addEventListener('abort', item.abort, { once: true })
      if (abortSignal?.aborted) {
        item.abort()
        this.finishMutation(item)
        return
      }
      this.mutationQueue.push(item)
      void this.drainMutations()
    })
  }

  private async drainMutations(): Promise<void> {
    if (this.runningMutation) return
    const item = this.mutationQueue.shift()
    if (!item) return
    const signal = item.controller.signal
    if (signal.aborted) {
      this.finishMutation(item)
      void this.drainMutations()
      return
    }
    item.phase = 'operation'
    this.runningMutation = item
    try {
      const result = await item.operation(signal)
      if (signal.aborted) throw signal.reason
      item.phase = 'refresh'
      this.resolveMutation(item, result)
      this.finishMutation(item)
      await this.refreshAuthoritative()
    } catch (error) {
      if (item.phase === 'operation') this.rejectMutation(item, error)
    } finally {
      this.finishMutation(item)
      this.runningMutation = null
      void this.drainMutations()
    }
  }

  private async refreshAuthoritative(): Promise<void> {
    try {
      const published = await this.refreshSnapshot()
      if (!published && this.listeners.size > 0) {
        await this.refreshAuthoritative()
      }
    } catch {
      // Durable mutation success is independent of snapshot refresh failure.
    }
  }

  private async refreshSnapshot(abortSignal?: AbortSignal): Promise<boolean> {
    const generation = ++this.generation
    this.loadController?.abort()
    const controller = new AbortController()
    const abort = () => controller.abort(abortSignal?.reason)
    abortSignal?.addEventListener('abort', abort, { once: true })
    if (abortSignal?.aborted) abort()
    this.loadController = controller
    this.setSnapshot({
      ...this.snapshot,
      loading: true,
      stale: true,
      error: null,
    })
    try {
      const data = await this.session.spacewave.listManagedBillingAccounts(
        controller.signal,
      )
      if (controller.signal.aborted || generation !== this.generation) {
        if (abortSignal?.aborted) throw abortSignal.reason
        return false
      }
      this.setSnapshot({ data, loading: false, stale: false, error: null })
      return true
    } catch (error) {
      if (controller.signal.aborted || generation !== this.generation) {
        if (abortSignal?.aborted) throw abortSignal.reason
        return false
      }
      const refreshError =
        error instanceof Error ? error : new Error(String(error))
      this.setSnapshot({
        ...this.snapshot,
        loading: false,
        stale: true,
        error: refreshError,
      })
      throw refreshError
    } finally {
      abortSignal?.removeEventListener('abort', abort)
      if (this.loadController === controller) this.loadController = null
    }
  }

  private abort() {
    this.snapshot = { ...this.snapshot, stale: true }
    this.generation++
    this.loadController?.abort()
    this.loadController = null
    if (this.runningMutation?.phase === 'operation') {
      this.runningMutation.controller.abort()
      this.rejectMutation(
        this.runningMutation,
        this.runningMutation.controller.signal.reason,
      )
    }
    for (const item of this.mutationQueue.splice(0)) {
      item.controller.abort()
      this.rejectMutation(item, item.controller.signal.reason)
      this.finishMutation(item)
    }
  }

  private resolveMutation(item: MutationItem, value: unknown) {
    if (item.settled) return
    item.settled = true
    item.resolve(value)
  }

  private rejectMutation(item: MutationItem, reason: unknown) {
    if (item.settled) return
    item.settled = true
    item.reject(reason)
  }

  private finishMutation(item: MutationItem) {
    item.abortSignal?.removeEventListener('abort', item.abort)
  }

  private setSnapshot(snapshot: ManagedBillingAccountsSnapshot) {
    this.snapshot = snapshot
    for (const listener of [...this.listeners]) {
      try {
        listener()
      } catch (error) {
        try {
          console.error('Managed billing account listener failed:', error)
        } catch {
          // Error reporting must not interrupt snapshot publication.
        }
      }
    }
  }
}

const stores = new WeakMap<Session, ManagedBillingAccountsStore>()

function getStore(session: Session): ManagedBillingAccountsStore {
  let store = stores.get(session)
  if (!store) {
    store = new ManagedBillingAccountsStore(session)
    stores.set(session, store)
  }
  return store
}

export function useManagedBillingAccounts() {
  const session = useResourceValue(SessionContext.useContext())
  const store = session ? getStore(session) : null
  const subscribe = useCallback(
    (listener: VoidFunction) => store?.subscribe(listener) ?? (() => {}),
    [store],
  )
  const getSnapshot = useCallback(
    () => store?.getSnapshot() ?? EMPTY_SNAPSHOT,
    [store],
  )
  const snapshot = useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
  return { ...snapshot, session, store }
}
