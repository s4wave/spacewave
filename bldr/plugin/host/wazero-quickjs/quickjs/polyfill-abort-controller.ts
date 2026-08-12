/* eslint-disable @typescript-eslint/no-explicit-any */
export interface AbortSignalPolyfillConstructor {
  new (): AbortSignal
  abort(reason?: any): AbortSignal
  any(signals: Iterable<AbortSignal>): AbortSignal
  timeout(delay: number): AbortSignal
}

export interface AbortControllerPolyfillConstructor {
  new (): AbortController
  AbortSignal: AbortSignalPolyfillConstructor
}

// createAbortController creates an AbortController polyfill implementation.
export function createAbortController(): AbortControllerPolyfillConstructor {
  const signalInstances = new WeakSet<object>()

  class AbortSignalImpl implements AbortSignal {
    constructor() {
      signalInstances.add(this)
    }
    static abort(reason?: any): AbortSignal {
      const signal = new AbortSignalImpl()
      signal._abort(reason)
      return signal
    }

    static any(signals: Iterable<AbortSignal>): AbortSignal {
      const inputs = Array.from(signals)
      for (const signal of inputs) {
        if (!signalInstances.has(signal as object)) {
          throw new TypeError('AbortSignal.any requires AbortSignal values')
        }
      }

      const result = new AbortSignalImpl()
      for (const signal of inputs) {
        if (signal.aborted) {
          result._abort(signal.reason)
          return result
        }
      }

      const listeners = new Map<AbortSignal, (event: Event) => void>()
      const cleanup = () => {
        for (const [signal, listener] of listeners) {
          signal.removeEventListener('abort', listener)
        }
        listeners.clear()
      }
      for (const signal of inputs) {
        if (listeners.has(signal)) continue
        const onAbort = () => {
          if (result.aborted) return
          result._abort(signal.reason)
          cleanup()
        }
        listeners.set(signal, onAbort)
        signal.addEventListener('abort', onAbort, { once: true })
      }
      return result
    }

    static timeout(delay: number): AbortSignal {
      const signal = new AbortSignalImpl()
      setTimeout(() => {
        signal._abort(new Error('TimeoutError'))
      }, delay)
      return signal
    }
    private _aborted = false
    private _reason: any = undefined
    private _listeners: Array<(event: Event) => void> = []
    private _onabort: ((event: Event) => void) | null = null

    get aborted(): boolean {
      return this._aborted
    }

    get reason(): any {
      return this._reason
    }

    get onabort(): ((event: Event) => void) | null {
      return this._onabort
    }

    set onabort(handler: ((event: Event) => void) | null) {
      this._onabort = handler
    }

    addEventListener(
      type: string,
      listener: (event: Event) => void,
      _options?: AddEventListenerOptions | boolean,
    ): void {
      if (
        type === 'abort' &&
        typeof listener === 'function' &&
        !this._listeners.includes(listener)
      ) {
        this._listeners.push(listener)
      }
    }

    removeEventListener(
      type: string,
      listener: (event: Event) => void,
      _options?: EventListenerOptions | boolean,
    ): void {
      if (type === 'abort' && typeof listener === 'function') {
        const index = this._listeners.indexOf(listener)
        if (index !== -1) {
          this._listeners.splice(index, 1)
        }
      }
    }

    dispatchEvent(event: Event): boolean {
      if (event.type === 'abort') {
        // Call the onabort handler if set
        if (this._onabort) {
          this._onabort(event)
        }
        // Dispatch a stable snapshot while honoring removals made by an
        // earlier listener before a later listener's turn.
        for (const listener of [...this._listeners]) {
          if (this._listeners.includes(listener)) listener(event)
        }
      }
      return true
    }

    throwIfAborted(): void {
      if (this._aborted) {
        throw this._reason
      }
    }

    // Make AbortSignal a proper constructor
    static [Symbol.hasInstance](instance: any): boolean {
      return (
        typeof instance === 'object' &&
        instance !== null &&
        signalInstances.has(instance)
      )
    }

    // Internal method to trigger abort
    _abort(reason?: any): void {
      if (this._aborted) return

      this._aborted = true
      this._reason = reason !== undefined ? reason : new Error('AbortError')

      const EventClass = globalThis.Event
      const event = new EventClass('abort')
      Object.defineProperty(event, 'target', { value: this, writable: false })
      this.dispatchEvent(event)
    }
  }

  class AbortControllerImpl implements AbortController {
    private _signal: AbortSignalImpl

    constructor() {
      this._signal = new AbortSignalImpl()
    }

    get signal(): AbortSignal {
      return this._signal
    }

    abort(reason?: any): void {
      this._signal._abort(reason)
    }
  }

  // Add static methods to AbortSignal
  const AbortControllerConstructor =
    AbortControllerImpl as unknown as AbortControllerPolyfillConstructor
  Object.defineProperty(AbortControllerConstructor, 'AbortSignal', {
    value: AbortSignalImpl,
    writable: false,
    enumerable: false,
    configurable: false,
  })

  return AbortControllerConstructor
}
