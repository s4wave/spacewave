import { WebRuntimeClientInit } from '../runtime/runtime.pb.js'
import {
  ConnectWebRuntimeAck,
  WebDocumentToWorker,
} from '../runtime/runtime.js'

import { randomId } from './random-id.js'
import { markStartupBoundary } from './startup-marks.js'

export type DedicatedWorkerHostRole =
  | 'pending'
  | 'host'
  | 'attached'
  | 'unavailable'
  | 'closed'

export interface DedicatedWorkerHostStartCallbacks {
  startHost(generation: string): void
  startAttached(): void
  startUnavailable(): void
}

export function buildDedicatedWorkerHostLockName(runtimeId: string): string {
  return `bldr-dedicated-runtime-host-${runtimeId}`
}

// DedicatedWorkerHostOwner makes Chromium issue 528332884's temporary
// DedicatedWorker fallback keep SharedWorker's one runtime-host owner shape.
export class DedicatedWorkerHostOwner {
  public role: DedicatedWorkerHostRole = 'pending'
  public generation?: string
  private leaseStarted = false
  private closed = false
  private closeLease?: () => void
  private callbacks?: DedicatedWorkerHostStartCallbacks

  constructor(
    private readonly webRuntimeId: string,
    private readonly webDocumentId: string,
  ) {}

  public start(callbacks: DedicatedWorkerHostStartCallbacks): void {
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
      })
      callbacks.startUnavailable()
      return
    }

    const lockName = buildDedicatedWorkerHostLockName(this.webRuntimeId)
    markStartupBoundary('dedicated-host.election-start', {
      source: 'browser',
      documentId: this.webDocumentId,
      runtimeId: this.webRuntimeId,
      lockName,
    })

    let selected = false
    Promise.resolve(
      navigator.locks.request(
        lockName,
        {
          ifAvailable: true,
        },
        (lock) => {
          if (this.closed) {
            return undefined
          }
          selected = true
          if (!lock) {
            this.role = 'attached'
            markStartupBoundary('dedicated-host.attach-selected', {
              source: 'browser',
              documentId: this.webDocumentId,
              runtimeId: this.webRuntimeId,
            })
            callbacks.startAttached()
            return undefined
          }

          this.role = 'host'
          this.generation = `${this.webDocumentId}-${randomId()}`
          markStartupBoundary('dedicated-host.lease-acquired', {
            source: 'browser',
            documentId: this.webDocumentId,
            runtimeId: this.webRuntimeId,
            generation: this.generation,
          })
          callbacks.startHost(this.generation)
          return new Promise<void>((resolve) => {
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
      if (!selected) {
        this.role = 'unavailable'
        callbacks.startUnavailable()
      }
    })
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
    const ackPromise = new Promise<ConnectWebRuntimeAck>((resolve) => {
      ackChannel.port1.onmessage = (ev) => {
        const data: ConnectWebRuntimeAck = ev.data
        if (!data || !data.from) {
          return
        }
        if (!data.webRuntimePort && ev.ports?.[0]) {
          data.webRuntimePort = ev.ports[0]
        }
        resolve(data)
      }
      ackChannel.port1.start()
    })

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
      markStartupBoundary('dedicated-host.attach-open-ready', {
        source: 'browser',
        documentId: this.webDocumentId,
        runtimeId: this.webRuntimeId,
        hostDocumentId: ack.from,
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
      this.restartElection(message)
      throw err
    } finally {
      ackChannel.port1.close()
    }
  }

  public handleHostLost(reason: string): void {
    this.restartElection(reason)
  }

  private restartElection(reason: string): void {
    if (this.closed || this.role !== 'attached' || !this.callbacks) {
      return
    }
    this.role = 'pending'
    this.leaseStarted = false
    markStartupBoundary('dedicated-host.attach-lost', {
      source: 'browser',
      documentId: this.webDocumentId,
      runtimeId: this.webRuntimeId,
      reason,
    })
    this.start(this.callbacks)
  }

  public close(): void {
    if (this.closed) {
      return
    }
    this.closed = true
    this.role = 'closed'
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

