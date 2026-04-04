import { HandleStreamFunc } from 'starpc'

import { WebDocumentTracker } from '../bldr/web-document-tracker.js'
import { WebDocumentToWorker } from './runtime.js'
import { WebRuntimeClientType } from './runtime.pb.js'
import { PluginStartInfo } from '../../plugin/plugin.pb.js'

export function checkSharedWorker(
  scope: SharedWorkerGlobalScope | DedicatedWorkerGlobalScope,
): scope is SharedWorkerGlobalScope {
  return (
    typeof SharedWorkerGlobalScope !== 'undefined' && // eslint-disable-line
    scope instanceof SharedWorkerGlobalScope
  )
}

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

  constructor(
    public readonly global:
      | SharedWorkerGlobalScope
      | DedicatedWorkerGlobalScope,
    private readonly startPlugin: (startInfo: PluginStartInfo) => void,
    handleIncomingStream: HandleStreamFunc | null,
  ) {
    // webDocumentTracker tracks the set of connected remote WebDocument.
    this.webDocumentTracker = new WebDocumentTracker(
      this.workerId,
      WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER,
      this.onWebDocumentsExhausted.bind(this),
      handleIncomingStream,
    )

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
    this.webDocumentTracker.close()
    this.global.close()
  }

  // handleStartPlugin handles the message to start the plugin.
  private handleStartPlugin(startInfoBin: Uint8Array) {
    if (this.pluginStarted) return
    this.pluginStarted = true

    // startInfo is b64 encoded json
    const startInfoJsonB64 = new TextDecoder().decode(startInfoBin)
    const startInfoJson = atob(startInfoJsonB64)
    const startInfo = PluginStartInfo.fromJsonString(startInfoJson)

    this.startPlugin(startInfo)
  }

  private handleWorkerMessage(msgEvent: MessageEvent<WebDocumentToWorker>) {
    // Expect the WebDocument to send a WebDocumentToWorker.
    const data: WebDocumentToWorker = msgEvent.data
    this.webDocumentTracker.handleWebDocumentMessage(data)

    if (data.initData) {
      this.handleStartPlugin(data.initData)

      // trigger connecting to web runtime
      this.webDocumentTracker.waitConn().catch((err) => {
        console.warn(
          `PluginWorker: ${this.workerId}: unable to contact WebRuntime, exiting!`,
          err,
        )
        this.webDocumentTracker.close()
        this.global.close()
      })
    }
  }
}
