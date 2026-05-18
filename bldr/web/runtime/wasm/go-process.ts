import { Retry, RetryOpts } from '../../bldr/retry.js'
import { fetchWithDecompress } from './fetch-decompress.js'
import { getBridgePort, installWebRTCShim } from './webrtc-bridge.js'

// GoWasmProcessOpts are optional parameters for GoWasmProcess.
export interface GoWasmProcessOpts {
  // abortSignal stops the runtime when aborted.
  abortSignal?: AbortSignal
  // retry controls whether the process should retry inside this worker.
  // Defaults to true.
  retry?: boolean
  // retryOpts are retry options excluding the abort signal
  // set errorCb to catch unexpected errors running the module.
  retryOpts?: Omit<RetryOpts, 'abortSignal'>
  // env are additional environment variables to pass
  env?: Record<string, string>
  // argv contains the args to pass
  argv?: string[]
}

// WasmSource allows specifying a URL, Module, or Promise for a Module to load.
export type WasmSource =
  | string
  | WebAssembly.Module
  | (() => Promise<WebAssembly.Module>)

const tinyGoPromiseErrorUnknown = 0
const tinyGoPromiseErrorNotFound = 1
const tinyGoPromiseErrorNoModificationAllowed = 2

type NavigatorWithLocks = Navigator & { locks?: LockManager }

let tinyGoWasmMemory: WebAssembly.Memory | undefined
let tinyGoStoredValueID = 1
const tinyGoStoredBytes = new Map<number, Uint8Array>()
const tinyGoCallbackQueue: (() => void)[] = []
let tinyGoCallbackScheduled = false
let tinyGoCallbackChannel: MessageChannel | undefined

function tinyGoPromiseErrorCode(reason: unknown): number {
  let name = ''
  if (reason && typeof reason === 'object') {
    const rawName = (reason as { name?: unknown }).name
    if (typeof rawName === 'string') {
      name = rawName
    }
    if (!name) {
      const ctorName = (reason as { constructor?: { name?: unknown } })
        .constructor?.name
      if (typeof ctorName === 'string') {
        name = ctorName
      }
    }
  }
  if (!name) {
    name = String(reason)
  }
  if (name.includes('NotFoundError')) {
    return tinyGoPromiseErrorNotFound
  }
  if (name.includes('NoModificationAllowedError')) {
    return tinyGoPromiseErrorNoModificationAllowed
  }
  return tinyGoPromiseErrorUnknown
}

function tinyGoMemoryView(ptr: number, len: number): Uint8Array {
  if (!tinyGoWasmMemory) {
    throw new Error('TinyGo runtime memory is not initialized')
  }
  return new Uint8Array(tinyGoWasmMemory.buffer, ptr, len)
}

function storeTinyGoBytes(bytes: Uint8Array): number {
  const id = tinyGoStoredValueID++
  tinyGoStoredBytes.set(id, bytes)
  return id
}

function encodeTinyGoNameList(names: string[]): Uint8Array {
  const encoder = new TextEncoder()
  const encoded = names.map((name) => encoder.encode(name))
  let size = 4
  for (const name of encoded) {
    size += 4 + name.byteLength
  }

  const bytes = new Uint8Array(size)
  let off = writeUint32(bytes, 0, encoded.length)
  for (const name of encoded) {
    off = writeUint32(bytes, off, name.byteLength)
    bytes.set(name, off)
    off += name.byteLength
  }
  return bytes
}

function writeUint32(bytes: Uint8Array, off: number, value: number): number {
  bytes[off] = (value >>> 24) & 0xff
  bytes[off + 1] = (value >>> 16) & 0xff
  bytes[off + 2] = (value >>> 8) & 0xff
  bytes[off + 3] = value & 0xff
  return off + 4
}

function isReleasedGoCallbackConsoleMessage(value: unknown): boolean {
  if (typeof value === 'string') {
    return value === 'call to released function'
  }
  return value instanceof Error && value.message === 'call to released function'
}

