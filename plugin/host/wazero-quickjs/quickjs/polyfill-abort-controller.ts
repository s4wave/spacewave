/* eslint-disable @typescript-eslint/no-explicit-any */
// createAbortController creates an AbortController polyfill implementation.
export function createAbortController(): new () => AbortController {
  class AbortSignalImpl implements AbortSignal {
    static abort(reason?: any): AbortSignal {
      const signal = new AbortSignalImpl()
      signal._abort(reason)
      return signal
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
      if (type === 'abort' && typeof listener === 'function') {
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
        // Call all addEventListener listeners
        this._listeners.forEach((listener) => listener(event))
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
      return instance instanceof AbortSignalImpl
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
  Object.defineProperty(AbortControllerImpl, 'AbortSignal', {
    value: AbortSignalImpl,
    writable: false,
    enumerable: false,
    configurable: false,
  })

  return AbortControllerImpl as new () => AbortController
}
