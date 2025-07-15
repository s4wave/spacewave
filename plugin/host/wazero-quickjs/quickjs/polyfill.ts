import { QuickjsGlobalScope } from './quickjs.js'
import { createEvent } from './polyfill-event.js'
import { createAbortController } from './polyfill-abort-controller.js'
import { TextEncoder, TextDecoder } from './text-encoding.js'
import { createQuickjsConsole, type Console } from './console.js'
import { createQuickjsPerformance, type Performance } from './performance.js'

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
  console: Console
  // performance is the polyfilled performance object.
  performance: Performance

  /**
   * Call the function func after delay ms. Return a handle to the timer.
   * @param func - Function to call
   * @param delay - Delay in milliseconds
   */
  setTimeout(func: () => void, delay: number): any

  /**
   * Cancel a timer.
   * @param handle - Timer handle
   */
  clearTimeout(handle: any): void

  /**
   * Call the function func periodically with the given interval. Return a handle to the timer.
   * @param func - Function to call
   * @param delay - Interval in milliseconds
   */
  setInterval(func: () => void, delay: number): any

  /**
   * Cancel an interval timer.
   * @param handle - Timer handle
   */
  clearInterval(handle: any): void

  // global is the polyfilled global reference.
  global: QuickjsPolyfillGlobalScope
  // window is the polyfilled window reference.
  window: QuickjsPolyfillGlobalScope
  // self is the polyfilled self reference.
  self: QuickjsPolyfillGlobalScope
}

// applyPolyfills applies the polyfills to the global scope.
export function applyPolyfills(
  to: QuickjsGlobalScope,
): QuickjsPolyfillGlobalScope {
  const target: QuickjsPolyfillGlobalScope = to as QuickjsPolyfillGlobalScope

  // Define global scope references that all point to the same object
  const globalRefs = ['global', 'window', 'self']
  globalRefs.forEach((name) => {
    Object.defineProperty(to, name, {
      enumerable: true,
      get() {
        return to
      },
      set() {},
    })
  })

  target.console = createQuickjsConsole(target.console)
  target.performance = createQuickjsPerformance(target.performance)
  target.Event = createEvent() as typeof Event
  target.AbortController = createAbortController()
  target.TextEncoder = TextEncoder
  target.TextDecoder = TextDecoder

  target.setTimeout = to.os.setTimeout
  target.clearTimeout = to.os.clearTimeout
  target.setInterval = to.os.setInterval
  target.clearInterval = to.os.clearInterval

  return target
}
