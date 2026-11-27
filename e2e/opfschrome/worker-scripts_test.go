package chrometest

// opfsWorkerScriptEnvelope bounds page evaluation and returns watchdog state so
// worker hangs report the last browser-side marker before the Go test timeout.
const opfsWorkerScriptEnvelope = `async ({ script, args, timeoutMs }) => {
  const watchdog = window.__opfsChromeWorkerWatchdog
  if (!watchdog) {
    throw new Error('OPFS worker watchdog is not installed')
  }
  watchdog.reset()
  const run = (async () => {
    try {
      const fn = (0, eval)('(' + script + ')')
      const results = await fn(args)
      return {
        ...watchdog.snapshot(),
        status: 'ok',
        results,
      }
    } catch (reason) {
      watchdog.exception(reason)
      return {
        ...watchdog.snapshot(),
        status: 'exception',
        timeoutMs,
      }
    }
  })()
  const timeout = new Promise((resolve) => {
    setTimeout(() => {
      resolve({
        ...watchdog.snapshot(),
        status: 'timeout',
        timeoutMs,
      })
    }, timeoutMs)
  })
  return await Promise.race([run, timeout])
}`

// indexHTML hosts the browser worker lifecycle and synchronization helpers.
const indexHTML = `<!doctype html>
<html>
  <head>
    <meta charset="utf-8">
    <title>opfs chrome test</title>
  </head>
  <body>
    <script type="module">
      window.__opfsChromeWorkerWatchdog = createOpfsChromeWorkerWatchdog()
      window.runOpfsWorkers = async (workers) => {
        return await waitWorkers(workers.map((args) => runWorker(args)))
      }

      window.runOpfsWorkersStaged = async (readyWorkers, workers) => {
        const ready = readyWorkers.map((args) => runWorker(args))
        const readyResults = await Promise.all(ready.map((item) => item.ready))
        if (readyResults.some((result) => result.kind === 'result' && !result.ok)) {
          return compactResults(readyResults)
        }
        const started = workers.map((args) => runWorker(args))
        return await waitWorkers([...ready, ...started])
      }

      window.runOpfsBlockedLockWorkers = async (holderArgs, workers) => {
        const holder = runWorker(holderArgs)
        const holderReady = await holder.ready
        if (holderReady.kind === 'result') {
          return compactResults([holderReady])
        }
        const queued = workers.map((args) => runWorker(args))
        const queuedReady = await Promise.all(queued.map((item) => item.ready))
        if (queuedReady.some((result) => result.kind === 'result' && !result.ok)) {
          holder.stop()
          return compactResults(queuedReady)
        }
        const release = new BroadcastChannel('opfs-chrometest-counter-release:' + holderArgs.root)
        release.postMessage({ type: 'release' })
        release.close()
        return await waitWorkers([holder, ...queued])
      }

      window.runOpfsHeldLockCheck = async (holderArgs, checkArgs) => {
        const holder = runWorker(holderArgs)
        const holderReady = await holder.ready
        if (holderReady.kind === 'result') {
          return compactResults([holderReady])
        }
        const check = runWorker(checkArgs)
        const checkResults = await waitWorkers([check])
        if (checkResults.some((result) => !result.ok)) {
          holder.stop()
          return checkResults
        }
        const release = new BroadcastChannel('opfs-chrometest-counter-release:' + holderArgs.root)
        release.postMessage({ type: 'release' })
        release.close()
        const holderResults = await waitWorkers([holder])
        return [...checkResults, ...holderResults]
      }

      window.runOpfsTerminatedLockHolderWorkers = async (holderArgs, workers) => {
        const holder = runWorker(holderArgs)
        const holderReady = await holder.ready
        if (holderReady.kind === 'result') {
          return compactResults([holderReady])
        }
        const queued = workers.map((args) => runWorker(args))
        const queuedReady = await Promise.all(queued.map((item) => item.ready))
        if (queuedReady.some((result) => result.kind === 'result' && !result.ok)) {
          holder.stop()
          return compactResults(queuedReady)
        }
        holder.stop()
        return await waitWorkers(queued)
      }

      window.runOpfsTerminateReadyWorker = async (args) => {
        const worker = runWorker(args)
        const ready = await worker.ready
        if (ready.kind === 'result') {
          return compactResults([ready])
        }
        worker.stop()
        return [{
          kind: 'result',
          scenario: args.scenario,
          worker: args.worker ?? 0,
          ok: true,
          durationMs: 0,
        }]
      }

      function waitWorkers(items) {
        return new Promise((resolve) => {
          const results = new Array(items.length)
          let remaining = items.length
          let resolved = false
          for (const [index, item] of items.entries()) {
            item.done.then((result) => {
              if (resolved) {
                return
              }
              results[index] = result
              if (!result.ok) {
                resolved = true
                for (const other of items) {
                  other.stop()
                }
                resolve(compactResults(results))
                return
              }
              remaining--
              if (remaining === 0) {
                resolved = true
                resolve(results)
              }
            })
          }
        })
      }

      function compactResults(results) {
        return results.filter((result) => result)
      }

      function createOpfsChromeWorkerWatchdog() {
        let progress = null
        let exception = ''
        return {
          reset: () => {
            progress = null
            exception = ''
          },
          record: (data) => {
            progress = normalizeProgress(data)
          },
          exception: (reason) => {
            exception = describeBrowserError(reason)
          },
          snapshot: () => ({
            status: 'state',
            progress,
            exception,
          }),
        }
      }

      function normalizeProgress(data) {
        const progress = {
          scenario: data?.scenario ?? '',
          worker: data?.worker ?? 0,
          phase: data?.phase ?? '',
        }
        if (data?.offset !== undefined) {
          progress.offset = data.offset
        }
        if (data?.total !== undefined) {
          progress.total = data.total
        }
        return progress
      }

      function recordWorkerProgress(data, phase) {
        window.__opfsChromeWorkerWatchdog.record({
          scenario: data?.scenario ?? '',
          worker: data?.worker ?? 0,
          phase: data?.phase ?? phase,
          offset: data?.offset,
          total: data?.total,
        })
      }

      function describeBrowserError(reason) {
        if (!reason) {
          return ''
        }
        if (typeof reason === 'string') {
          return reason
        }
        if (typeof reason.stack === 'string') {
          return reason.stack
        }
        if (typeof reason.message === 'string') {
          return reason.message
        }
        return String(reason)
      }

      function runWorker(args) {
        let readyResolve
        recordWorkerProgress(args, 'start')
        const worker = new Worker('/worker.js', { type: 'classic' })
        const ready = new Promise((resolve) => {
          readyResolve = resolve
        })
        const done = new Promise((resolve) => {
          const fail = (message) => {
            worker.terminate()
            window.__opfsChromeWorkerWatchdog.exception(message)
            const data = {
              kind: 'result',
              scenario: args.scenario,
              worker: args.worker ?? 0,
              ok: false,
              error: message,
            }
            recordWorkerProgress(data, 'worker-error')
            readyResolve(data)
            resolve(data)
          }
          worker.onmessage = (event) => {
            const data = event.data
            if (data.kind === 'ready') {
              recordWorkerProgress(data, 'ready')
              readyResolve(data)
              return
            }
            if (data.kind === 'progress') {
              recordWorkerProgress(data, 'progress')
              console.log(formatProgress(data))
              return
            }
            if (data.kind === 'result') {
              recordWorkerProgress(data, data.ok ? 'result-ok' : 'result-error')
              worker.terminate()
              readyResolve(data)
              resolve(data)
            }
          }
          worker.onerror = (event) => {
            fail(event.message)
          }
          worker.postMessage(args)
        })
        return {
          ready,
          done,
          stop: () => {
            worker.terminate()
          },
        }
      }

      function formatProgress(data) {
        let msg = 'opfs worker progress scenario=' + (data.scenario ?? '') +
          ' worker=' + (data.worker ?? 0) +
          ' phase=' + (data.phase ?? '')
        if (data.offset !== undefined || data.total !== undefined) {
          msg += ' offset=' + (data.offset ?? 0) + ' total=' + (data.total ?? 0)
        }
        return msg
      }
    </script>
  </body>
</html>
`

