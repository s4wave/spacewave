import { WebRuntimeClientInit } from '../runtime/runtime.pb.js'
import {
  ConnectWebRuntimeAck,
  type DedicatedRuntimeHostConnectControl,
  WebDocumentToWorker,
} from '../runtime/runtime.js'

import { randomId } from './random-id.js'
import {
  buildRuntimeLifeLockName,
  watchRuntimeLifeLock,
} from './runtime-life-lock.js'
import { markStartupBoundary } from './startup-marks.js'

// DedicatedWorkerHostRole is the host-election role of this document in the
// dedicated worker fallback. A runtime failure ends the term of every document
// that hosted or attached to that runtime: failed holds no lease and waits for
// none, so the next election, from a reloaded document, starts a fresh runtime
// instead of reconnecting to the dead one or promoting a document that shows
// the failure.
export type DedicatedWorkerHostRole =
  | 'pending'
  | 'host'
  | 'attached'
  | 'unavailable'
  | 'failed'
  | 'closed'

// DedicatedWorkerHostCallbacks receives host-state transitions.
export interface DedicatedWorkerHostCallbacks {
  startHost(generation: string): void
  startAttached(): void
  startUnavailable(): void
  promoteToHost(): void
  // hostWorkerLost reports that this host's runtime Worker died, or that it
  // cannot take its life lock, so its death could not be seen.
  hostWorkerLost(reason: string): void
  // attachedHostGone reports that the Worker of the host this document is
  // attached to died.
  attachedHostGone(hostDocumentId: string, hostGeneration: string): void
}

interface DedicatedWorkerHostOpen {
  port: MessagePort
  reject(err: Error): void
}

// buildDedicatedWorkerHostLockName returns the Web Lock name used to elect
// the dedicated runtime host for a runtime id.
export function buildDedicatedWorkerHostLockName(runtimeId: string): string {
  return `bldr-dedicated-runtime-host-${runtimeId}`
}

// DedicatedWorkerHostOwner makes Chromium issue 528332884's temporary
// DedicatedWorker fallback keep SharedWorker's one runtime-host owner shape.
//
// Each host term's Worker holds the term's life lock (runtime-life-lock.ts).
// The owner watches the life lock of the term its document runs on: a host
// once its Worker reports holding it, an attached document once the host acks
// its attach. The host acks only after its Worker holds the lock, so no watch
// is requested before the Worker holds it. A grant ends a host's term and
// sends an attached document to the next election.
export class DedicatedWorkerHostOwner {
  public role: DedicatedWorkerHostRole = 'pending'
  public generation?: string
  public connectedHostDocumentId?: string
  public connectedHostGeneration?: string
  private leaseStarted = false
  private closed = false
  private closeLease?: () => void
  private standingAbort?: AbortController
  private callbacks?: DedicatedWorkerHostCallbacks
  // lifeWatch withdraws the watch on the current term's life lock.
  private lifeWatch?: AbortController
  // workerHeld settles once this host term's Worker holds its life lock, or
  // rejects when the term ends first.
  private workerHeld?: PromiseWithResolvers<void>
  private readonly pendingClientChannels = new Set<DedicatedWorkerHostOpen>()

  constructor(
    private readonly webRuntimeId: string,
    private readonly webDocumentId: string,
  ) {}

  public start(callbacks: DedicatedWorkerHostCallbacks): void {
    this.callbacks = callbacks
    if (this.leaseStarted || this.closed) {
      return
    }
    this.leaseStarted = true

    if (typeof navigator === 'undefined' || !navigator.locks) {
      this.role = 'unavailable'
      markStartupBoundary('dedicated-host.election-unavailable', {
        source: 'browser',
        documentId: this.webDocumentId,
        runtimeId: this.webRuntimeId,
        hazard:
          'Web Locks unavailable; DedicatedWorker fallback may run multiple OPFS writers',
      })
      console.warn(
        'WebDocument: Web Locks unavailable; DedicatedWorker fallback may run multiple OPFS writers',
      )
      callbacks.startUnavailable()
      return
    }

    const lockName = buildDedicatedWorkerHostLockName(this.webRuntimeId)
    this.standingAbort = new AbortController()
    markStartupBoundary('dedicated-host.election-start', {
      source: 'browser',
      documentId: this.webDocumentId,
      runtimeId: this.webRuntimeId,
      lockName,
    })

    // The grant callback owns the host lease until close so Web Locks, not
    // relay failure timing, chooses the next host.
    Promise.resolve(
      navigator.locks.request(
        lockName,
        { signal: this.standingAbort.signal },
        async () => {
          if (!this.onLockGranted()) {
            return
          }
          await new Promise<void>((resolve) => {
            this.closeLease = resolve
          })
        },
      ),
    ).catch((err: unknown) => {
      if (this.closed || this.role === 'failed') {
        return
      }
      markStartupBoundary('dedicated-host.election-failed', {
        source: 'browser',
        documentId: this.webDocumentId,
        runtimeId: this.webRuntimeId,
        error: err instanceof Error ? err.message : String(err),
      })
      this.cancelPendingClientChannels('dedicated runtime host election failed')
      this.role = 'unavailable'
      callbacks.startUnavailable()
    })

    void navigator.locks.query().then((snapshot) => {
      // A grant can beat the query; only a still-pending role can attach to
      // the already-held standing lock.
      if (this.role !== 'pending') {
        return
      }
      const heldByAnother = snapshot.held?.some(
        (lock) => lock.name === lockName,
      )
      if (!heldByAnother) {
        return
      }
      this.role = 'attached'
      markStartupBoundary('dedicated-host.attach-selected', {
        source: 'browser',
        documentId: this.webDocumentId,
        runtimeId: this.webRuntimeId,
      })
      callbacks.startAttached()
    })
  }