function runTinyGoCallback(callback: () => void): void {
  const consoleError = console.error
  console.error = (...args: unknown[]) => {
    if (args.length === 1 && isReleasedGoCallbackConsoleMessage(args[0])) {
      return
    }
    consoleError(...args)
  }
  try {
    callback()
  } catch (err) {
    if (!isReleasedGoCallbackConsoleMessage(err)) {
      throw err
    }
  } finally {
    console.error = consoleError
  }
}

function flushTinyGoCallbacks(): void {
  tinyGoCallbackScheduled = false
  const callbacks = tinyGoCallbackQueue.splice(0)
  for (const callback of callbacks) {
    // Go/TinyGo can release callback functions before a queued JS callback
    // runs. Filter that known runtime edge only while invoking the callback
    // owner; do not patch worker console state globally.
    runTinyGoCallback(callback)
  }
  if (tinyGoCallbackQueue.length !== 0) {
    scheduleTinyGoCallbackFlush()
  }
}

function scheduleTinyGoCallbackFlush(): void {
  if (tinyGoCallbackScheduled) {
    return
  }
  tinyGoCallbackScheduled = true
  if (typeof MessageChannel === 'function') {
    tinyGoCallbackChannel ??= new MessageChannel()
    tinyGoCallbackChannel.port1.onmessage = flushTinyGoCallbacks
    tinyGoCallbackChannel.port2.postMessage(undefined)
    return
  }
  setTimeout(flushTinyGoCallbacks, 0)
}

function deferTinyGoCallback(callback: () => void): void {
  tinyGoCallbackQueue.push(callback)
  scheduleTinyGoCallbackFlush()
}

// patchWorkerBrowserGlobals makes browser-only global lookups available inside
// worker-hosted Go WASM modules. Some JS/WASM libraries still reach through
// window even when the equivalent constructor already exists on globalThis.
function patchWorkerBrowserGlobals() {
  installTinyGoJSHelpers()
  installOPFSBroadcastHelpers()
  if (typeof globalThis.window === 'undefined') {
    Object.defineProperty(globalThis, 'window', {
      value: globalThis,
      configurable: true,
      writable: true,
    })
  }
  // Install the WebRTC bridge shim if a bridge port is available.
  // This makes RTCPeerConnection available to Go WASM (pion-webrtc)
  // by proxying signaling to the main thread and transferring DCs back.
  if (getBridgePort()) {
    installWebRTCShim()
    console.log('GoWasmProcess: WebRTC bridge shim installed')
  }
}

