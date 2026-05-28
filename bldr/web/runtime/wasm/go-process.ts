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
  // tinyGoRuntimeImports installs per-process imports that need access to the
  // TinyGo runtime before WebAssembly instantiation.
  tinyGoRuntimeImports?: (go: TinyGoRuntime) => void
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

let tinyGoStoredValueID = 1
const tinyGoStoredBytes = new Map<number, Uint8Array>()
let tinyGoOPFSWriteSessionID = 1
const tinyGoOPFSWriteSessions = new Map<
  number,
  { writable: FileSystemWritableFileStream; written: number }
>()
let tinyGoWebLockReleaseID = 1
const tinyGoWebLockReleases = new Map<number, () => void>()
const tinyGoWebLockReleaseOps = new Map<number, number>()
const tinyGoWebLockRequests = new Map<
  number,
  { abort?: AbortController; canceled?: boolean; releaseID?: number }
>()
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

function resolveTinyGoOPFSHelper(
  go: TinyGoRuntime,
  opID: number,
  ...values: number[]
): void {
  const resolve = tinyGoExport(go, 'BLDR_OPFS_HELPER_RESOLVE')
  if (!resolve) {
    throw new Error('TinyGo OPFS resolve export is not initialized')
  }
  deferTinyGoCallback(() =>
    callTinyGoExport(
      go,
      resolve,
      opID,
      values.length,
      values[0] ?? 0,
      values[1] ?? 0,
    ),
  )
}

function rejectTinyGoOPFSHelper(
  go: TinyGoRuntime,
  opID: number,
  code: number,
): void {
  const reject = tinyGoExport(go, 'BLDR_OPFS_HELPER_REJECT')
  if (!reject) {
    throw new Error('TinyGo OPFS reject export is not initialized')
  }
  deferTinyGoCallback(() => callTinyGoExport(go, reject, opID, code))
}

function rejectTinyGoOPFSOp(
  go: TinyGoRuntime,
  opID: number,
  reason: unknown,
): void {
  const code = tinyGoPromiseErrorCode(reason)
  rejectTinyGoOPFSHelper(go, opID, code)
}

function closeOPFSWritableQuietly(
  writable: FileSystemWritableFileStream | undefined,
): void {
  if (!writable) {
    return
  }
  try {
    void writable.close().catch(() => {})
  } catch {
    // Best-effort cleanup only; the caller reports the primary OPFS failure.
  }
}

function copyUint8Array(bytes: Uint8Array): Uint8Array<ArrayBuffer> {
  if (!(bytes instanceof Uint8Array)) {
    throw new TypeError('expected Uint8Array')
  }
  const copy = new Uint8Array(bytes.byteLength)
  copy.set(bytes)
  return copy
}

function storeTinyGoBytes(bytes: Uint8Array): number {
  const id = tinyGoStoredValueID++
  tinyGoStoredBytes.set(id, bytes)
  return id
}

function takeTinyGoBytes(id: number): Uint8Array | undefined {
  const bytes = tinyGoStoredBytes.get(id)
  tinyGoStoredBytes.delete(id)
  return bytes
}

function storeOPFSWriteSession(writable: FileSystemWritableFileStream): number {
  const id = tinyGoOPFSWriteSessionID++
  tinyGoOPFSWriteSessions.set(id, { writable, written: 0 })
  return id
}

function takeOPFSWriteSession(
  id: number,
): { writable: FileSystemWritableFileStream; written: number } | undefined {
  const session = tinyGoOPFSWriteSessions.get(id)
  tinyGoOPFSWriteSessions.delete(id)
  return session
}

function storeTinyGoWebLockRelease(release: () => void, opID?: number): number {
  const id = tinyGoWebLockReleaseID++
  tinyGoWebLockReleases.set(id, release)
  if (opID !== undefined) {
    tinyGoWebLockReleaseOps.set(id, opID)
    const request = tinyGoWebLockRequests.get(opID)
    if (request) {
      request.releaseID = id
    }
  }
  return id
}

function takeTinyGoWebLockRelease(id: number): (() => void) | undefined {
  const opID = tinyGoWebLockReleaseOps.get(id)
  tinyGoWebLockReleaseOps.delete(id)
  if (opID !== undefined) {
    tinyGoWebLockRequests.delete(opID)
  }
  const release = tinyGoWebLockReleases.get(id)
  tinyGoWebLockReleases.delete(id)
  return release
}

function cancelTinyGoWebLock(opID: number): boolean {
  const request = tinyGoWebLockRequests.get(opID)
  if (!request) {
    return false
  }
  request.canceled = true
  if (request.releaseID !== undefined) {
    const release = takeTinyGoWebLockRelease(request.releaseID)
    if (release) {
      release()
      return true
    }
  }
  request.abort?.abort()
  return true
}

