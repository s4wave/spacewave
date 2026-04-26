// Shared test utility functions for typed e2e test scripts.
// Used by both spacewave and spacewave-cloud e2e/wasm/*.ts files.

// Disposable is any object with a Symbol.dispose method.
interface Disposable {
  [Symbol.dispose](): void
}

// CleanupTracker collects disposable resources and releases them in
// reverse order. Pass cleanup() to wrap each acquired resource.
export interface CleanupTracker {
  // Track a resource for cleanup. Returns the resource for chaining.
  <T>(resource: T): T
  // Release all tracked resources in reverse order.
  releaseAll(): void
}

// withCleanup creates a CleanupTracker and ensures all tracked resources
// are released when the callback completes (or throws).
export async function withCleanup<T>(
  fn: (cleanup: CleanupTracker) => Promise<T>,
): Promise<T> {
  const resources: Disposable[] = []
  const tracker = Object.assign(
    <T>(resource: T): T => {
      if (
        resource &&
        typeof (resource as unknown as Disposable)[Symbol.dispose] ===
          'function'
      ) {
        resources.push(resource as unknown as Disposable)
      }
      return resource
    },
    {
      releaseAll() {
        for (let i = resources.length - 1; i >= 0; i--) {
          try {
            resources[i][Symbol.dispose]()
          } catch {
            // best-effort cleanup
          }
        }
        resources.length = 0
      },
    },
  ) as CleanupTracker
  try {
    return await fn(tracker)
  } finally {
    tracker.releaseAll()
  }
}

// withTimeout runs an async callback with an AbortSignal that fires
// after the given deadline in milliseconds. Clears the timer on
// completion regardless of success or failure.
export async function withTimeout<T>(
  ms: number,
  fn: (signal: AbortSignal) => Promise<T>,
): Promise<T> {
  const ctrl = new AbortController()
  const tid = setTimeout(() => ctrl.abort('timeout'), ms)
  try {
    return await fn(ctrl.signal)
  } finally {
    clearTimeout(tid)
  }
}

// RetryOptions configures the retryUntil polling loop.
export interface RetryOptions {
  // Total deadline in milliseconds from the start of the first attempt.
  deadlineMs: number
  // Delay between attempts in milliseconds. Default 500.
  intervalMs?: number
  // Error message when the deadline is exceeded without success.
  message?: string
  // AbortSignal from an outer timeout. If aborted, the loop throws
  // immediately instead of retrying.
  signal?: AbortSignal
}

// retryUntil polls an async condition until it returns a truthy value or
// the deadline expires. Returns the first truthy result. Throws if the
// deadline is exceeded or the signal is aborted.
export async function retryUntil<T>(
  fn: (signal: AbortSignal) => Promise<T | null | undefined | false>,
  opts: RetryOptions,
): Promise<T> {
  const interval = opts.intervalMs ?? 500
  const deadline = Date.now() + opts.deadlineMs
  const msg = opts.message ?? 'retryUntil deadline exceeded'
  while (Date.now() < deadline) {
    if (opts.signal?.aborted) {
      throw new Error('aborted')
    }
    try {
      const result = await fn(opts.signal ?? new AbortController().signal)
      if (result) {
        return result
      }
    } catch (err) {
      if (opts.signal?.aborted) {
        throw err
      }
    }
    await new Promise((resolve) => setTimeout(resolve, interval))
  }
  throw new Error(msg)
}