export function installTinyGoJSHelpers(): void {
  const g = globalThis as typeof globalThis & {
    BLDR_TINYGO_JS_CALL?: (
      target: Record<PropertyKey, unknown>,
      method: PropertyKey,
      ...args: unknown[]
    ) => unknown
    BLDR_TINYGO_JS_NEW?: <TArgs extends unknown[]>(
      ctor: new (...args: TArgs) => unknown,
      ...args: TArgs
    ) => unknown
    BLDR_TINYGO_PROMISE_AWAIT?: <TValue>(
      promise: Promise<TValue>,
      resolve: (value: TValue) => void,
      reject: (reason: number) => void,
    ) => void
    BLDR_TINYGO_COPY_STORED_BYTES?: (
      id: number,
      ptr: number,
      len: number,
    ) => number
    BLDR_OPFS_ACQUIRE_WEB_LOCK?: (
      name: string,
      mode: LockMode,
      ifAvailable: boolean,
      resolve: (release: () => void, acquired: boolean) => void,
      reject: (code: number) => void,
    ) => void
    BLDR_TINYGO_PUSH_BYTES?: (
      sink: { push: (message: Uint8Array) => void },
      ptr: number,
      len: number,
    ) => void
    BLDR_TINYGO_POST_BYTES?: (
      port: { postMessage: (message: Uint8Array) => void },
      ptr: number,
      len: number,
    ) => void
    BLDR_OPFS_READ_FILE?: (
      dir: FileSystemDirectoryHandle,
      name: string,
      opID: number,
      resolve: (opID: number, id: number, len: number) => void,
      reject: (opID: number, code: number) => void,
    ) => void
    BLDR_OPFS_READ_AT?: (
      handle: FileSystemFileHandle,
      ptr: number,
      len: number,
      off: number,
      opID: number,
      resolve: (opID: number, read: number) => void,
      reject: (opID: number, code: number) => void,
    ) => void
    BLDR_OPFS_LIST_DIRECTORY?: (
      dir: FileSystemDirectoryHandle,
      opID: number,
      resolve: (opID: number, id: number, len: number) => void,
      reject: (opID: number, code: number) => void,
    ) => void
    BLDR_OPFS_WRITE_AT?: (
      handle: FileSystemFileHandle,
      ptr: number,
      len: number,
      off: number,
      keepExisting: boolean,
      opID: number,
      resolve: (opID: number, written: number) => void,
      reject: (opID: number, code: number) => void,
    ) => void
    BLDR_OPFS_WRITE_FILE?: (
      dir: FileSystemDirectoryHandle,
      name: string,
      ptr: number,
      len: number,
      opID: number,
      resolve: (opID: number, written: number) => void,
      reject: (opID: number, code: number) => void,
    ) => void
  }

  g.BLDR_TINYGO_JS_CALL ??= (
    target: Record<PropertyKey, unknown>,
    method: PropertyKey,
    ...args: unknown[]
  ) => {
    const fn = target[method]
    if (typeof fn !== 'function') {
      throw new TypeError(`method ${String(method)} is not callable`)
    }
    return fn.apply(target, args)
  }
  g.BLDR_TINYGO_JS_NEW ??= <TArgs extends unknown[]>(
    ctor: new (...args: TArgs) => unknown,
    ...args: TArgs
  ) => new ctor(...args)
  g.BLDR_TINYGO_PROMISE_AWAIT ??= <TValue>(
    promise: Promise<TValue>,
    resolve: (value: TValue) => void,
    reject: (reason: number) => void,
  ) => {
    promise
      .then((value) => {
        deferTinyGoCallback(() => resolve(value))
      })
      .catch((reason) => {
        const code = tinyGoPromiseErrorCode(reason)
        deferTinyGoCallback(() => reject(code))
      })
  }
  g.BLDR_TINYGO_COPY_STORED_BYTES ??= (
    id: number,
    ptr: number,
    len: number,
  ) => {
    const bytes = tinyGoStoredBytes.get(id)
    tinyGoStoredBytes.delete(id)
    if (!bytes) {
      return 0
    }
    const n = Math.min(len, bytes.byteLength)
    tinyGoMemoryView(ptr, n).set(bytes.subarray(0, n))
    return n
  }
  g.BLDR_OPFS_ACQUIRE_WEB_LOCK ??= (
    name: string,
    mode: LockMode,
    ifAvailable: boolean,
    resolve: (release: () => void, acquired: boolean) => void,
    reject: (code: number) => void,
  ) => {
    const locks = (globalThis.navigator as NavigatorWithLocks | undefined)
      ?.locks
    if (!locks) {
      // TinyGo waits for exactly one helper callback. Missing Web Locks must
      // reject asynchronously instead of throwing before Go can unblock.
      deferTinyGoCallback(() => reject(tinyGoPromiseErrorUnknown))
      return
    }
    const lockOptions: LockOptions = { mode }
    if (ifAvailable) {
      lockOptions.ifAvailable = true
    }
    locks
      .request(name, lockOptions, (lock) => {
        if (ifAvailable && !lock) {
          deferTinyGoCallback(() => resolve(() => {}, false))
          return undefined
        }
        return new Promise<void>((releaseLock) => {
          deferTinyGoCallback(() => resolve(releaseLock, true))
        })
      })
      .catch((reason) => {
        const code = tinyGoPromiseErrorCode(reason)
        deferTinyGoCallback(() => reject(code))
      })
  }
  g.BLDR_TINYGO_PUSH_BYTES ??= (
    sink: { push: (message: Uint8Array) => void },
    ptr: number,
    len: number,
  ) => {
    const msg = new Uint8Array(len)
    msg.set(tinyGoMemoryView(ptr, len))
    sink.push(msg)
  }
  g.BLDR_TINYGO_POST_BYTES ??= (
    port: { postMessage: (message: Uint8Array) => void },
    ptr: number,
    len: number,
  ) => {
    const msg = new Uint8Array(len)
    msg.set(tinyGoMemoryView(ptr, len))
    port.postMessage(msg)
  }
  g.BLDR_OPFS_READ_FILE ??= (
    dir: FileSystemDirectoryHandle,
    name: string,
    opID: number,
    resolve: (opID: number, id: number, len: number) => void,
    reject: (opID: number, code: number) => void,
  ) => {
    dir
      .getFileHandle(name)
      .then((handle) => handle.getFile())
      .then((file) => file.arrayBuffer())
      .then((buf) => {
        const bytes = new Uint8Array(buf)
        const id = storeTinyGoBytes(bytes)
        const len = bytes.byteLength
        deferTinyGoCallback(() => resolve(opID, id, len))
      })
      .catch((reason) => {
        const code = tinyGoPromiseErrorCode(reason)
        deferTinyGoCallback(() => reject(opID, code))
      })
  }
  g.BLDR_OPFS_READ_AT ??= (
    handle: FileSystemFileHandle,
    ptr: number,
    len: number,
    off: number,
    opID: number,
    resolve: (opID: number, read: number) => void,
    reject: (opID: number, code: number) => void,
  ) => {
    handle
      .getFile()
      .then(async (file) => {
        if (off >= file.size || len === 0) {
          deferTinyGoCallback(() => resolve(opID, 0))
          return
        }
        const end = Math.min(off + len, file.size)
        const buf = await file.slice(off, end).arrayBuffer()
        const bytes = new Uint8Array(buf)
        if (bytes.byteLength !== 0) {
          tinyGoMemoryView(ptr, bytes.byteLength).set(bytes)
        }
        const read = bytes.byteLength
        deferTinyGoCallback(() => resolve(opID, read))
      })
      .catch((reason) => {
        const code = tinyGoPromiseErrorCode(reason)
        deferTinyGoCallback(() => reject(opID, code))
      })
  }
  g.BLDR_OPFS_LIST_DIRECTORY ??= (
    dir: FileSystemDirectoryHandle,
    opID: number,
    resolve: (opID: number, id: number, len: number) => void,
    reject: (opID: number, code: number) => void,
  ) => {
    ;(async () => {
      const names: string[] = []
      for await (const [name] of dir.entries()) {
        names.push(name)
      }
      const bytes = encodeTinyGoNameList(names)
      const id = storeTinyGoBytes(bytes)
      const len = bytes.byteLength
      deferTinyGoCallback(() => resolve(opID, id, len))
    })().catch((reason) => {
      const code = tinyGoPromiseErrorCode(reason)
      deferTinyGoCallback(() => reject(opID, code))
    })
  }
  g.BLDR_OPFS_WRITE_AT ??= (
    handle: FileSystemFileHandle,
    ptr: number,
    len: number,
    off: number,
    keepExisting: boolean,
    opID: number,
    resolve: (opID: number, written: number) => void,
    reject: (opID: number, code: number) => void,
  ) => {
    const data = new Uint8Array(len)
    if (len !== 0) {
      data.set(tinyGoMemoryView(ptr, len))
    }
    let writable: FileSystemWritableFileStream | undefined
    const opts = keepExisting ? { keepExistingData: true } : undefined
    const writablePromise = opts
      ? handle.createWritable(opts)
      : handle.createWritable()
    writablePromise
      .then(async (next) => {
        writable = next
        if (off !== 0) {
          await writable.seek(off)
        }
        if (len !== 0) {
          await writable.write(data)
        }
        await writable.close()
        deferTinyGoCallback(() => resolve(opID, len))
      })
      .catch((reason) => {
        if (writable) {
          void writable.close().catch(() => {})
        }
        const code = tinyGoPromiseErrorCode(reason)
        deferTinyGoCallback(() => reject(opID, code))
      })
  }
  g.BLDR_OPFS_WRITE_FILE ??= (
    dir: FileSystemDirectoryHandle,
    name: string,
    ptr: number,
    len: number,
    opID: number,
    resolve: (opID: number, written: number) => void,
    reject: (opID: number, code: number) => void,
  ) => {
    const data = new Uint8Array(len)
    if (len !== 0) {
      data.set(tinyGoMemoryView(ptr, len))
    }
    let writable: FileSystemWritableFileStream | undefined
    dir
      .getFileHandle(name, { create: true })
      .then(async (handle) => {
        writable = await handle.createWritable()
        if (len !== 0) {
          await writable.write(data)
        }
        await writable.close()
        deferTinyGoCallback(() => resolve(opID, len))
      })
      .catch((reason) => {
        if (writable) {
          void writable.close().catch(() => {})
        }
        const code = tinyGoPromiseErrorCode(reason)
        deferTinyGoCallback(() => reject(opID, code))
      })
  }
}

