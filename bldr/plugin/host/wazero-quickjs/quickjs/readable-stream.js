// Minimal ReadableStream polyfill for the QuickJS plugin host.
//
// QuickJS ships a reduced standard library with no WHATWG Streams. This
// implements the default-reader subset the plugin SDK actually uses: an
// underlying source with start/pull/cancel, a controller with
// enqueue/close/error/desiredSize, and getReader() returning a default reader
// with read/releaseLock/cancel/closed. It is a count-queuing stream (each chunk
// counts as 1 against highWaterMark); BYOB readers, tee, and pipe are omitted.
//
// Consumers: sdk/unixfs FSHandle.uploadFile drains via getReader().read();
// app/quickstart and plugin/notes produce via start(controller) enqueue/close.

function callOrUndefined(fn, thisArg, ...args) {
  if (typeof fn !== 'function') {
    return undefined
  }
  return fn.apply(thisArg, args)
}

class ReadableStreamDefaultController {
  constructor(stream, underlyingSource, highWaterMark, sizeAlgorithm) {
    this._stream = stream
    this._queue = []
    this._queueTotalSize = 0
    this._highWaterMark = highWaterMark
    this._sizeAlgorithm = sizeAlgorithm
    this._started = false
    this._closeRequested = false
    this._pulling = false
    this._pullAgain = false
    this._pullAlgorithm = () =>
      Promise.resolve(
        callOrUndefined(underlyingSource.pull, underlyingSource, this),
      )
    this._cancelAlgorithm = (reason) =>
      Promise.resolve(
        callOrUndefined(underlyingSource.cancel, underlyingSource, reason),
      )

    const startResult = callOrUndefined(
      underlyingSource.start,
      underlyingSource,
      this,
    )
    Promise.resolve(startResult).then(
      () => {
        this._started = true
        this._callPullIfNeeded()
      },
      (err) => {
        this._error(err)
      },
    )
  }

  get desiredSize() {
    if (this._stream._state === 'errored') {
      return null
    }
    if (this._stream._state === 'closed') {
      return 0
    }
    return this._highWaterMark - this._queueTotalSize
  }

  enqueue(chunk) {
    if (this._closeRequested || this._stream._state !== 'readable') {
      throw new TypeError('cannot enqueue: stream is closing or not readable')
    }
    const reader = this._stream._reader
    if (reader && reader._readRequests.length > 0) {
      const request = reader._readRequests.shift()
      request.resolve({ value: chunk, done: false })
    } else {
      let size = 1
      if (this._sizeAlgorithm) {
        size = this._sizeAlgorithm(chunk)
      }
      this._queue.push({ chunk, size })
      this._queueTotalSize += size
    }
    this._callPullIfNeeded()
  }

  close() {
    if (this._closeRequested || this._stream._state !== 'readable') {
      throw new TypeError('cannot close: stream is closing or not readable')
    }
    this._closeRequested = true
    if (this._queue.length === 0) {
      this._closeStream()
    }
  }

  error(err) {
    this._error(err)
  }

  _closeStream() {
    const stream = this._stream
    stream._state = 'closed'
    const reader = stream._reader
    if (reader) {
      for (const request of reader._readRequests) {
        request.resolve({ value: undefined, done: true })
      }
      reader._readRequests = []
      reader._resolveClosed()
    }
  }

  _error(err) {
    const stream = this._stream
    if (stream._state !== 'readable') {
      return
    }
    stream._state = 'errored'
    stream._storedError = err
    this._queue = []
    this._queueTotalSize = 0
    const reader = stream._reader
    if (reader) {
      for (const request of reader._readRequests) {
        request.reject(err)
      }
      reader._readRequests = []
      reader._rejectClosed(err)
    }
  }

  _pullChunk() {
    const entry = this._queue.shift()
    this._queueTotalSize -= entry.size
    if (this._queue.length === 0 && this._closeRequested) {
      this._closeStream()
    } else {
      this._callPullIfNeeded()
    }
    return entry.chunk
  }