// workerJS loads the compiled Go program inside a disposable worker.
const workerJS = `importScripts('/wasm_exec.js')

self.__BLDR_TINYGO_STORED_BYTES = new Map()
self.__BLDR_TINYGO_STORED_BYTES_NEXT_ID = 1
self.__BLDR_TINYGO_WEB_LOCK_RELEASES = new Map()
self.__BLDR_TINYGO_WEB_LOCK_RELEASE_NEXT_ID = 1
self.__BLDR_TINYGO_WEB_LOCK_RELEASE_OPS = new Map()
self.__BLDR_TINYGO_WEB_LOCK_REQUESTS = new Map()
self.__BLDR_TINYGO_COPY_BYTES = (bytes) => {
  if (!(bytes instanceof Uint8Array)) {
    throw new TypeError('expected Uint8Array')
  }
  const copy = new Uint8Array(bytes.byteLength)
  copy.set(bytes)
  return copy
}
self.__BLDR_TINYGO_STORE_BYTES = (bytes) => {
  const id = self.__BLDR_TINYGO_STORED_BYTES_NEXT_ID++
  self.__BLDR_TINYGO_STORED_BYTES.set(id, bytes)
  return id
}
self.__BLDR_TINYGO_RUNTIME_EXITED = false
self.__BLDR_TINYGO_OPFS_RUNTIME_TASKS = new Set()
self.__BLDR_TINYGO_ABORT_OPFS_WRITABLE = (writable) => {
  if (!writable) {
    return Promise.resolve()
  }
  try {
    return writable.abort()
  } catch (reason) {
    return Promise.reject(reason)
  }
}
self.__BLDR_TINYGO_ABORT_OPFS_WRITABLE_QUIETLY = (writable) => {
  void self.__BLDR_TINYGO_ABORT_OPFS_WRITABLE(writable).catch(() => {})
}
self.__BLDR_TINYGO_TRACK_OPFS_TASK = (task) => {
  const tracked = Promise.resolve(task)
    .then(() => undefined, () => undefined)
    .finally(() => self.__BLDR_TINYGO_OPFS_RUNTIME_TASKS.delete(tracked))
  self.__BLDR_TINYGO_OPFS_RUNTIME_TASKS.add(tracked)
  return tracked
}
self.__BLDR_TINYGO_AWAIT_OPFS_TASKS = async () => {
  while (self.__BLDR_TINYGO_OPFS_RUNTIME_TASKS.size !== 0) {
    await Promise.all([...self.__BLDR_TINYGO_OPFS_RUNTIME_TASKS])
  }
}
self.__BLDR_TINYGO_CREATE_OPFS_WRITABLE = (handle, opts) => {
  if (self.__BLDR_TINYGO_RUNTIME_EXITED) {
    return Promise.resolve(undefined)
  }
  const created = opts ? handle.createWritable(opts) : handle.createWritable()
  return created.then(async (writable) => {
    if (self.__BLDR_TINYGO_RUNTIME_EXITED) {
      await self.__BLDR_TINYGO_ABORT_OPFS_WRITABLE(writable).catch(() => {})
      return undefined
    }
    return writable
  })
}
self.__BLDR_TINYGO_EXPORT = (go, name) => {
  const fn = go._inst?.exports?.[name]
  if (typeof fn !== 'function') {
    throw new Error('missing TinyGo export ' + name)
  }
  return fn
}
self.__BLDR_TINYGO_READ_STRING = (go, ptr, len) => {
  const memory = go._inst?.exports?.memory
  if (!(memory instanceof WebAssembly.Memory)) {
    throw new Error('TinyGo runtime memory is not initialized')
  }
  return new TextDecoder().decode(new Uint8Array(memory.buffer, ptr >>> 0, len))
}
self.__BLDR_TINYGO_MEMORY_VIEW = (go, ptr, len) => {
  const memory = go._inst?.exports?.memory
  if (!(memory instanceof WebAssembly.Memory)) {
    throw new Error('TinyGo runtime memory is not initialized')
  }
  return new Uint8Array(memory.buffer, ptr >>> 0, len)
}
self.__BLDR_TINYGO_UNBOX_VALUE = (go, rawRef) => {
  const ref = typeof rawRef === 'bigint' ? rawRef : BigInt(rawRef)
  const nanHead = 0x7ff80000n
  if (((ref >> 32n) & nanHead) !== nanHead) {
    throw new Error('TinyGo numeric js.Value refs are unsupported here')
  }
  const id = Number(ref & 0xffffffffn)
  const value = go._values?.[id]
  if (value === undefined) {
    throw new Error('TinyGo js.Value ref ' + id + ' is unavailable')
  }
  return value
}
self.__BLDR_TINYGO_BOX_VALUE = (go, value) => {
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
  if (!go._values || !go._ids || !go._goRefCounts || !go._idPool) {
    throw new Error('TinyGo js.Value table is not initialized')
  }
  let id = go._ids.get(value)
  if (id === undefined) {
    id = go._idPool.pop()
    if (id === undefined) {
      id = BigInt(go._values.length)
    }
    const index = Number(id)
    go._values[index] = value
    go._goRefCounts[index] = 0
    go._ids.set(value, id)
  }
  go._goRefCounts[Number(id)]++
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
self.__BLDR_TINYGO_OPFS_RESOLVE_REF = (opID, value) => {
  const go = self.__BLDR_TINYGO_CURRENT_GO
  const ref = self.__BLDR_TINYGO_BOX_VALUE(go, value)
  self.__BLDR_TINYGO_OPFS_RESOLVE(
    opID,
    Number((ref >> 32n) & 0xffffffffn),
    Number(ref & 0xffffffffn),
  )
}
self.__BLDR_TINYGO_DEFER_QUEUE = []
self.__BLDR_TINYGO_DEFER_SCHEDULED = false
self.__BLDR_TINYGO_DEFER_CHANNEL = new MessageChannel()
self.__BLDR_TINYGO_DEFER_CHANNEL.port1.onmessage = () => {
  self.__BLDR_TINYGO_DEFER_SCHEDULED = false
  const cb = self.__BLDR_TINYGO_DEFER_QUEUE.shift()
  if (cb) {
    cb()
  }
  if (self.__BLDR_TINYGO_DEFER_QUEUE.length !== 0) {
    self.__BLDR_TINYGO_DEFER_SCHEDULED = true
    self.__BLDR_TINYGO_DEFER_CHANNEL.port2.postMessage(undefined)
  }
}
self.__BLDR_TINYGO_DEFER = (cb) => {
  self.__BLDR_TINYGO_DEFER_QUEUE.push(cb)
  if (!self.__BLDR_TINYGO_DEFER_SCHEDULED) {
    self.__BLDR_TINYGO_DEFER_SCHEDULED = true
    self.__BLDR_TINYGO_DEFER_CHANNEL.port2.postMessage(undefined)
  }
}
self.__BLDR_TINYGO_CALL_EXPORT = (go, fn, ...args) => {
  fn(...args)
  const scheduler = go._inst?.exports?.go_scheduler
  if (typeof scheduler === 'function') {
    self.__BLDR_TINYGO_DEFER(() => scheduler())
    return
  }
  if (typeof go._resume === 'function') {
    self.__BLDR_TINYGO_DEFER(() => go._resume.call(go))
  }
}
self.__BLDR_TINYGO_OPFS_RESOLVE = (opID, ...values) => {
  const go = self.__BLDR_TINYGO_CURRENT_GO
  const resolve = self.__BLDR_TINYGO_EXPORT(go, 'BLDR_OPFS_HELPER_RESOLVE')
  self.__BLDR_TINYGO_DEFER(() => {
    if (self.__BLDR_TINYGO_RUNTIME_EXITED) {
      return
    }
    self.__BLDR_TINYGO_CALL_EXPORT(go, resolve, opID, values.length, values[0] ?? 0, values[1] ?? 0)
  })
}
self.__BLDR_TINYGO_OPFS_REJECT = (opID, code) => {
  const go = self.__BLDR_TINYGO_CURRENT_GO
  const reject = self.__BLDR_TINYGO_EXPORT(go, 'BLDR_OPFS_HELPER_REJECT')
  self.__BLDR_TINYGO_DEFER(() => {
    if (self.__BLDR_TINYGO_RUNTIME_EXITED) {
      return
    }
    self.__BLDR_TINYGO_CALL_EXPORT(go, reject, opID, code)
  })
}
self.__BLDR_TINYGO_REJECT_OPFS_WRITABLE_FAILURE = async (opID, writable, reason) => {
  const abortReason = await self.__BLDR_TINYGO_ABORT_OPFS_WRITABLE(writable)
    .then(() => undefined, (value) => value)
  const report = abortReason === undefined ? reason : abortReason
  if (!self.__BLDR_TINYGO_RUNTIME_EXITED) {
    self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(report))
  }
}
self.__BLDR_TINYGO_STORE_WEB_LOCK_RELEASE = (release, opID) => {
  const id = self.__BLDR_TINYGO_WEB_LOCK_RELEASE_NEXT_ID++
  self.__BLDR_TINYGO_WEB_LOCK_RELEASES.set(id, release)
  if (opID !== undefined) {
    self.__BLDR_TINYGO_WEB_LOCK_RELEASE_OPS.set(id, opID)
    const request = self.__BLDR_TINYGO_WEB_LOCK_REQUESTS.get(opID)
    if (request) {
      request.releaseID = id
    }
  }
  return id
}
self.__BLDR_TINYGO_RELEASE_WEB_LOCK = (releaseID) => {
  const opID = self.__BLDR_TINYGO_WEB_LOCK_RELEASE_OPS.get(releaseID)
  self.__BLDR_TINYGO_WEB_LOCK_RELEASE_OPS.delete(releaseID)
  if (opID !== undefined) {
    self.__BLDR_TINYGO_WEB_LOCK_REQUESTS.delete(opID)
  }
  const release = self.__BLDR_TINYGO_WEB_LOCK_RELEASES.get(releaseID)
  self.__BLDR_TINYGO_WEB_LOCK_RELEASES.delete(releaseID)
  if (!release) {
    return 0
  }
  release()
  return 1
}
self.__BLDR_TINYGO_CANCEL_WEB_LOCK = (opID) => {
  const request = self.__BLDR_TINYGO_WEB_LOCK_REQUESTS.get(opID)
  if (!request) {
    return 0
  }
  request.canceled = true
  if (request.releaseID !== undefined) {
    const release = self.__BLDR_TINYGO_WEB_LOCK_RELEASES.get(request.releaseID)
    self.__BLDR_TINYGO_RELEASE_WEB_LOCK(request.releaseID)
    return release ? 1 : 0
  }
  if (request.abort) {
    request.abort.abort()
  }
  return 1
}
self.BLDR_TINYGO_NEW_BYTES ??= (len) => new Uint8Array(len)
self.BLDR_TINYGO_TAKE_STORED_BYTES ??= (id) => {
  const bytes = self.__BLDR_TINYGO_STORED_BYTES.get(id)
  self.__BLDR_TINYGO_STORED_BYTES.delete(id)
  return bytes
}
self.BLDR_TINYGO_JS_CALL ??= (target, method, ...args) => {
  const fn = target[method]
  if (typeof fn !== 'function') {
    throw new TypeError('method ' + String(method) + ' is not callable')
  }
  return fn.apply(target, args)
}
self.BLDR_TINYGO_JS_NEW ??= (ctor, ...args) => new ctor(...args)
self.BLDR_TINYGO_PROMISE_ERROR_CODE ??= (reason) => {
  let name = ''
  if (reason && typeof reason === 'object') {
    if (typeof reason.name === 'string') {
      name = reason.name
    }
    if (!name && reason.constructor && typeof reason.constructor.name === 'string') {
      name = reason.constructor.name
    }
  }
  if (!name) {
    name = String(reason)
  }
  if (name.includes('NotFoundError')) {
    return 1
  }
  if (name.includes('NoModificationAllowedError')) {
    return 2
  }
  if (name.includes('QuotaExceededError')) {
    return 3
  }
  return 0
}
self.BLDR_TINYGO_PROMISE_AWAIT ??= (promise, resolve, reject) => {
  promise.then(resolve).catch((reason) => reject(self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
}
self.__BLDR_TINYGO_OPFS_WRITE_STREAM_ID ??= 1
self.__BLDR_TINYGO_OPFS_WRITE_STREAMS ??= new Map()
self.__BLDR_TINYGO_OPFS_READ_SNAPSHOT_ID ??= 1
self.__BLDR_TINYGO_OPFS_READ_SNAPSHOTS ??= new Map()
self.__BLDR_TINYGO_ABORT_OPFS_WRITE_STREAM ??= (streamID, strict = false) => {
  const stream = self.__BLDR_TINYGO_OPFS_WRITE_STREAMS.get(streamID)
  if (!stream) {
    return Promise.resolve(false)
  }
  self.__BLDR_TINYGO_TRACK_OPFS_TASK(stream.chain)
  const abort = strict
    ? self.__BLDR_TINYGO_ABORT_OPFS_WRITABLE(stream.writable)
    : self.__BLDR_TINYGO_ABORT_OPFS_WRITABLE(stream.writable).catch(() => {})
  const aborted = abort.then(() => {
    self.__BLDR_TINYGO_OPFS_WRITE_STREAMS.delete(streamID)
    return true
  })
  self.__BLDR_TINYGO_TRACK_OPFS_TASK(aborted)
  return aborted
}
self.BLDR_TINYGO_PUSH_BYTES ??= (sink, bytes) => {
  try {
    sink.push(self.__BLDR_TINYGO_COPY_BYTES(bytes))
    return true
  } catch {
    return false
  }
}
self.BLDR_TINYGO_POST_BYTES ??= (port, bytes) => {
  try {
    port.postMessage(self.__BLDR_TINYGO_COPY_BYTES(bytes))
    return true
  } catch {
    return false
  }
}
self.__BLDR_TINYGO_ENCODE_NAMES ??= (names) => {
  const encoder = new TextEncoder()
  const encoded = names.map((name) => encoder.encode(name))
  let size = 4
  for (const name of encoded) {
    size += 4 + name.byteLength
  }
  const bytes = new Uint8Array(size)
  const writeUint32 = (off, value) => {
    bytes[off] = (value >>> 24) & 0xff
    bytes[off + 1] = (value >>> 16) & 0xff
    bytes[off + 2] = (value >>> 8) & 0xff
    bytes[off + 3] = value & 0xff
    return off + 4
  }
  let off = writeUint32(0, encoded.length)
  for (const name of encoded) {
    off = writeUint32(off, name.byteLength)
    bytes.set(name, off)
    off += name.byteLength
  }
  return bytes
}
self.BLDR_OPFS_READ_FILE ??= (dir, name, opID) => {
  dir.getFileHandle(name)
    .then((handle) => handle.getFile())
    .then((file) => file.arrayBuffer())
    .then((buf) => {
      const bytes = new Uint8Array(buf)
      self.__BLDR_TINYGO_OPFS_RESOLVE(opID, self.__BLDR_TINYGO_STORE_BYTES(bytes), bytes.byteLength)
    })
    .catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
}
self.BLDR_OPFS_READ_AT ??= (handle, dst, off, opID) => {
  const dstLen = dst.byteLength
  handle.getFile()
    .then(async (file) => {
      if (off >= file.size || dstLen === 0) {
        self.__BLDR_TINYGO_OPFS_RESOLVE(opID, 0)
        return
      }
      const end = Math.min(off + dstLen, file.size)
      const buf = await file.slice(off, end).arrayBuffer()
      const bytes = new Uint8Array(buf)
      if (bytes.byteLength !== 0) {
        dst.subarray(0, bytes.byteLength).set(bytes)
      }
      self.__BLDR_TINYGO_OPFS_RESOLVE(opID, bytes.byteLength)
    })
    .catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
}
self.BLDR_OPFS_LIST_DIRECTORY ??= (dir, opID) => {
  ;(async () => {
    const names = []
    for await (const [name] of dir.entries()) {
      names.push(name)
    }
    const bytes = self.__BLDR_TINYGO_ENCODE_NAMES(names)
    self.__BLDR_TINYGO_OPFS_RESOLVE(opID, self.__BLDR_TINYGO_STORE_BYTES(bytes), bytes.byteLength)
  })().catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
}
self.BLDR_OPFS_WRITE_AT ??= (handle, data, off, keepExisting, opID) => {
  const writeData = self.__BLDR_TINYGO_COPY_BYTES(data)
  const state = {}
  const opts = keepExisting ? { keepExistingData: true } : undefined
  const task = self.__BLDR_TINYGO_CREATE_OPFS_WRITABLE(handle, opts)
    .then(async (next) => {
      if (!next) {
        return
      }
      state.writable = next
      if (off !== 0) {
        await next.seek(off)
      }
      if (writeData.byteLength !== 0) {
        await next.write(writeData)
      }
      await next.close()
      if (self.__BLDR_TINYGO_RUNTIME_EXITED) {
        return
      }
      self.__BLDR_TINYGO_OPFS_RESOLVE(opID, writeData.byteLength)
    })
    .catch(async (reason) => {
      await self.__BLDR_TINYGO_REJECT_OPFS_WRITABLE_FAILURE(opID, state.writable, reason)
    })
  self.__BLDR_TINYGO_TRACK_OPFS_TASK(task)
}
self.BLDR_OPFS_WRITE_FILE ??= (dir, name, data, opID) => {
  const writeData = self.__BLDR_TINYGO_COPY_BYTES(data)
  const state = {}
  const task = dir.getFileHandle(name, { create: true })
    .then((handle) => self.__BLDR_TINYGO_CREATE_OPFS_WRITABLE(handle))
    .then(async (next) => {
      if (!next) {
        return
      }
      state.writable = next
      if (writeData.byteLength !== 0) {
        await next.write(writeData)
      }
      await next.close()
      if (self.__BLDR_TINYGO_RUNTIME_EXITED) {
        return
      }
      self.__BLDR_TINYGO_OPFS_RESOLVE(opID, writeData.byteLength)
    })
    .catch(async (reason) => {
      await self.__BLDR_TINYGO_REJECT_OPFS_WRITABLE_FAILURE(opID, state.writable, reason)
    })
  self.__BLDR_TINYGO_TRACK_OPFS_TASK(task)
}

let __opfsChrometestCurrentArgs = null
function __opfsChrometestErrorText(reason) {
  if (reason && typeof reason === 'object' && typeof reason.stack === 'string') {
    return reason.stack
  }
  if (reason && typeof reason === 'object' && typeof reason.message === 'string') {
    return reason.message
  }
  return String(reason)
}
function __opfsChrometestPostUnhandled(reason) {
  const args = __opfsChrometestCurrentArgs ?? {}
  self.postMessage({
    kind: 'result',
    scenario: args.scenario ?? '',
    worker: args.worker ?? 0,
    ok: false,
    error: __opfsChrometestErrorText(reason),
  })
}
self.onerror = (message, source, lineno, colno, error) => {
  __opfsChrometestPostUnhandled(error || (String(message) + ' at ' + source + ':' + lineno + ':' + colno))
  return true
}
self.onunhandledrejection = (event) => {
  __opfsChrometestPostUnhandled(event.reason)
}


self.onmessage = async (event) => {
  const args = event.data
  __opfsChrometestCurrentArgs = args
  const go = new Go()
  self.__BLDR_TINYGO_CURRENT_GO = go
  go.argv = [
    'testprog',
    args.scenario ?? '',
    args.root ?? '',
    String(args.worker ?? 0),
    String(args.workers ?? 1),
    String(args.iterations ?? 1),
    String(args.batch ?? 1),
  ]
  self.__OPFS_CHROMETEST_ARGS = go.argv
  if (go.importObject.gojs && typeof go.importObject.gojs['runtime.getRandomData'] !== 'function') {
    go.importObject.gojs['runtime.getRandomData'] = (ptr, len) => {
      const memory = go._inst?.exports.memory
      if (!(memory instanceof WebAssembly.Memory)) {
        throw new Error('TinyGo runtime memory is not initialized')
      }
      crypto.getRandomValues(new Uint8Array(memory.buffer, ptr >>> 0, len))
    }
  }
  if (go.importObject.gojs) {
    go.importObject.gojs['bldr.opfs.acquireWebLock'] ??= (opID, namePtr, nameLen, exclusive, ifAvailable) => {
      const resolve = self.__BLDR_TINYGO_EXPORT(go, 'BLDR_OPFS_WEB_LOCK_RESOLVE')
      const reject = self.__BLDR_TINYGO_EXPORT(go, 'BLDR_OPFS_WEB_LOCK_REJECT')
      const released = self.__BLDR_TINYGO_EXPORT(go, 'BLDR_OPFS_WEB_LOCK_RELEASED')
      const locks = self.navigator?.locks
      if (!locks) {
        self.__BLDR_TINYGO_DEFER(() => self.__BLDR_TINYGO_CALL_EXPORT(go, reject, opID, 0))
        return
      }
      const opts = { mode: exclusive ? 'exclusive' : 'shared' }
      const abort = ifAvailable ? undefined : new AbortController()
      if (abort) {
        opts.signal = abort.signal
      }
      if (ifAvailable) {
        opts.ifAvailable = true
      }
      const name = self.__BLDR_TINYGO_READ_STRING(go, namePtr, nameLen)
      const request = { abort }
      self.__BLDR_TINYGO_WEB_LOCK_REQUESTS.set(opID, request)
      let acquired = false
      locks.request(name, opts, (lock) => {
        if (ifAvailable && !lock) {
          self.__BLDR_TINYGO_WEB_LOCK_REQUESTS.delete(opID)
          self.__BLDR_TINYGO_DEFER(() => self.__BLDR_TINYGO_CALL_EXPORT(go, resolve, opID, 0, 0))
          return undefined
        }
        return new Promise((releaseLock) => {
          if (request.canceled) {
            releaseLock()
            self.__BLDR_TINYGO_WEB_LOCK_REQUESTS.delete(opID)
            return
          }
          acquired = true
          const releaseID = self.__BLDR_TINYGO_STORE_WEB_LOCK_RELEASE(releaseLock, opID)
          self.__BLDR_TINYGO_DEFER(() => self.__BLDR_TINYGO_CALL_EXPORT(go, resolve, opID, releaseID, 1))
        })
      }).then(() => {
        if (acquired) {
          self.__BLDR_TINYGO_DEFER(() => self.__BLDR_TINYGO_CALL_EXPORT(go, released, opID))
        }
      }).catch((reason) => {
        self.__BLDR_TINYGO_WEB_LOCK_REQUESTS.delete(opID)
        if (request.canceled) {
          return
        }
        self.__BLDR_TINYGO_DEFER(() => self.__BLDR_TINYGO_CALL_EXPORT(go, reject, opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
      })
    }
    go.importObject.gojs['bldr.opfs.cancelWebLock'] ??= (opID) => {
      return self.__BLDR_TINYGO_CANCEL_WEB_LOCK(opID)
    }
    go.importObject.gojs['bldr.opfs.releaseWebLock'] ??= (releaseID) => {
      return self.__BLDR_TINYGO_RELEASE_WEB_LOCK(releaseID)
    }
    go.importObject.gojs['bldr.opfs.getRootRef'] ??= (opID) => {
      self.navigator.storage.getDirectory()
        .then((dir) => self.__BLDR_TINYGO_OPFS_RESOLVE_REF(opID, dir))
        .catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
    }
    go.importObject.gojs['bldr.opfs.getDirectoryRef'] ??= (opID, parentRef, namePtr, nameLen, create) => {
      const parent = self.__BLDR_TINYGO_UNBOX_VALUE(go, parentRef)
      const name = self.__BLDR_TINYGO_READ_STRING(go, namePtr, nameLen)
      parent.getDirectoryHandle(name, { create: Boolean(create) })
        .then((dir) => self.__BLDR_TINYGO_OPFS_RESOLVE_REF(opID, dir))
        .catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
    }
    go.importObject.gojs['bldr.opfs.openFileRef'] ??= (opID, dirRef, namePtr, nameLen, create) => {
      const dir = self.__BLDR_TINYGO_UNBOX_VALUE(go, dirRef)
      const name = self.__BLDR_TINYGO_READ_STRING(go, namePtr, nameLen)
      const opts = create ? { create: true } : undefined
      const filePromise = opts ? dir.getFileHandle(name, opts) : dir.getFileHandle(name)
      filePromise
        .then((handle) => self.__BLDR_TINYGO_OPFS_RESOLVE_REF(opID, handle))
        .catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
    }
    go.importObject.gojs['bldr.opfs.openReadSnapshotRef'] ??= (opID, dirRef, namePtr, nameLen) => {
      const dir = self.__BLDR_TINYGO_UNBOX_VALUE(go, dirRef)
      const name = self.__BLDR_TINYGO_READ_STRING(go, namePtr, nameLen)
      const task = dir.getFileHandle(name)
        .then((handle) => handle.getFile())
        .then((file) => {
          if (self.__BLDR_TINYGO_RUNTIME_EXITED) {
            return
          }
          const snapshotID = self.__BLDR_TINYGO_OPFS_READ_SNAPSHOT_ID++
          self.__BLDR_TINYGO_OPFS_READ_SNAPSHOTS.set(snapshotID, file)
          self.__BLDR_TINYGO_OPFS_RESOLVE(opID, snapshotID, file.size)
        })
        .catch((reason) => {
          if (!self.__BLDR_TINYGO_RUNTIME_EXITED) {
            self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason))
          }
        })
      self.__BLDR_TINYGO_TRACK_OPFS_TASK(task)
    }
    go.importObject.gojs['bldr.opfs.fileExistsRef'] ??= (opID, dirRef, namePtr, nameLen) => {
      const dir = self.__BLDR_TINYGO_UNBOX_VALUE(go, dirRef)
      const name = self.__BLDR_TINYGO_READ_STRING(go, namePtr, nameLen)
      dir.getFileHandle(name)
        .then(() => self.__BLDR_TINYGO_OPFS_RESOLVE(opID, 1))
        .catch((reason) => {
          const code = self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)
          if (code === 1) {
            self.__BLDR_TINYGO_OPFS_RESOLVE(opID, 0)
            return
          }
          self.__BLDR_TINYGO_OPFS_REJECT(opID, code)
        })
    }
    go.importObject.gojs['bldr.opfs.deleteEntryRef'] ??= (opID, dirRef, namePtr, nameLen, recursive) => {
      const dir = self.__BLDR_TINYGO_UNBOX_VALUE(go, dirRef)
      const name = self.__BLDR_TINYGO_READ_STRING(go, namePtr, nameLen)
      dir.removeEntry(name, { recursive: Boolean(recursive) })
        .then(() => self.__BLDR_TINYGO_OPFS_RESOLVE(opID, 1))
        .catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
    }
    go.importObject.gojs['bldr.opfs.yieldMicrotask'] ??= (opID) => {
      queueMicrotask(() => self.__BLDR_TINYGO_OPFS_RESOLVE(opID, 1))
    }
    go.importObject.gojs['bldr.opfs.sizeRef'] ??= (opID, handleRef) => {
      const handle = self.__BLDR_TINYGO_UNBOX_VALUE(go, handleRef)
      handle.getFile()
        .then((file) => self.__BLDR_TINYGO_OPFS_RESOLVE(opID, file.size))
        .catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
    }
    go.importObject.gojs['bldr.opfs.truncateRef'] ??= (opID, handleRef, size) => {
      const handle = self.__BLDR_TINYGO_UNBOX_VALUE(go, handleRef)
      const state = {}
      const task = self.__BLDR_TINYGO_CREATE_OPFS_WRITABLE(handle, { keepExistingData: true })
        .then(async (next) => {
          if (!next) {
            return
          }
          state.writable = next
          await next.truncate(Number(size))
          await next.close()
          if (self.__BLDR_TINYGO_RUNTIME_EXITED) {
            return
          }
          self.__BLDR_TINYGO_OPFS_RESOLVE(opID, 1)
        })
        .catch(async (reason) => {
          await self.__BLDR_TINYGO_REJECT_OPFS_WRITABLE_FAILURE(opID, state.writable, reason)
        })
      self.__BLDR_TINYGO_TRACK_OPFS_TASK(task)
    }
    go.importObject.gojs['bldr.opfs.takeStoredBytes'] ??= (bytesID, ptr, len) => {
      const bytes = self.BLDR_TINYGO_TAKE_STORED_BYTES(bytesID)
      if (!bytes || bytes.byteLength !== len) {
        return 0
      }
      if (len !== 0) {
        self.__BLDR_TINYGO_MEMORY_VIEW(go, ptr, len).set(bytes)
      }
      return 1
    }
    go.importObject.gojs['bldr.opfs.readFileRef'] ??= (opID, dirRef, namePtr, nameLen) => {
      const dir = self.__BLDR_TINYGO_UNBOX_VALUE(go, dirRef)
      const name = self.__BLDR_TINYGO_READ_STRING(go, namePtr, nameLen)
      dir.getFileHandle(name)
        .then((handle) => handle.getFile())
        .then((file) => file.arrayBuffer())
        .then((buf) => {
          const bytes = new Uint8Array(buf)
          self.__BLDR_TINYGO_OPFS_RESOLVE(opID, self.__BLDR_TINYGO_STORE_BYTES(bytes), bytes.byteLength)
        })
        .catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
    }
    go.importObject.gojs['bldr.opfs.readAtRef'] ??= (opID, handleRef, dstPtr, dstLen, off) => {
      const handle = self.__BLDR_TINYGO_UNBOX_VALUE(go, handleRef)
      const offset = Number(off)
      handle.getFile()
        .then(async (file) => {
          if (offset >= file.size || dstLen === 0) {
            self.__BLDR_TINYGO_OPFS_RESOLVE(opID, 0)
            return
          }
          const end = Math.min(offset + dstLen, file.size)
          const buf = await file.slice(offset, end).arrayBuffer()
          const bytes = new Uint8Array(buf)
          if (bytes.byteLength !== 0) {
            self.__BLDR_TINYGO_MEMORY_VIEW(go, dstPtr, bytes.byteLength).set(bytes)
          }
          self.__BLDR_TINYGO_OPFS_RESOLVE(opID, bytes.byteLength)
        })
        .catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
    }
    go.importObject.gojs['bldr.opfs.readSnapshotAtRef'] ??= (opID, snapshotID, dstPtr, dstLen, off) => {
      const file = self.__BLDR_TINYGO_OPFS_READ_SNAPSHOTS.get(snapshotID)
      if (!file) {
        self.__BLDR_TINYGO_OPFS_REJECT(opID, 1)
        return
      }
      const offset = Number(off)
      const task = (async () => {
        if (offset >= file.size || dstLen === 0) {
          self.__BLDR_TINYGO_OPFS_RESOLVE(opID, 0)
          return
        }
        const end = Math.min(offset + dstLen, file.size)
        const buf = await file.slice(offset, end).arrayBuffer()
        if (
          self.__BLDR_TINYGO_RUNTIME_EXITED ||
          self.__BLDR_TINYGO_OPFS_READ_SNAPSHOTS.get(snapshotID) !== file
        ) {
          return
        }
        const bytes = new Uint8Array(buf)
        if (bytes.byteLength !== 0) {
          self.__BLDR_TINYGO_MEMORY_VIEW(go, dstPtr, bytes.byteLength).set(bytes)
        }
        self.__BLDR_TINYGO_OPFS_RESOLVE(opID, bytes.byteLength)
      })().catch((reason) => {
        if (
          !self.__BLDR_TINYGO_RUNTIME_EXITED &&
          self.__BLDR_TINYGO_OPFS_READ_SNAPSHOTS.get(snapshotID) === file
        ) {
          const code = reason?.name === 'NotReadableError'
            ? 1
            : self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)
          self.__BLDR_TINYGO_OPFS_REJECT(opID, code)
        }
      })
      self.__BLDR_TINYGO_TRACK_OPFS_TASK(task)
    }
    go.importObject.gojs['bldr.opfs.closeReadSnapshotRef'] ??= (opID, snapshotID) => {
      if (!self.__BLDR_TINYGO_OPFS_READ_SNAPSHOTS.has(snapshotID)) {
        self.__BLDR_TINYGO_OPFS_REJECT(opID, 1)
        return
      }
      self.__BLDR_TINYGO_OPFS_READ_SNAPSHOTS.delete(snapshotID)
      self.__BLDR_TINYGO_OPFS_RESOLVE(opID, 1)
    }
    go.importObject.gojs['bldr.opfs.listDirectoryRef'] ??= (opID, dirRef) => {
      const dir = self.__BLDR_TINYGO_UNBOX_VALUE(go, dirRef)
      ;(async () => {
        const names = []
        for await (const [name] of dir.entries()) {
          names.push(name)
        }
        const bytes = self.__BLDR_TINYGO_ENCODE_NAMES(names)
        self.__BLDR_TINYGO_OPFS_RESOLVE(opID, self.__BLDR_TINYGO_STORE_BYTES(bytes), bytes.byteLength)
      })().catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
    }
    go.importObject.gojs['bldr.opfs.writeAtRef'] ??= (opID, handleRef, dataPtr, dataLen, off, keepExisting) => {
      const handle = self.__BLDR_TINYGO_UNBOX_VALUE(go, handleRef)
      const state = {}
      try {
        const writeData = self.__BLDR_TINYGO_COPY_BYTES(self.__BLDR_TINYGO_MEMORY_VIEW(go, dataPtr, dataLen))
        const opts = keepExisting ? { keepExistingData: true } : undefined
        const task = self.__BLDR_TINYGO_CREATE_OPFS_WRITABLE(handle, opts)
          .then(async (next) => {
            if (!next) {
              return
            }
            state.writable = next
            const offset = Number(off)
            if (offset !== 0) {
              await next.seek(offset)
            }
            if (writeData.byteLength !== 0) {
              await next.write(writeData)
            }
            await next.close()
            if (self.__BLDR_TINYGO_RUNTIME_EXITED) {
              return
            }
            self.__BLDR_TINYGO_OPFS_RESOLVE(opID, writeData.byteLength)
          })
          .catch(async (reason) => {
            await self.__BLDR_TINYGO_REJECT_OPFS_WRITABLE_FAILURE(opID, state.writable, reason)
          })
        self.__BLDR_TINYGO_TRACK_OPFS_TASK(task)
      } catch (reason) {
        self.__BLDR_TINYGO_ABORT_OPFS_WRITABLE_QUIETLY(state.writable)
        if (!self.__BLDR_TINYGO_RUNTIME_EXITED) {
          self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason))
        }
      }
    }
    go.importObject.gojs['bldr.opfs.writeFileRef'] ??= (opID, dirRef, namePtr, nameLen, dataPtr, dataLen) => {
      const dir = self.__BLDR_TINYGO_UNBOX_VALUE(go, dirRef)
      const state = {}
      try {
        const name = self.__BLDR_TINYGO_READ_STRING(go, namePtr, nameLen)
        const writeData = self.__BLDR_TINYGO_COPY_BYTES(self.__BLDR_TINYGO_MEMORY_VIEW(go, dataPtr, dataLen))
        const task = dir.getFileHandle(name, { create: true })
          .then((handle) => self.__BLDR_TINYGO_CREATE_OPFS_WRITABLE(handle))
          .then(async (next) => {
            if (!next) {
              return
            }
            state.writable = next
            if (writeData.byteLength !== 0) {
              await next.write(writeData)
            }
            await next.close()
            if (self.__BLDR_TINYGO_RUNTIME_EXITED) {
              return
            }
            self.__BLDR_TINYGO_OPFS_RESOLVE(opID, writeData.byteLength)
          })
          .catch(async (reason) => {
            await self.__BLDR_TINYGO_REJECT_OPFS_WRITABLE_FAILURE(opID, state.writable, reason)
          })
        self.__BLDR_TINYGO_TRACK_OPFS_TASK(task)
    } catch (reason) {
      self.__BLDR_TINYGO_ABORT_OPFS_WRITABLE_QUIETLY(state.writable)
      if (!self.__BLDR_TINYGO_RUNTIME_EXITED) {
        self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason))
      }
    }
  }
    go.importObject.gojs['bldr.opfs.openWriteStreamRef'] ??= (opID, dirRef, namePtr, nameLen) => {
      const dir = self.__BLDR_TINYGO_UNBOX_VALUE(go, dirRef)
      const state = {}
      try {
        const name = self.__BLDR_TINYGO_READ_STRING(go, namePtr, nameLen)
        const task = dir.getFileHandle(name, { create: true })
          .then((handle) => self.__BLDR_TINYGO_CREATE_OPFS_WRITABLE(handle))
          .then((next) => {
            if (!next) {
              return
            }
            state.writable = next
            const streamID = self.__BLDR_TINYGO_OPFS_WRITE_STREAM_ID++
            self.__BLDR_TINYGO_OPFS_WRITE_STREAMS.set(streamID, {
              writable: next,
              chain: Promise.resolve(),
            })
            self.__BLDR_TINYGO_OPFS_RESOLVE(opID, streamID)
          })
          .catch(async (reason) => {
            await self.__BLDR_TINYGO_REJECT_OPFS_WRITABLE_FAILURE(opID, state.writable, reason)
          })
        self.__BLDR_TINYGO_TRACK_OPFS_TASK(task)
      } catch (reason) {
        self.__BLDR_TINYGO_ABORT_OPFS_WRITABLE_QUIETLY(state.writable)
        if (!self.__BLDR_TINYGO_RUNTIME_EXITED) {
          self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason))
        }
      }
    }
    go.importObject.gojs['bldr.opfs.writeStreamRef'] ??= (opID, streamID, dataPtr, dataLen) => {
      const stream = self.__BLDR_TINYGO_OPFS_WRITE_STREAMS.get(streamID)
      if (!stream) {
        self.__BLDR_TINYGO_OPFS_REJECT(opID, 1)
        return
      }
      try {
        const writeData = self.__BLDR_TINYGO_COPY_BYTES(self.__BLDR_TINYGO_MEMORY_VIEW(go, dataPtr, dataLen))
        stream.chain = stream.chain
          .then(async () => {
            if (writeData.byteLength !== 0) {
              await stream.writable.write(writeData)
            }
            self.__BLDR_TINYGO_OPFS_RESOLVE(opID, writeData.byteLength)
          })
          .catch((reason) => {
            if (!self.__BLDR_TINYGO_RUNTIME_EXITED) {
              self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason))
            }
          })
      } catch (reason) {
        if (!self.__BLDR_TINYGO_RUNTIME_EXITED) {
          self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason))
        }
      }
    }
    go.importObject.gojs['bldr.opfs.closeWriteStreamRef'] ??= (opID, streamID) => {
      const stream = self.__BLDR_TINYGO_OPFS_WRITE_STREAMS.get(streamID)
      if (!stream) {
        self.__BLDR_TINYGO_OPFS_REJECT(opID, 1)
        return
      }
      stream.chain = stream.chain
        .then(async () => {
          await stream.writable.close()
          self.__BLDR_TINYGO_OPFS_WRITE_STREAMS.delete(streamID)
          self.__BLDR_TINYGO_OPFS_RESOLVE(opID, 1)
        })
        .catch((reason) => {
          if (!self.__BLDR_TINYGO_RUNTIME_EXITED) {
            self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason))
          }
        })
    }
    go.importObject.gojs['bldr.opfs.abortWriteStreamRef'] ??= (opID, streamID) => {
      Promise.resolve(self.__BLDR_TINYGO_ABORT_OPFS_WRITE_STREAM(streamID, true))
        .then((aborted) => self.__BLDR_TINYGO_OPFS_RESOLVE(opID, aborted ? 1 : 0))
        .catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
    }
    go.importObject.gojs['bldr.opfs.broadcastChannelNewRef'] ??= (namePtr, nameLen) => {
      const name = self.__BLDR_TINYGO_READ_STRING(go, namePtr, nameLen)
      return self.__BLDR_TINYGO_BOX_VALUE(go, new BroadcastChannel(name))
    }
    go.importObject.gojs['bldr.opfs.broadcastSendRef'] ??= (channelRef, shardID, generationHi, generationLo) => {
      const channel = self.__BLDR_TINYGO_UNBOX_VALUE(go, channelRef)
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
    go.importObject.gojs['bldr.opfs.broadcastCloseRef'] ??= (channelRef) => {
      self.__BLDR_TINYGO_UNBOX_VALUE(go, channelRef).close()
    }
  }
  const res = await WebAssembly.instantiateStreaming(fetch('/testprog.wasm'), go.importObject)
  try {
    await go.run(res.instance)
  } finally {
    self.__BLDR_TINYGO_RUNTIME_EXITED = true
    for (const [streamID] of self.__BLDR_TINYGO_OPFS_WRITE_STREAMS) {
      void self.__BLDR_TINYGO_ABORT_OPFS_WRITE_STREAM(streamID)
    }
    self.__BLDR_TINYGO_OPFS_READ_SNAPSHOTS.clear()
    await self.__BLDR_TINYGO_AWAIT_OPFS_TASKS()
  }
}
`
