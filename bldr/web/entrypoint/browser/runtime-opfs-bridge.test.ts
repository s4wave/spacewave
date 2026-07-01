import { afterEach, describe, expect, it, vi } from 'vitest'

import { OpfsBridgeClient } from '../../runtime/opfs-bridge-client.js'
import type {
  ClientToWebDocument,
  WebDocumentToClient,
} from '../../runtime/runtime.js'
import { RuntimeOpfsBridge } from './runtime-opfs-bridge.js'

type OpfsBridgeGlobal = typeof globalThis & {
  __spacewaveOpfsBridgePort?: OpfsBridgeClient
}

function installedClient(): OpfsBridgeClient | undefined {
  return (globalThis as OpfsBridgeGlobal).__spacewaveOpfsBridgePort
}

// installControllableLocks replaces navigator.locks so a tracked WebDocument
// lock stays held until the test releases it, matching a live document that
// holds its liveness lock while alive. Without this the real Web Lock would be
// granted immediately and the bridge would treat every document as already
// gone.
function installControllableLocks(): { release(name: string): void } {
  const releasers = new Map<string, () => void>()
  vi.stubGlobal('navigator', {
    locks: {
      request: (
        name: string,
        opts: { signal?: AbortSignal },
        cb: () => unknown,
      ) =>
        new Promise<unknown>((resolve, reject) => {
          const abort = () => reject(new DOMException('aborted', 'AbortError'))
          opts.signal?.addEventListener('abort', abort, { once: true })
          releasers.set(name, () => {
            opts.signal?.removeEventListener('abort', abort)
            resolve(cb())
          })
        }),
    },
  })
  return {
    release(name: string) {
      releasers.get(name)?.()
    },
  }
}

type FakeDocMode = 'ok' | 'error' | 'silent'

// FakeWebDocument plays the WebDocument end of a broker port. In 'ok' mode it
// answers each openOpfsWorker request with an openOpfsWorkerAck carrying a fresh
// bridge MessagePort; in 'error' mode it answers with an error ack and no port,
// modeling a live document whose OPFS worker cannot start; in 'silent' mode it
// never replies, modeling a request still in flight. It counts requests so a
// test can prove which document served a (re-)host.
class FakeWebDocument {
  public requests = 0
  public readonly bridgePorts: MessagePort[] = []
  public readonly requestIds: string[] = []
  public readonly docPort: MessagePort
  private readonly workerPort: MessagePort
  private mode: FakeDocMode = 'ok'

  public constructor(private readonly docId: string) {
    const channel = new MessageChannel()
    this.docPort = channel.port1
    this.workerPort = channel.port2
    this.docPort.onmessage = (ev: MessageEvent<ClientToWebDocument>) => {
      const request = ev.data?.openOpfsWorker
      if (!request) {
        return
      }
      this.requests++
      this.requestIds.push(request.requestId)
      if (this.mode === 'silent') {
        return
      }
      if (this.mode === 'error') {
        const ack: WebDocumentToClient = {
          from: this.docId,
          openOpfsWorkerAck: {
            from: this.docId,
            requestId: request.requestId,
            error: 'opfs unavailable',
          },
        }
        this.docPort.postMessage(ack)
        return
      }
      const bridge = new MessageChannel()
      this.bridgePorts.push(bridge.port2)
      this.sendOpenAck(request.requestId, bridge.port1)
    }
    this.docPort.start()
  }

  public attach(bridge: RuntimeOpfsBridge): void {
    bridge.addWebDocument(this.docId, this.workerPort)
  }

  public sendOpenAck(requestId: string, port: MessagePort): void {
    const ack: WebDocumentToClient = {
      from: this.docId,
      openOpfsWorkerAck: { from: this.docId, requestId },
    }
    this.docPort.postMessage(ack, [port])
  }

  public sendWorkerClosed(): void {
    const msg: WebDocumentToClient = { from: this.docId, opfsWorkerClosed: true }
    this.docPort.postMessage(msg)
  }

  public sendClose(): void {
    const msg: WebDocumentToClient = { from: this.docId, close: true }
    this.docPort.postMessage(msg)
  }

  public setMode(mode: FakeDocMode): void {
    this.mode = mode
  }
}

function addDoc(
  bridge: RuntimeOpfsBridge,
  docId: string,
  mode: FakeDocMode = 'ok',
): FakeWebDocument {
  const doc = new FakeWebDocument(docId)
  doc.setMode(mode)
  doc.attach(bridge)
  return doc
}

