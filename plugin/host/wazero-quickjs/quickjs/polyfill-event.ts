// Event interface provides a simplified Event type compatible with standard JavaScript Event usage.
export interface Event {
  readonly type: string
  target: EventTarget | null
  currentTarget: EventTarget | null
  readonly bubbles: boolean
  readonly cancelable: boolean
  readonly defaultPrevented: boolean
  readonly composed: boolean
  readonly isTrusted: boolean
  readonly timeStamp: number
  preventDefault(): void
  stopPropagation(): void
  stopImmediatePropagation(): void
}

// Internal Event interface with additional properties for implementation
interface InternalEvent extends Event {
  _dispatched: boolean
  _stopPropagation: boolean
  _stopImmediatePropagation: boolean
}

// createEvent creates an Event polyfill implementation.
export function createEvent(): {
  new (type: string, eventInitDict?: EventInit): Event
  readonly NONE: 0
  readonly CAPTURING_PHASE: 1
  readonly AT_TARGET: 2
  readonly BUBBLING_PHASE: 3
} {
  class EventImpl implements InternalEvent {
    public readonly type: string
    public target: EventTarget | null = null
    public currentTarget: EventTarget | null = null
    public readonly bubbles: boolean
    public readonly cancelable: boolean
    private _defaultPrevented: boolean = false
    public readonly composed: boolean
    public readonly isTrusted: boolean
    public readonly timeStamp: number
    public _dispatched: boolean = false
    public _stopPropagation: boolean = false
    public _stopImmediatePropagation: boolean = false

    // Event phase constants
    static readonly NONE = 0
    static readonly CAPTURING_PHASE = 1
    static readonly AT_TARGET = 2
    static readonly BUBBLING_PHASE = 3

    constructor(type: string, eventInitDict?: EventInit) {
      if (typeof type !== 'string') {
        throw new TypeError('Event constructor: type must be a string')
      }
      this.type = type
      this.bubbles = eventInitDict?.bubbles ?? false
      this.cancelable = eventInitDict?.cancelable ?? false
      this.composed = eventInitDict?.composed ?? false
      this.isTrusted = false
      this.timeStamp = Date.now()
    }

    get defaultPrevented(): boolean {
      return this._defaultPrevented
    }

    preventDefault(): void {
      if (this.cancelable) {
        this._defaultPrevented = true
      }
    }

    stopPropagation(): void {
      // Set flag to prevent further propagation
      this._stopPropagation = true
    }

    stopImmediatePropagation(): void {
      // Set flag to prevent further propagation and immediate listeners
      this._stopPropagation = true
      this._stopImmediatePropagation = true
    }
  }

  // Add static phase constants to the constructor
  Object.defineProperty(EventImpl, 'NONE', {
    value: 0,
    writable: false,
    enumerable: true,
    configurable: false,
  })
  Object.defineProperty(EventImpl, 'CAPTURING_PHASE', {
    value: 1,
    writable: false,
    enumerable: true,
    configurable: false,
  })
  Object.defineProperty(EventImpl, 'AT_TARGET', {
    value: 2,
    writable: false,
    enumerable: true,
    configurable: false,
  })
  Object.defineProperty(EventImpl, 'BUBBLING_PHASE', {
    value: 3,
    writable: false,
    enumerable: true,
    configurable: false,
  })

  return EventImpl as {
    new (type: string, eventInitDict?: EventInit): Event
    readonly NONE: 0
    readonly CAPTURING_PHASE: 1
    readonly AT_TARGET: 2
    readonly BUBBLING_PHASE: 3
  }
}