export function tinyGoMemory(go: TinyGoRuntime): WebAssembly.Memory {
  const memory = go._inst?.exports.memory
  if (!(memory instanceof WebAssembly.Memory)) {
    throw new Error('TinyGo runtime memory is not initialized')
  }
  return memory
}

function tinyGoMemoryView(
  go: TinyGoRuntime,
  ptr: number,
  len: number,
): Uint8Array {
  return new Uint8Array(tinyGoMemory(go).buffer, ptr >>> 0, len)
}

function tinyGoUnboxValue(go: TinyGoRuntime, rawRef: bigint | number): unknown {
  const ref = typeof rawRef === 'bigint' ? rawRef : BigInt(rawRef)
  const nanHead = 0x7ff80000n
  if (((ref >> 32n) & nanHead) !== nanHead) {
    throw new Error('TinyGo numeric js.Value refs are unsupported here')
  }
  const id = Number(ref & 0xffffffffn)
  const value = go._values?.[id]
  if (value === undefined) {
    throw new Error(`TinyGo js.Value ref ${id} is unavailable`)
  }
  return value
}

function tinyGoBoxValue(go: TinyGoRuntime, value: unknown): bigint {
  const nanHead = 0x7ff80000n
  if (typeof value === 'number') {
    if (Number.isNaN(value)) {
      return nanHead << 32n
    }
    if (value === 0) {
      return (nanHead << 32n) | 1n
    }
    const buf = new ArrayBuffer(8)
    const view = new DataView(buf)
    view.setFloat64(0, value, true)
    return view.getBigInt64(0, true)
  }
  switch (value) {
    case undefined:
      return 0n
    case null:
      return (nanHead << 32n) | 2n
    case true:
      return (nanHead << 32n) | 3n
    case false:
      return (nanHead << 32n) | 4n
  }
  const values = go._values
  const ids = go._ids
  const refCounts = go._goRefCounts
  const idPool = go._idPool
  if (!values || !ids || !refCounts || !idPool) {
    throw new Error('TinyGo js.Value table is not initialized')
  }
  let id = ids.get(value)
  if (id === undefined) {
    id = idPool.pop()
    if (id === undefined) {
      id = BigInt(values.length)
    }
    const index = Number(id)
    values[index] = value
    refCounts[index] = 0
    ids.set(value, id)
  }
  refCounts[Number(id)]++
  let typeFlag = 1n
  switch (typeof value) {
    case 'string':
      typeFlag = 2n
      break
    case 'symbol':
      typeFlag = 3n
      break
    case 'function':
      typeFlag = 4n
      break
  }
  return id | ((nanHead | typeFlag) << 32n)
}

function resolveTinyGoOPFSRef(
  go: TinyGoRuntime,
  opID: number,
  value: unknown,
): void {
  const ref = tinyGoBoxValue(go, value)
  resolveTinyGoOPFSHelper(
    go,
    opID,
    Number((ref >> 32n) & 0xffffffffn),
    Number(ref & 0xffffffffn),
  )
}

function readTinyGoString(go: TinyGoRuntime, ptr: number, len: number): string {
  return new TextDecoder().decode(tinyGoMemoryView(go, ptr, len))
}

type TinyGoCallable = (...args: unknown[]) => unknown

function tinyGoCallableMethod(
  target: unknown,
  method: string,
): TinyGoCallable {
  if (
    target === null ||
    (typeof target !== 'object' && typeof target !== 'function')
  ) {
    throw new TypeError(`TinyGo JS target for ${method} is not an object`)
  }
  const fn = Reflect.get(target, method)
  if (typeof fn !== 'function') {
    throw new TypeError(`method ${method} is not callable`)
  }
  return (...args: unknown[]) => Reflect.apply(fn, target, args)
}

function tinyGoCallResult(
  go: TinyGoRuntime,
  targetRef: bigint,
  methodPtr: number,
  methodLen: number,
  args: unknown[],
): bigint {
  const target = tinyGoUnboxValue(go, targetRef)
  const method = readTinyGoString(go, methodPtr, methodLen)
  const fn = tinyGoCallableMethod(target, method)
  return tinyGoBoxValue(go, fn(...args))
}

function tinyGoConstructResult(
  go: TinyGoRuntime,
  ctorRef: bigint,
  args: unknown[],
): bigint {
  const ctor = tinyGoUnboxValue(go, ctorRef)
  if (typeof ctor !== 'function') {
    throw new TypeError('TinyGo JS constructor ref is not callable')
  }
  return tinyGoBoxValue(go, Reflect.construct(ctor, args))
}

function tinyGoFunctionRef(
  go: TinyGoRuntime,
  rawRef: bigint,
  label: string,
): TinyGoCallable {
  const fn = tinyGoUnboxValue(go, rawRef)
  if (typeof fn !== 'function') {
    throw new TypeError(`TinyGo JS ${label} ref is not callable`)
  }
  return (...args: unknown[]) => Reflect.apply(fn, undefined, args)
}