export function installOPFSBroadcastHelpers(): void {
  const g = globalThis as typeof globalThis & {
    BLDR_OPFS_BROADCAST_CHANNEL_NEW?: (name: string) => BroadcastChannel
    BLDR_OPFS_BROADCAST_SEND?: (
      channel: BroadcastChannel,
      shardID: number,
      generationHi: number,
      generationLo: number,
    ) => void
    BLDR_OPFS_BROADCAST_CLOSE?: (channel: BroadcastChannel) => void
  }

  if (
    typeof g.BroadcastChannel !== 'function' ||
    typeof g.Uint8Array !== 'function'
  ) {
    return
  }

  g.BLDR_OPFS_BROADCAST_CHANNEL_NEW ??= (name: string) =>
    new BroadcastChannel(name)
  g.BLDR_OPFS_BROADCAST_SEND ??= (
    channel: BroadcastChannel,
    shardID: number,
    generationHi: number,
    generationLo: number,
  ) => {
    const msg = new Uint8Array(10)
    const sid = shardID >>> 0
    const hi = generationHi >>> 0
    const lo = generationLo >>> 0
    msg[0] = (sid >>> 8) & 0xff
    msg[1] = sid & 0xff
    msg[2] = (hi >>> 24) & 0xff
    msg[3] = (hi >>> 16) & 0xff
    msg[4] = (hi >>> 8) & 0xff
    msg[5] = hi & 0xff
    msg[6] = (lo >>> 24) & 0xff
    msg[7] = (lo >>> 16) & 0xff
    msg[8] = (lo >>> 8) & 0xff
    msg[9] = lo & 0xff
    channel.postMessage(msg)
  }
  g.BLDR_OPFS_BROADCAST_CLOSE ??= (channel: BroadcastChannel) => {
    channel.close()
  }
}

