// BackoffFn returns the number of milliseconds to wait till next retry.
export type BackoffFn = () => number

// constantBackoff constructs a new constant BackoffFn.
//
// argument is the number of milliseconds to wait.
export function constantBackoff(waitMs: number = 500): BackoffFn {
  return () => {
    return waitMs
  }
}

// ExponentialBackoff is a BackoffFn whose wait doubles on each call.
export interface ExponentialBackoff extends BackoffFn {
  // reset returns the wait to its initial value, such as after a success.
  reset(): void
  // capped reports whether the last wait returned was the maximum, so a
  // caller can stop treating a failure that keeps happening as transient.
  capped(): boolean
}

// exponentialBackoff constructs a new ExponentialBackoff.
//
// The wait starts at initialMs and doubles on each call up to maxMs. Each call
// returns a random wait between half and all of it, so clients that failed
// together do not retry together.
export function exponentialBackoff(
  initialMs: number = 100,
  maxMs: number = 5000,
): ExponentialBackoff {
  let current = initialMs
  let last = 0
  const backoff = () => {
    const wait = current
    last = wait
    current = Math.min(current * 2, maxMs)
    return wait / 2 + (Math.random() * wait) / 2
  }
  return Object.assign(backoff, {
    reset: () => {
      current = initialMs
      last = 0
    },
    capped: () => last >= maxMs,
  })
}

// RetryOpts are options passed to Retry.
export interface RetryOpts {
  // backoffFn controls backoff timing.
  // defaults to exponentialBackoff with its default bounds.
  backoffFn?: BackoffFn
  // errorCb is an optional callback for when the function returns an error.
  errorCb?: (err: unknown) => void
  // abortSignal is an optional signal to use to cancel retries.
  abortSignal?: AbortSignal
  // setTimeout is an optional function to use for setting timeouts.
  // defaults to global setTimeout.
  setTimeout?: typeof setTimeout
  // clearTimeout is an optional function to use for clearing timeouts.
  // defaults to global clearTimeout.
  clearTimeout?: typeof clearTimeout
}

// Retry attempts to call a function until the function returns success.
export class Retry<T = void> {
  // result is the result promise.
  public readonly result: Promise<T>

  // canceled returns if the retry has been canceled.
  public get canceled() {
    return this._canceled
  }

  // _backoffFn is the backoff function (if any)
  private _backoffFn: BackoffFn
  // _errorCb is the error callback.
  private _errorCb?: (err: unknown) => void
  // _abortSignal is the current abort signal (if set).
  private _abortSignal?: AbortSignal
  // _abortListener cancels retry when the abort signal fires.
  private _abortListener?: () => void

  // _canceled indicates retrying this has been canceled
  private _canceled?: boolean
  // _resolve resolves the promise.
  private _resolve?: (val: T) => void
  // _reject rejects the promise.
  private _reject?: (err: unknown) => void
  // _currError contains the current error.
  private _currError?: unknown
  // _cancelRetry is a function to cancel the current retry attempt.
  private _cancelRetry?: () => void

  // _setTimeout is the function to use for setting timeouts.
  private _setTimeout: typeof setTimeout
  // _clearTimeout is the function to use for clearing timeouts.
  private _clearTimeout: typeof clearTimeout

  constructor(
    private fn: () => Promise<T>,
    opts?: RetryOpts,
  ) {
    // Cancel the retry when the abort signal fires.
    this._abortSignal = opts?.abortSignal
    if (this._abortSignal) {
      this._abortListener = () => this.cancel()
      this._abortSignal.addEventListener('abort', this._abortListener, {
        once: true,
      })
    }

    // Apply the options, warning on each failure by default.
    this._backoffFn = opts?.backoffFn || exponentialBackoff()
    this._errorCb =
      opts?.errorCb ??
      ((err) => {
        console.warn('Retry: retrying after error', { error: err })
      })
    this._setTimeout = opts?.setTimeout || setTimeout.bind(globalThis)
    this._clearTimeout = opts?.clearTimeout || clearTimeout.bind(globalThis)

    // Expose the result, which never reports an unhandled rejection.
    this.result = new Promise<T>((resolve, reject) => {
      this._resolve = resolve
      this._reject = reject
    })
    this.result.catch(() => {})

    // Start the first attempt on the next tick.
    queueMicrotask(this._execute.bind(this))
  }

  // cancel prevents further retrying of the function.
  public cancel() {
    // Cancel once, stopping any pending wait and rejecting the result.
    if (this._canceled) {
      return
    }
    this._canceled = true
    this._clearAbortListener()
    if (this._cancelRetry) {
      this._cancelRetry()
    }
    if (this._reject) {
      this._reject(this._currError)
    }
  }

  private async _execute() {
    do {
      try {
        const abortSignal = this._abortSignal
        if (this._canceled || abortSignal?.aborted) {
          this.cancel()
          return
        }

        const res = await this.fn()
        if (this._resolve) {
          this._clearAbortListener()
          this._resolve(res)
        }
        return
      } catch (err) {
        const abortSignal = this._abortSignal
        this._currError = err
        if (this._canceled || abortSignal?.aborted) {
          if (this._reject) {
            this._clearAbortListener()
            this._reject(err)
          }
          return
        }
        if (this._errorCb) {
          this._errorCb(err)
        }
        await new Promise<void>((resolve) => {
          let timeoutId: NodeJS.Timeout | null = null
          if (abortSignal?.aborted) {
            resolve()
            return
          }
          this._cancelRetry = () => {
            if (timeoutId) this._clearTimeout(timeoutId)
            resolve()
          }
          timeoutId = this._setTimeout(() => {
            this._cancelRetry = undefined
            resolve()
          }, this._backoffFn())
        })
      }
    } while (true) /* eslint-disable-line */
  }

  private _clearAbortListener() {
    if (!this._abortSignal || !this._abortListener) {
      return
    }
    this._abortSignal.removeEventListener('abort', this._abortListener)
    this._abortListener = undefined
  }
}

// RetryWithAbortOpts are options for retryWithAbort.
export interface RetryWithAbortOpts extends Omit<RetryOpts, 'abortSignal'> {}

// retryWithAbort builds a retry with the given abort signal & abort func.
export function retryWithAbort<T = void>(
  abortSignal: AbortSignal,
  cb: (abortSignal: AbortSignal) => Promise<T>,
  opts?: RetryWithAbortOpts,
) {
  return new Retry(cb.bind(undefined, abortSignal), {
    ...opts,
    abortSignal,
  }).result
}
