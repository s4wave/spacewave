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

// Retry attempts to call a function until the function returns success.
export class Retry<T=void> {
    // _result is the result promise.
    public readonly result: Promise<T>

    // canceled returns if the retry has been canceled.
    public get canceled() {
        return this._canceled
    }

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

    // result returns a promise that is fulfilled with the result.
    // backoffFn controls backoff timing.
    // errorCb is an optional callback for when the function returns an error.
    // abortSignal is an optional signal to use to cancel retries.
    constructor(
        private fn: () => Promise<T>,
        private backoffFn: BackoffFn = constantBackoff(),
        private errorCb?: (err: unknown) => void,
        abortSignal?: AbortSignal,
    ) {
        if (abortSignal) {
            abortSignal.addEventListener('abort', this.cancel.bind(this))
        }
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
        if (this._canceled) {
            return
        }
        this.fn().then((res) => {
            if (this._resolve) {
                this._resolve(res)
            }
        }).catch(err => {
            this._currError = err
            if (this.errorCb) {
                this.errorCb(err)
            }
            if (this._canceled) {
                if (this._reject) {
                    this._reject(err)
                }
            } else {
                this._scheduleRetry()
            }
        })
    }

    // _scheduleRetry schedules the next retry.
    private _scheduleRetry() {
        const backoffMs = this.backoffFn()
        this._currRetry = setTimeout(this._start.bind(this), backoffMs)
    }
}
