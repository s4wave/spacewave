import { EventIterator } from 'event-iterator'

// ItStateChangedEvent is an event emitted when ItState changes.
export class ItStateChangedEvent<T> extends Event {
  constructor(public readonly changeEvent: T) {
    super('changed')
  }
}

// StateContainer contains an observable state comprised of an initial snapshot
// followed by updates.
//
// it-state: iterator which emits initial state followed by updates.
// events:
//  - changed: emits the change event object
export class ItState<T> extends EventTarget {
  constructor(public readonly getSnapshot: () => Promise<T>) {
    super()
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
          queue.push(snapshot)
          listener = (evt: Event) => {
            const changedEvent = evt as ItStateChangedEvent<T>
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

  // pushChangeEvent pushes a change event to the ItState.
  public pushChangeEvent(changeEvent: T) {
    this.dispatchEvent(new ItStateChangedEvent(changeEvent))
  }
}
