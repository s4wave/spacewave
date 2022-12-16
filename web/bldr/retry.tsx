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

    // result returns a promise that is fulfilled with the result.
    // errorCb is an optional callback for when the function returns an error.
    // abortSignal is an optional signal to use to cancel retries.
    constructor(
        private fn: () => Promise<T>,
        private errorCb?: (err: unknown) => void,
        private abortSignal?: AbortSignal,
    ) {
        if (abortSignal) {
            abortSignal.addEventListener('abort', this.cancel.bind(this))
        }
        setTimeout(this._start.bind(this), 1)
        this.result = new Promise<T>((resolve, reject) => {
            this._resolve = resolve
            this._reject = reject
        })
        // prevent unhandled rejection error in node.js
        this.result.catch(() => {})
    }

    // cancel prevents further retrying of the function.
    public cancel() {
        this._canceled = true
        if (this._reject) {
            this._reject(this._currError)
        }
    }

    private _start() {
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
            if (this._canceled && this._reject) {
                this._reject(err)
            }
        })
    }
}
