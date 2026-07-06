import { HandleStreamFunc } from 'starpc'

import { WebDocumentTracker } from '../bldr/web-document-tracker.js'
import { timeoutPromise } from '../bldr/timeout.js'
import type { WorkerCommsDetectResult } from '../bldr/worker-comms-detect.js'
import { setOpfsBridgePort } from './opfs-bridge-client.js'
import {
  buildWebWorkerLockName,
  ClientToWebDocument,
  WebDocumentToWorker,
} from './runtime.js'
import { WebRuntimeClientType } from './runtime.pb.js'
import { PluginStartInfo } from '../../plugin/plugin.pb.js'
import { installWebRTCShim, setBridgePort } from './wasm/webrtc-bridge.js'

export const PLUGIN_STARTUP_FAILURE_SHUTDOWN_DELAY_MS = 5000

// shouldRequestOpfsBridge reports whether this plugin worker must host its own
// OPFS bridge. Only Config A/F runs the plugin as a SharedWorker that backs its
// own OPFS storage, and a SharedWorker scope cannot call
// navigator.storage.getDirectory() (Chrome throws SecurityError). In B/C the
// plugin's storage routes through the engine runtime worker, which owns the
// SharedWorker OPFS bridge for the shared engine; requesting a second bridge
// from a B/C plugin worker only blocks native plugin startup on a document that
// is already tearing down, so the plugin must skip it.
function shouldRequestOpfsBridge(
  workerCommsDetect?: WorkerCommsDetectResult,
): boolean {
  const config = workerCommsDetect?.config
  return config === 'A' || config === 'F'
}

export function waitPluginStartupFailureShutdownDelay(): Promise<void> {
  return timeoutPromise(PLUGIN_STARTUP_FAILURE_SHUTDOWN_DELAY_MS)
}

export function checkSharedWorker(
  scope: SharedWorkerGlobalScope | DedicatedWorkerGlobalScope,
): scope is SharedWorkerGlobalScope {
  return (
    typeof SharedWorkerGlobalScope !== 'undefined' &&
    scope instanceof SharedWorkerGlobalScope
  )
}

export function parsePluginWorkerName(name: string): string {
  return name.split('?')[0] ?? ''
}

// PluginStartOpts contains start info and worker communication detection.
export interface PluginStartOpts {
  startInfo: PluginStartInfo
  workerCommsDetect?: WorkerCommsDetectResult
  runtimeWasmEnv?: Record<string, string>
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
    return parsePluginWorkerName(this.global.name)
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
  // failureCloseReported records that this worker has already published a fatal close.
  private failureCloseReported?: boolean
  // shuttingDown records that worker shutdown has already started.
  private shuttingDown?: boolean
  // shouldMaintainOpfsBridge records that this worker needs a dedicated OPFS bridge.
  private shouldMaintainOpfsBridge?: boolean
  // opfsBridgeRefresh tracks an in-flight OPFS bridge replacement.
  private opfsBridgeRefresh?: Promise<void>
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
      null,
      undefined,
      this.refreshOpfsBridge.bind(this),
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

  // onWebDocumentsExhausted observes a transient zero-WebDocument window.
  private async onWebDocumentsExhausted() {
    // A page reload can detach the old WebDocument before the replacement
    // attaches; re-adoptable workers must let WebDocumentTracker's
    // next-document waiter resume the in-flight open. Genuine orphan teardown
    // stays with the worker liveness lock and browser worker reclamation.
    console.log(
      `PluginWorker: ${this.workerId}: no WebDocument available, waiting for next WebDocument`,
    )
  }

