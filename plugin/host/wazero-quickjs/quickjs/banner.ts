// banner.ts - This file is compiled into an IIFE banner that runs before any
// bundled module code. It sets up polyfills that must exist before ES module
// imports are evaluated (e.g., for `class Foo extends Event` to work).

import { createSymbolPolyfills } from './polyfill-symbol.js'
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore - event-target.js is untyped, types come from lib.dom
import { Event, EventTarget, CustomEvent } from './event-target.js'

const quickjsGlobalThis = globalThis as typeof globalThis & {
  Event: typeof Event
  EventTarget: typeof EventTarget
  CustomEvent: typeof CustomEvent
}

// Apply Symbol polyfills first
createSymbolPolyfills()

// Set Event classes on globalThis
quickjsGlobalThis.Event = Event
quickjsGlobalThis.EventTarget = EventTarget
quickjsGlobalThis.CustomEvent = CustomEvent
