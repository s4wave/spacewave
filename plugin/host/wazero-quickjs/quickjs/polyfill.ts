import { QuickjsGlobalScope } from './quickjs.js'
import { createEvent } from './polyfill-event.js'
import { createAbortController } from './polyfill-abort-controller.js'
import { TextEncoder, TextDecoder } from './text-encoding.js'

// quickjs has a reduced standard library.
// this file polyfills exactly what we need and probably will need to be expanded over time.
// the implementations are often significantly simplified from the "real" ones.
// it may be better long term to integrate a more capable js wasm engine.
// see: https://github.com/saghul/txiki.js

// QuickjsPolyfillGlobalScope represents QuickjsGlobalScope after the polyfills are applied.
export interface QuickjsPolyfillGlobalScope extends QuickjsGlobalScope {
  // AbortController is the polyfilled abort controller type.
  AbortController: new () => AbortController
  // Event is the polyfilled event constructor type.
  Event: typeof Event
  // TextEncoder is the polyfilled text encoder type.
  TextEncoder: typeof TextEncoder
  // TextDecoder is the polyfilled text encoder type.
  TextDecoder: typeof TextDecoder
  // console is the polyfilled console object.
  console: QuickjsGlobalScope['console'] & {
    error: (...args: any[]) => void
  }
}

// applyPolyfills applies the polyfills to the global scope.
export function applyPolyfills(
  to: QuickjsGlobalScope,
): QuickjsPolyfillGlobalScope {
  const target: QuickjsPolyfillGlobalScope = to as QuickjsPolyfillGlobalScope
  target.Event = createEvent() as typeof Event
  target.AbortController = createAbortController()
  target.TextEncoder = TextEncoder
  target.TextDecoder = TextDecoder

  // Add console.error polyfill
  target.console.error = (...args: any[]) => {
    target.console.log('ERROR', ...args)
  }

  return target
}
