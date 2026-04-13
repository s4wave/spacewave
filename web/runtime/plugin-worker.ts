import { HandleStreamFunc } from 'starpc'

import { WebDocumentTracker } from '../bldr/web-document-tracker.js'
import type { WorkerCommsDetectResult } from '../bldr/worker-comms-detect.js'
import {
  buildWebWorkerLockName,
  ClientToWebDocument,
  WebDocumentToWorker,
} from './runtime.js'
import { WebRuntimeClientType } from './runtime.pb.js'
import { PluginStartInfo } from '../../plugin/plugin.pb.js'
import { installWebRTCShim, setBridgePort } from './wasm/webrtc-bridge.js'

export function checkSharedWorker(
  scope: SharedWorkerGlobalScope | DedicatedWorkerGlobalScope,
): scope is SharedWorkerGlobalScope {
  return (
    typeof SharedWorkerGlobalScope !== 'undefined' &&
    scope instanceof SharedWorkerGlobalScope
  )
}

// PluginStartOpts contains start info and optional bus configuration.
export interface PluginStartOpts {
  startInfo: PluginStartInfo
  busSab?: SharedArrayBuffer
  busPluginId?: number
  workerCommsDetect?: WorkerCommsDetectResult
}

// SnapshotNowCallback is called when the WebDocument requests an urgent snapshot.
export type SnapshotNowCallback = () => void

// PluginWorker wraps common logic for running a plugin within a WebWorker or SharedWorker.
export class PluginWorker {
  // webDocumentTracker tracks the set of connected WebDocument.
  public readonly webDocumentTracker: WebDocumentTracker

  // isSharedWorker checks if this is a shared worker.
  get isSharedWorker() {
    return checkSharedWorker(this.global)
  }

  // workerId is the id to use for the worker.
  get workerId() {
    return this.global.name
  }

  // webRuntimeClient is the connection to the WebRuntime.
  get webRuntimeClient() {
    return this.webDocumentTracker.webRuntimeClient
  }

  // started returns if the plugin was started yet.
  get started() {
    return this.pluginStarted ?? false
  }

  // pluginStarted is the private field for started.
  private pluginStarted?: true
  // startPluginPromise tracks the in-flight startup sequence.
  private startPluginPromise?: Promise<void>
  // lockAbortController aborts the worker liveness lock on shutdown.
  private lockAbortController?: AbortController
  // onSnapshotNow is called when the WebDocument requests an urgent snapshot.
  public onSnapshotNow?: SnapshotNowCallback

  constructor(
    public readonly global:
      | SharedWorkerGlobalScope
      | DedicatedWorkerGlobalScope,
    private readonly startPlugin: (opts: PluginStartOpts) => Promise<void>,
    handleIncomingStream: HandleStreamFunc | null,
  ) {
    // webDocumentTracker tracks the set of connected remote WebDocument.
    this.webDocumentTracker = new WebDocumentTracker(
      this.workerId,
      WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER,
      this.onWebDocumentsExhausted.bind(this),
      handleIncomingStream,
    )
    this.armWorkerLock()

    if (checkSharedWorker(global)) {
      // If this is a SharedWorker, handle the "connect" event when a WebDocument connects.
      global.addEventListener('connect', (ev) => {
        // With a shared worker, "connect" is fired when "new SharedWorker" is called.
        // The port passed with the event is connected to the sharedWorker.port on the WebDocument.
        const ports = ev.ports
        if (!ports || !ports.length) {
          return
        }

        const port = ev.ports[0]
        if (!port) {
          return
        }

        port.onmessage = this.handleWorkerMessage.bind(this)
        port.start()
      })
    } else {
      // Otherwise this must be a DedicatedWorker.
      global.addEventListener('message', this.handleWorkerMessage.bind(this))
    }
  }

  // onWebDocumentsExhausted handles when no WebDocument can be contacted anymore.
  private async onWebDocumentsExhausted() {
    // Unlike the ServiceWorker, the WebWorker / SharedWorker has no way to
    // contact a WebDocument proactively. (client.postMessage). If there are no
    // available connections to WebDocument, then we should exit.
    console.log(
      `PluginWorker: ${this.workerId}: no WebDocument available, exiting!`,
    )
    this.shutdown()
  }

