// opfsBridgePortGlobal is the global key holding the active OpfsBridgeClient.
// The Go WASM/GoScript runtime reads it at startup via InstallRemoteDriverFromGlobal
// and routes File System Access ops through the dedicated OPFS bridge worker, so
// the port must be installed before the Go process starts.
const opfsBridgePortGlobal = '__spacewaveOpfsBridgePort'

type OpfsRequestID = number

type OpfsBridgeGlobal = typeof globalThis & {
  __spacewaveOpfsBridgePort?: OpfsBridgeClient
  __spacewaveInstallOpfsRemoteDriver?: (client: OpfsBridgeClient) => boolean
}

type OpfsBridgeResponse = {
  id?: OpfsRequestID
  ok?: boolean
  result?: unknown
  error?: { name?: string; message?: string }
}

// OpfsBridgeClient correlates id-tagged requests to a DedicatedWorker OPFS
// bridge over a MessagePort, exposing a promise-based request(). It holds the
// bridge MessagePort exclusively; closing it rejects in-flight requests so a
// port swap (re-host) fails the Go side deterministically instead of hanging.
export class OpfsBridgeClient {
  private readonly pending = new Map<
    OpfsRequestID,
    { resolve: (value: unknown) => void; reject: (reason?: unknown) => void }
  >()

  private nextID = 0

  public constructor(private readonly port: MessagePort) {
    port.addEventListener(
      'message',
      (event: MessageEvent<OpfsBridgeResponse>) => {
        this.receive(event.data)
      },
    )
    port.start()
  }

  public request(
    op: string,
    args: unknown,
    transfer?: Transferable[],
  ): Promise<unknown> {
    const id = ++this.nextID
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject })
      this.port.postMessage({ id, op, args }, transfer ?? [])
    })
  }

  public close(): void {
    this.port.close()
    for (const { reject } of this.pending.values()) {
      reject(new DOMException('OPFS bridge closed', 'AbortError'))
    }
    this.pending.clear()
  }

  private receive(response: OpfsBridgeResponse): void {
    if (typeof response.id !== 'number') {
      return
    }
    const pending = this.pending.get(response.id)
    if (!pending) {
      return
    }
    this.pending.delete(response.id)
    if (response.ok === false) {
      pending.reject(remoteError(response.error))
      return
    }
    pending.resolve(response.result)
  }
}

function remoteError(error: OpfsBridgeResponse['error']): Error {
  const err = new Error(error?.message ?? 'OPFS bridge request failed')
  err.name = error?.name ?? 'Error'
  return err
}

// setOpfsBridgePort publishes a bridge port to the WASM global and swaps any
// running remote driver onto it. The previous client is closed first so its
// in-flight Go requests reject and the volume controller remounts on the
// resulting stale-handle error.
export function setOpfsBridgePort(port: MessagePort): OpfsBridgeClient {
  const globals = globalThis as OpfsBridgeGlobal
  const previous = globals[opfsBridgePortGlobal]
  const client = new OpfsBridgeClient(port)
  previous?.close()
  globals[opfsBridgePortGlobal] = client
  globals.__spacewaveInstallOpfsRemoteDriver?.(client)
  return client
}

// clearOpfsBridgePort closes and removes the active bridge client. The host is
// gone and no replacement port exists yet, so in-flight Go requests must reject
// now (the volume controller remounts on the error) instead of hanging on a
// dead MessagePort until a re-host happens to install a fresh client.
export function clearOpfsBridgePort(): void {
  const globals = globalThis as OpfsBridgeGlobal
  const previous = globals[opfsBridgePortGlobal]
  if (!previous) {
    return
  }
  delete globals[opfsBridgePortGlobal]
  previous.close()
}