describe('RuntimeOpfsBridge', () => {
  afterEach(() => {
    installedClient()?.close()
    delete (globalThis as OpfsBridgeGlobal).__spacewaveOpfsBridgePort
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('hosts the bridge from a live WebDocument and installs the global port', async () => {
    installControllableLocks()
    const bridge = new RuntimeOpfsBridge('worker-1')
    const doc = addDoc(bridge, 'doc-1')

    const ok = await bridge.ensureBridge()
    expect(ok).toBe(true)
    expect(doc.requests).toBe(1)
    expect(installedClient()).toBeInstanceOf(OpfsBridgeClient)

    bridge.close()
  })

  it('reports no bridge when no WebDocument is tracked', async () => {
    installControllableLocks()
    const bridge = new RuntimeOpfsBridge('worker-1')

    expect(await bridge.ensureBridge()).toBe(false)
    expect(installedClient()).toBeUndefined()

    bridge.close()
  })

  it('re-hosts and swaps the global client when the host OPFS worker dies', async () => {
    installControllableLocks()
    const bridge = new RuntimeOpfsBridge('worker-1')
    const doc = addDoc(bridge, 'doc-1')

    await bridge.ensureBridge()
    const first = installedClient()
    expect(first).toBeInstanceOf(OpfsBridgeClient)

    doc.sendWorkerClosed()

    await vi.waitFor(() => {
      expect(doc.requests).toBe(2)
      expect(installedClient()).not.toBe(first)
    })

    bridge.close()
  })

  it('re-hosts from a surviving document when the host document closes', async () => {
    installControllableLocks()
    const bridge = new RuntimeOpfsBridge('worker-1')
    const host = addDoc(bridge, 'doc-1')

    await bridge.ensureBridge()
    expect(host.requests).toBe(1)

    const survivor = addDoc(bridge, 'doc-2')
    // The host is still live, so adding a second document must not re-request.
    await Promise.resolve()
    expect(survivor.requests).toBe(0)

    host.sendClose()

    await vi.waitFor(() => {
      expect(survivor.requests).toBe(1)
      expect(installedClient()).toBeInstanceOf(OpfsBridgeClient)
    })

    bridge.close()
  })

  it('re-hosts from a surviving document when the host document lock releases', async () => {
    const locks = installControllableLocks()
    const bridge = new RuntimeOpfsBridge('worker-1')
    const host = addDoc(bridge, 'doc-1')

    await bridge.ensureBridge()
    expect(host.requests).toBe(1)

    const survivor = addDoc(bridge, 'doc-2')
    locks.release('bldr-doc-doc-1')

    await vi.waitFor(() => {
      expect(survivor.requests).toBe(1)
    })

    bridge.close()
  })

  it('rejects in-flight requests and clears the global when the only host is lost', async () => {
    installControllableLocks()
    const bridge = new RuntimeOpfsBridge('worker-1')
    const host = addDoc(bridge, 'doc-1')

    await bridge.ensureBridge()
    const client = installedClient()
    if (!client) {
      throw new Error('expected an installed bridge client')
    }

    const pending = client.request('getRoot', {})
    host.sendClose()

    await expect(pending).rejects.toMatchObject({ name: 'AbortError' })
    expect(installedClient()).toBeUndefined()

    bridge.close()
  })

  it('tries another live document when the first cannot host', async () => {
    installControllableLocks()
    const bridge = new RuntimeOpfsBridge('worker-1')
    const failing = addDoc(bridge, 'doc-1', 'error')

    const ensure = bridge.ensureBridge()
    const survivor = addDoc(bridge, 'doc-2', 'ok')

    expect(await ensure).toBe(true)
    expect(failing.requests).toBe(1)
    expect(survivor.requests).toBe(1)
    expect(installedClient()).toBeInstanceOf(OpfsBridgeClient)

    bridge.close()
  })

  it('retries a same-id document that reconnects during an in-flight attempt', async () => {
    installControllableLocks()
    const bridge = new RuntimeOpfsBridge('worker-1')
    // The first connection never replies, so the attempt is still in flight when
    // the same document reconnects with a fresh broker port and a working worker.
    // The tried set keys on the tracked object, so the reconnect is retried even
    // though an earlier attempt for the same id already failed this loop.
    addDoc(bridge, 'doc-1', 'silent')
    const reconnect = addDoc(bridge, 'doc-1', 'ok')

    expect(await bridge.ensureBridge()).toBe(true)
    expect(reconnect.requests).toBe(1)
    expect(installedClient()).toBeInstanceOf(OpfsBridgeClient)

    bridge.close()
  })

  it('closes stale same-document OPFS acks with the wrong requestId', async () => {
    installControllableLocks()
    const bridge = new RuntimeOpfsBridge('worker-1')
    const doc = addDoc(bridge, 'doc-1', 'silent')

    const ensure = bridge.ensureBridge()
    await vi.waitFor(() => {
      expect(doc.requests).toBe(1)
    })

    let settled = false
    ensure.then(() => {
      settled = true
    })
    const stalePort = {
      close: vi.fn(),
    } as unknown as MessagePort
    const handleMessage = Reflect.get(bridge, 'handleWebDocumentMessage') as (
      this: RuntimeOpfsBridge,
      webDocumentId: string,
      ev: MessageEvent<WebDocumentToClient>,
    ) => void
    handleMessage.call(bridge, 'doc-1', {
      data: {
        from: 'doc-1',
        openOpfsWorkerAck: {
          from: 'doc-1',
          requestId: 'wrong-request-id',
        },
      },
      ports: [stalePort],
    } as unknown as MessageEvent<WebDocumentToClient>)
    expect(stalePort.close).toHaveBeenCalledOnce()
    await Promise.resolve()
    expect(settled).toBe(false)

    const current = new MessageChannel()
    doc.sendOpenAck(doc.requestIds[0], current.port1)
    await expect(ensure).resolves.toBe(true)

    bridge.close()
    current.port2.close()
  })

  it('stops serving after close', async () => {
    installControllableLocks()
    const bridge = new RuntimeOpfsBridge('worker-1')
    addDoc(bridge, 'doc-1')

    await bridge.ensureBridge()
    bridge.close()

    expect(await bridge.ensureBridge()).toBe(false)
  })
})