  // armWorkerLock acquires a worker-scoped liveness lock before runtime registration.
  private armWorkerLock() {
    if (
      typeof navigator === 'undefined' ||
      !('locks' in navigator) ||
      this.lockAbortController
    ) {
      return
    }

    this.lockAbortController = new AbortController()
    navigator.locks
      .request(
        buildWebWorkerLockName(this.workerId),
        { signal: this.lockAbortController.signal },
        () => {
          return new Promise<void>(() => {})
        },
      )
      .catch((err) => {
        if (isAbortError(err)) {
          return
        }
        console.warn(
          `PluginWorker: ${this.workerId}: worker liveness lock failed`,
          err,
        )
      })
  }

  // shutdown tears down the worker, releasing the liveness lock first.
  private shutdown() {
    this.lockAbortController?.abort()
    this.lockAbortController = undefined
    this.webDocumentTracker.close()
    this.global.close()
  }

  // handleStartPlugin handles the message to start the plugin.
  private async handleStartPlugin(
    startInfoBin: Uint8Array,
    busSab?: SharedArrayBuffer,
    busPluginId?: number,
    workerCommsDetect?: WorkerCommsDetectResult,
  ) {
    if (this.startPluginPromise) {
      await this.startPluginPromise
      this.notifyReady()
      return
    }

    this.startPluginPromise = this.startPluginImpl(
      startInfoBin,
      busSab,
      busPluginId,
      workerCommsDetect,
    ).catch((err) => {
      this.startPluginPromise = undefined
      throw err
    })
    await this.startPluginPromise
    this.notifyReady()
  }

  // startPluginImpl runs the actual startup sequence.
  private async startPluginImpl(
    startInfoBin: Uint8Array,
    busSab?: SharedArrayBuffer,
    busPluginId?: number,
    workerCommsDetect?: WorkerCommsDetectResult,
  ) {
    // startInfo is b64 encoded json
    const startInfoJsonB64 = new TextDecoder().decode(startInfoBin)
    const startInfoJson = atob(startInfoJsonB64)
    const startInfo = PluginStartInfo.fromJsonString(startInfoJson)

    await this.webDocumentTracker.waitConn()

    // Request a WebRTC bridge port from the WebDocument before starting the
    // plugin. The bridge port must be available before patchWorkerBrowserGlobals()
    // runs in GoWasmProcess so the RTCPeerConnection shim can be installed.
    const bridgePort =
      await this.webDocumentTracker.requestWebRtcBridge()
    if (bridgePort) {
      setBridgePort(bridgePort)
      installWebRTCShim()
      const globals = globalThis as typeof globalThis & {
        window?: typeof globalThis & { RTCPeerConnection?: unknown }
        RTCPeerConnection?: unknown
      }
      console.log(
        `PluginWorker: ${this.workerId}: WebRTC shim visible window=${typeof globals.window?.RTCPeerConnection} global=${typeof globals.RTCPeerConnection}`,
      )
      console.log(
        `PluginWorker: ${this.workerId}: WebRTC bridge port acquired`,
      )
    }

    await this.startPlugin({
      startInfo,
      busSab,
      busPluginId,
      workerCommsDetect,
    })
    this.pluginStarted = true
  }

  // notifyReady notifies all connected web documents that startup completed.
  private notifyReady() {
    const msg: ClientToWebDocument = {
      from: this.workerId,
      ready: true,
    }
    this.webDocumentTracker.postMessage(msg)
  }

  private handleWorkerMessage(msgEvent: MessageEvent<WebDocumentToWorker>) {
    // Expect the WebDocument to send a WebDocumentToWorker.
    const data: WebDocumentToWorker = msgEvent.data
    this.webDocumentTracker.handleWebDocumentMessage(data)

    if (data.snapshotNow && this.onSnapshotNow) {
      console.log(`PluginWorker: ${this.workerId}: received snapshotNow`)
      this.onSnapshotNow()
      return
    }

    if (data.initData) {
      this.handleStartPlugin(
        data.initData,
        data.busSab,
        data.busPluginId,
        data.workerCommsDetect,
      ).catch((err) => {
        console.warn(
          `PluginWorker: ${this.workerId}: startup failed, exiting!`,
          err,
        )
        this.shutdown()
      })
    }
  }
}

function isAbortError(err: unknown): boolean {
  return (
    typeof err === 'object' &&
    err !== null &&
    'name' in err &&
    (err as { name?: string }).name === 'AbortError'
  )
}
