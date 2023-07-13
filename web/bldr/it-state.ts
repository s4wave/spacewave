import { EventIterator } from 'event-iterator'

// ItStateChangedEvent is an event emitted when ItState changes.
export class ItStateChangedEvent<T> extends Event {
  constructor(
    public readonly changeEvent: T,
    public readonly nonce?: number,
  ) {
    super('changed')
  }
}

// ItStateOptions are optional settings for ItState.
export interface ItStateOptions {
  // mostRecentOnly drops all messages but the most recent.
  // messages are only dropped during backpressure.
  // in other words: the queue size will be capped to 1.
  mostRecentOnly?: boolean
}

// ItState is an iterable which emits an initial snapshot followed by updates. The updates
// pushed to the pushChangeEvent function are emitted to the iterable.
//
// if getSnapshot is unset or returns undefined, no snapshot will be emitted.
export class ItState<T> extends EventTarget {
  // nonce is only used if mostRecentOnly is enabled.
  private nonce?: number
  // getSnapshot returns the initial snapshot or undefined
  private getSnapshot: () => Promise<T | undefined>

  constructor(
    getSnapshot?: () => Promise<T | undefined>,
    private opts?: ItStateOptions,
  ) {
    super()
    this.getSnapshot = getSnapshot || (async () => undefined)
  }

  // getIterable builds the initial snapshot and returns the iterable.
  public getIterable(): AsyncIterable<T> {
    return new EventIterator<T>((queue) => {
      let closed = false
      let listener: EventListener | null = null

      this.getSnapshot()
        .then((snapshot) => {
          if (closed) {
            return
          }
          if (snapshot !== undefined) {
            queue.push(snapshot)
          }
          listener = (evt: Event) => {
            const changedEvent = evt as ItStateChangedEvent<T>
            if (
              this.opts?.mostRecentOnly &&
              changedEvent.nonce !== this.nonce
            ) {
              // skip this message, use most recent only.
              return
            }
            queue.push(changedEvent.changeEvent)
          }
          this.addEventListener('changed', listener)
        })
        .catch((err) => {
          closed = true
          queue.fail(err)
        })

      return () => {
        closed = true
        if (listener) {
          this.removeEventListener('changed', listener)
          listener = null
        }
      }
    })
  }

  // pushChangeEvent pushes an event to the subscribers.
  public pushChangeEvent(changeEvent: T) {
    if (this.opts?.mostRecentOnly) {
      this.nonce = (this.nonce ?? 0) + 1
    }
    this.dispatchEvent(new ItStateChangedEvent(changeEvent, this.nonce))
  }
}
