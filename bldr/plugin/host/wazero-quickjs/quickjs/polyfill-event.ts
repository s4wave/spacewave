/* eslint-disable @typescript-eslint/no-explicit-any */

// Re-export Event, EventTarget, CustomEvent from the txiki.js polyfill.
// The actual classes are defined in event-target.js which is injected via
// esbuild banner to ensure they exist on globalThis before any module code runs.
//
// These functions return the already-defined globals for use in applyPolyfills.

// createEvent returns the Event constructor from globalThis.
export function createEvent(): {
  new (type: string, eventInitDict?: EventInit): Event
  readonly NONE: 0
  readonly CAPTURING_PHASE: 1
  readonly AT_TARGET: 2
  readonly BUBBLING_PHASE: 3
} {
  return globalThis.Event as any
}

// createEventTarget returns the EventTarget constructor from globalThis.
export function createEventTarget(): new () => EventTarget {
  return globalThis.EventTarget as any
}

// createCustomEvent returns the CustomEvent constructor from globalThis.
export function createCustomEvent(): new <T = any>(
  type: string,
  eventInitDict?: CustomEventInit<T>,
) => CustomEvent<T> {
  return globalThis.CustomEvent as any
}