function setTinyGoJSImport(imports: object, name: string, fn: unknown): void {
  if (!Reflect.has(imports, name)) {
    Reflect.set(imports, name, fn)
  }
}

function installTinyGoJSImportHelpers(go: TinyGoRuntime): void {
  const gojs = go.importObject['gojs']
  if (!gojs || typeof gojs !== 'object') {
    return
  }
  setTinyGoJSImport(gojs, 'bldr.tinygo.jsCall0', (
    targetRef: bigint,
    methodPtr: number,
    methodLen: number,
  ) => tinyGoCallResult(go, targetRef, methodPtr, methodLen, []))
  setTinyGoJSImport(gojs, 'bldr.tinygo.jsCall1Value', (
    targetRef: bigint,
    methodPtr: number,
    methodLen: number,
    arg0Ref: bigint,
  ) =>
    tinyGoCallResult(go, targetRef, methodPtr, methodLen, [
      tinyGoUnboxValue(go, arg0Ref),
    ]))
  setTinyGoJSImport(gojs, 'bldr.tinygo.jsCall2StringValue', (
    targetRef: bigint,
    methodPtr: number,
    methodLen: number,
    arg0Ptr: number,
    arg0Len: number,
    arg1Ref: bigint,
  ) =>
    tinyGoCallResult(go, targetRef, methodPtr, methodLen, [
      readTinyGoString(go, arg0Ptr, arg0Len),
      tinyGoUnboxValue(go, arg1Ref),
    ]))
  setTinyGoJSImport(gojs, 'bldr.tinygo.jsNew0', (ctorRef: bigint) =>
    tinyGoConstructResult(go, ctorRef, []))
  setTinyGoJSImport(gojs, 'bldr.tinygo.jsNew1Int', (
    ctorRef: bigint,
    arg0: number,
  ) => tinyGoConstructResult(go, ctorRef, [arg0]))
  setTinyGoJSImport(gojs, 'bldr.tinygo.promiseAwait', (
    promiseRef: bigint,
    resolveRef: bigint,
    rejectRef: bigint,
  ) => {
    const promise = tinyGoUnboxValue(go, promiseRef)
    const resolve = tinyGoFunctionRef(go, resolveRef, 'promise resolve')
    const reject = tinyGoFunctionRef(go, rejectRef, 'promise reject')
    Promise.resolve(promise)
      .then((value) => {
        deferTinyGoCallback(() => {
          resolve(value)
        })
      })
      .catch((reason) => {
        const code = tinyGoPromiseErrorCode(reason)
        deferTinyGoCallback(() => {
          reject(code)
        })
      })
  })
}

export function tinyGoExport(
  go: TinyGoRuntime,
  name: string,
): ((...args: number[]) => void) | undefined {
  const fn = go._inst?.exports[name]
  return typeof fn === 'function'
    ? (fn as (...args: number[]) => void)
    : undefined
}

function tinyGoScheduler(go: TinyGoRuntime): (() => void) | undefined {
  const scheduler = go._inst?.exports.go_scheduler
  if (typeof scheduler === 'function') {
    return () => {
      scheduler()
    }
  }
  const resume = go._resume
  return typeof resume === 'function' ? () => resume.call(go) : undefined
}

