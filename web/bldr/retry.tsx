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
  // _currRetry is the current scheduled retry timeout.
  private _currRetry?: NodeJS.Timeout

  constructor(
    private fn: () => Promise<T>,
    opts?: RetryOpts,
  ) {
    opts?.abortSignal?.addEventListener('abort', this.cancel.bind(this))
    this._abortSignal = opts?.abortSignal

    this._backoffFn = opts?.backoffFn || constantBackoff()
    this._errorCb = opts?.errorCb

    this.result = new Promise<T>((resolve, reject) => {
      this._resolve = resolve
      this._reject = reject
    })
    // prevent unhandled rejection error in node.js
    this.result.catch(() => {})
    // call _start on next tick
    setTimeout(this._start.bind(this), 1)
  }

  // cancel prevents further retrying of the function.
  public cancel() {
    this._canceled = true
    if (this._reject) {
      this._reject(this._currError)
    }
    if (this._currRetry) {
      clearTimeout(this._currRetry)
      delete this._currRetry
    }
  }

  private _start() {
    if (this._currRetry) {
      clearTimeout(this._currRetry)
      delete this._currRetry
    }
    if (this._canceled || this._abortSignal?.aborted) {
      return
    }
    this.fn()
      .then((res) => {
        if (this._resolve) {
          this._resolve(res)
        }
      })
      .catch((err) => {
        this._currError = err
        if (this._canceled) {
          if (this._reject) {
            this._reject(err)
          }
        } else {
          if (this._errorCb) {
            this._errorCb(err)
          }
          this._scheduleRetry()
        }
      })
  }

  // _scheduleRetry schedules the next retry.
  private _scheduleRetry() {
    const backoffMs = this._backoffFn()
    this._currRetry = setTimeout(this._start.bind(this), backoffMs)
  }
}

// RetryWithAbortOpts are options for retryWithAbort.
export interface RetryWithAbortOpts extends Omit<RetryOpts, 'abortSignal'> {}

// retryWithAbort builds a retry with the given abort signal & abort func.
// does not return an error (promise is never rejected)
export async function retryWithAbort<T = void>(
  abortSignal: AbortSignal,
  cb: (abortSignal: AbortSignal) => Promise<T>,
  opts?: RetryWithAbortOpts,
) {
  const retry = new Retry(cb.bind(undefined, abortSignal), {
    ...opts,
    abortSignal,
  })
  return new Promise<void>((resolve) => {
    retry.result
      .then(() => {
        resolve()
      })
      .catch(() => {
        resolve()
      })
  })
}
