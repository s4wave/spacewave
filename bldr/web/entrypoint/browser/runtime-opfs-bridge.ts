import {
  clearOpfsBridgePort,
  setOpfsBridgePort,
} from '../../runtime/opfs-bridge-client.js'
import {
  buildWebDocumentLockName,
  ClientToWebDocument,
  WebDocumentToClient,
} from '../../runtime/runtime.js'

interface TrackedWebDocument {
  port: MessagePort
  lockAbort?: AbortController
}

interface PendingRequest {
  webDocumentId: string
  resolve: (port: MessagePort | null) => void
}

// RuntimeOpfsBridge brokers a DedicatedWorker OPFS bridge for the engine runtime
// when it runs in a SharedWorker, which cannot call
// navigator.storage.getDirectory() (Chrome throws SecurityError). It tracks the
// broker MessagePort of each connected WebDocument, requests an OPFS worker from
// a live document, installs it as the WASM global bridge port via
// setOpfsBridgePort, and re-hosts from a surviving document when the hosting
// document closes or its worker dies. WebDocument liveness uses the same
// per-document Web Lock the WebDocumentTracker uses, so a hidden or throttled
// tab is not mistaken for a closed one.
//
// The broker channel speaks the existing ClientToWebDocument (request) /
// WebDocumentToClient (ack) OPFS messages; the WebDocument owns OPFS worker
// creation and emits the runtime.opfs-bridge-ready startup mark.
export class RuntimeOpfsBridge {
  private readonly webDocuments = new Map<string, TrackedWebDocument>()
  private hostWebDocumentId?: string
  private pendingRequest?: PendingRequest
  private ensurePromise?: Promise<boolean>
  private closed = false

  public constructor(private readonly workerId: string) {}

  // addWebDocument tracks a WebDocument broker port and ensures a bridge exists.
  // A reconnect for the same document replaces the prior port.
  public addWebDocument(webDocumentId: string, port: MessagePort): void {
    if (this.closed || !webDocumentId) {
      port.close()
      return
    }
    this.removeWebDocument(webDocumentId)

    const tracked: TrackedWebDocument = { port }
    this.webDocuments.set(webDocumentId, tracked)
    port.onmessage = (ev: MessageEvent<WebDocumentToClient>) => {
      this.handleWebDocumentMessage(webDocumentId, ev)
    }
    port.start()
    this.watchWebDocumentLiveness(webDocumentId, tracked)

    void this.ensureBridge()
  }

  // ensureBridge installs a bridge from a live WebDocument if none is hosted.
  // The runtime worker awaits the first call before starting the Go process so
  // the global bridge port is set before RemoteDriver.GetRoot() runs.
  public ensureBridge(): Promise<boolean> {
    if (this.closed) {
      return Promise.resolve(false)
    }
    if (this.hostWebDocumentId && this.webDocuments.has(this.hostWebDocumentId)) {
      return Promise.resolve(true)
    }
    if (this.ensurePromise) {
      return this.ensurePromise
    }
    this.ensurePromise = this.requestBridge().finally(() => {
      this.ensurePromise = undefined
    })
    return this.ensurePromise
  }

  // close tears down all tracked documents and the pending request.
  public close(): void {
    if (this.closed) {
      return
    }
    this.closed = true
    for (const webDocumentId of [...this.webDocuments.keys()]) {
      this.removeWebDocument(webDocumentId)
    }
    this.resolvePending(null)
  }

  // requestBridge tries every live WebDocument, freshest first, until one opens
  // an OPFS worker. It re-reads webDocuments each iteration so a document that
  // connects while an earlier openOpfsWorker is in flight is still tried, and one
  // document that cannot host does not strand the runtime when another can. The
  // tried set keys on the tracked object, not the id, so a same-id reconnect
  // (addWebDocument replaces the port with a fresh tracked object) is retried
  // even when an earlier attempt for that id already failed this loop.
  private async requestBridge(): Promise<boolean> {
    const tried = new Set<TrackedWebDocument>()
    while (!this.closed) {
      const entry = [...this.webDocuments.entries()]
        .reverse()
        .find(([, tracked]) => !tried.has(tracked))
      if (!entry) {
        return false
      }
      const [webDocumentId, tracked] = entry
      tried.add(tracked)
      const port = await this.openOpfsWorker(webDocumentId)
      if (!port) {
        continue
      }
      setOpfsBridgePort(port)
      this.hostWebDocumentId = webDocumentId
      console.log(
        `RuntimeOpfsBridge: ${this.workerId}: OPFS bridge hosted by ${webDocumentId}`,
      )
      return true
    }
    return false
  }