  // onLockGranted takes the host role and reports whether this document now
  // holds the lease. A document that closed or failed first returns the lease.
  private onLockGranted(): boolean {
    if (this.role !== 'pending' && this.role !== 'attached') {
      return false
    }

    const promoted = this.role === 'attached'
    this.role = 'host'
    this.generation = `${this.webDocumentId}-${randomId()}`
    this.stopLifeWatch()
    this.workerHeld = Promise.withResolvers<void>()
    this.workerHeld.promise.catch(() => {})
    markStartupBoundary(
      promoted ? 'dedicated-host.promoted' : 'dedicated-host.lease-acquired',
      {
        source: 'browser',
        documentId: this.webDocumentId,
        runtimeId: this.webRuntimeId,
        generation: this.generation,
      },
    )
    if (promoted) {
      this.connectedHostDocumentId = undefined
      this.connectedHostGeneration = undefined
      this.cancelPendingClientChannels(
        'attached runtime host relay promoted to host',
      )
      this.callbacks?.promoteToHost()
      return true
    }
    this.callbacks?.startHost(this.generation)
    return true
  }

  // fail ends this document's term after the runtime it hosted or attached to
  // failed. A host returns its lease, so call fail only after the dead runtime
  // worker is ended and holds no storage handles; an attached or pending
  // document withdraws its queued request.
  public fail(reason: string): void {
    if (this.closed || this.role === 'failed') {
      return
    }
    const from = this.role
    this.role = 'failed'
    this.connectedHostDocumentId = undefined
    this.connectedHostGeneration = undefined
    this.cancelPendingClientChannels('dedicated runtime host failed')
    this.endTerm('dedicated runtime host failed')
    this.standingAbort?.abort()
    this.standingAbort = undefined
    this.closeLease?.()
    this.closeLease = undefined
    markStartupBoundary('dedicated-host.failed', {
      source: 'browser',
      documentId: this.webDocumentId,
      runtimeId: this.webRuntimeId,
      generation: this.generation,
      from,
      reason,
    })
  }

  // lifeLockName is the life lock of this host's current term.
  public get lifeLockName(): string | undefined {
    if (this.role !== 'host' || !this.generation) {
      return undefined
    }
    return buildRuntimeLifeLockName(this.webRuntimeId, this.generation)
  }

  // workerReportsLifeLock records whether the Worker of the named term took
  // its life lock. A held lock lets the host ack attaches and is watched for
  // the Worker's death. A Worker without one would die unseen, leaving every
  // page on it waiting forever, so its term fails into Retry instead.
  public workerReportsLifeLock(name: string, held: boolean): void {
    const term = this.workerHeld
    if (!term || name !== this.lifeLockName) {
      return
    }
    if (!held) {
      this.callbacks?.hostWorkerLost('runtime worker holds no life lock')
      return
    }
    term.resolve()
    markStartupBoundary('dedicated-host.life-lock-held', {
      source: 'browser',
      documentId: this.webDocumentId,
      runtimeId: this.webRuntimeId,
      generation: this.generation,
    })
    this.watchLifeLock(name, () =>
      this.callbacks?.hostWorkerLost('runtime worker exited'),
    )
  }

  // waitWorkerHoldsLifeLock resolves once this host term's Worker reports its
  // life lock and rejects if the term ends first. Without Web Locks there is
  // no life lock to wait for.
  public waitWorkerHoldsLifeLock(): Promise<void> {
    return this.workerHeld?.promise ?? Promise.resolve()
  }

  // hostLost reports whether the named host term is the one this attached
  // document runs on, and forgets it, so one loss reroutes once whichever
  // signal reports it first.
  public hostLost(hostDocumentId: string, hostGeneration?: string): boolean {
    if (
      this.role !== 'attached' ||
      hostDocumentId !== this.connectedHostDocumentId ||
      hostGeneration !== this.connectedHostGeneration
    ) {
      return false
    }
    this.connectedHostDocumentId = undefined
    this.connectedHostGeneration = undefined
    this.stopLifeWatch()
    return true
  }