export function callTinyGoExport(
  go: TinyGoRuntime,
  fn: (...args: number[]) => void,
  ...args: number[]
): void {
  fn(...args)
  const scheduler = tinyGoScheduler(go)
  if (!scheduler) {
    return
  }
  // Export callbacks only publish completion. Wake ordinary TinyGo goroutines
  // through go_scheduler from a later task; _resume is the js.FuncOf event path
  // and can corrupt syscall/js argument frames when no pending event exists.
  deferTinyGoCallback(() => {
    scheduler()
  })
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
  const callback = tinyGoCallbackQueue.shift()
  if (callback) {
    // TinyGo's asyncified runtime owns a single pending JS callback event.
    // Give each callback a fresh task boundary so resumed goroutines can
    // issue syscall/js calls without the next callback overwriting that state.
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
function patchWorkerBrowserGlobals(go: TinyGoRuntime) {
  installTinyGoJSHelpers(go)
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

export function installTinyGoJSHelpers(go: TinyGoRuntime): void {
  installTinyGoJSImportHelpers(go)

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
    BLDR_TINYGO_NEW_BYTES?: (len: number) => Uint8Array
    BLDR_TINYGO_TAKE_STORED_BYTES?: (id: number) => Uint8Array | undefined
    BLDR_OPFS_ACQUIRE_WEB_LOCK?: (
      name: string,
      mode: LockMode,
      ifAvailable: boolean,
      resolve: (release: () => void, acquired: boolean) => void,
      reject: (code: number) => void,
    ) => void
    BLDR_TINYGO_PUSH_BYTES?: (
      sink: { push: (message: Uint8Array) => void },
      bytes: Uint8Array,
    ) => boolean
    BLDR_TINYGO_POST_BYTES?: (
      port: { postMessage: (message: Uint8Array) => void },
      bytes: Uint8Array,
    ) => boolean
    BLDR_OPFS_READ_FILE?: (
      dir: FileSystemDirectoryHandle,
      name: string,
      opID: number,
    ) => void
    BLDR_OPFS_READ_AT?: (
      handle: FileSystemFileHandle,
      dst: Uint8Array,
      off: number,
      opID: number,
    ) => void
    BLDR_OPFS_LIST_DIRECTORY?: (
      dir: FileSystemDirectoryHandle,
      opID: number,
    ) => void
    BLDR_OPFS_WRITE_AT?: (
      handle: FileSystemFileHandle,
      data: Uint8Array,
      off: number,
      keepExisting: boolean,
      opID: number,
    ) => void
    BLDR_OPFS_WRITE_FILE?: (
      dir: FileSystemDirectoryHandle,
      name: string,
      data: Uint8Array,
      opID: number,
    ) => void
    BLDR_OPFS_WRITE_FILE_BEGIN?: (
      dir: FileSystemDirectoryHandle,
      name: string,
      opID: number,
    ) => void
    BLDR_OPFS_WRITE_FILE_CHUNK?: (
      sessionID: number,
      data: Uint8Array,
      opID: number,
    ) => void
    BLDR_OPFS_WRITE_FILE_CLOSE?: (sessionID: number, opID: number) => void
    BLDR_OPFS_WRITE_FILE_ABORT?: (sessionID: number) => boolean
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
  g.BLDR_TINYGO_NEW_BYTES ??= (len: number) => new Uint8Array(len)
  g.BLDR_TINYGO_TAKE_STORED_BYTES ??= (id: number) => takeTinyGoBytes(id)
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
    bytes: Uint8Array,
  ) => {
    try {
      sink.push(copyUint8Array(bytes))
      return true
    } catch {
      return false
    }
  }
  g.BLDR_TINYGO_POST_BYTES ??= (
    port: { postMessage: (message: Uint8Array) => void },
    bytes: Uint8Array,
  ) => {
    try {
      port.postMessage(copyUint8Array(bytes))
      return true
    } catch {
      return false
    }
  }
  g.BLDR_OPFS_READ_FILE ??= (
    dir: FileSystemDirectoryHandle,
    name: string,
    opID: number,
  ) => {
    dir
      .getFileHandle(name)
      .then((handle) => handle.getFile())
      .then((file) => file.arrayBuffer())
      .then((buf) => {
        const bytes = new Uint8Array(buf)
        const id = storeTinyGoBytes(bytes)
        const len = bytes.byteLength
        resolveTinyGoOPFSHelper(go, opID, id, len)
      })
      .catch((reason) => {
        rejectTinyGoOPFSOp(go, opID, reason)
      })
  }
  g.BLDR_OPFS_READ_AT ??= (
    handle: FileSystemFileHandle,
    dst: Uint8Array,
    off: number,
    opID: number,
  ) => {
    const dstLen = dst.byteLength
    handle
      .getFile()
      .then(async (file) => {
        if (off >= file.size || dstLen === 0) {
          resolveTinyGoOPFSHelper(go, opID, 0)
          return
        }
        const end = Math.min(off + dstLen, file.size)
        const buf = await file.slice(off, end).arrayBuffer()
        const bytes = new Uint8Array(buf)
        if (bytes.byteLength !== 0) {
          dst.subarray(0, bytes.byteLength).set(bytes)
        }
        const read = bytes.byteLength
        resolveTinyGoOPFSHelper(go, opID, read)
      })
      .catch((reason) => {
        rejectTinyGoOPFSOp(go, opID, reason)
      })
  }
  g.BLDR_OPFS_LIST_DIRECTORY ??= (
    dir: FileSystemDirectoryHandle,
    opID: number,
  ) => {
    ;(async () => {
      const names: string[] = []
      for await (const [name] of dir.entries()) {
        names.push(name)
      }
      const bytes = encodeTinyGoNameList(names)
      const id = storeTinyGoBytes(bytes)
      const len = bytes.byteLength
      resolveTinyGoOPFSHelper(go, opID, id, len)
    })().catch((reason) => {
      rejectTinyGoOPFSOp(go, opID, reason)
    })
  }
  g.BLDR_OPFS_WRITE_AT ??= (
    handle: FileSystemFileHandle,
    data: Uint8Array,
    off: number,
    keepExisting: boolean,
    opID: number,
  ) => {
    let writable: FileSystemWritableFileStream | undefined
    try {
      const writeData = copyUint8Array(data)
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
          if (writeData.byteLength !== 0) {
            await writable.write(writeData)
          }
          await writable.close()
          resolveTinyGoOPFSHelper(go, opID, writeData.byteLength)
        })
        .catch((reason) => {
          closeOPFSWritableQuietly(writable)
          rejectTinyGoOPFSOp(go, opID, reason)
        })
    } catch (reason) {
      closeOPFSWritableQuietly(writable)
      rejectTinyGoOPFSOp(go, opID, reason)
    }
  }
  g.BLDR_OPFS_WRITE_FILE ??= (
    dir: FileSystemDirectoryHandle,
    name: string,
    data: Uint8Array,
    opID: number,
  ) => {
    let writable: FileSystemWritableFileStream | undefined
    try {
      const writeData = copyUint8Array(data)
      dir
        .getFileHandle(name, { create: true })
        .then(async (handle) => {
          writable = await handle.createWritable()
          if (writeData.byteLength !== 0) {
            await writable.write(writeData)
          }
          await writable.close()
          resolveTinyGoOPFSHelper(go, opID, writeData.byteLength)
        })
        .catch((reason) => {
          closeOPFSWritableQuietly(writable)
          rejectTinyGoOPFSOp(go, opID, reason)
        })
    } catch (reason) {
      closeOPFSWritableQuietly(writable)
      rejectTinyGoOPFSOp(go, opID, reason)
    }
  }
  g.BLDR_OPFS_WRITE_FILE_BEGIN ??= (
    dir: FileSystemDirectoryHandle,
    name: string,
    opID: number,
  ) => {
    try {
      dir
        .getFileHandle(name, { create: true })
        .then((handle) => handle.createWritable())
        .then((writable) => {
          const sessionID = storeOPFSWriteSession(writable)
          resolveTinyGoOPFSHelper(go, opID, sessionID)
        })
        .catch((reason) => {
          rejectTinyGoOPFSOp(go, opID, reason)
        })
    } catch (reason) {
      rejectTinyGoOPFSOp(go, opID, reason)
    }
  }
  g.BLDR_OPFS_WRITE_FILE_CHUNK ??= (
    sessionID: number,
    data: Uint8Array,
    opID: number,
  ) => {
    const session = tinyGoOPFSWriteSessions.get(sessionID)
    if (!session) {
      rejectTinyGoOPFSHelper(go, opID, tinyGoPromiseErrorNotFound)
      return
    }
    let writeData: Uint8Array<ArrayBuffer>
    try {
      writeData = copyUint8Array(data)
    } catch {
      rejectTinyGoOPFSHelper(go, opID, tinyGoPromiseErrorUnknown)
      return
    }
    try {
      session.writable
        .write(writeData)
        .then(() => {
          session.written += writeData.byteLength
          resolveTinyGoOPFSHelper(go, opID, writeData.byteLength)
        })
        .catch((reason) => {
          tinyGoOPFSWriteSessions.delete(sessionID)
          closeOPFSWritableQuietly(session.writable)
          rejectTinyGoOPFSOp(go, opID, reason)
        })
    } catch (reason) {
      tinyGoOPFSWriteSessions.delete(sessionID)
      closeOPFSWritableQuietly(session.writable)
      rejectTinyGoOPFSOp(go, opID, reason)
    }
  }
  g.BLDR_OPFS_WRITE_FILE_CLOSE ??= (sessionID: number, opID: number) => {
    const session = takeOPFSWriteSession(sessionID)
    if (!session) {
      rejectTinyGoOPFSHelper(go, opID, tinyGoPromiseErrorNotFound)
      return
    }
    try {
      session.writable
        .close()
        .then(() => {
          resolveTinyGoOPFSHelper(go, opID, session.written)
        })
        .catch((reason) => {
          rejectTinyGoOPFSOp(go, opID, reason)
        })
    } catch (reason) {
      rejectTinyGoOPFSOp(go, opID, reason)
    }
  }
  g.BLDR_OPFS_WRITE_FILE_ABORT ??= (sessionID: number) => {
    const session = takeOPFSWriteSession(sessionID)
    if (!session) {
      return false
    }
    closeOPFSWritableQuietly(session.writable)
    return true
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
  _resume?: () => void
  _values?: unknown[]
  _goRefCounts?: number[]
  _ids?: Map<unknown, bigint>
  _idPool?: bigint[]
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
  if (!gojs) {
    return
  }
  gojs['runtime.getRandomData'] ??= (ptr: number, len: number) => {
    crypto.getRandomValues(
      new Uint8Array(tinyGoMemory(go).buffer, ptr >>> 0, len),
    )
  }
  gojs['bldr.opfs.acquireWebLock'] ??= (
    opID: number,
    namePtr: number,
    nameLen: number,
    exclusive: number,
    ifAvailable: number,
  ) => {
    const resolve = tinyGoExport(go, 'BLDR_OPFS_WEB_LOCK_RESOLVE')
    const reject = tinyGoExport(go, 'BLDR_OPFS_WEB_LOCK_REJECT')
    if (!resolve || !reject) {
      throw new Error('TinyGo WebLock callback exports are not initialized')
    }
    const locks = (globalThis.navigator as NavigatorWithLocks | undefined)
      ?.locks
    if (!locks) {
      deferTinyGoCallback(() =>
        callTinyGoExport(go, reject, opID, tinyGoPromiseErrorUnknown),
      )
      return
    }
    const lockOptions: LockOptions = {
      mode: exclusive ? 'exclusive' : 'shared',
    }
    const abort = ifAvailable ? undefined : new AbortController()
    if (abort) {
      lockOptions.signal = abort.signal
    }
    if (ifAvailable) {
      lockOptions.ifAvailable = true
    }
    const name = readTinyGoString(go, namePtr, nameLen)
    const request: {
      abort?: AbortController
      canceled?: boolean
      releaseID?: number
    } = { abort }
    tinyGoWebLockRequests.set(opID, request)
    locks
      .request(name, lockOptions, (lock) => {
        if (ifAvailable && !lock) {
          tinyGoWebLockRequests.delete(opID)
          deferTinyGoCallback(() => callTinyGoExport(go, resolve, opID, 0, 0))
          return undefined
        }
        return new Promise<void>((releaseLock) => {
          if (request.canceled) {
            releaseLock()
            tinyGoWebLockRequests.delete(opID)
            return
          }
          const releaseID = storeTinyGoWebLockRelease(releaseLock, opID)
          deferTinyGoCallback(() =>
            callTinyGoExport(go, resolve, opID, releaseID, 1),
          )
        })
      })
      .catch((reason) => {
        tinyGoWebLockRequests.delete(opID)
        if (request.canceled) {
          return
        }
        deferTinyGoCallback(() =>
          callTinyGoExport(go, reject, opID, tinyGoPromiseErrorCode(reason)),
        )
      })
  }
  gojs['bldr.opfs.cancelWebLock'] ??= (opID: number) =>
    cancelTinyGoWebLock(opID) ? 1 : 0
  gojs['bldr.opfs.releaseWebLock'] ??= (releaseID: number) => {
    const release = takeTinyGoWebLockRelease(releaseID)
    if (!release) {
      return 0
    }
    release()
    return 1
  }
  gojs['bldr.opfs.getRootRef'] ??= (opID: number) => {
    globalThis.navigator.storage
      .getDirectory()
      .then((dir) => resolveTinyGoOPFSRef(go, opID, dir))
      .catch((reason) => rejectTinyGoOPFSOp(go, opID, reason))
  }
  gojs['bldr.opfs.getDirectoryRef'] ??= (
    opID: number,
    parentRef: bigint,
    namePtr: number,
    nameLen: number,
    create: number,
  ) => {
    const parent = tinyGoUnboxValue(go, parentRef) as FileSystemDirectoryHandle
    const name = readTinyGoString(go, namePtr, nameLen)
    parent
      .getDirectoryHandle(name, { create: Boolean(create) })
      .then((dir) => resolveTinyGoOPFSRef(go, opID, dir))
      .catch((reason) => rejectTinyGoOPFSOp(go, opID, reason))
  }
  gojs['bldr.opfs.openFileRef'] ??= (
    opID: number,
    dirRef: bigint,
    namePtr: number,
    nameLen: number,
    create: number,
  ) => {
    const dir = tinyGoUnboxValue(go, dirRef) as FileSystemDirectoryHandle
    const name = readTinyGoString(go, namePtr, nameLen)
    const opts = create ? { create: true } : undefined
    const filePromise = opts
      ? dir.getFileHandle(name, opts)
      : dir.getFileHandle(name)
    filePromise
      .then((handle) => resolveTinyGoOPFSRef(go, opID, handle))
      .catch((reason) => rejectTinyGoOPFSOp(go, opID, reason))
  }
  gojs['bldr.opfs.fileExistsRef'] ??= (
    opID: number,
    dirRef: bigint,
    namePtr: number,
    nameLen: number,
  ) => {
    const dir = tinyGoUnboxValue(go, dirRef) as FileSystemDirectoryHandle
    const name = readTinyGoString(go, namePtr, nameLen)
    dir
      .getFileHandle(name)
      .then(() => resolveTinyGoOPFSHelper(go, opID, 1))
      .catch((reason) => {
        const code = tinyGoPromiseErrorCode(reason)
        if (code === tinyGoPromiseErrorNotFound) {
          resolveTinyGoOPFSHelper(go, opID, 0)
          return
        }
        rejectTinyGoOPFSHelper(go, opID, code)
      })
  }
  gojs['bldr.opfs.deleteEntryRef'] ??= (
    opID: number,
    dirRef: bigint,
    namePtr: number,
    nameLen: number,
    recursive: number,
  ) => {
    const dir = tinyGoUnboxValue(go, dirRef) as FileSystemDirectoryHandle
    const name = readTinyGoString(go, namePtr, nameLen)
    dir
      .removeEntry(name, { recursive: Boolean(recursive) })
      .then(() => resolveTinyGoOPFSHelper(go, opID, 1))
      .catch((reason) => rejectTinyGoOPFSOp(go, opID, reason))
  }
  gojs['bldr.opfs.yieldMicrotask'] ??= (opID: number) => {
    queueMicrotask(() => resolveTinyGoOPFSHelper(go, opID, 1))
  }
  gojs['bldr.opfs.sizeRef'] ??= (opID: number, handleRef: bigint) => {
    const handle = tinyGoUnboxValue(go, handleRef) as FileSystemFileHandle
    handle
      .getFile()
      .then((file) => resolveTinyGoOPFSHelper(go, opID, file.size))
      .catch((reason) => rejectTinyGoOPFSOp(go, opID, reason))
  }
  gojs['bldr.opfs.truncateRef'] ??= (
    opID: number,
    handleRef: bigint,
    size: bigint,
  ) => {
    const handle = tinyGoUnboxValue(go, handleRef) as FileSystemFileHandle
    let writable: FileSystemWritableFileStream | undefined
    handle
      .createWritable({ keepExistingData: true })
      .then(async (next) => {
        writable = next
        await writable.truncate(Number(size))
        await writable.close()
        resolveTinyGoOPFSHelper(go, opID, 1)
      })
      .catch((reason) => {
        closeOPFSWritableQuietly(writable)
        rejectTinyGoOPFSOp(go, opID, reason)
      })
  }
  gojs['bldr.opfs.takeStoredBytes'] ??= (
    bytesID: number,
    ptr: number,
    len: number,
  ) => {
    const bytes = takeTinyGoBytes(bytesID)
    if (!bytes || bytes.byteLength !== len) {
      return 0
    }
    if (len !== 0) {
      tinyGoMemoryView(go, ptr, len).set(bytes)
    }
    return 1
  }
  gojs['bldr.opfs.readFileRef'] ??= (
    opID: number,
    dirRef: bigint,
    namePtr: number,
    nameLen: number,
  ) => {
    const dir = tinyGoUnboxValue(go, dirRef) as FileSystemDirectoryHandle
    const name = readTinyGoString(go, namePtr, nameLen)
    dir
      .getFileHandle(name)
      .then((handle) => handle.getFile())
      .then((file) => file.arrayBuffer())
      .then((buf) => {
        const bytes = new Uint8Array(buf)
        resolveTinyGoOPFSHelper(
          go,
          opID,
          storeTinyGoBytes(bytes),
          bytes.byteLength,
        )
      })
      .catch((reason) => rejectTinyGoOPFSOp(go, opID, reason))
  }
  gojs['bldr.opfs.readAtRef'] ??= (
    opID: number,
    handleRef: bigint,
    dstPtr: number,
    dstLen: number,
    off: bigint,
  ) => {
    const handle = tinyGoUnboxValue(go, handleRef) as FileSystemFileHandle
    const offset = Number(off)
    handle
      .getFile()
      .then(async (file) => {
        if (offset >= file.size || dstLen === 0) {
          resolveTinyGoOPFSHelper(go, opID, 0)
          return
        }
        const end = Math.min(offset + dstLen, file.size)
        const buf = await file.slice(offset, end).arrayBuffer()
        const bytes = new Uint8Array(buf)
        if (bytes.byteLength !== 0) {
          tinyGoMemoryView(go, dstPtr, bytes.byteLength).set(bytes)
        }
        resolveTinyGoOPFSHelper(go, opID, bytes.byteLength)
      })
      .catch((reason) => rejectTinyGoOPFSOp(go, opID, reason))
  }
  gojs['bldr.opfs.listDirectoryRef'] ??= (opID: number, dirRef: bigint) => {
    const dir = tinyGoUnboxValue(go, dirRef) as FileSystemDirectoryHandle
    ;(async () => {
      const names: string[] = []
      for await (const [name] of dir.entries()) {
        names.push(name)
      }
      const bytes = encodeTinyGoNameList(names)
      resolveTinyGoOPFSHelper(
        go,
        opID,
        storeTinyGoBytes(bytes),
        bytes.byteLength,
      )
    })().catch((reason) => rejectTinyGoOPFSOp(go, opID, reason))
  }
  gojs['bldr.opfs.writeAtRef'] ??= (
    opID: number,
    handleRef: bigint,
    dataPtr: number,
    dataLen: number,
    off: bigint,
    keepExisting: number,
  ) => {
    const handle = tinyGoUnboxValue(go, handleRef) as FileSystemFileHandle
    let writable: FileSystemWritableFileStream | undefined
    try {
      const writeData = copyUint8Array(tinyGoMemoryView(go, dataPtr, dataLen))
      const opts = keepExisting ? { keepExistingData: true } : undefined
      const writablePromise = opts
        ? handle.createWritable(opts)
        : handle.createWritable()
      writablePromise
        .then(async (next) => {
          writable = next
          const offset = Number(off)
          if (offset !== 0) {
            await writable.seek(offset)
          }
          if (writeData.byteLength !== 0) {
            await writable.write(writeData)
          }
          await writable.close()
          resolveTinyGoOPFSHelper(go, opID, writeData.byteLength)
        })
        .catch((reason) => {
          closeOPFSWritableQuietly(writable)
          rejectTinyGoOPFSOp(go, opID, reason)
        })
    } catch (reason) {
      closeOPFSWritableQuietly(writable)
      rejectTinyGoOPFSOp(go, opID, reason)
    }
  }
  gojs['bldr.opfs.writeFileBeginRef'] ??= (
    opID: number,
    dirRef: bigint,
    namePtr: number,
    nameLen: number,
  ) => {
    const dir = tinyGoUnboxValue(go, dirRef) as FileSystemDirectoryHandle
    const name = readTinyGoString(go, namePtr, nameLen)
    try {
      dir
        .getFileHandle(name, { create: true })
        .then((handle) => handle.createWritable())
        .then((writable) => {
          resolveTinyGoOPFSHelper(go, opID, storeOPFSWriteSession(writable))
        })
        .catch((reason) => rejectTinyGoOPFSOp(go, opID, reason))
    } catch (reason) {
      rejectTinyGoOPFSOp(go, opID, reason)
    }
  }
  gojs['bldr.opfs.writeFileChunkRef'] ??= (
    opID: number,
    sessionID: number,
    dataPtr: number,
    dataLen: number,
  ) => {
    const session = tinyGoOPFSWriteSessions.get(sessionID)
    if (!session) {
      rejectTinyGoOPFSHelper(go, opID, tinyGoPromiseErrorNotFound)
      return
    }
    let writeData: Uint8Array<ArrayBuffer>
    try {
      writeData = copyUint8Array(tinyGoMemoryView(go, dataPtr, dataLen))
    } catch {
      rejectTinyGoOPFSHelper(go, opID, tinyGoPromiseErrorUnknown)
      return
    }
    try {
      session.writable
        .write(writeData)
        .then(() => {
          session.written += writeData.byteLength
          resolveTinyGoOPFSHelper(go, opID, writeData.byteLength)
        })
        .catch((reason) => {
          tinyGoOPFSWriteSessions.delete(sessionID)
          closeOPFSWritableQuietly(session.writable)
          rejectTinyGoOPFSOp(go, opID, reason)
        })
    } catch (reason) {
      tinyGoOPFSWriteSessions.delete(sessionID)
      closeOPFSWritableQuietly(session.writable)
      rejectTinyGoOPFSOp(go, opID, reason)
    }
  }
  gojs['bldr.opfs.writeFileCloseRef'] ??= (opID: number, sessionID: number) => {
    const session = takeOPFSWriteSession(sessionID)
    if (!session) {
      rejectTinyGoOPFSHelper(go, opID, tinyGoPromiseErrorNotFound)
      return
    }
    try {
      session.writable
        .close()
        .then(() => resolveTinyGoOPFSHelper(go, opID, session.written))
        .catch((reason) => rejectTinyGoOPFSOp(go, opID, reason))
    } catch (reason) {
      rejectTinyGoOPFSOp(go, opID, reason)
    }
  }
  gojs['bldr.opfs.writeFileAbortRef'] ??= (sessionID: number) => {
    const session = takeOPFSWriteSession(sessionID)
    if (!session) {
      return 0
    }
    closeOPFSWritableQuietly(session.writable)
    return 1
  }
  gojs['bldr.opfs.broadcastChannelNewRef'] ??= (
    namePtr: number,
    nameLen: number,
  ) => {
    const name = readTinyGoString(go, namePtr, nameLen)
    return tinyGoBoxValue(go, new BroadcastChannel(name))
  }
  gojs['bldr.opfs.broadcastSendRef'] ??= (
    channelRef: bigint,
    shardID: number,
    generationHi: number,
    generationLo: number,
  ) => {
    const channel = tinyGoUnboxValue(go, channelRef) as BroadcastChannel
    const msg = new Uint8Array(10)
    const sid = shardID & 0xffff
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
  gojs['bldr.opfs.broadcastCloseRef'] ??= (channelRef: bigint) => {
    const channel = tinyGoUnboxValue(go, channelRef) as BroadcastChannel
    channel.close()
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
    const go = new Go()
    const wasmModule = await loadWebAssemblyModule(this.wasmSource)
    patchWorkerBrowserGlobals(go)
    patchTinyGoRuntimeImports(go)
    this.opts?.tinyGoRuntimeImports?.(go)
    if (this.opts?.argv) {
      go.argv = this.opts.argv
    }
    if (this.opts?.env) {
      go.env = { ...this.opts.env }
    }

    const instance = await WebAssembly.instantiate(wasmModule, go.importObject)
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