  // onHostLost reacts to the hosting document or its OPFS worker dying. It clears
  // the installed client immediately so pending Go requests reject deterministically
  // rather than hang on the dead port, then re-hosts from a surviving document.
  private onHostLost(): void {
    this.hostWebDocumentId = undefined
    clearOpfsBridgePort()
    if (!this.closed && this.webDocuments.size) {
      void this.ensureBridge()
    }
  }

  // openOpfsWorker sends an openOpfsWorker request to a WebDocument and resolves
  // with the bridge MessagePort, or null if the document errors or closes. The
  // WebDocument always replies with an ack (success or error); document death is
  // surfaced by the liveness watch rejecting the pending request.
  private openOpfsWorker(webDocumentId: string): Promise<MessagePort | null> {
    const tracked = this.webDocuments.get(webDocumentId)
    if (!tracked) {
      return Promise.resolve(null)
    }
    return new Promise<MessagePort | null>((resolve) => {
      this.resolvePending(null)
      this.pendingRequest = { webDocumentId, resolve }
      const msg: ClientToWebDocument = {
        from: this.workerId,
        openOpfsWorker: true,
      }
      try {
        tracked.port.postMessage(msg)
      } catch (err) {
        console.warn(
          `RuntimeOpfsBridge: ${this.workerId}: OPFS request to ${webDocumentId} failed:`,
          err,
        )
        this.resolvePending(null)
      }
    })
  }

  private handleWebDocumentMessage(
    webDocumentId: string,
    ev: MessageEvent<WebDocumentToClient>,
  ): void {
    const data = ev.data
    if (typeof data !== 'object' || data === null) {
      return
    }

    if (data.openOpfsWorkerAck) {
      if (this.pendingRequest?.webDocumentId !== webDocumentId) {
        ev.ports?.[0]?.close()
        return
      }
      const port = ev.ports?.[0]
      if (data.openOpfsWorkerAck.error || !port) {
        port?.close()
        this.resolvePending(null)
        return
      }
      this.resolvePending(port)
      return
    }

    if (data.opfsWorkerClosed) {
      if (this.hostWebDocumentId === webDocumentId) {
        this.onHostLost()
      }
      return
    }

    if (data.close) {
      this.removeWebDocument(webDocumentId)
    }
  }

  // watchWebDocumentLiveness re-hosts the bridge when a tracked document closes.
  // The document holds its liveness lock while alive (web-document.ts); acquiring
  // it means the document is gone.
  private watchWebDocumentLiveness(
    webDocumentId: string,
    tracked: TrackedWebDocument,
  ): void {
    if (typeof navigator === 'undefined' || !navigator.locks) {
      return
    }
    const lockAbort = new AbortController()
    tracked.lockAbort = lockAbort
    navigator.locks
      .request(
        buildWebDocumentLockName(webDocumentId),
        { signal: lockAbort.signal },
        () => {
          if (this.webDocuments.get(webDocumentId) === tracked) {
            this.removeWebDocument(webDocumentId)
          }
          return Promise.resolve()
        },
      )
      .catch((err: unknown) => {
        if (isAbortError(err)) {
          return
        }
        console.warn(
          `RuntimeOpfsBridge: ${this.workerId}: liveness watch failed for ${webDocumentId}:`,
          err,
        )
      })
  }

  private removeWebDocument(webDocumentId: string): void {
    const tracked = this.webDocuments.get(webDocumentId)
    if (!tracked) {
      return
    }
    this.webDocuments.delete(webDocumentId)
    tracked.lockAbort?.abort()
    tracked.port.onmessage = null
    tracked.port.close()

    if (this.pendingRequest?.webDocumentId === webDocumentId) {
      this.resolvePending(null)
    }
    if (this.hostWebDocumentId === webDocumentId) {
      this.onHostLost()
    }
  }

  private resolvePending(port: MessagePort | null): void {
    const pending = this.pendingRequest
    if (!pending) {
      return
    }
    this.pendingRequest = undefined
    pending.resolve(port)
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