  // watchLifeLock replaces the watch with one on the named life lock. Only
  // the current watch reports; a grant that races its own withdrawal does not.
  private watchLifeLock(name: string, onGone: () => void): void {
    this.stopLifeWatch()
    const watch = new AbortController()
    this.lifeWatch = watch
    watchRuntimeLifeLock(name, watch.signal, () => {
      if (this.lifeWatch !== watch) {
        return
      }
      this.lifeWatch = undefined
      markStartupBoundary('dedicated-host.worker-gone', {
        source: 'browser',
        documentId: this.webDocumentId,
        runtimeId: this.webRuntimeId,
        role: this.role,
        lockName: name,
      })
      onGone()
    })
  }

  private stopLifeWatch(): void {
    this.lifeWatch?.abort()
    this.lifeWatch = undefined
  }

  // endTerm withdraws the life watch and fails a wait on a lock this term's
  // Worker will no longer take.
  private endTerm(reason: string): void {
    this.stopLifeWatch()
    this.workerHeld?.reject(new Error(reason))
    this.workerHeld = undefined
  }

  public async openClientChannel(
    init: WebRuntimeClientInit,
  ): Promise<MessagePort> {
    if (this.closed || this.role === 'closed') {
      throw new Error('dedicated runtime host owner is closed')
    }
    if (this.role !== 'attached') {
      throw new Error(`dedicated runtime host role is ${this.role}`)
    }

    const controller = navigator.serviceWorker?.controller
    if (!controller) {
      throw new Error('service worker controller unavailable')
    }

    const ackChannel = new MessageChannel()
    const {
      promise: ackPromise,
      resolve: resolveAck,
      reject: rejectAck,
    } = Promise.withResolvers<ConnectWebRuntimeAck>()
    const pendingChannel: DedicatedWorkerHostOpen = {
      port: ackChannel.port1,
      reject: rejectAck,
    }
    this.pendingClientChannels.add(pendingChannel)
    ackChannel.port1.onmessage = (ev) => {
      const data: ConnectWebRuntimeAck = ev.data
      if (!data || !data.from) {
        return
      }
      if (!data.webRuntimePort && ev.ports?.[0]) {
        data.webRuntimePort = ev.ports[0]
      }
      resolveAck(data)
    }
    ackChannel.port1.start()

    markStartupBoundary('dedicated-host.attach-open-start', {
      source: 'browser',
      documentId: this.webDocumentId,
      runtimeId: this.webRuntimeId,
    })
    const msg: WebDocumentToWorker = {
      from: this.webDocumentId,
      connectDedicatedRuntimeHost: {
        webRuntimeId: this.webRuntimeId,
        init: WebRuntimeClientInit.toBinary(init),
        port: ackChannel.port2,
      },
    }

    try {
      controller.postMessage(msg, [ackChannel.port2])
      const ack = await ackPromise
      if (ack.error) {
        throw new Error(ack.error)
      }
      if (!ack.webRuntimePort) {
        throw new Error('dedicated runtime host ack missing runtime port')
      }
      if (!ack.hostDocumentId || !ack.hostGeneration) {
        throw new Error(
          'dedicated runtime host ack missing elected host identity',
        )
      }
      this.connectedHostDocumentId = ack.hostDocumentId
      this.connectedHostGeneration = ack.hostGeneration
      const { hostDocumentId, hostGeneration } = ack
      this.watchLifeLock(
        buildRuntimeLifeLockName(this.webRuntimeId, hostGeneration),
        () => this.callbacks?.attachedHostGone(hostDocumentId, hostGeneration),
      )
      markStartupBoundary('dedicated-host.attach-open-ready', {
        source: 'browser',
        documentId: this.webDocumentId,
        runtimeId: this.webRuntimeId,
        hostDocumentId: ack.hostDocumentId,
        hostGeneration: ack.hostGeneration,
      })
      return ack.webRuntimePort
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      markStartupBoundary('dedicated-host.attach-open-failed', {
        source: 'browser',
        documentId: this.webDocumentId,
        runtimeId: this.webRuntimeId,
        error: message,
      })
      throw err
    } finally {
      this.pendingClientChannels.delete(pendingChannel)
      ackChannel.port1.close()
    }
  }

  private cancelPendingClientChannels(reason: string): void {
    const err = new DOMException(reason, 'AbortError')
    for (const pending of this.pendingClientChannels) {
      pending.port.postMessage({
        cancelDedicatedRuntimeHostConnect: true,
      } satisfies DedicatedRuntimeHostConnectControl)
      pending.reject(err)
    }
    this.pendingClientChannels.clear()
  }

  public close(): void {
    if (this.closed) {
      return
    }
    this.connectedHostDocumentId = undefined
    this.connectedHostGeneration = undefined
    this.closed = true
    this.role = 'closed'
    this.cancelPendingClientChannels('dedicated runtime host owner closed')
    this.endTerm('dedicated runtime host owner closed')
    this.standingAbort?.abort()
    this.standingAbort = undefined
    this.closeLease?.()
    this.closeLease = undefined
    markStartupBoundary('dedicated-host.closed', {
      source: 'browser',
      documentId: this.webDocumentId,
      runtimeId: this.webRuntimeId,
      generation: this.generation,
    })
  }
}