// loadWebAssemblyModule loads the WebAssembly.Module from the WasmSource.
//
// When using fetch() (if source is a string) if the filename ends in .gz the gzip decompressor is used.
export async function loadWebAssemblyModule(
  source: WasmSource,
): Promise<WebAssembly.Module> {
  switch (typeof source) {
    case 'string': {
      const response = await fetchWithDecompress(source)
      if (
        source.endsWith('.gz') &&
        response.headers.get('content-type')?.toLowerCase() !==
          'application/wasm'
      ) {
        // Set the response content type.
        response.headers.set('content-type', 'application/wasm')
      }
      return WebAssembly.compileStreaming(response)
    }
    case 'function':
      return source()
    case 'object':
      return source
    default:
      throw new Error('unexpected WasmSource type')
  }
}

// See wasm_exec.js from the Go standard library.
//
// wasm_exec.js is combined with this file via esbuild as a build step.
export interface TinyGoRuntime {
  importObject: WebAssembly.Imports
  _inst?: WebAssembly.Instance
}

declare class Go {
  importObject: WebAssembly.Imports
  env: Record<string, string>
  argv: string[]
  run(inst: WebAssembly.Instance): Promise<void>
}

// patchTinyGoRuntimeImports adds imports newer TinyGo output can request before
// TinyGo's bundled wasm_exec.js has grown matching browser shims.
export function patchTinyGoRuntimeImports(go: TinyGoRuntime) {
  const gojs = go.importObject['gojs']
  if (!gojs || typeof gojs['runtime.getRandomData'] === 'function') {
    return
  }
  gojs['runtime.getRandomData'] = (ptr: number, len: number) => {
    const memory = go._inst?.exports.memory
    if (!(memory instanceof WebAssembly.Memory)) {
      throw new Error('TinyGo runtime memory is not initialized')
    }
    crypto.getRandomValues(new Uint8Array(memory.buffer, ptr, len))
  }
}

