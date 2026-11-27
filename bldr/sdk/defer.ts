import './dispose-symbol.js'

/**
 * DisposableStack manages synchronous disposable resources, mimicking Go's defer behavior.
 * Functions added via `defer` are executed in LIFO order when the stack is disposed.
 * Implements the `Disposable` interface for use with `using` declarations.
 */
export class DisposableStack implements Disposable {
  private stack: (() => void)[] = []

  /**
   * defer adds a function to be executed when the stack is disposed.
   * @param fn The function to defer.
   */
  defer(fn: () => void): void {
    this.stack.push(fn)
  }

  /**
   * [Symbol.dispose] disposes of the resources in the stack by executing the
   * deferred functions in Last-In, First-Out (LIFO) order.
   * If a deferred function throws an error, disposal stops, and the error is
   * rethrown, similar to Go's panic behavior during defer execution.
   */
  [Symbol.dispose](): void {
    // Emulate Go: if a deferred function throws, stop and rethrow
    while (this.stack.length) {
      const fn = this.stack.pop()!
      fn()
    }
  }
}

/**
 * AsyncDisposableStack manages asynchronous disposable resources, mimicking Go's defer behavior.
 * Functions added via `defer` are executed sequentially in LIFO order when the stack is disposed.
 * Implements the `AsyncDisposable` interface for use with `await using` declarations.
 */
export class AsyncDisposableStack implements AsyncDisposable {
  private stack: (() => Promise<void> | void)[] = []

  /**
   * defer adds a synchronous or asynchronous function to be executed when the
   * stack is disposed.
   * @param fn The function to defer. Can return void or a Promise<void>.
   */
  defer(fn: () => Promise<void> | void): void {
    this.stack.push(fn)
  }

  /**
   * [Symbol.asyncDispose] asynchronously disposes of the resources in the stack
   * by executing the deferred functions sequentially in Last-In, First-Out
   * (LIFO) order. The method awaits each function if the function returns a
   * promise.
   */
  async [Symbol.asyncDispose](): Promise<void> {
    // Execute in LIFO order, awaiting each potentially async function
    for (let i = this.stack.length - 1; i >= 0; --i) {
      await this.stack[i]()
    }
  }
}