  _callPullIfNeeded() {
    if (!this._shouldCallPull()) {
      return
    }
    if (this._pulling) {
      this._pullAgain = true
      return
    }
    this._pulling = true
    this._pullAlgorithm().then(
      () => {
        this._pulling = false
        if (this._pullAgain) {
          this._pullAgain = false
          this._callPullIfNeeded()
        }
      },
      (err) => {
        this._error(err)
      },
    )
  }

  _shouldCallPull() {
    const stream = this._stream
    if (stream._state !== 'readable') {
      return false
    }
    if (this._closeRequested) {
      return false
    }
    if (!this._started) {
      return false
    }
    const reader = stream._reader
    if (reader && reader._readRequests.length > 0) {
      return true
    }
    return this.desiredSize > 0
  }
}

class ReadableStreamDefaultReader {
  constructor(stream) {
    if (stream._reader) {
      throw new TypeError('ReadableStream is already locked to a reader')
    }
    this._stream = stream
    this._readRequests = []
    stream._reader = this
    this._closedPromise = new Promise((resolve, reject) => {
      this._resolveClosed = resolve
      this._rejectClosed = reject
    })
    // Avoid unhandled-rejection noise; callers observe via .closed explicitly.
    this._closedPromise.catch(() => {})
    if (stream._state === 'closed') {
      this._resolveClosed()
    } else if (stream._state === 'errored') {
      this._rejectClosed(stream._storedError)
    }
  }

  get closed() {
    return this._closedPromise
  }

  read() {
    const stream = this._stream
    if (!stream) {
      return Promise.reject(new TypeError('reader has been released'))
    }
    if (stream._state === 'errored') {
      return Promise.reject(stream._storedError)
    }
    const controller = stream._controller
    if (controller._queue.length > 0) {
      return Promise.resolve({ value: controller._pullChunk(), done: false })
    }
    if (stream._state === 'closed') {
      return Promise.resolve({ value: undefined, done: true })
    }
    return new Promise((resolve, reject) => {
      this._readRequests.push({ resolve, reject })
      controller._callPullIfNeeded()
    })
  }

  cancel(reason) {
    if (!this._stream) {
      return Promise.reject(new TypeError('reader has been released'))
    }
    return this._stream._cancel(reason)
  }

  releaseLock() {
    const stream = this._stream
    if (!stream) {
      return
    }
    if (this._readRequests.length > 0) {
      throw new TypeError('cannot release a reader with pending read requests')
    }
    if (stream._state === 'readable') {
      this._rejectClosed(
        new TypeError('reader released while stream was readable'),
      )
    }
    stream._reader = undefined
    this._stream = undefined
  }
}

export class ReadableStream {
  constructor(underlyingSource = {}, strategy = {}) {
    this._state = 'readable'
    this._storedError = undefined
    this._reader = undefined
    const highWaterMark =
      strategy.highWaterMark === undefined ? 1 : Number(strategy.highWaterMark)
    if (Number.isNaN(highWaterMark) || highWaterMark < 0) {
      throw new RangeError('invalid highWaterMark')
    }
    this._controller = new ReadableStreamDefaultController(
      this,
      underlyingSource,
      highWaterMark,
      strategy.size,
    )
  }

  get locked() {
    return this._reader !== undefined
  }

  getReader() {
    return new ReadableStreamDefaultReader(this)
  }

  cancel(reason) {
    if (this._reader) {
      return Promise.reject(new TypeError('cannot cancel a locked stream'))
    }
    return this._cancel(reason)
  }

  _cancel(reason) {
    if (this._state === 'closed') {
      return Promise.resolve()
    }
    if (this._state === 'errored') {
      return Promise.reject(this._storedError)
    }
    this._controller._queue = []
    this._controller._queueTotalSize = 0
    const result = this._controller._cancelAlgorithm(reason)
    this._controller._closeStream()
    return result.then(() => undefined)
  }
}