function installTinyGoRuntimeMemory(instance: WebAssembly.Instance) {
  const memory = instance.exports.memory
  if (memory instanceof WebAssembly.Memory) {
    tinyGoWasmMemory = memory
  }
}

// GoWasmProcess contains an instance of the bldr plugin host (entrypoint) running
// within a WASI environment. It uses a File to communicate with the WebEntrypoint
// and the GoWasmProcessHost via starpc RPC calls.
//
// This class is used in the SharedWorker under web/entrypoint/browser.
//
// NOTE: this currently uses globals and is expected to be a singleton within a Worker.
// NOTE: WebAssembly does not provide a way to "kill" the process.
export class GoWasmProcess {
  // wasmSource is the source for the wasm module
  private wasmSource: WasmSource
  // opts are the optional params
  private opts?: GoWasmProcessOpts
  // retry manages retrying starting the wasi runtime.
  // undefined unless the runtime is running
  private retry?: Retry
  // abortController is the abort controller for the current instance
  private abortController?: AbortController

  constructor(wasmSource: WasmSource, opts?: GoWasmProcessOpts) {
    this.wasmSource = wasmSource
    if (opts) {
      this.opts = { ...opts }
    }
  }

  // start starts the Go runtime.
  public start(): Promise<void> {
    this.stop()

    // build the abort controller
    const abortController = new AbortController()
    this.abortController = abortController

    // handle the parent abort signal if any
    const parentSignal = this.opts?.abortSignal
    let retry: Retry | null = null
    if (parentSignal) {
      if (parentSignal.aborted) {
        // already aborted
        return Promise.reject(parentSignal.reason)
      }
      const abortListener = () => {
        abortController.abort()
        retry?.cancel()
      }
      parentSignal.addEventListener('abort', abortListener)
      abortController.signal.addEventListener('abort', () =>
        parentSignal.removeEventListener('abort', abortListener),
      )
    }

    if (this.opts?.retry === false) {
      const result = this.runGoWasmProcess(abortController.signal)
      result.catch(() => {})
      return result
    }

    // start the runtime retry loop
    retry = this.retry = new Retry<void>(
      () => this.runGoWasmProcess(abortController.signal),
      {
        ...this.opts?.retryOpts,
        abortSignal: abortController.signal,
      },
    )
    return retry.result
  }

  // runGoWasmProcess attempts to run the wasm runtime once.
  private async runGoWasmProcess(
    // TODO: Find a way to kill the module if abortSignal is aborted.
    abortSignal: AbortSignal,
  ) {
    const wasmModule = await loadWebAssemblyModule(this.wasmSource)
    patchWorkerBrowserGlobals()

    const go = new Go()
    patchTinyGoRuntimeImports(go)
    if (this.opts?.argv) {
      go.argv = this.opts.argv
    }
    if (this.opts?.env) {
      go.env = { ...this.opts.env }
    }

    const instance = await WebAssembly.instantiate(wasmModule, go.importObject)
    installTinyGoRuntimeMemory(instance)
    abortSignal.throwIfAborted()

    await go.run(instance)
  }

  // stop stops the runtime, if running.
  //
  // NOTE: it is not possible to kill the process.
  public stop() {
    if (this.abortController) {
      this.abortController.abort()
      delete this.abortController
    }
    if (this.retry) {
      this.retry.cancel()
      delete this.retry
    }
  }
}
