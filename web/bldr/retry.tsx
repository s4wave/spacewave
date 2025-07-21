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

// RetryOpts are options passed to Retry.
export interface RetryOpts {
  // backoffFn controls backoff timing.
  // defaults to constant wait of 500ms.
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
    opts?.abortSignal?.addEventListener('abort', this.cancel.bind(this))
    this._abortSignal = opts?.abortSignal

    this._backoffFn = opts?.backoffFn || constantBackoff()
    this._errorCb = opts?.errorCb

    this._setTimeout = opts?.setTimeout || setTimeout.bind(globalThis)
    this._clearTimeout = opts?.clearTimeout || clearTimeout.bind(globalThis)

    this.result = new Promise<T>((resolve, reject) => {
      this._resolve = resolve
      this._reject = reject
    })
    // prevent unhandled rejection error in node.js
    this.result.catch(() => {})
    // call _execute on next tick
    queueMicrotask(this._execute.bind(this))
  }

  // cancel prevents further retrying of the function.
  public cancel() {
    this._canceled = true
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
        if (this._canceled || this._abortSignal?.aborted) {
          this.cancel()
          return
        }

        const res = await this.fn()
        if (this._resolve) {
          this._resolve(res)
        }
        return
      } catch (err) {
        this._currError = err
        if (this._canceled || this._abortSignal?.aborted) {
          if (this._reject) {
            this._reject(err)
          }
          return
        }
        if (this._errorCb) {
          this._errorCb(err)
        }
        await new Promise<void>((resolve) => {
          let timeoutId: NodeJS.Timeout | null = null
          if (this._abortSignal?.aborted) {
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
