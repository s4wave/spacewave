// ItStateOpts are optional settings for ItState.
export interface ItStateOpts {
  // mostRecentOnly drops all messages but the most recent.
  // messages are only dropped during backpressure.
  // in other words: the queue size will be capped to 1.
  mostRecentOnly?: boolean
}

// ItState is an iterable which emits an initial snapshot followed by updates. The updates
// pushed to the pushChangeEvent function are emitted to the iterable.
//
// if getSnapshot is unset or returns undefined, no snapshot will be emitted.
export class ItState<T> {
  // nonce is only used if mostRecentOnly is enabled.
  private nonce?: number
  // getSnapshot returns the initial snapshot or undefined
  private getSnapshot: () => Promise<T | undefined>
  private listeners: Set<(value: T) => void> = new Set()
  private errorListeners: Set<(error: Error) => void> = new Set()

  constructor(
    getSnapshot?: () => Promise<T | undefined>,
    private opts?: ItStateOpts,
  ) {
    this.getSnapshot = getSnapshot || (async () => undefined)
  }

  // snapshot returns a snapshot or undefined
  public get snapshot(): Promise<T | undefined> {
    return this.getSnapshot()
  }

  // getIterable builds the initial snapshot and returns the iterable.
  public getIterable(): AsyncIterable<T> {
    return {
      [Symbol.asyncIterator]: () => {
        const queue: T[] = []
        let resolveNext: ((value: IteratorResult<T>) => void) | null = null
        let rejectNext: ((reason: any) => void) | null = null
        let done = false
        let mostRecentValue: { value: T; nonce?: number } | null = null

        // Function to handle new values
        const handleValue = (value: T) => {
          if (this.opts?.mostRecentOnly) {
            // Always update the most recent value with the current nonce
            mostRecentValue = { value, nonce: this.nonce }

            // If someone is waiting, resolve with the most recent value
            if (resolveNext) {
              const resolve = resolveNext
              resolveNext = null
              // Important: Use the latest value, not the one passed to this function
              resolve({ value: mostRecentValue.value, done: false })
              mostRecentValue = null
            }
          } else {
            // Normal queue behavior
            if (resolveNext) {
              const resolve = resolveNext
              resolveNext = null
              resolve({ value, done: false })
            } else {
              queue.push(value)
            }
          }
        }

        // Function to handle errors
        const handleError = (error: Error) => {
          if (rejectNext) {
            const reject = rejectNext
            rejectNext = null
            reject(error)
          }
          done = true
        }

        // Add listeners
        this.listeners.add(handleValue)
        this.errorListeners.add(handleError)

        // Initialize with snapshot
        const initialize = async () => {
          try {
            const snapshot = await this.getSnapshot()
            if (snapshot !== undefined) {
              handleValue(snapshot)
            }
          } catch (error) {
            handleError(
              error instanceof Error ? error : new Error(String(error)),
            )
          }
        }

        // Start initialization
        initialize()

        return {
          next: async (): Promise<IteratorResult<T>> => {
            if (done) {
              return { value: undefined as any, done: true }
            }

            // If we have a most recent value waiting (mostRecentOnly mode)
            if (mostRecentValue) {
              const { value } = mostRecentValue
              mostRecentValue = null
              return { value, done: false }
            }

            // If we have queued values
            if (queue.length > 0) {
              return { value: queue.shift()!, done: false }
            }

            // Wait for the next value
            return new Promise<IteratorResult<T>>((resolve, reject) => {
              resolveNext = resolve
              rejectNext = reject
            })
          },
          return: async (): Promise<IteratorResult<T>> => {
            // Clean up
            this.listeners.delete(handleValue)
            this.errorListeners.delete(handleError)
            done = true

            // Resolve any pending next() call with done:true
            if (resolveNext) {
              const resolve = resolveNext
              resolveNext = null
              resolve({ value: undefined as any, done: true })
            }

            rejectNext = null
            return { value: undefined as any, done: true }
          },
          throw: async (error: any): Promise<IteratorResult<T>> => {
            // Clean up
            this.listeners.delete(handleValue)
            this.errorListeners.delete(handleError)
            done = true
            resolveNext = null
            rejectNext = null
            return { value: undefined as any, done: true }
          },
        }
      },
    }
  }

  // pushChangeEvent pushes an event to the subscribers.
  public pushChangeEvent(changeEvent: T) {
    if (this.opts?.mostRecentOnly) {
      this.nonce = (this.nonce ?? 0) + 1
      // We need to make a copy of the listeners to avoid race conditions
      // when multiple events are pushed in rapid succession
      const currentListeners = Array.from(this.listeners)
      // We don't pass the nonce to the listener as it's only used internally
      currentListeners.forEach((listener) => listener(changeEvent))
    } else {
      this.listeners.forEach((listener) => listener(changeEvent))
    }
  }

  // pushSnapshot calls the snapshot function and pushes it as a change event.
  public async pushSnapshot() {
    try {
      const snapshot = await this.getSnapshot()
      if (snapshot) {
        this.pushChangeEvent(snapshot)
      }
    } catch (error) {
      this.errorListeners.forEach((listener) =>
        listener(error instanceof Error ? error : new Error(String(error))),
      )
    }
  }
}
