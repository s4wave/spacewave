import { WebRuntimeClientInit } from '../runtime/runtime.pb.js'
import {
  ConnectWebRuntimeAck,
  type DedicatedRuntimeHostConnectControl,
  WebDocumentToWorker,
} from '../runtime/runtime.js'

import { randomId } from './random-id.js'
import { markStartupBoundary } from './startup-marks.js'

// DedicatedWorkerHostRole is the host-election role of this document in the
// dedicated worker fallback.
export type DedicatedWorkerHostRole =
  | 'pending'
  | 'host'
  | 'attached'
  | 'unavailable'
  | 'closed'

// DedicatedWorkerHostCallbacks receives host-state transitions.
export interface DedicatedWorkerHostCallbacks {
  startHost(generation: string): void
  startAttached(): void
  startUnavailable(): void
  promoteToHost(): void
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
          this.onLockGranted()
          await new Promise<void>((resolve) => {
            this.closeLease = resolve
          })
        },
      ),
    ).catch((err: unknown) => {
      if (this.closed) {
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

  private onLockGranted(): void {
    if (this.role === 'closed') {
      this.closeLease?.()
      return
    }
    if (this.role !== 'pending' && this.role !== 'attached') {
      return
    }

    const promoted = this.role === 'attached'
    this.role = 'host'
    this.generation = `${this.webDocumentId}-${randomId()}`
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
      return
    }
    this.callbacks?.startHost(this.generation)
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