  // armWorkerLock acquires a worker-scoped liveness lock before runtime registration.
  private armWorkerLock() {
    if (
      typeof navigator === 'undefined' ||
      !navigator.locks ||
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
  private async shutdown() {
    if (this.shuttingDown) {
      return
    }
    this.shuttingDown = true
    this.lockAbortController?.abort()
    this.lockAbortController = undefined
    this.webDocumentTracker.close()
    await timeoutPromise(0)
    this.global.close()
  }

  public async reportRuntimeFailure(err: unknown) {
    this.notifyFailureClose(err)
    await waitPluginStartupFailureShutdownDelay()
    await this.shutdown()
  }

  public notifyStartupMark(label: string, detail?: Record<string, unknown>) {
    const msg: ClientToWebDocument = {
      from: this.workerId,
      startupMark: {
        label,
        startTimeMs:
          typeof performance !== 'undefined' ? performance.now() : undefined,
        detail,
      },
    }
    this.webDocumentTracker.postMessage(msg)
  }

  // handleStartPlugin handles the message to start the plugin.
  private async handleStartPlugin(
    startInfoBin: Uint8Array,
    workerCommsDetect?: WorkerCommsDetectResult,
    runtimeWasmEnv?: Record<string, string>,
  ) {
    if (this.startPluginPromise) {
      await this.startPluginPromise
      this.notifyReady()
      return
    }

    this.startPluginPromise = this.startPluginImpl(
      startInfoBin,
      workerCommsDetect,
      runtimeWasmEnv,
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
    workerCommsDetect?: WorkerCommsDetectResult,
    runtimeWasmEnv?: Record<string, string>,
  ) {
    // startInfo is b64 encoded json
    const startInfoJsonB64 = new TextDecoder().decode(startInfoBin)
    const startInfoJson = atob(startInfoJsonB64)
    const startInfo = PluginStartInfo.fromJsonString(startInfoJson)
    this.notifyStartupMark('worker.start-info-decoded')

    this.notifyStartupMark('worker.runtime-connect-wait-start')
    await this.webDocumentTracker.waitConn()
    this.notifyStartupMark('worker.runtime-connect-wait-ready')

    // Request a WebRTC bridge port from the WebDocument before starting the
    // plugin. The bridge port must be available before patchWorkerBrowserGlobals()
    // runs in GoWasmProcess so the RTCPeerConnection shim can be installed.
    const bridgePort = await this.webDocumentTracker.requestWebRtcBridge()
    if (bridgePort) {
      setBridgePort(bridgePort)
      installWebRTCShim()
      console.log(`PluginWorker: ${this.workerId}: WebRTC bridge enabled`)
    }
    this.notifyStartupMark('worker.webrtc-bridge-ready', {
      enabled: !!bridgePort,
    })

    this.shouldMaintainOpfsBridge = shouldRequestOpfsBridge(workerCommsDetect)
    if (this.shouldMaintainOpfsBridge) {
      await this.requestAndInstallOpfsBridge('worker.opfs-bridge-ready')
    } else {
      this.notifyStartupMark('worker.opfs-bridge-ready', { enabled: false })
    }

    this.notifyStartupMark('plugin.entrypoint-start')
    await this.startPlugin({
      startInfo,
      workerCommsDetect,
      runtimeWasmEnv,
    })
    this.notifyStartupMark('plugin.entrypoint-ready')
    this.pluginStarted = true
  }

  private async requestAndInstallOpfsBridge(label: string): Promise<boolean> {
    // Publish the bridge port to the WASM global and swap any running remote
    // driver onto it. Readiness and OPFS failure semantics are owned by the Go
    // RemoteDriver.GetRoot() call during volume mount and the volume
    // controller's terminal path, so the worker does not run its own getRoot
    // handshake here.
    const opfsPort = await this.webDocumentTracker.requestOpfsWorker()
    if (opfsPort) {
      setOpfsBridgePort(opfsPort)
    }
    this.notifyStartupMark(label, {
      enabled: !!opfsPort,
    })
    return !!opfsPort
  }

  // refreshOpfsBridge re-hosts the OPFS bridge after the host WebDocument was
  // removed or its bridge worker died. Swapping the port closes the prior
  // OpfsBridgeClient, which rejects in-flight Go requests, and installs a fresh
  // worker; the volume controller remounts on the resulting stale-handle error.
  private refreshOpfsBridge() {
    if (!this.shouldMaintainOpfsBridge || this.shuttingDown) {
      return
    }
    if (this.opfsBridgeRefresh) {
      return
    }
    this.opfsBridgeRefresh = this.requestAndInstallOpfsBridge(
      'worker.opfs-bridge-refreshed',
    )
      .then(() => undefined)
      .catch((err: unknown) => {
        console.warn(
          `PluginWorker: ${this.workerId}: OPFS bridge refresh failed:`,
          err,
        )
      })
      .finally(() => {
        this.opfsBridgeRefresh = undefined
      })
  }

  // notifyFrontendReady notifies connected web documents that frontend setup completed.
  public notifyFrontendReady() {
    const msg: ClientToWebDocument = {
      from: this.workerId,
      frontendReady: true,
    }
    this.webDocumentTracker.postMessage(msg)
  }

  // notifyCapabilityReady notifies connected web documents that startup capability is ready.
  public notifyCapabilityReady() {
    const msg: ClientToWebDocument = {
      from: this.workerId,
      capabilityReady: true,
    }
    this.webDocumentTracker.postMessage(msg)
  }

  // notifyReady notifies all connected web documents that startup completed.
  private notifyReady() {
    const msg: ClientToWebDocument = {
      from: this.workerId,
      ready: true,
    }
    this.webDocumentTracker.postMessage(msg)
  }

  // notifyFailureClose notifies connected documents that the worker is closing
  // because plugin startup or runtime execution failed.
  private notifyFailureClose(err: unknown) {
    if (this.failureCloseReported) {
      return
    }
    this.failureCloseReported = true
    const msg: ClientToWebDocument = {
      from: this.workerId,
      close: true,
      failureReason: stringifyError(err),
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
      this.notifyStartupMark('worker.init-message-received')
      this.handleStartPlugin(
        data.initData,
        data.workerCommsDetect,
        data.runtimeWasmEnv,
      ).catch((err) => {
        if (isExpectedPluginWorkerShutdownError(err)) {
          console.warn(
            `PluginWorker: ${this.workerId}: startup canceled because WebDocument closed`,
          )
          return
        }
        console.warn(
          `PluginWorker: ${this.workerId}: startup failed, exiting!`,
          err,
        )
        void this.reportRuntimeFailure(err)
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

function isExpectedPluginWorkerShutdownError(err: unknown): boolean {
  const msg = err instanceof Error ? err.message : String(err)
  return msg.includes('closed while waiting for WebDocument')
}

function stringifyError(err: unknown): string {
  if (err instanceof Error) {
    return err.message || err.toString()
  }
  if (typeof err === 'string') {
    return err
  }
  return String(err)
}
